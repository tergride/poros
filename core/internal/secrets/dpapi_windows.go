package secrets

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// The scope of the computer: a core raised with the rights of another account opens the key too.
const dpapiFlags = windows.CRYPTPROTECT_LOCAL_MACHINE | windows.CRYPTPROTECT_UI_FORBIDDEN

var dpapiEntropy = []byte("Poros data key")

func blob(data []byte) *windows.DataBlob {
	if len(data) == 0 {
		return &windows.DataBlob{}
	}
	return &windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
}

func takeBlob(out *windows.DataBlob) []byte {
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...)
}

func protect(key []byte) ([]byte, error) {
	var out windows.DataBlob
	if err := windows.CryptProtectData(blob(key), nil, blob(dpapiEntropy), 0, nil, dpapiFlags, &out); err != nil {
		return nil, err
	}
	return append([]byte{schemeDPAPI}, takeBlob(&out)...), nil
}

func unprotectDPAPI(data []byte) ([]byte, error) {
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(blob(data), nil, blob(dpapiEntropy), 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	return takeBlob(&out), nil
}
