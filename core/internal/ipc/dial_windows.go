//go:build windows

// Package ipc connects the core to the channel its host created for it.
package ipc

import (
	"net"
	"time"

	"github.com/tailscale/go-winio"
)

// Dial opens the host's named pipe with overlapped I/O, so events can be written while a read is pending.
func Dial(name string) (net.Conn, error) {
	timeout := 5 * time.Second
	return winio.DialPipe(name, &timeout)
}
