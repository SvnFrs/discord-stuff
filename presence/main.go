package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"
	psnet "github.com/shirou/gopsutil/net"

	"example.com/presence/lib/client"
)

// Configuration
const (
	clientID           = ""
	pollInterval       = 10 * time.Second
	cpuThresholdPct    = 2.0
	memThresholdPct    = 2.0
	netThresholdBytes  = 10 * 1024
	detailsMaxRunes    = 128
	pasteUploadTimeout = 5 * time.Second
	reconnectAttempts  = 3
	reconnectBackoff   = 1 * time.Second
)

// RunFastfetch runs fastfetch -l none and returns its output (may be partial on error)
func RunFastfetch(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "fastfetch", "-l", "none")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return out.String(), err
	}
	return out.String(), nil
}

// ParseFastfetch parses fastfetch output into a map and returns:
//  - the parsed map
//  - a concise staticDetails string containing only the requested fields
//  - a short staticState (user@host or empty)
func ParseFastfetch(output string) (map[string]string, string, string) {
	lines := strings.Split(output, "\n")
	m := map[string]string{}

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "---") {
			continue
		}
		// capture user@host first non-empty line
		if _, ok := m["UserHost"]; !ok {
			if strings.Contains(line, "@") && !strings.Contains(line, " ") {
				m["UserHost"] = line
				continue
			}
		}
		if idx := strings.Index(line, ":"); idx != -1 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			if key != "" && val != "" {
				m[key] = val
			}
		}
	}

	// Build static details with only the requested keys:
	// OS, Kernel, Packages, CPU (model), Memory (used/total (pct))
	osVal := m["OS"]
	kernelVal := m["Kernel"]
	pkgsVal := m["Packages"]
	cpuVal := m["CPU"]
	memVal := m["Memory"]

	// Construct multi-line details (each field on its own line)
	parts := []string{}
	if osVal != "" {
		parts = append(parts, fmt.Sprintf("OS: %s", osVal))
	}
	if kernelVal != "" {
		parts = append(parts, fmt.Sprintf("Kernel: %s", kernelVal))
	}
	if pkgsVal != "" {
		parts = append(parts, fmt.Sprintf("Packages: %s", pkgsVal))
	}
	if cpuVal != "" {
		parts = append(parts, fmt.Sprintf("CPU: %s", cpuVal))
	}
	if memVal != "" {
		parts = append(parts, fmt.Sprintf("Memory: %s", memVal))
	}
	staticDetails := strings.Join(parts, "\n")
	staticDetails = truncateRunes(staticDetails, detailsMaxRunes)

	staticState := ""
	if uh, ok := m["UserHost"]; ok {
		staticState = uh
	}

	return m, staticDetails, staticState
}

func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	if n > 3 {
		return string(r[:n-3]) + "..."
	}
	return string(r[:n])
}

// uploadToPasteService uploads text to paste.rs and returns the resulting URL or empty string on failure.
func uploadToPasteService(text string) string {
	clientHTTP := &http.Client{Timeout: pasteUploadTimeout}
	resp, err := clientHTTP.Post("https://paste.rs", "text/plain; charset=utf-8", strings.NewReader(text))
	if err != nil {
		fmt.Println("paste upload failed:", err)
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		fmt.Println("paste upload unexpected status:", resp.Status)
		return ""
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("paste read failed:", err)
		return ""
	}
	url := strings.TrimSpace(string(b))
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return url
	}
	return ""
}

func humanBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

func ptrTime(t time.Time) *time.Time { return &t }

func pickImageKey(m map[string]string) string {
	if os, ok := m["OS"]; ok {
		os = strings.ToLower(os)
		if strings.Contains(os, "arch") {
			return "arch"
		}
		if strings.Contains(os, "ubuntu") {
			return "ubuntu"
		}
	}
	if gpu, ok := m["GPU"]; ok {
		gpu = strings.ToLower(gpu)
		if strings.Contains(gpu, "amd") || strings.Contains(gpu, "radeon") {
			return "radeon"
		}
		if strings.Contains(gpu, "nvidia") {
			return "nvidia"
		}
	}
	return "default_os"
}

// monitorLoop polls CPU/memory/network and updates Discord presence using client.SetActivity.
// It keeps staticDetails (only OS/Kernel/Packages/CPU/Memory) in Details and
// uses State/SmallText for dynamic stats.
func monitorLoop(ctx context.Context, static map[string]string, staticDetails, staticState string) {
	largeImageKey := pickImageKey(static)
	largeText := static["DE"]
	if largeText == "" {
		largeText = static["WM"]
	}
	smallImageKey := "dot"

	// upload paste for full fastfetch output (optional)
	pasteURL := ""
	fullText := staticDetails
	if d, ok := static["UserHost"]; ok {
		fullText = d + "\n" + fullText
	}
	pasteURL = uploadToPasteService(fullText)

	// initial handshake/login
	if err := client.Login(clientID); err != nil {
		fmt.Println("initial Login failed:", err)
		for i := 0; i < reconnectAttempts; i++ {
			time.Sleep(reconnectBackoff)
			if err := client.Login(clientID); err == nil {
				break
			} else if i == reconnectAttempts-1 {
				fmt.Println("could not login after retries:", err)
				return
			}
		}
	}
	fmt.Println("Logged in")

	_, _ = cpu.Percent(0, false)
	prevNet, _ := psnet.IOCounters(false)
	prevTime := time.Now()

	var lastState string

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			client.Logout()
			return
		case <-ticker.C:
			// CPU percent (overall)
			cpuPctSlice, err := cpu.Percent(0, false)
			if err != nil || len(cpuPctSlice) == 0 {
				fmt.Println("cpu read failed:", err)
				continue
			}
			cpuPct := cpuPctSlice[0]

			// Memory
			vm, err := mem.VirtualMemory()
			if err != nil {
				fmt.Println("mem read failed:", err)
				continue
			}
			memPct := vm.UsedPercent

			// Network throughput
			curNet, _ := psnet.IOCounters(false)
			now := time.Now()
			interval := now.Sub(prevTime).Seconds()
			var rxPerSec, txPerSec float64
			if len(prevNet) > 0 && len(curNet) > 0 && interval > 0 {
				rxPerSec = float64(curNet[0].BytesRecv-prevNet[0].BytesRecv) / interval
				txPerSec = float64(curNet[0].BytesSent-prevNet[0].BytesSent) / interval
			}
			prevNet = curNet
			prevTime = now

			// Build dynamic strings
			state := fmt.Sprintf("CPU %.0f%% • RAM %.0f%%", cpuPct, memPct)
			// Put network into Details? user asked to leave rest to stats; we keep Details static only.
			smallTextNow := fmt.Sprintf("%.0f%% RAM", memPct)

			// thresholding
			shouldUpdate := false
			if lastState == "" {
				shouldUpdate = true
			} else {
				var lastCpuPct, lastMemPct float64
				fmt.Sscanf(lastState, "CPU %f%% • RAM %f%%", &lastCpuPct, &lastMemPct)
				if absFloat(cpuPct-lastCpuPct) >= cpuThresholdPct || absFloat(memPct-lastMemPct) >= memThresholdPct {
					shouldUpdate = true
				} else if (rxPerSec+txPerSec) >= float64(netThresholdBytes) {
					shouldUpdate = true
				}
			}

			if !shouldUpdate {
				continue
			}

			// Build activity with only the concise staticDetails in Details
			act := client.Activity{
				Details:    staticDetails,
				State:      state,
				LargeImage: largeImageKey,
				LargeText:  largeText,
				SmallImage: smallImageKey,
				SmallText:  smallTextNow,
				Timestamps: &client.Timestamps{Start: ptrTime(time.Now())},
			}
			if pasteURL != "" {
				act.Buttons = []*client.Button{
					{Label: "Full fastfetch output", Url: pasteURL},
				}
			}

			if err := client.SetActivity(act); err != nil {
				fmt.Println("SetActivity failed:", err)
				// reconnect and retry once
				client.Logout()
				var loginErr error
				for i := 0; i < reconnectAttempts; i++ {
					loginErr = client.Login(clientID)
					if loginErr == nil {
						break
					}
					time.Sleep(reconnectBackoff * time.Duration(i+1))
				}
				if loginErr == nil {
					if retryErr := client.SetActivity(act); retryErr != nil {
						fmt.Println("SetActivity retry failed:", retryErr)
					} else {
						lastState = act.State
					}
				} else {
					fmt.Println("reconnect attempts failed:", loginErr)
				}
			} else {
				lastState = act.State
			}
		}
	}
}

func absFloat(a float64) float64 {
	if a < 0 {
		return -a
	}
	return a
}

func main() {
	out, _ := RunFastfetch(context.Background())
	staticMap, staticDetails, staticState := ParseFastfetch(out)

	// print static details to stdout (optional)
	fmt.Println("Static details to be used in presence:")
	fmt.Println(staticDetails)

	ctx := context.Background()
	monitorLoop(ctx, staticMap, staticDetails, staticState)
}