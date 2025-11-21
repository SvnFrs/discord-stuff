package ipc

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
)

var (
	socket  net.Conn
	ipcPath string
)

// GetIpcPath returns the directory for the IPC socket
// cache the result so that filesystem checks and environment lookups are done once
func GetIpcPath() string {
	if ipcPath != "" {
		return ipcPath
	}

	names := []string{"XDG_RUNTIME_DIR", "TMPDIR", "TMP", "TEMP"}

	// Check known paths first
	if _, err := os.Stat("/run/user/1000/snap.discord"); err == nil {
		ipcPath = "/run/user/1000/snap.discord"
		return ipcPath
	}

	if _, err := os.Stat("/run/user/1000/.flatpak/com.discordapp.Discord/xdg-run"); err == nil {
		ipcPath = "/run/user/1000/.flatpak/com.discordapp.Discord/xdg-run"
		return ipcPath
	}

	// fall back to environment variables
	for _, name := range names {
		if path, exists := os.LookupEnv(name); exists && path != "" {
			ipcPath = path
			return ipcPath
		}
	}

	ipcPath = "/tmp"
	return ipcPath
}

func CloseSocket() error {
	if socket != nil {
		socket.Close()
		socket = nil
	}
	return nil
}

// Read returns the IPC socket response as a string
// simply slice the backing array.
func Read() (string, error) {
    // read exactly 8-byte header
    hdr := make([]byte, 8)
    n := 0
    for n < 8 {
        nn, err := socket.Read(hdr[n:])
        if err != nil {
            return "", fmt.Errorf("read header error: %w", err)
        }
        n += nn
    }
    // opcode := binary.LittleEndian.Uint32(hdr[0:4])
    length := binary.LittleEndian.Uint32(hdr[4:8])

    if length == 0 {
        return "", nil
    }

    buf := make([]byte, length)
    got := 0
    for uint32(got) < length {
        nn, err := socket.Read(buf[got:])
        if err != nil {
            return "", fmt.Errorf("read payload error: %w", err)
        }
        got += nn
    }

    // optional: debug log
    // fmt.Printf("Read opcode=%d length=%d\n", opcode, length)
    return string(buf), nil
}

func Send(opcode int, payload string) (string, error) {
    hdr := make([]byte, 8)
    binary.LittleEndian.PutUint32(hdr[0:4], uint32(opcode))
    binary.LittleEndian.PutUint32(hdr[4:8], uint32(len(payload)))

    msg := append(hdr, []byte(payload)...)
    if _, err := socket.Write(msg); err != nil {
        return "", fmt.Errorf("write error: %w", err)
    }

    resp, err := Read()
    if err != nil {
        return "", err
    }
    return resp, nil
}
