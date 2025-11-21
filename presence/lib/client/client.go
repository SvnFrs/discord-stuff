package client

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"

	"example.com/presence/lib/ipc"
)

var logged bool

// login sends a handshake via IPC.
func Login(clientid string) error {
    if !logged {
        payload, err := json.Marshal(Handshake{V: 1, ClientId: clientid})
        if err != nil {
            return err
        }
        if err = ipc.OpenSocket(); err != nil {
            return err
        }
        resp, err := ipc.Send(0, string(payload))
        if err != nil {
            return fmt.Errorf("handshake send/read failed: %w (resp=%q)", err, resp)
        }
        fmt.Println("handshake response:", resp)
        // DO NOT send handshake again
    }
    logged = true
    return nil
}

func Logout() {
	logged = false
	if err := ipc.CloseSocket(); err != nil {
		panic(err)
	}
}

// SetActivity sends an activity update.
func SetActivity(activity Activity) error {
    if !logged {
        return nil
    }

    payload, err := json.Marshal(Frame{
        Cmd: "SET_ACTIVITY",
        Args: Args{
            Pid:      os.Getpid(),
            Activity: mapActivity(&activity),
        },
        Nonce: getNonce(),
    })
    if err != nil {
        return err
    }

    fmt.Println("SET_ACTIVITY payload:", string(payload))

    // First try
    resp, err := ipc.Send(1, string(payload))
    if err != nil {
        fmt.Println("SET_ACTIVITY first send failed:", err, "resp:", resp)
        // If broken pipe or connection dropped, try to reopen once
        if err := ipc.CloseSocket(); err == nil {
            if openErr := ipc.OpenSocket(); openErr == nil {
                fmt.Println("reopened socket, retrying SET_ACTIVITY")
                resp, err = ipc.Send(1, string(payload))
            } else {
                fmt.Println("failed to reopen socket:", openErr)
            }
        }
    }
    if err != nil {
        return fmt.Errorf("SET_ACTIVITY failed: %w (resp=%q)", err, resp)
    }
    fmt.Println("SET_ACTIVITY response:", resp)
    return nil
}

// getNonce creates a nonce string.
// uses a fixed-size array and bit-level operations without extra allocations.
func getNonce() string {
	var buf [16]byte
	_, err := rand.Read(buf[:])
	if err != nil {
		fmt.Println("getNonce error:", err)
	}
	buf[6] = (buf[6] & 0x0F) | 0x40
	buf[8] = (buf[8] & 0x3F) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:])
}
