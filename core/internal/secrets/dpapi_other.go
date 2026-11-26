//go:build !windows

package secrets

import "errors"

func protect(key []byte) ([]byte, error) {
	return append([]byte{schemeNone}, key...), nil
}

func unprotectDPAPI([]byte) ([]byte, error) {
	return nil, errors.New("secrets: the data key is sealed by DPAPI of a Windows computer")
}
