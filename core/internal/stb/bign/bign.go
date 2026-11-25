// Package bign implements the signatures, Diffie-Hellman and key transport of STB 34.101.45 (bign) on its standard
// curves. The code follows the bee2 library (Apache-2.0); the arithmetic runs in constant time.
package bign

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/asn1"
	"errors"
	"io"

	"PorosCore/internal/stb/belt"
)

var (
	// OIDPublicKey is bign-pubkey, the algorithm of bign keys in certificates and PKCS#8.
	OIDPublicKey = asn1.ObjectIdentifier{1, 2, 112, 0, 2, 0, 34, 101, 45, 2, 1}
	// OIDKeyTransport is bign-keytransport.
	OIDKeyTransport = asn1.ObjectIdentifier{1, 2, 112, 0, 2, 0, 34, 101, 45, 41}

	OIDBeltHash = asn1.ObjectIdentifier{1, 2, 112, 0, 2, 0, 34, 101, 31, 81}
	OIDBash256  = asn1.ObjectIdentifier{1, 2, 112, 0, 2, 0, 34, 101, 77, 11}
	OIDBash384  = asn1.ObjectIdentifier{1, 2, 112, 0, 2, 0, 34, 101, 77, 12}
	OIDBash512  = asn1.ObjectIdentifier{1, 2, 112, 0, 2, 0, 34, 101, 77, 13}

	// The signature algorithms: bign with the hash named, or with the hash in the parameters (hspec).
	OIDSignWithBeltHash = asn1.ObjectIdentifier{1, 2, 112, 0, 2, 0, 34, 101, 45, 12}
	OIDSignWithBash256  = asn1.ObjectIdentifier{1, 2, 112, 0, 2, 0, 34, 101, 45, 13}
	OIDSignWithBash384  = asn1.ObjectIdentifier{1, 2, 112, 0, 2, 0, 34, 101, 45, 14}
	OIDSignWithBash512  = asn1.ObjectIdentifier{1, 2, 112, 0, 2, 0, 34, 101, 45, 15}
	OIDSignWithHashSpec = asn1.ObjectIdentifier{1, 2, 112, 0, 2, 0, 34, 101, 45, 11}
)

var (
	ErrInvalidPrivateKey = errors.New("bign: invalid private key")
	ErrInvalidPublicKey  = errors.New("bign: invalid public key")
	ErrInvalidHash       = errors.New("bign: the hash does not fit the curve")
	ErrInvalidToken      = errors.New("bign: invalid key token")
)

// OIDDER returns the DER of an object identifier, which bign mixes into its signatures.
func OIDDER(oid asn1.ObjectIdentifier) []byte {
	der, err := asn1.Marshal(oid)
	if err != nil {
		panic(err)
	}
	return der
}

// HashOID is the hash a curve signs with: belt-hash at level 128, bash384 at 192, bash512 at 256.
func (c *Curve) HashOID() asn1.ObjectIdentifier {
	switch c.Level {
	case 192:
		return OIDBash384
	case 256:
		return OIDBash512
	}
	return OIDBeltHash
}

// privateScalar reads a private key: l/4 little-endian bytes, 0 < d < q.
func (c *Curve) privateScalar(d []byte) ([]uint64, error) {
	if len(d) != c.size {
		return nil, ErrInvalidPrivateKey
	}
	x, ok := c.q.fromLE(d)
	if !ok || isZeroMask(x) != 0 {
		return nil, ErrInvalidPrivateKey
	}
	return x, nil
}

// randomScalar draws a number in [1, q-1].
func (c *Curve) randomScalar(random io.Reader) ([]byte, error) {
	if random == nil {
		random = rand.Reader
	}
	k := make([]byte, c.size)
	for {
		if _, err := io.ReadFull(random, k); err != nil {
			return nil, err
		}
		if x, ok := c.q.fromLE(k); ok && isZeroMask(x) == 0 {
			return k, nil
		}
	}
}

// GenerateKey returns a new private key and its public key x‖y.
func (c *Curve) GenerateKey(random io.Reader) (priv, pub []byte, err error) {
	priv, err = c.randomScalar(random)
	if err != nil {
		return nil, nil, err
	}
	pub, err = c.PublicKey(priv)
	return priv, pub, err
}

// PublicKey returns d·G as x‖y.
func (c *Curve) PublicKey(priv []byte) ([]byte, error) {
	if _, err := c.privateScalar(priv); err != nil {
		return nil, err
	}
	x, y, ok := c.affine(c.scalarMult(priv, c.g))
	if !ok {
		return nil, ErrInvalidPrivateKey
	}
	return append(x, y...), nil
}

// ValidPublicKey tells a public key x‖y of a point of the curve.
func (c *Curve) ValidPublicKey(pub []byte) bool {
	_, ok := c.decodePoint(pub)
	return ok
}

// reduce returns the l/4-byte number h modulo q; h < 2q because q > 2^(2l-1).
func (c *Curve) reduce(h []byte) []uint64 {
	x := loadLE(h)
	diff := c.q.newElement()
	c.q.sub(diff, x, c.q.m)
	// sub adds q back on a borrow, so diff is x - q when x >= q and x otherwise.
	return diff
}

// SignatureSize is the length of a signature: 3l/8 bytes.
func (c *Curve) SignatureSize() int { return c.size + c.size/2 }

// Sign signs a hash of l/4 bytes made by the algorithm with the DER identifier oidDER. The one-time key comes from
// the private key, the hash and fresh randomness, so a weak source of randomness does not reveal the key.
func (c *Curve) Sign(priv, oidDER, hash []byte, random io.Reader) ([]byte, error) {
	if random == nil {
		random = rand.Reader
	}
	t := make([]byte, 32)
	if _, err := io.ReadFull(random, t); err != nil {
		return nil, err
	}
	return c.SignDeterministic(priv, oidDER, hash, t)
}

// SignDeterministic is the signature of STB 34.101.45, 6.3, whose one-time key comes from the private key, the hash
// and the optional extra data t.
func (c *Curve) SignDeterministic(priv, oidDER, hash, t []byte) ([]byte, error) {
	d, err := c.privateScalar(priv)
	if err != nil {
		return nil, err
	}
	if len(hash) != c.size {
		return nil, ErrInvalidHash
	}
	seed := belt.NewHash()
	seed.Write(oidDER)
	seed.Write(priv)
	seed.Write(t)
	theta := seed.Sum(nil)
	k := append([]byte(nil), hash...)
	for {
		if k, err = belt.WBLEncrypt(theta, k); err != nil {
			return nil, err
		}
		if x, ok := c.q.fromLE(k); ok && isZeroMask(x) == 0 {
			break
		}
	}
	defer clear(k)
	xr, _, ok := c.affine(c.scalarMult(k, c.g))
	if !ok {
		return nil, ErrInvalidPrivateKey
	}
	h := belt.NewHash()
	h.Write(oidDER)
	h.Write(xr)
	h.Write(hash)
	s0 := h.Sum(nil)[:c.size/2]
	// S1 = (k - H - (S0 + 2^l)·d) mod q
	q := c.q
	e := q.newElement
	s0e := e()
	copy(s0e, loadLE(append(append([]byte(nil), s0...), make([]byte, c.size-len(s0))...)))
	q.add(s0e, s0e, c.twoToLevel)
	dm, km, hm, prod := e(), e(), e(), e()
	q.toMont(dm, d)
	q.toMont(s0e, s0e)
	q.mul(prod, s0e, dm)
	kw, _ := q.fromLE(k)
	q.toMont(km, kw)
	q.toMont(hm, c.reduce(hash))
	s1 := e()
	q.sub(s1, km, prod)
	q.sub(s1, s1, hm)
	q.fromMont(s1, s1)
	return append(s0, q.toLE(s1)...), nil
}

// Verify checks a signature of a hash under the public key x‖y.
func (c *Curve) Verify(pub, oidDER, hash, sig []byte) bool {
	qp, ok := c.decodePoint(pub)
	if !ok || len(hash) != c.size || len(sig) != c.SignatureSize() {
		return false
	}
	s0, s1b := sig[:c.size/2], sig[c.size/2:]
	s1, ok := c.q.fromLE(s1b)
	if !ok {
		return false
	}
	q := c.q
	u := q.newElement()
	q.add(u, s1, c.reduce(hash))
	v := loadLE(append(append([]byte(nil), s0...), make([]byte, c.size-len(s0))...))
	q.add(v, v, c.twoToLevel)
	r := c.add(c.scalarMult(q.toLE(u), c.g), c.scalarMult(q.toLE(v), qp))
	xr, _, ok := c.affine(r)
	if !ok {
		return false
	}
	h := belt.NewHash()
	h.Write(oidDER)
	h.Write(xr)
	h.Write(hash)
	return subtle.ConstantTimeCompare(h.Sum(nil)[:c.size/2], s0) == 1
}

// DH returns the first length bytes of x‖y of d·Q, at most l/2 bytes.
func (c *Curve) DH(priv, pub []byte, length int) ([]byte, error) {
	if _, err := c.privateScalar(priv); err != nil {
		return nil, err
	}
	if length <= 0 || length > 2*c.size {
		return nil, errors.New("bign: invalid length of the shared key")
	}
	qp, ok := c.decodePoint(pub)
	if !ok {
		return nil, ErrInvalidPublicKey
	}
	x, y, ok := c.affine(c.scalarMult(priv, qp))
	if !ok {
		return nil, ErrInvalidPublicKey
	}
	return append(x, y...)[:length], nil
}

// KeyWrap protects a key of at least 16 bytes for the holder of the public key, with a header of 16 bytes or none
// for zeros: x(k·G) ‖ belt-kwp under the first 32 bytes of x(k·Q).
func (c *Curve) KeyWrap(pub, key, header []byte, random io.Reader) ([]byte, error) {
	qp, ok := c.decodePoint(pub)
	if !ok {
		return nil, ErrInvalidPublicKey
	}
	k, err := c.randomScalar(random)
	if err != nil {
		return nil, err
	}
	defer clear(k)
	shared, _, ok := c.affine(c.scalarMult(k, qp))
	if !ok {
		return nil, ErrInvalidPublicKey
	}
	xr, _, _ := c.affine(c.scalarMult(k, c.g))
	wrapped, err := belt.KeyWrap(shared[:32], key, header)
	if err != nil {
		return nil, err
	}
	return append(xr, wrapped...), nil
}

// KeyUnwrap reverses KeyWrap with the private key.
func (c *Curve) KeyUnwrap(priv, token, header []byte) ([]byte, error) {
	if _, err := c.privateScalar(priv); err != nil {
		return nil, err
	}
	if len(token) < c.size+32 {
		return nil, ErrInvalidToken
	}
	r, ok := c.liftX(token[:c.size])
	if !ok {
		return nil, ErrInvalidToken
	}
	shared, _, ok := c.affine(c.scalarMult(priv, r))
	if !ok {
		return nil, ErrInvalidToken
	}
	key, err := belt.KeyUnwrap(shared[:32], token[c.size:], header)
	if err != nil {
		return nil, ErrInvalidToken
	}
	return key, nil
}
