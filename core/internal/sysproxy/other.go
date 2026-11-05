//go:build !windows

package sysproxy

// Linux desktops keep the proxy in GNOME or KDE settings, and Android sets none; neither is done yet.
func platformBackend() backend { return nil }
