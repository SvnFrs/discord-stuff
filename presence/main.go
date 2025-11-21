package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"

	"example.com/presence/lib/client"
)

// RunFastfetch runs `fastfetch -l none` with timeout and returns stdout/stderr as string.
func RunFastfetch(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "fastfetch", "-l", "none")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		// If fastfetch is missing or times out, return partial output + error
		return out.String(), err
	}
	return out.String(), nil
}

// ParseFastfetch parses the output into a map of keys and returns a client.Activity built from them.
func ParseFastfetch(output string) (client.Activity, error) {
	lines := strings.Split(output, "\n")
	m := map[string]string{}

	// capture first non-empty line (user@host) as "UserHost"
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "---") {
			continue
		}
		if strings.Contains(l, "@") && strings.Contains(l, " ") == false {
			m["UserHost"] = l
			break
		}
	}

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "---") {
			continue
		}
		// Key: Value pairs
		if idx := strings.Index(line, ":"); idx != -1 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			if key != "" {
				m[key] = val
			}
		}
	}

	// Build a compact summary for "Details" (multiline) and "State" (one-liner).
	// Choose which keys are interesting and their order:
	order := []string{"OS", "Kernel", "Uptime", "Packages", "Shell", "DE", "WM", "CPU", "GPU", "Memory", "Disk (/)", "Locale"}
	var detailsParts []string
	for _, k := range order {
		if v, ok := m[k]; ok && v != "" {
			detailsParts = append(detailsParts, fmt.Sprintf("%s: %s", k, v))
		}
	}
	details := strings.Join(detailsParts, "\n")

	// state short: user@host + display/res if present
	stateParts := []string{}
	if uh, ok := m["UserHost"]; ok {
		stateParts = append(stateParts, uh)
	}
	if disp, ok := m["Display"]; ok {
		// Display line often contains resolution and model in fastfetch
		stateParts = append(stateParts, disp)
	}
	state := strings.Join(stateParts, " • ")

	btns := []*client.Button{}
	pasteURL := uploadToPasteService(details + "\n\n" + state)
	if pasteURL != "" {
		btns = append(btns, &client.Button{
			Label: "Full fastfetch output",
			Url:   pasteURL,
		})
	}

	// map some keys to assets (you must upload images to your Discord app and use these keys)
	largeImageKey := pickImageKey(m) // helper below
	largeText := m["DE"]
	if largeText == "" {
		largeText = m["WM"]
	}
	smallImageKey := "dot" // fallback; upload a generic asset named "dot" or change accordingly
	smallText := m["Kernel"]

	// ensure we don't exceed Discord field length limits (use conservative 128)
	details = truncateRunes(details, 128)
	state = truncateRunes(state, 128)
	largeText = truncateRunes(largeText, 128)
	smallText = truncateRunes(smallText, 128)

	activity := client.Activity{
		Details:    details,
		State:      state,
		LargeImage: largeImageKey,
		LargeText:  largeText,
		SmallImage: smallImageKey,
		SmallText:  smallText,
		Timestamps: &client.Timestamps{Start: ptrTime(time.Now())},
		Buttons:    btns,
	}

	return activity, nil
}

func ptrTime(t time.Time) *time.Time { return &t }

// truncateRunes truncates a string to at most n runes and appends ellipsis if trimmed.
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

// pickImageKey maps fastfetch fields to your uploaded asset keys.
// Edit this to match the asset keys you uploaded to the Discord app.
func pickImageKey(m map[string]string) string {
	// example rules (customize)
	if os, ok := m["OS"]; ok {
		os = strings.ToLower(os)
		if strings.Contains(os, "arch") {
			return "arch" // asset key "arch"
		}
		if strings.Contains(os, "ubuntu") {
			return "ubuntu"
		}
		if strings.Contains(os, "fedora") {
			return "fedora"
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
		if strings.Contains(gpu, "intel") {
			return "intel"
		}
	}
	return "default_os"
}

// uploadToPasteService is a stub. You can implement uploading to a pastebin/Hastebin service and return the URL.
// For now, just return an empty string (Discord buttons require a valid URL, so implement if you want buttons).
func uploadToPasteService(text string) string {
	// implement network upload & return URL
	// return "" to omit button
	return ""
}

func main() {
	// Example usage
	out, err := RunFastfetch(context.Background())
	if err != nil && out == "" {
		fmt.Println("fastfetch failed:", err)
		// continue with defaults or return
	}

	act, err := ParseFastfetch(out)
	if err != nil {
		fmt.Println("parse failed:", err)
		return
	}

	if err := client.Login(""); err != nil {
		panic(err)
	}
	if err := client.SetActivity(act); err != nil {
		fmt.Println("set activity failed:", err)
	}
	select {} // keep running to show presence
}
