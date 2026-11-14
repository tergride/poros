package engine

import "golang.org/x/sys/windows"

// elevated tells whether the core has the administrator rights TUN needs.
func elevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}
