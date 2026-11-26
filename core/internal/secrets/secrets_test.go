package secrets

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func newBox(t *testing.T) *Box {
	t.Helper()
	key, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	box, err := New(key)
	if err != nil {
		t.Fatal(err)
	}
	return box
}

func TestBoxSealsAndOpens(t *testing.T) {
	box := newBox(t)
	sealed := box.Seal("pässword")
	if !Sealed(sealed) || strings.Contains(sealed, "pässword") {
		t.Fatalf("a sealed value: %q", sealed)
	}
	if again := box.Seal("pässword"); again == sealed {
		t.Error("two seals of one value are the same: the nonce does not change")
	}
	if box.Seal(sealed) != sealed || box.Seal("") != "" {
		t.Error("a sealed or an empty value is sealed again")
	}
	if opened, err := box.Open(sealed); err != nil || opened != "pässword" {
		t.Fatalf("open: %q, %v", opened, err)
	}
	if opened, err := box.Open("plain"); err != nil || opened != "plain" {
		t.Errorf("a plain value opens as it is: %q, %v", opened, err)
	}

	if opened, err := newBox(t).Open(sealed); !errors.Is(err, ErrLost) || opened != "" {
		t.Errorf("another key: %q, %v", opened, err)
	}
	changed := []byte(sealed)
	changed[len(Prefix)+20] ^= 'A' ^ 'B'
	for name, value := range map[string]string{
		"not base64": Prefix + "%%%",
		"too short":  Prefix + "AAAA",
		"cut":        sealed[:len(sealed)-4],
		"changed":    string(changed),
	} {
		if opened, err := box.Open(value); !errors.Is(err, ErrLost) || opened != "" {
			t.Errorf("%s: %q, %v", name, opened, err)
		}
	}
	if _, err := New(make([]byte, 16)); err == nil {
		t.Error("a key of 16 bytes")
	}
}

// The data key comes back from its protection, and a protection this computer did not make is an error.
func TestProtectedKey(t *testing.T) {
	key, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	blob, err := Protect(key)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := Unprotect(blob)
	if err != nil || !bytes.Equal(opened, key) {
		t.Fatalf("unprotect: %x, %v", opened, err)
	}
	for name, blob := range map[string][]byte{
		"empty":           nil,
		"an unknown kind": {9, 1, 2, 3},
		"a foreign DPAPI": append([]byte{schemeDPAPI}, bytes.Repeat([]byte{0x5a}, 200)...),
	} {
		if _, err := Unprotect(blob); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}
