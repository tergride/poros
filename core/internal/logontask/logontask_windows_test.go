//go:build windows

package logontask

import "testing"

func TestEveryCopyHasItsTask(t *testing.T) {
	a, b := Name(`C:\A\PorosCore.exe`), Name(`C:\B\PorosCore.exe`)
	if a == b || a != Name(`c:\a\PorosCore.exe`) {
		t.Errorf("names %q and %q", a, b)
	}
}
