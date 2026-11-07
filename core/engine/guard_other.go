//go:build !windows

package engine

// NewGuard is nil: the kill switch is there on Windows only.
func NewGuard(int, string) Guard { return nil }
