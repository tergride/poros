package belt

import (
	"crypto/cipher"
	"crypto/subtle"
	"encoding/binary"
	"errors"
)

// DWPTagSize is the size of a belt-dwp tag in bytes.
const DWPTagSize = 8

// poly is an element of GF(2^128) modulo x^128 + x^7 + x^2 + x + 1; bit i of the little-endian number is the
// coefficient of x^i.
type poly [2]uint64

func loadPoly(b []byte) poly {
	return poly{binary.LittleEndian.Uint64(b), binary.LittleEndian.Uint64(b[8:])}
}

// mul returns a·b without branches on the data.
func (a poly) mul(b poly) poly {
	var r poly
	for i := 127; i >= 0; i-- {
		// r = r·x mod f
		carry := r[1] >> 63
		r[1] = r[1]<<1 | r[0]>>63
		r[0] = r[0]<<1 ^ (0x87 & -carry)
		bit := (b[i/64] >> (i % 64)) & 1
		mask := -bit
		r[0] ^= a[0] & mask
		r[1] ^= a[1] & mask
	}
	return r
}

type dwp struct {
	key [8]uint32
}

// NewDWP returns belt-dwp, authenticated encryption with a nonce of 16 bytes and a tag of 8 bytes.
func NewDWP(key []byte) (cipher.AEAD, error) {
	k, err := expandKey(key)
	if err != nil {
		return nil, err
	}
	return &dwp{key: k}, nil
}

func (d *dwp) NonceSize() int { return BlockSize }
func (d *dwp) Overhead() int  { return DWPTagSize }

// start returns the stream for the data and the multiplier r = belt-block(belt-block(IV)).
func (d *dwp) start(nonce []byte) (*ctr, poly) {
	c := &ctr{key: d.key, counter: loadWords(nonce)}
	encryptWords(&c.counter, &c.key)
	r := c.counter
	encryptWords(&r, &d.key)
	var rb [BlockSize]byte
	storeWords(rb[:], r)
	return c, loadPoly(rb[:])
}

func absorb(t poly, r poly, data []byte) poly {
	for len(data) > 0 {
		var block [BlockSize]byte
		n := copy(block[:], data)
		data = data[n:]
		x := loadPoly(block[:])
		t = poly{t[0] ^ x[0], t[1] ^ x[1]}.mul(r)
	}
	return t
}

func (d *dwp) tag(r poly, additionalData, ciphertext []byte) [DWPTagSize]byte {
	t := loadPoly(H[:BlockSize])
	t = absorb(t, r, additionalData)
	t = absorb(t, r, ciphertext)
	t = poly{t[0] ^ uint64(len(additionalData))*8, t[1] ^ uint64(len(ciphertext))*8}.mul(r)
	var block [BlockSize]byte
	binary.LittleEndian.PutUint64(block[:], t[0])
	binary.LittleEndian.PutUint64(block[8:], t[1])
	x := loadWords(block[:])
	encryptWords(&x, &d.key)
	storeWords(block[:], x)
	var out [DWPTagSize]byte
	copy(out[:], block[:])
	return out
}

func (d *dwp) Seal(dst, nonce, plaintext, additionalData []byte) []byte {
	if len(nonce) != BlockSize {
		panic("belt: invalid nonce size")
	}
	stream, r := d.start(nonce)
	ret, out := sliceForAppend(dst, len(plaintext)+DWPTagSize)
	stream.XORKeyStream(out[:len(plaintext)], plaintext)
	tag := d.tag(r, additionalData, out[:len(plaintext)])
	copy(out[len(plaintext):], tag[:])
	return ret
}

var errOpen = errors.New("belt: message authentication failed")

func (d *dwp) Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error) {
	if len(nonce) != BlockSize {
		panic("belt: invalid nonce size")
	}
	if len(ciphertext) < DWPTagSize {
		return nil, errOpen
	}
	body := ciphertext[:len(ciphertext)-DWPTagSize]
	stream, r := d.start(nonce)
	tag := d.tag(r, additionalData, body)
	if subtle.ConstantTimeCompare(tag[:], ciphertext[len(body):]) != 1 {
		return nil, errOpen
	}
	ret, out := sliceForAppend(dst, len(body))
	stream.XORKeyStream(out, body)
	return ret, nil
}

func sliceForAppend(in []byte, n int) (head, tail []byte) {
	if total := len(in) + n; cap(in) >= total {
		head = in[:total]
	} else {
		head = make([]byte, total)
		copy(head, in)
	}
	tail = head[len(in):]
	return head, tail
}
