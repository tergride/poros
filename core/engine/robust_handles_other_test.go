//go:build !windows

package engine

// processHandles is Windows's count of handles; elsewhere there is none to compare.
func processHandles() int { return -1 }
