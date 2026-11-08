//go:build !windows

package logontask

import (
	"errors"
	"net"
)

var errUnsupported = errors.New("rights at sign-in are for Windows")

func Register(core, dataDir string) error {
	return errUnsupported
}

func Remove(core, dataDir string) error {
	clearRemoval(dataDir)
	return nil
}

func AutostartWaits(core string) bool {
	return false
}

func VerifyShell(conn net.Conn, hostPID int, core string) error {
	return errUnsupported
}
