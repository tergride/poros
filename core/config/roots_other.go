//go:build !windows

package config

// Only Windows goes to the internet for a chain, so elsewhere a public DNS is left to the system like any other.
func dnsRoots() string {
	return ""
}
