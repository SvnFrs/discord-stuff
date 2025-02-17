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
func Read() string {
	buf := make([]byte, 512)
	n, err := socket.Read(buf)
	if err != nil {
		fmt.Println("Nothing to read:", err)
		// return an empty string if nothing is read.
		return ""
	}

	if n <= 8 {
		return ""
	}

	return string(buf[8:n])
}

// Send builds the message and writes it to the socket
func Send(opcode int, payload string) string {
	// create a fixed-size header buffer to avoid multiple allocations
	hdr := make([]byte, 8)
	binary.LittleEndian.PutUint32(hdr[0:4], uint32(opcode))
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(len(payload)))

	// concatenate header and payload.
	msg := append(hdr, payload...)
	_, err := socket.Write(msg)
	if err != nil {
		fmt.Println("Error writing to socket:", err)
	}

	return Read()
}
