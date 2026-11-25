// Package belt implements the block cipher and modes of STB 34.101.31 (belt) and the HMAC of STB 34.101.47 that
// rests on it. The code follows the bee2 library (Apache-2.0).
package belt

import (
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"math/bits"
)

// BlockSize is the size of a belt block in bytes.
const BlockSize = 16

// H is the substitution table of belt, which also seeds belt-hash and the test vectors of the standard.
var H = [256]byte{
	0xB1, 0x94, 0xBA, 0xC8, 0x0A, 0x08, 0xF5, 0x3B, 0x36, 0x6D, 0x00, 0x8E, 0x58, 0x4A, 0x5D, 0xE4,
	0x85, 0x04, 0xFA, 0x9D, 0x1B, 0xB6, 0xC7, 0xAC, 0x25, 0x2E, 0x72, 0xC2, 0x02, 0xFD, 0xCE, 0x0D,
	0x5B, 0xE3, 0xD6, 0x12, 0x17, 0xB9, 0x61, 0x81, 0xFE, 0x67, 0x86, 0xAD, 0x71, 0x6B, 0x89, 0x0B,
	0x5C, 0xB0, 0xC0, 0xFF, 0x33, 0xC3, 0x56, 0xB8, 0x35, 0xC4, 0x05, 0xAE, 0xD8, 0xE0, 0x7F, 0x99,
	0xE1, 0x2B, 0xDC, 0x1A, 0xE2, 0x82, 0x57, 0xEC, 0x70, 0x3F, 0xCC, 0xF0, 0x95, 0xEE, 0x8D, 0xF1,
	0xC1, 0xAB, 0x76, 0x38, 0x9F, 0xE6, 0x78, 0xCA, 0xF7, 0xC6, 0xF8, 0x60, 0xD5, 0xBB, 0x9C, 0x4F,
	0xF3, 0x3C, 0x65, 0x7B, 0x63, 0x7C, 0x30, 0x6A, 0xDD, 0x4E, 0xA7, 0x79, 0x9E, 0xB2, 0x3D, 0x31,
	0x3E, 0x98, 0xB5, 0x6E, 0x27, 0xD3, 0xBC, 0xCF, 0x59, 0x1E, 0x18, 0x1F, 0x4C, 0x5A, 0xB7, 0x93,
	0xE9, 0xDE, 0xE7, 0x2C, 0x8F, 0x0C, 0x0F, 0xA6, 0x2D, 0xDB, 0x49, 0xF4, 0x6F, 0x73, 0x96, 0x47,
	0x06, 0x07, 0x53, 0x16, 0xED, 0x24, 0x7A, 0x37, 0x39, 0xCB, 0xA3, 0x83, 0x03, 0xA9, 0x8B, 0xF6,
	0x92, 0xBD, 0x9B, 0x1C, 0xE5, 0xD1, 0x41, 0x01, 0x54, 0x45, 0xFB, 0xC9, 0x5E, 0x4D, 0x0E, 0xF2,
	0x68, 0x20, 0x80, 0xAA, 0x22, 0x7D, 0x64, 0x2F, 0x26, 0x87, 0xF9, 0x34, 0x90, 0x40, 0x55, 0x11,
	0xBE, 0x32, 0x97, 0x13, 0x43, 0xFC, 0x9A, 0x48, 0xA0, 0x2A, 0x88, 0x5F, 0x19, 0x4B, 0x09, 0xA1,
	0x7E, 0xCD, 0xA4, 0xD0, 0x15, 0x44, 0xAF, 0x8C, 0xA5, 0x84, 0x50, 0xBF, 0x66, 0xD2, 0xE8, 0x8A,
	0xA2, 0xD7, 0x46, 0x52, 0x42, 0xA8, 0xDF, 0xB3, 0x69, 0x74, 0xC5, 0x51, 0xEB, 0x23, 0x29, 0x21,
	0xD4, 0xEF, 0xD9, 0xB4, 0x3A, 0x62, 0x28, 0x75, 0x91, 0x14, 0x10, 0xEA, 0x77, 0x6C, 0xDA, 0x1D,
}

// h5, h13, h21 and h29 are H with each entry rotated left by 5, 13, 21 and 29 bits, so that a G-box is four lookups.
var h5, h13, h21, h29 [256]uint32

func init() {
	for i, v := range H {
		h5[i] = bits.RotateLeft32(uint32(v), 5)
		h13[i] = bits.RotateLeft32(uint32(v), 13)
		h21[i] = bits.RotateLeft32(uint32(v), 21)
		h29[i] = bits.RotateLeft32(uint32(v), 29)
	}
}

func g5(x uint32) uint32  { return h5[x&255] ^ h13[x>>8&255] ^ h21[x>>16&255] ^ h29[x>>24] }
func g13(x uint32) uint32 { return h13[x&255] ^ h21[x>>8&255] ^ h29[x>>16&255] ^ h5[x>>24] }
func g21(x uint32) uint32 { return h21[x&255] ^ h29[x>>8&255] ^ h5[x>>16&255] ^ h13[x>>24] }

// KeySizeError reports a key whose length belt does not take.
type KeySizeError int

func (k KeySizeError) Error() string {
	return "belt: invalid key size " + itoa(int(k))
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// expandKey turns a key of 16, 24 or 32 bytes into the eight words belt works with.
func expandKey(key []byte) ([8]uint32, error) {
	var k [8]uint32
	switch len(key) {
	case 16, 24, 32:
	default:
		return k, KeySizeError(len(key))
	}
	for i := 0; i < len(key)/4; i++ {
		k[i] = binary.LittleEndian.Uint32(key[4*i:])
	}
	switch len(key) {
	case 16:
		copy(k[4:], k[:4])
	case 24:
		k[6] = k[0] ^ k[1] ^ k[2]
		k[7] = k[3] ^ k[4] ^ k[5]
	}
	return k, nil
}

// round is one of the eight rounds: a..d come in the order the round permutes them to, and k0..k6 are its subkeys.
func round(a, b, c, d uint32, k *[8]uint32, i uint32, ks [7]int) (uint32, uint32, uint32, uint32) {
	b ^= g5(a + k[ks[0]])
	c ^= g21(d + k[ks[1]])
	a -= g13(b + k[ks[2]])
	c += b
	b += g21(c+k[ks[3]]) ^ i
	c -= b
	d += g13(c + k[ks[4]])
	b ^= g21(a + k[ks[5]])
	c ^= g5(d + k[ks[6]])
	return a, b, c, d
}

// encSubkeys and decSubkeys are the subkey indexes of each round for encryption and decryption.
var encSubkeys, decSubkeys [9][7]int

func init() {
	for i := 1; i <= 8; i++ {
		for j := 0; j < 7; j++ {
			encSubkeys[i][j] = (7*i - 7 + j) % 8
			decSubkeys[i][j] = (7*i - 1 - j + 8) % 8
		}
	}
}

func encryptWords(x *[4]uint32, k *[8]uint32) {
	a, b, c, d := x[0], x[1], x[2], x[3]
	for i := uint32(1); i <= 8; i += 4 {
		a, b, c, d = round(a, b, c, d, k, i, encSubkeys[i])
		b, d, a, c = round(b, d, a, c, k, i+1, encSubkeys[i+1])
		d, c, b, a = round(d, c, b, a, k, i+2, encSubkeys[i+2])
		c, a, d, b = round(c, a, d, b, k, i+3, encSubkeys[i+3])
	}
	x[0], x[1], x[2], x[3] = b, d, a, c
}

func decryptWords(x *[4]uint32, k *[8]uint32) {
	a, b, c, d := x[0], x[1], x[2], x[3]
	for i := uint32(8); i >= 1; i -= 4 {
		a, b, c, d = round(a, b, c, d, k, i, decSubkeys[i])
		c, a, d, b = round(c, a, d, b, k, i-1, decSubkeys[i-1])
		d, c, b, a = round(d, c, b, a, k, i-2, decSubkeys[i-2])
		b, d, a, c = round(b, d, a, c, k, i-3, decSubkeys[i-3])
	}
	x[0], x[1], x[2], x[3] = c, a, d, b
}

func loadWords(src []byte) [4]uint32 {
	return [4]uint32{
		binary.LittleEndian.Uint32(src[0:]), binary.LittleEndian.Uint32(src[4:]),
		binary.LittleEndian.Uint32(src[8:]), binary.LittleEndian.Uint32(src[12:]),
	}
}

func storeWords(dst []byte, x [4]uint32) {
	binary.LittleEndian.PutUint32(dst[0:], x[0])
	binary.LittleEndian.PutUint32(dst[4:], x[1])
	binary.LittleEndian.PutUint32(dst[8:], x[2])
	binary.LittleEndian.PutUint32(dst[12:], x[3])
}

type blockCipher struct {
	key [8]uint32
}

// NewCipher returns belt-block with a key of 16, 24 or 32 bytes.
func NewCipher(key []byte) (cipher.Block, error) {
	k, err := expandKey(key)
	if err != nil {
		return nil, err
	}
	return &blockCipher{key: k}, nil
}

func (c *blockCipher) BlockSize() int { return BlockSize }

func (c *blockCipher) Encrypt(dst, src []byte) {
	if len(src) < BlockSize || len(dst) < BlockSize {
		panic("belt: input not full block")
	}
	x := loadWords(src)
	encryptWords(&x, &c.key)
	storeWords(dst, x)
}

func (c *blockCipher) Decrypt(dst, src []byte) {
	if len(src) < BlockSize || len(dst) < BlockSize {
		panic("belt: input not full block")
	}
	x := loadWords(src)
	decryptWords(&x, &c.key)
	storeWords(dst, x)
}

var errShortInput = errors.New("belt: input too short")
