// Package bash implements the hash functions of STB 34.101.77 (bash) on the sponge function bash-f. The code
// follows the bee2 library (Apache-2.0).
package bash

import (
	"encoding/binary"
	"hash"
	"math/bits"
)

const stateSize = 192

var roundConstants = [24]uint64{
	0x3BF5080AC8BA94B1, 0xC1D1659C1BBD92F6, 0x60E8B2CE0DDEC97B, 0xEC5FB8FE790FBC13,
	0xAA043DE6436706A7, 0x8929FF6A5E535BFD, 0x98BF1E2C50C97550, 0x4C5F8F162864BAA8,
	0x262FC78B14325D54, 0x1317E3C58A192EAA, 0x098BF1E2C50C9755, 0xD8EE19681D669304,
	0x6C770CB40EB34982, 0x363B865A0759A4C1, 0xC73622B47C4C0ACE, 0x639B115A3E260567,
	0xEDE6693460F3DA1D, 0xAAD8D5034F9935A0, 0x556C6A81A7CC9AD0, 0x2AB63540D3E64D68,
	0x155B1AA069F326B4, 0x0AAD8D5034F9935A, 0x0556C6A81A7CC9AD, 0xDE8082CD72DEBC78,
}

// sboxRotations are m1, n1, m2, n2 of bash-s for each column of the state.
var sboxRotations = [8][4]int{
	{8, 53, 14, 1}, {56, 51, 34, 7}, {8, 37, 46, 49}, {56, 3, 2, 23},
	{8, 21, 14, 33}, {56, 19, 34, 39}, {8, 5, 46, 17}, {56, 35, 2, 55},
}

// permutation[x] is the word that moves to position x after the S-boxes of a round.
var permutation = [24]int{15, 10, 9, 12, 11, 14, 13, 8, 17, 16, 19, 18, 21, 20, 23, 22, 6, 3, 0, 5, 2, 7, 4, 1}

func bashS(w0, w1, w2 uint64, r [4]int) (uint64, uint64, uint64) {
	t2 := bits.RotateLeft64(w0, r[0])
	w0 ^= w1 ^ w2
	t1 := w1 ^ bits.RotateLeft64(w0, r[1])
	w1 = t1 ^ t2
	w2 ^= bits.RotateLeft64(w2, r[2]) ^ bits.RotateLeft64(t1, r[3])
	t1 = w0 | w2
	t2 = w0 & w1
	t0 := ^w2 | w1
	return w0 ^ t0, w1 ^ t1, w2 ^ t2
}

// permute is bash-f on the state as 24 little-endian words.
func permute(s *[24]uint64) {
	for i := range roundConstants {
		for j := 0; j < 8; j++ {
			s[j], s[j+8], s[j+16] = bashS(s[j], s[j+8], s[j+16], sboxRotations[j])
		}
		var next [24]uint64
		for x, from := range permutation {
			next[x] = s[from]
		}
		*s = next
		s[23] ^= roundConstants[i]
	}
}

// F applies bash-f to a state of 192 bytes in place.
func F(state *[stateSize]byte) {
	var s [24]uint64
	for i := range s {
		s[i] = binary.LittleEndian.Uint64(state[8*i:])
	}
	permute(&s)
	for i, w := range s {
		binary.LittleEndian.PutUint64(state[8*i:], w)
	}
}

type digest struct {
	level int // l: 128, 192 or 256
	rate  int
	state [stateSize]byte
	pos   int
}

// New returns bash-hash of security level l (128, 192 or 256), which gives l/4 bytes. Its block is the rate of the
// sponge, so HMAC over bash512 works with blocks of 64 bytes, as bee2evp does.
func New(level int) hash.Hash {
	if level != 128 && level != 192 && level != 256 {
		panic("bash: invalid level")
	}
	d := &digest{level: level, rate: stateSize - level/2}
	d.Reset()
	return d
}

// New256, New384 and New512 are bash256, bash384 and bash512.
func New256() hash.Hash { return New(128) }
func New384() hash.Hash { return New(192) }
func New512() hash.Hash { return New(256) }

func (d *digest) Reset() {
	clear(d.state[:])
	d.state[stateSize-8] = byte(d.level / 4)
	d.pos = 0
}

func (d *digest) Size() int      { return d.level / 4 }
func (d *digest) BlockSize() int { return d.rate }

// Write absorbs data by overwriting the rate part of the state, as the standard does, not by adding to it.
func (d *digest) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		k := copy(d.state[d.pos:d.rate], p)
		d.pos += k
		p = p[k:]
		if d.pos == d.rate {
			F(&d.state)
			d.pos = 0
		}
	}
	return n, nil
}

func (d *digest) Sum(in []byte) []byte {
	s := d.state
	clear(s[d.pos:d.rate])
	s[d.pos] = 0x40
	F(&s)
	return append(in, s[:d.level/4]...)
}
