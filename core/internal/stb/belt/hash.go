package belt

import (
	"encoding/binary"
	"hash"
)

// HashSize is the size of a belt-hash value in bytes.
const HashSize = 32

// compress is belt-compress: it updates h with the 32-byte block x and, when s is not nil, adds σ1 to s.
func compress(s *[4]uint32, h *[8]uint32, x *[8]uint32) {
	var sigma [4]uint32
	for i := range sigma {
		sigma[i] = h[i] ^ h[4+i]
	}
	t := sigma
	encryptWords(&sigma, x)
	for i := range sigma {
		sigma[i] ^= t[i]
	}
	if s != nil {
		for i := range s {
			s[i] ^= sigma[i]
		}
	}
	k1 := [8]uint32{sigma[0], sigma[1], sigma[2], sigma[3], h[4], h[5], h[6], h[7]}
	k2 := [8]uint32{^sigma[0], ^sigma[1], ^sigma[2], ^sigma[3], h[0], h[1], h[2], h[3]}
	x0 := [4]uint32{x[0], x[1], x[2], x[3]}
	x1 := [4]uint32{x[4], x[5], x[6], x[7]}
	y0, y1 := x0, x1
	encryptWords(&y0, &k1)
	encryptWords(&y1, &k2)
	for i := 0; i < 4; i++ {
		h[i] = y0[i] ^ x0[i]
		h[4+i] = y1[i] ^ x1[i]
	}
}

func loadBlock32(src []byte) [8]uint32 {
	var x [8]uint32
	for i := range x {
		x[i] = binary.LittleEndian.Uint32(src[4*i:])
	}
	return x
}

type digest struct {
	length [4]uint32 // bits hashed, 128-bit little-endian
	s      [4]uint32
	h      [8]uint32
	block  [HashSize]byte
	filled int
}

// NewHash returns belt-hash. Its block is 32 bytes, so HMAC over it is hmac-hbelt of STB 34.101.47.
func NewHash() hash.Hash {
	d := new(digest)
	d.Reset()
	return d
}

// Hash returns belt-hash of data.
func Hash(data []byte) [HashSize]byte {
	d := new(digest)
	d.Reset()
	d.Write(data)
	var out [HashSize]byte
	d.Sum(out[:0])
	return out
}

func (d *digest) Reset() {
	d.length = [4]uint32{}
	d.s = [4]uint32{}
	d.h = loadBlock32(H[:32])
	d.filled = 0
}

func (d *digest) Size() int      { return HashSize }
func (d *digest) BlockSize() int { return HashSize }

func (d *digest) addLength(n int) {
	bitsLow := uint64(d.length[0]) | uint64(d.length[1])<<32
	bitsHigh := uint64(d.length[2]) | uint64(d.length[3])<<32
	added := uint64(n) << 3
	sum := bitsLow + added
	if sum < bitsLow {
		bitsHigh++
	}
	bitsHigh += uint64(n) >> 61
	d.length = [4]uint32{uint32(sum), uint32(sum >> 32), uint32(bitsHigh), uint32(bitsHigh >> 32)}
}

func (d *digest) Write(p []byte) (int, error) {
	n := len(p)
	d.addLength(n)
	if d.filled > 0 {
		k := copy(d.block[d.filled:], p)
		d.filled += k
		p = p[k:]
		if d.filled < HashSize {
			return n, nil
		}
		x := loadBlock32(d.block[:])
		compress(&d.s, &d.h, &x)
		d.filled = 0
	}
	for len(p) >= HashSize {
		x := loadBlock32(p)
		compress(&d.s, &d.h, &x)
		p = p[HashSize:]
	}
	d.filled = copy(d.block[:], p)
	return n, nil
}

func (d *digest) Sum(in []byte) []byte {
	s, h := d.s, d.h
	if d.filled > 0 {
		var last [HashSize]byte
		copy(last[:], d.block[:d.filled])
		x := loadBlock32(last[:])
		compress(&s, &h, &x)
	}
	ls := [8]uint32{d.length[0], d.length[1], d.length[2], d.length[3], s[0], s[1], s[2], s[3]}
	compress(nil, &h, &ls)
	var out [HashSize]byte
	for i, w := range h {
		binary.LittleEndian.PutUint32(out[4*i:], w)
	}
	return append(in, out[:]...)
}
