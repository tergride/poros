package stbkey

import (
	"bytes"
	"crypto/rand"
	"testing"

	"PorosCore/internal/stb/bign"
)

// FuzzParsePrivateKey feeds key files of any shape: they come from users and from imported configs.
func FuzzParsePrivateKey(f *testing.F) {
	for _, c := range bign.Curves {
		d, _, _ := c.GenerateKey(rand.Reader)
		key := &PrivateKey{Curve: c, D: d}
		plain, _ := EncodePEM(key)
		f.Add(plain, []byte(nil))
		sealed, _ := Encrypt(key, []byte("x"), rand.Reader)
		f.Add(sealed, []byte("x"))
		der, _ := MarshalPKCS8(key)
		f.Add(der, []byte(nil))
	}
	f.Add([]byte("-----BEGIN ENCRYPTED PRIVATE KEY-----\nMAA=\n-----END ENCRYPTED PRIVATE KEY-----\n"), []byte("x"))
	f.Fuzz(func(t *testing.T, data, password []byte) {
		key, err := ParsePrivateKey(data, password)
		if err != nil {
			return
		}
		pub, err := key.PublicKey()
		if err != nil || !key.Curve.ValidPublicKey(pub) {
			t.Fatalf("a parsed key has no public key: %v", err)
		}
		again, err := ParsePrivateKey(mustPEM(t, key), nil)
		if err != nil || !bytes.Equal(again.D, key.D) {
			t.Fatalf("the parsed key does not survive a round trip: %v", err)
		}
	})
}

func mustPEM(t *testing.T, key *PrivateKey) []byte {
	t.Helper()
	out, err := EncodePEM(key)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// FuzzParsePublicKeyInfo feeds public keys of any shape: they come from certificates of servers.
func FuzzParsePublicKeyInfo(f *testing.F) {
	for _, c := range bign.Curves {
		_, pub, _ := c.GenerateKey(rand.Reader)
		der, _ := MarshalPublicKeyInfo(&PublicKey{Curve: c, Point: pub})
		f.Add(der)
	}
	f.Fuzz(func(t *testing.T, der []byte) {
		key, err := ParsePublicKeyInfo(der)
		if err == nil && !key.Curve.ValidPublicKey(key.Point) {
			t.Fatal("a parsed public key is not on its curve")
		}
	})
}
