package bign

import (
	"encoding/binary"
	"math/big"
	"math/bits"
)

// modulus does arithmetic modulo an odd number of n 64-bit words in the Montgomery form. Every operation runs the
// same steps whatever the values, so private keys and nonces leave no trace in timing; only exp branches, and only
// on its public exponent.
type modulus struct {
	n     int
	m     []uint64
	minv  uint64 // -m^-1 mod 2^64
	rr    []uint64
	one   []uint64 // R mod m, the Montgomery form of 1
	bytes int
}

func newModulus(le []byte) *modulus {
	n := len(le) / 8
	md := &modulus{n: n, m: loadLE(le), bytes: len(le)}
	// Newton's iteration for the inverse of m[0] modulo 2^64.
	inv := uint64(1)
	for i := 0; i < 6; i++ {
		inv *= 2 - md.m[0]*inv
	}
	md.minv = -inv
	mb := new(big.Int).SetBytes(reverse(le))
	r := new(big.Int).Lsh(big.NewInt(1), uint(64*n))
	md.one = bigToWords(new(big.Int).Mod(r, mb), n)
	md.rr = bigToWords(new(big.Int).Mod(new(big.Int).Mul(r, r), mb), n)
	return md
}

func reverse(b []byte) []byte {
	out := make([]byte, len(b))
	for i, v := range b {
		out[len(b)-1-i] = v
	}
	return out
}

func bigToWords(x *big.Int, n int) []uint64 {
	le := reverse(x.FillBytes(make([]byte, 8*n)))
	return loadLE(le)
}

func loadLE(le []byte) []uint64 {
	w := make([]uint64, len(le)/8)
	for i := range w {
		w[i] = binary.LittleEndian.Uint64(le[8*i:])
	}
	return w
}

func (md *modulus) newElement() []uint64 { return make([]uint64, md.n) }

// lessMask is all ones when x < m, else zero.
func (md *modulus) lessMask(x []uint64) uint64 {
	var borrow uint64
	for i := 0; i < md.n; i++ {
		_, borrow = bits.Sub64(x[i], md.m[i], borrow)
	}
	return -borrow
}

// fromLE reads a little-endian number of md.bytes bytes; ok is false when it is not below m.
func (md *modulus) fromLE(le []byte) (x []uint64, ok bool) {
	x = loadLE(le)
	return x, md.lessMask(x) != 0
}

func (md *modulus) toLE(x []uint64) []byte {
	out := make([]byte, md.bytes)
	for i, w := range x {
		binary.LittleEndian.PutUint64(out[8*i:], w)
	}
	return out
}

// selectInto sets z to x where mask is all ones, to y where it is zero.
func selectInto(z, x, y []uint64, mask uint64) {
	for i := range z {
		z[i] = y[i] ^ (mask & (x[i] ^ y[i]))
	}
}

func (md *modulus) add(z, x, y []uint64) {
	sum := make([]uint64, md.n)
	var carry uint64
	for i := 0; i < md.n; i++ {
		sum[i], carry = bits.Add64(x[i], y[i], carry)
	}
	diff := make([]uint64, md.n)
	var borrow uint64
	for i := 0; i < md.n; i++ {
		diff[i], borrow = bits.Sub64(sum[i], md.m[i], borrow)
	}
	// The difference is the result when the sum overflowed or did not go below m.
	useDiff := -(carry | (borrow ^ 1))
	selectInto(z, diff, sum, useDiff)
}

func (md *modulus) sub(z, x, y []uint64) {
	diff := make([]uint64, md.n)
	var borrow uint64
	for i := 0; i < md.n; i++ {
		diff[i], borrow = bits.Sub64(x[i], y[i], borrow)
	}
	sum := make([]uint64, md.n)
	var carry uint64
	for i := 0; i < md.n; i++ {
		sum[i], carry = bits.Add64(diff[i], md.m[i], carry)
	}
	selectInto(z, sum, diff, -borrow)
}

// mul is the Montgomery product x·y·R^-1 mod m (CIOS).
func (md *modulus) mul(z, x, y []uint64) {
	n := md.n
	t := make([]uint64, n+2)
	for i := 0; i < n; i++ {
		var c uint64
		for j := 0; j < n; j++ {
			hi, lo := bits.Mul64(x[j], y[i])
			var cc uint64
			lo, cc = bits.Add64(lo, t[j], 0)
			hi += cc
			lo, cc = bits.Add64(lo, c, 0)
			hi += cc
			t[j], c = lo, hi
		}
		var cc uint64
		t[n], cc = bits.Add64(t[n], c, 0)
		t[n+1] = cc
		q := t[0] * md.minv
		hi, lo := bits.Mul64(q, md.m[0])
		_, cc = bits.Add64(lo, t[0], 0)
		c = hi + cc
		for j := 1; j < n; j++ {
			hi, lo = bits.Mul64(q, md.m[j])
			lo, cc = bits.Add64(lo, t[j], 0)
			hi += cc
			lo, cc = bits.Add64(lo, c, 0)
			hi += cc
			t[j-1], c = lo, hi
		}
		t[n-1], cc = bits.Add64(t[n], c, 0)
		t[n] = t[n+1] + cc
	}
	diff := make([]uint64, n)
	var borrow uint64
	for i := 0; i < n; i++ {
		diff[i], borrow = bits.Sub64(t[i], md.m[i], borrow)
	}
	_, borrow = bits.Sub64(t[n], 0, borrow)
	selectInto(z, t[:n], diff, -borrow)
}

func (md *modulus) toMont(z, x []uint64) { md.mul(z, x, md.rr) }

func (md *modulus) fromMont(z, x []uint64) {
	one := make([]uint64, md.n)
	one[0] = 1
	md.mul(z, x, one)
}

// exp sets z to x^e for an exponent that is public, as little-endian bytes; x and z are in the Montgomery form.
func (md *modulus) exp(z, x []uint64, e []byte) {
	acc := append([]uint64(nil), md.one...)
	for i := len(e) - 1; i >= 0; i-- {
		for b := 7; b >= 0; b-- {
			md.mul(acc, acc, acc)
			if e[i]>>b&1 == 1 {
				md.mul(acc, acc, x)
			}
		}
	}
	copy(z, acc)
}

// isZeroMask is all ones when x is zero.
func isZeroMask(x []uint64) uint64 {
	var acc uint64
	for _, w := range x {
		acc |= w
	}
	return ((acc | -acc) >> 63) - 1
}

func equalMask(x, y []uint64) uint64 {
	var acc uint64
	for i := range x {
		acc |= x[i] ^ y[i]
	}
	return ((acc | -acc) >> 63) - 1
}

// exponent returns f(m) as little-endian bytes of the length of m: the public exponents of inversion and roots.
func exponent(m []byte, f func(*big.Int) *big.Int) []byte {
	x := f(new(big.Int).SetBytes(reverse(m)))
	return reverse(x.FillBytes(make([]byte, len(m))))
}
