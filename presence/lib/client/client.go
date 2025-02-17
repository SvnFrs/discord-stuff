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
		payload, err := json.Marshal(Handshake{"1", clientid})
		if err != nil {
			return err
		}

		if err = ipc.OpenSocket(); err != nil {
			return err
		}

		ipc.Send(0, string(payload))
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

	ipc.Send(1, string(payload))
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
