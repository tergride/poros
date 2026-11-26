package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"

	"golang.org/x/crypto/argon2"
)

// KDFArgon2id names the derivation of a key from a password: Argon2id.
const KDFArgon2id = "argon2id"

// The cost of a derivation, the second choice of RFC 9106: 3 passes over 64 MiB in 4 lanes.
const (
	passwordTime    = 3
	passwordMemory  = 64 * 1024
	passwordThreads = 4
)

// A sealed file brings its own cost; a file that asks for more than these is refused rather than let use up the
// computer.
const (
	maxPasswordTime    = 10
	maxPasswordMemory  = 1024 * 1024
	maxPasswordThreads = 16
)

// ErrWrongPassword is a password that does not open what was sealed with another one, or data that was changed.
var ErrWrongPassword = errors.New("the password is wrong")

// PasswordSealed is data sealed with a password: AES-256-GCM under the key Argon2id derives from it with the cost and
// the salt kept alongside. Memory is in KiB.
type PasswordSealed struct {
	KDF     string `json:"kdf"`
	Time    uint32 `json:"time"`
	Memory  uint32 `json:"memory"`
	Threads uint8  `json:"threads"`
	Salt    []byte `json:"salt"`
	Nonce   []byte `json:"nonce"`
	Data    []byte `json:"data"`
}

// SealWithPassword seals data with a password; context binds it to its use, as a file of one kind.
func SealWithPassword(data []byte, password, context string) (PasswordSealed, error) {
	if password == "" {
		return PasswordSealed{}, errors.New("secrets: no password")
	}
	s := PasswordSealed{KDF: KDFArgon2id, Time: passwordTime, Memory: passwordMemory, Threads: passwordThreads, Salt: make([]byte, 16)}
	if _, err := rand.Read(s.Salt); err != nil {
		return PasswordSealed{}, err
	}
	aead, err := s.aead(password)
	if err != nil {
		return PasswordSealed{}, err
	}
	s.Nonce = make([]byte, aead.NonceSize())
	if _, err := rand.Read(s.Nonce); err != nil {
		return PasswordSealed{}, err
	}
	s.Data = aead.Seal(nil, s.Nonce, data, []byte(context))
	return s, nil
}

// OpenWithPassword opens what SealWithPassword sealed under the same context.
func (s PasswordSealed) OpenWithPassword(password, context string) ([]byte, error) {
	switch {
	case s.KDF != KDFArgon2id:
		return nil, fmt.Errorf("secrets: the key comes from %q, and Poros knows %s", s.KDF, KDFArgon2id)
	case s.Time < 1 || s.Time > maxPasswordTime || s.Memory < 8*uint32(s.Threads) || s.Memory > maxPasswordMemory ||
		s.Threads < 1 || s.Threads > maxPasswordThreads || len(s.Salt) < 8:
		return nil, errors.New("secrets: the cost of the key is out of bounds")
	}
	aead, err := s.aead(password)
	if err != nil {
		return nil, err
	}
	if len(s.Nonce) != aead.NonceSize() {
		return nil, errors.New("secrets: the nonce has a wrong length")
	}
	data, err := aead.Open(nil, s.Nonce, s.Data, []byte(context))
	if err != nil {
		return nil, ErrWrongPassword
	}
	return data, nil
}

func (s PasswordSealed) aead(password string) (cipher.AEAD, error) {
	key := argon2.IDKey([]byte(password), s.Salt, s.Time, s.Memory, s.Threads, KeySize)
	defer clear(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
