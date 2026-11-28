package host

import (
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

// The administrators and the account named may wait for the process and read its program, and nothing more; the
// account of the process keeps its own entry.
func TestOpenToGrantsWaitingOnly(t *testing.T) {
	if Elevated() {
		t.Skip("an elevated process lets the administrators in already")
	}
	if err := OpenTo("S-1-5-32-545"); err != nil {
		t.Fatal(err)
	}
	descriptor, err := windows.GetSecurityInfo(windows.CurrentProcess(), windows.SE_KERNEL_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	sddl := descriptor.String()
	for _, ace := range []string{"(A;;0x101000;;;BA)", "(A;;0x101000;;;BU)"} {
		if !strings.Contains(sddl, ace) {
			t.Errorf("no %s in %s", ace, sddl)
		}
	}
	own, err := User()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sddl, "(A;;0x1fffff;;;"+own+")") {
		t.Errorf("the account of the process lost its entry: %s", sddl)
	}
}
