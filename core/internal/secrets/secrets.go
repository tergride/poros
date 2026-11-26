// Package secrets keeps the secrets of the data file. One data key, sealed for this computer (DPAPI on Windows),
// encrypts each secret with AES-256-GCM; the rest of a record stays as it is, so what a record lacks is plain to see
// where the key cannot be opened. A file for another computer, as a backup, is sealed with a password instead.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
)

// Prefix marks a sealed value; a value without it is a plain one, as data written before the secrets were sealed.
const Prefix = "poros:sec:1:"

// KeySize is the length of a data key.
const KeySize = 32

// ErrLost is a sealed value that the data key of this computer does not open.
var ErrLost = errors.New("the secret was sealed on another computer")

// additionalData binds a sealed value to Poros: a ciphertext of another program under the same key does not open.
var additionalData = []byte("poros secret")

// Box seals and opens values with one data key.
type Box struct {
	aead cipher.AEAD
}

// NewKey returns a new random data key.
func NewKey() ([]byte, error) {
	key := make([]byte, KeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return key, nil
}

// New returns a box for a data key of KeySize bytes.
func New(key []byte) (*Box, error) {
	if len(key) != KeySize {
		return nil, errors.New("secrets: a data key has 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Sealed tells a sealed value.
func Sealed(value string) bool { return strings.HasPrefix(value, Prefix) }

// Seal returns the value sealed; an empty or already sealed value stays as it is.
func (b *Box) Seal(value string) string {
	if value == "" || Sealed(value) {
		return value
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		panic(err) // crypto/rand does not fail on the systems Poros runs on
	}
	sealed := b.aead.Seal(nonce, nonce, []byte(value), additionalData)
	return Prefix + base64.RawStdEncoding.EncodeToString(sealed)
}

// Open returns a sealed value open and a plain value as it is. A value this key does not open is ErrLost.
func (b *Box) Open(value string) (string, error) {
	if !Sealed(value) {
		return value, nil
	}
	data, err := base64.RawStdEncoding.DecodeString(value[len(Prefix):])
	if err != nil || len(data) < b.aead.NonceSize() {
		return "", ErrLost
	}
	nonce, ciphertext := data[:b.aead.NonceSize()], data[b.aead.NonceSize():]
	plain, err := b.aead.Open(nil, nonce, ciphertext, additionalData)
	if err != nil {
		return "", ErrLost
	}
	return string(plain), nil
}

// The first byte of a protected data key says how it is protected.
const (
	schemeNone  byte = 0
	schemeDPAPI byte = 1
)

// Protect seals a data key for this computer: with DPAPI of the computer on Windows; elsewhere, until Poros has a
// store of keys there, it is kept as it is.
func Protect(key []byte) ([]byte, error) {
	return protect(key)
}

// Unprotect opens a data key Protect sealed. A key of another computer is an error.
func Unprotect(blob []byte) ([]byte, error) {
	if len(blob) == 0 {
		return nil, errors.New("secrets: no data key")
	}
	switch blob[0] {
	case schemeNone:
		return append([]byte(nil), blob[1:]...), nil
	case schemeDPAPI:
		return unprotectDPAPI(blob[1:])
	}
	return nil, errors.New("secrets: a data key of an unknown kind")
}
