//go:build windows

package marker

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

// testName keeps a test away from the markers of a Poros that runs on this computer.
func testName(t *testing.T) string {
	return fmt.Sprintf("Test-%s-%d", t.Name(), os.Getpid())
}

func TestMarkerHeldAndReleased(t *testing.T) {
	name := testName(t)
	if Taken(name) {
		t.Fatal("a marker nobody holds is taken")
	}
	held, err := Hold(name)
	if err != nil {
		t.Fatalf("an account without administrator rights must hold a marker: %v", err)
	}
	if !Taken(name) {
		t.Fatal("a held marker is not taken")
	}
	if _, err = Hold(name); !errors.Is(err, ErrTaken) {
		t.Fatalf("a second holder got %v, want ErrTaken", err)
	}
	held.Release()
	held.Release()
	if Taken(name) {
		t.Fatal("a released marker is still taken")
	}
	again, err := Hold(name)
	if err != nil {
		t.Fatalf("a released marker could not be held again: %v", err)
	}
	again.Release()
}

// Another process holds the mutex as Hold makes it: the marker is taken, and nothing here may hold it.
func TestMarkerOfAnotherHolder(t *testing.T) {
	name := testName(t)
	object, err := objectName(name)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateMutex(nil, false, object)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	if !Taken(name) {
		t.Fatal("the marker of another holder is not taken")
	}
	if _, err = Hold(name); !errors.Is(err, ErrTaken) {
		t.Fatalf("Hold got %v, want ErrTaken", err)
	}
}

func TestPortNames(t *testing.T) {
	if Port(2090) != "Port-2090" || Port(2090) == Port(2091) {
		t.Fatalf("Port(2090) = %q", Port(2090))
	}
}
