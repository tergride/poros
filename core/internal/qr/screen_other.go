//go:build !windows

package qr

import "image"

// Screen waits for the snapshots of Linux and Android, which come with their stages.
func Screen() (*image.Gray, error) {
	return nil, ErrUnsupported
}
