//go:build windows

package winsys

import (
	"os"
	"slices"
	"unsafe"

	"github.com/sagernet/sing/common"

	"golang.org/x/sys/windows"
)

func CreateDisplayData(name, description string) FWPM_DISPLAY_DATA0 {
	namePtr, err := windows.UTF16PtrFromString(name)
	common.Must(err)

	descriptionPtr, err := windows.UTF16PtrFromString(description)
	common.Must(err)

	return FWPM_DISPLAY_DATA0{
		Name:        namePtr,
		Description: descriptionPtr,
	}
}

var procRtlDowncaseUnicodeChar = windows.NewLazySystemDLL("ntdll.dll").NewProc("RtlDowncaseUnicodeChar")

// LowerCaseAppID is appID wholly in lower case, as Windows compares it, or nil when appID is that already:
// FwpmGetAppIdFromFileName0 lowers ASCII alone. (A change of Poros.)
func LowerCaseAppID(appID *FWP_BYTE_BLOB) *FWP_BYTE_BLOB {
	given := unsafe.Slice((*uint16)(unsafe.Pointer(appID.data)), appID.size/2)
	lower := make([]uint16, len(given))
	for i, unit := range given {
		r1, _, _ := procRtlDowncaseUnicodeChar.Call(uintptr(unit))
		lower[i] = uint16(r1)
	}
	if slices.Equal(given, lower) {
		return nil
	}
	return &FWP_BYTE_BLOB{size: appID.size, data: (*uint8)(unsafe.Pointer(&lower[0]))}
}

func GetCurrentProcessAppID() (*FWP_BYTE_BLOB, error) {
	currentFile, err := os.Executable()
	if err != nil {
		return nil, err
	}

	curFilePtr, err := windows.UTF16PtrFromString(currentFile)
	if err != nil {
		return nil, err
	}

	windows.GetCurrentProcessId()

	var appID *FWP_BYTE_BLOB
	err = FwpmGetAppIdFromFileName0(curFilePtr, unsafe.Pointer(&appID))
	if err != nil {
		return nil, err
	}
	return appID, nil
}
