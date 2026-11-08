//go:build !windows

package engine

// ProxyRecovery: no other platform has a system proxy Poros sets yet.
func ProxyRecovery(string) func(bool) {
	return nil
}
