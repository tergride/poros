package secrets

import (
	"bytes"
	"errors"
	"testing"
)

func TestSealWithPassword(t *testing.T) {
	data := []byte(`{"kind":"poros-backup","password":"pw"}`)
	sealed, err := SealWithPassword(data, "correct horse", "poros-backup")
	if err != nil {
		t.Fatal(err)
	}
	if sealed.KDF != KDFArgon2id || len(sealed.Salt) != 16 || bytes.Contains(sealed.Data, []byte("poros-backup")) {
		t.Fatalf("a sealed file: %+v", sealed)
	}
	if opened, err := sealed.OpenWithPassword("correct horse", "poros-backup"); err != nil || !bytes.Equal(opened, data) {
		t.Fatalf("open: %q, %v", opened, err)
	}
	for name, open := range map[string]func() ([]byte, error){
		"a wrong password": func() ([]byte, error) { return sealed.OpenWithPassword("correct horse ", "poros-backup") },
		"another context":  func() ([]byte, error) { return sealed.OpenWithPassword("correct horse", "poros-route") },
		"changed data": func() ([]byte, error) {
			s := sealed
			s.Data = bytes.Clone(s.Data)
			s.Data[0] ^= 1
			return s.OpenWithPassword("correct horse", "poros-backup")
		},
		"another salt": func() ([]byte, error) {
			s := sealed
			s.Salt = make([]byte, 16)
			return s.OpenWithPassword("correct horse", "poros-backup")
		},
	} {
		if _, err := open(); !errors.Is(err, ErrWrongPassword) {
			t.Errorf("%s: %v", name, err)
		}
	}

	// A file that asks for a cost out of bounds is refused before the key is derived.
	for name, change := range map[string]func(s *PasswordSealed){
		"another KDF":       func(s *PasswordSealed) { s.KDF = "scrypt" },
		"16 GiB":            func(s *PasswordSealed) { s.Memory = 16 * 1024 * 1024 },
		"a thousand passes": func(s *PasswordSealed) { s.Time = 1000 },
		"no lanes":          func(s *PasswordSealed) { s.Threads = 0 },
		"no salt":           func(s *PasswordSealed) { s.Salt = nil },
		"a short nonce":     func(s *PasswordSealed) { s.Nonce = s.Nonce[:4] },
	} {
		s := sealed
		change(&s)
		if _, err := s.OpenWithPassword("correct horse", "poros-backup"); err == nil || errors.Is(err, ErrWrongPassword) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := SealWithPassword(data, "", "poros-backup"); err == nil {
		t.Error("a seal without a password")
	}
}
