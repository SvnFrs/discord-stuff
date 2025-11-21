package utils

import (
	"context"
	"fmt"
	"time"

	"example.com/presence/lib/client"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"
	"github.com/shirou/gopsutil/net"
)

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

func monitorLoop(ctx context.Context, clientID string) {
    // initial handshake
    if err := client.Login(clientID); err != nil {
        panic(err)
    }

    // initial network counters
    prevNet, _ := net.IOCounters(false)
    prevTime := time.Now()

    // initial CPU warmup call (for more stable values)
    _, _ = cpu.Percent(0, false)

    // poll loop
    ticker := time.NewTicker(10 * time.Second) // tune interval
    defer ticker.Stop()

    var lastActivity client.Activity

    for {
        select {
        case <-ctx.Done():
            client.Logout()
            return
        case <-ticker.C:
            // CPU percent (since last call) - this is instantaneous for overall
            cpus, _ := cpu.Percent(0, false)
            cpuPct := 0.0
            if len(cpus) > 0 {
                cpuPct = cpus[0]
            }

            // Memory
            vm, _ := mem.VirtualMemory()

            // Network throughput: compute delta since last check
            curNet, _ := net.IOCounters(false)
            var rxPerSec, txPerSec float64
            now := time.Now()
            interval := now.Sub(prevTime).Seconds()
            if len(prevNet) > 0 && len(curNet) > 0 && interval > 0 {
                rxPerSec = float64(curNet[0].BytesRecv-prevNet[0].BytesRecv) / interval
                txPerSec = float64(curNet[0].BytesSent-prevNet[0].BytesSent) / interval
            }
            prevNet = curNet
            prevTime = now

            // Build a short state/details
            // Keep strings short: e.g. "CPU 12% • RAM 33% • ↓ 1.3MiB/s ↑ 0.2MiB/s"
            state := fmt.Sprintf("CPU %.0f%% • RAM %.0f%%", cpuPct, vm.UsedPercent)
            netStr := fmt.Sprintf("↓ %s/s ↑ %s/s", humanBytes(uint64(rxPerSec)), humanBytes(uint64(txPerSec)))
            // Optionally include a slightly longer details field
            details := fmt.Sprintf("%s • %s", state, netStr)

            // Map to your activity structure (keep small text concise)
            ts := now
            act := client.Activity{
                State:      state,
                Details:    details, // you can also include OS info from fastfetch
                LargeImage: "arch",  // adjust your uploaded assets as desired
                LargeText:  "GNOME 49.1",
                SmallImage: "dot",
                SmallText:  fmt.Sprintf("%.0f%% RAM", vm.UsedPercent),
                Timestamps: &client.Timestamps{Start: &ts},
            }

            // Only update if different (simple string compare) to reduce sends
            if act.State != lastActivity.State || act.Details != lastActivity.Details || act.SmallText != lastActivity.SmallText {
                if err := client.SetActivity(act); err != nil {
                    fmt.Println("SetActivity failed:", err)
                    // consider reconnecting logic here
                } else {
                    lastActivity = act
                }
            }
        }
    }
}