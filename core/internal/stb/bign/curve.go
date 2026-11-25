package bign

import (
	"encoding/asn1"
	"encoding/hex"
	"math/big"
)

// Curve is a standard curve of STB 34.101.45: y² = x³ + ax + b over p with a = p - 3, a group of prime order q and
// the base point G = (0, yG). Numbers are little-endian, as in the standard.
type Curve struct {
	Name string
	OID  asn1.ObjectIdentifier
	// Level is the security level l: 128, 192 or 256. Keys and coordinates take l/4 bytes, signatures 3l/8.
	Level int

	p, q       *modulus
	a, b       []uint64 // in the Montgomery form of p
	g          point
	pMinusTwo  []byte
	sqrtExp    []byte
	qMinusTwo  []byte
	size       int
	qBytes     []byte
	pBytes     []byte
	twoToLevel []uint64 // 2^l as a number below q
}

// point is a point in projective coordinates (X:Y:Z) in the Montgomery form; the identity is (0:1:0).
type point struct {
	x, y, z []uint64
}

func mustHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

func newCurve(name string, oid asn1.ObjectIdentifier, level int, p, a, b, q, yG string) *Curve {
	c := &Curve{Name: name, OID: oid, Level: level, size: level / 4}
	c.pBytes, c.qBytes = mustHex(p), mustHex(q)
	c.p, c.q = newModulus(c.pBytes), newModulus(c.qBytes)
	toMont := func(le []byte) []uint64 {
		x := loadLE(le)
		c.p.toMont(x, x)
		return x
	}
	c.a, c.b = toMont(mustHex(a)), toMont(mustHex(b))
	c.g = point{x: c.p.newElement(), y: toMont(mustHex(yG)), z: append([]uint64(nil), c.p.one...)}
	c.pMinusTwo = exponent(c.pBytes, func(x *big.Int) *big.Int { return x.Sub(x, big.NewInt(2)) })
	c.qMinusTwo = exponent(c.qBytes, func(x *big.Int) *big.Int { return x.Sub(x, big.NewInt(2)) })
	c.sqrtExp = exponent(c.pBytes, func(x *big.Int) *big.Int { return x.Rsh(x.Add(x, big.NewInt(1)), 2) })
	c.twoToLevel = c.q.newElement()
	c.twoToLevel[level/64] = 1
	return c
}

var oidBignCurve = asn1.ObjectIdentifier{1, 2, 112, 0, 2, 0, 34, 101, 45, 3}

// Curve256, Curve384 and Curve512 are bign-curve256v1, bign-curve384v1 and bign-curve512v1.
var (
	Curve256 = newCurve("bign-curve256v1", append(oidBignCurve[:len(oidBignCurve):len(oidBignCurve)], 1), 128,
		"43ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		"40ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		"f1039cd66b7d2eb253928b976950f54cbefbd8e4ab3ac1d2eda8f315156cce77",
		"07663d2699bf5a7efc4dfb0dd68e5cd9ffffffffffffffffffffffffffffffff",
		"936a510418cf291e52f608c4663991785d83d651a3c9e45c9fd616fb3cfcf76b")
	Curve384 = newCurve("bign-curve384v1", append(oidBignCurve[:len(oidBignCurve):len(oidBignCurve)], 2), 192,
		"c3feffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		"c0feffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		"64bf736823fca7bc7cbdcef3f0e2bd143a2e71e9f96a21a696b1fb0fbb482771d2345d65ab5a073320ef9c95e1df753c",
		"b7a70cf33fdcb73d0affa4a6e7da4680bb7baf7303c4cc6cfeffffffffffffffffffffffffffffffffffffffffffffff",
		"51c433f731cb5eeaf9422a6b273e408455d3b1669ee74905a0ff86dc119a723a89bf2d437e1130639e9e2ea82482435d")
	Curve512 = newCurve("bign-curve512v1", append(oidBignCurve[:len(oidBignCurve):len(oidBignCurve)], 3), 256,
		"c7fdffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		"c4fdffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		"909c13d6986934097aa2493a272286ea43a2ac878c003329955e24c4b5dc112788b0addae313ce1751255dddeea9c65b8958fd606a5d8cd8438c3b934459b46c",
		"f18e060d49adffdc32df5695e5ca1b36f413212eb0eb6bf24e0098012c09c0b2ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		"bdedefce6fae92b7040d4cc9b983aa676122e8ee957377ffd26ffa0ee2dd7369dacacc001bf8edd2e2bc61b3b341abb0ab8fd1a0f7e682b1817603e47aff26a8")
)

// Curves lists the standard curves.
var Curves = []*Curve{Curve256, Curve384, Curve512}

// CurveByOID returns the standard curve with the identifier, or nil.
func CurveByOID(oid asn1.ObjectIdentifier) *Curve {
	for _, c := range Curves {
		if c.OID.Equal(oid) {
			return c
		}
	}
	return nil
}

// Size is the length of a private key and of a coordinate in bytes: l/4.
func (c *Curve) Size() int { return c.size }

func (c *Curve) identity() point {
	return point{x: c.p.newElement(), y: append([]uint64(nil), c.p.one...), z: c.p.newElement()}
}

// add is the complete addition of Renes, Costello and Batina for a = -3 (https://eprint.iacr.org/2015/1060,
// algorithm 4): it has no exceptions, so it works the same for equal points and the identity.
func (c *Curve) add(p1, p2 point) point {
	f := c.p
	e := f.newElement
	t0, t1, t2, t3, t4 := e(), e(), e(), e(), e()
	x3, y3, z3 := e(), e(), e()
	f.mul(t0, p1.x, p2.x)
	f.mul(t1, p1.y, p2.y)
	f.mul(t2, p1.z, p2.z)
	f.add(t3, p1.x, p1.y)
	f.add(t4, p2.x, p2.y)
	f.mul(t3, t3, t4)
	f.add(t4, t0, t1)
	f.sub(t3, t3, t4)
	f.add(t4, p1.y, p1.z)
	f.add(x3, p2.y, p2.z)
	f.mul(t4, t4, x3)
	f.add(x3, t1, t2)
	f.sub(t4, t4, x3)
	f.add(x3, p1.x, p1.z)
	f.add(y3, p2.x, p2.z)
	f.mul(x3, x3, y3)
	f.add(y3, t0, t2)
	f.sub(y3, x3, y3)
	f.mul(z3, c.b, t2)
	f.sub(x3, y3, z3)
	f.add(z3, x3, x3)
	f.add(x3, x3, z3)
	f.sub(z3, t1, x3)
	f.add(x3, t1, x3)
	f.mul(y3, c.b, y3)
	f.add(t1, t2, t2)
	f.add(t2, t1, t2)
	f.sub(y3, y3, t2)
	f.sub(y3, y3, t0)
	f.add(t1, y3, y3)
	f.add(y3, t1, y3)
	f.add(t1, t0, t0)
	f.add(t0, t1, t0)
	f.sub(t0, t0, t2)
	f.mul(t1, t4, y3)
	f.mul(t2, t0, y3)
	f.mul(y3, x3, z3)
	f.add(y3, y3, t2)
	f.mul(x3, t3, x3)
	f.sub(x3, x3, t1)
	f.mul(z3, t4, z3)
	f.mul(t1, t3, t0)
	f.add(z3, z3, t1)
	return point{x: x3, y: y3, z: z3}
}

// double is the complete doubling of the same paper for a = -3 (algorithm 6).
func (c *Curve) double(p point) point {
	f := c.p
	e := f.newElement
	t0, t1, t2, t3 := e(), e(), e(), e()
	x3, y3, z3 := e(), e(), e()
	f.mul(t0, p.x, p.x)
	f.mul(t1, p.y, p.y)
	f.mul(t2, p.z, p.z)
	f.mul(t3, p.x, p.y)
	f.add(t3, t3, t3)
	f.mul(z3, p.x, p.z)
	f.add(z3, z3, z3)
	f.mul(y3, c.b, t2)
	f.sub(y3, y3, z3)
	f.add(x3, y3, y3)
	f.add(y3, x3, y3)
	f.sub(x3, t1, y3)
	f.add(y3, t1, y3)
	f.mul(y3, x3, y3)
	f.mul(x3, x3, t3)
	f.add(t3, t2, t2)
	f.add(t2, t2, t3)
	f.mul(z3, c.b, z3)
	f.sub(z3, z3, t2)
	f.sub(z3, z3, t0)
	f.add(t3, z3, z3)
	f.add(z3, z3, t3)
	f.add(t3, t0, t0)
	f.add(t0, t3, t0)
	f.sub(t0, t0, t2)
	f.mul(t0, t0, z3)
	f.add(y3, y3, t0)
	f.mul(t0, p.y, p.z)
	f.add(t0, t0, t0)
	f.mul(z3, t0, z3)
	f.sub(x3, x3, z3)
	f.mul(z3, t0, t1)
	f.add(z3, z3, z3)
	f.add(z3, z3, z3)
	return point{x: x3, y: y3, z: z3}
}

func (c *Curve) selectPoint(dst *point, src point, mask uint64) {
	selectInto(dst.x, src.x, dst.x, mask)
	selectInto(dst.y, src.y, dst.y, mask)
	selectInto(dst.z, src.z, dst.z, mask)
}

// scalarMult returns k·P for a little-endian scalar, four bits at a time, choosing the table entry without branches
// or memory access that depend on k.
func (c *Curve) scalarMult(k []byte, p point) point {
	var table [16]point
	table[0] = c.identity()
	table[1] = p
	for i := 2; i < 16; i++ {
		table[i] = c.add(table[i-1], p)
	}
	q := c.identity()
	for i := len(k) - 1; i >= 0; i-- {
		for _, nibble := range [2]byte{k[i] >> 4, k[i] & 15} {
			for j := 0; j < 4; j++ {
				q = c.double(q)
			}
			chosen := c.identity()
			for j := range table {
				mask := uint64(subtleEq(byte(j), nibble))
				c.selectPoint(&chosen, table[j], -mask)
			}
			q = c.add(q, chosen)
		}
	}
	return q
}

// subtleEq is 1 when a == b and 0 otherwise, without a branch.
func subtleEq(a, b byte) byte {
	x := a ^ b
	return byte((uint32(x) - 1) >> 31)
}

// affine returns the coordinates of p as little-endian bytes; ok is false for the identity.
func (c *Curve) affine(p point) (x, y []byte, ok bool) {
	f := c.p
	if isZeroMask(p.z) != 0 {
		return nil, nil, false
	}
	inv := f.newElement()
	f.exp(inv, p.z, c.pMinusTwo)
	xs, ys := f.newElement(), f.newElement()
	f.mul(xs, p.x, inv)
	f.mul(ys, p.y, inv)
	f.fromMont(xs, xs)
	f.fromMont(ys, ys)
	return f.toLE(xs), f.toLE(ys), true
}

// rightSide returns x³ + ax + b for x in the Montgomery form.
func (c *Curve) rightSide(x []uint64) []uint64 {
	f := c.p
	r := f.newElement()
	f.mul(r, x, x)
	f.add(r, r, c.a)
	f.mul(r, r, x)
	f.add(r, r, c.b)
	return r
}

// decodePoint reads x‖y and checks that the point is on the curve.
func (c *Curve) decodePoint(pub []byte) (point, bool) {
	if len(pub) != 2*c.size {
		return point{}, false
	}
	x, okX := c.p.fromLE(pub[:c.size])
	y, okY := c.p.fromLE(pub[c.size:])
	if !okX || !okY {
		return point{}, false
	}
	c.p.toMont(x, x)
	c.p.toMont(y, y)
	y2 := c.p.newElement()
	c.p.mul(y2, y, y)
	if equalMask(y2, c.rightSide(x)) == 0 {
		return point{}, false
	}
	return point{x: x, y: y, z: append([]uint64(nil), c.p.one...)}, true
}

// liftX returns a point with the coordinate x, if there is one.
func (c *Curve) liftX(xle []byte) (point, bool) {
	x, ok := c.p.fromLE(xle)
	if !ok {
		return point{}, false
	}
	c.p.toMont(x, x)
	rhs := c.rightSide(x)
	y := c.p.newElement()
	c.p.exp(y, rhs, c.sqrtExp)
	y2 := c.p.newElement()
	c.p.mul(y2, y, y)
	if equalMask(y2, rhs) == 0 {
		return point{}, false
	}
	return point{x: x, y: y, z: append([]uint64(nil), c.p.one...)}, true
}
