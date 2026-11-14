// Package marker tells whether another Poros on this computer holds what only one can: the port of the local proxy,
// the TUN, the block of Windows. A marker is a named object of the system that lives exactly as long as the process
// holding it, so a Poros that crashed leaves none behind. Markers are seen by every Windows user, as the port, the TUN
// and the block are shared by the whole computer.
package marker

import (
	"errors"
	"strconv"
)

// Namespace starts the name of every marker; a test gives its own, so that it never meets a Poros running here.
var Namespace = "Poros"

// ErrTaken is a marker another process holds.
var ErrTaken = errors.New("another Poros on this computer holds it")

// Names of the markers.
const (
	// Tun is held while a TUN of Poros is up.
	Tun = "Tun"
	// Block is held by the guard while its block of Windows is on.
	Block = "Block"
)

// Port names the marker of the local proxy on port.
func Port(port uint16) string {
	return "Port-" + strconv.Itoa(int(port))
}
