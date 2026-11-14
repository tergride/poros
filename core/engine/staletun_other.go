//go:build !windows

package engine

// removeStaleTun has nothing to remove: a TUN that is gone leaves no device behind.
func removeStaleTun() (bool, error) { return false, nil }
