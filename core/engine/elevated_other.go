//go:build !windows

package engine

import "os"

// elevated tells whether the core may create a TUN; Android hands one over from its VPN service instead.
func elevated() bool {
	return os.Geteuid() == 0
}
