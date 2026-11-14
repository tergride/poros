//go:build !windows

// Package ipc connects the core to the channel its host created for it.
package ipc

import (
	"net"
	"time"
)

// Dial opens the host's unix socket.
func Dial(name string) (net.Conn, error) {
	return net.DialTimeout("unix", name, 5*time.Second)
}
