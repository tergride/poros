package patches

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	_ "github.com/sagernet/sing-tun"
	"golang.org/x/sys/windows"
)

// appIDBlob is winsys.FWP_BYTE_BLOB of sing-tun, which is internal there.
type appIDBlob struct {
	size uint32
	data *uint16
}

//go:linkname lowerCaseAppID github.com/sagernet/sing-tun/internal/winsys.LowerCaseAppID
func lowerCaseAppID(appID *appIDBlob) *appIDBlob

// Without the change there is no lowerCaseAppID to link to: Strict Route permitted the program by the ID Windows
// gave, and blocked its DNS in a folder such as "Мои программы".
func TestStrictRoutePermitsTheProgramByItsPathInLowerCase(t *testing.T) {
	getAppID := windows.NewLazySystemDLL("fwpuclnt.dll").NewProc("FwpmGetAppIdFromFileName0")
	free := windows.NewLazySystemDLL("fwpuclnt.dll").NewProc("FwpmFreeMemory0")
	for folder, differs := range map[string]bool{"Poros Folder": false, "Мои Программы": true, "Überprüfung": true} {
		dir := filepath.Join(t.TempDir(), folder)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		file := filepath.Join(dir, "PorosCore.exe")
		if err := os.WriteFile(file, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		name, err := windows.UTF16PtrFromString(file)
		if err != nil {
			t.Fatal(err)
		}
		var given *appIDBlob
		if status, _, _ := getAppID.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&given))); status != 0 {
			t.Fatalf("%s: %v", folder, windows.Errno(status))
		}
		text := windows.UTF16ToString(unsafe.Slice(given.data, given.size/2))
		lower := lowerCaseAppID(given)
		switch {
		case (lower != nil) != differs:
			t.Errorf("%s: another app ID %t", folder, lower != nil)
		case lower != nil:
			if got := windows.UTF16ToString(unsafe.Slice(lower.data, lower.size/2)); got != strings.ToLower(text) {
				t.Errorf("%s: %q", folder, got)
			}
		}
		free.Call(uintptr(unsafe.Pointer(&given)))
	}
}
