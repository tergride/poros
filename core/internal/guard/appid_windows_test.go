package guard

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func blobText(blob *fwpByteBlob) string {
	return windows.UTF16ToString(unsafe.Slice((*uint16)(unsafe.Pointer(blob.data)), blob.size/2))
}

func TestPermitPorosByItsPathInLowerCase(t *testing.T) {
	// The app ID of a connection is the path of its program in lower case.
	for _, folder := range []string{"Poros Folder", "Мои Программы Ёж", "Überprüfung"} {
		dir := filepath.Join(t.TempDir(), folder)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		file := filepath.Join(dir, "PorosCore.exe")
		if err := os.WriteFile(file, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		appID, err := appIDFromFile(file)
		if err != nil {
			t.Fatal(err)
		}
		given := blobText(appID)
		var pin runtime.Pinner
		var permitted []string
		for _, c := range baseRules(&config{}, appID, &pin)[0].conditions {
			permitted = append(permitted, blobText(*(**fwpByteBlob)(unsafe.Pointer(&c.conditionValue.value))))
		}
		if !slices.Contains(permitted, strings.ToLower(given)) {
			t.Errorf("%s: no permit for the path in lower case, only %q", folder, permitted)
		}
		if !slices.Contains(permitted, given) {
			t.Errorf("%s: the app ID Windows gave is gone", folder)
		}
		for _, text := range permitted {
			if !strings.EqualFold(text, given) {
				t.Errorf("%s: %q is another program", folder, text)
			}
		}
		if len(slices.Compact(slices.Clone(permitted))) != len(permitted) {
			t.Errorf("%s: the same app ID twice", folder)
		}
		pin.Unpin()
		fwpmFreeMemory0(unsafe.Pointer(&appID))
	}
}
