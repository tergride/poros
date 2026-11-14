package engine

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenTellsADamagedDataFile(t *testing.T) {
	dir := t.TempDir()
	e, _ := openTest(t, dir)
	e.Close()
	path := filepath.Join(dir, DataFile)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// What a power cut in the middle of a write may leave: the first pages overwritten.
	for i := range 8192 {
		data[i] = byte(i * 7)
	}
	if err = os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = Open(Config{DataDir: dir}); !errors.Is(err, ErrDataDamaged) {
		t.Fatalf("a damaged file must be told apart: %v", err)
	}
	if err = os.WriteFile(path, []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = Open(Config{DataDir: dir}); !errors.Is(err, ErrDataDamaged) {
		t.Fatalf("a cut file must be told apart: %v", err)
	}
}
