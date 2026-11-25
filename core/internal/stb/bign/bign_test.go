package bign

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"math/big"
	"testing"

	"PorosCore/internal/stb/bash"
	"PorosCore/internal/stb/belt"
)

// The vectors come from annex Г of STB 34.101.45, as bee2 checks them on bign-curve256v1.

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

var (
	table   = belt.H
	privG1  = "1F66B5B84B7339674533F0329C74F21834281FED0732429E0C79235FC273E269"
	pubG1   = "BD1A5650179D79E03FCEE49D4C2BD5DDF54CE46D0CF11E4FF87BF7A890857FD07AC6A60361E8C8173491686D461B2826190C2EDA5909054A9AB84D2AB9D99A90"
	hashOID = OIDDER(OIDBeltHash)
)

func beltHash(data []byte) []byte {
	h := belt.Hash(data)
	return h[:]
}

func TestCurves(t *testing.T) {
	for _, c := range Curves {
		x, y, ok := c.affine(c.g)
		if !ok || !c.ValidPublicKey(append(x, y...)) {
			t.Fatalf("%s: G is not on the curve", c.Name)
		}
		if _, _, ok := c.affine(c.scalarMult(c.qBytes, c.g)); ok {
			t.Fatalf("%s: q·G is not the identity", c.Name)
		}
		if CurveByOID(c.OID) != c {
			t.Fatalf("%s: not found by its OID", c.Name)
		}
	}
	if len(OIDDER(OIDBeltHash)) != 11 {
		t.Fatal("the DER of belt-hash is not 11 bytes")
	}
}

func TestKeyPairAndDH(t *testing.T) {
	c := Curve256
	priv := unhex(t, privG1)
	pub, err := c.PublicKey(priv)
	if err != nil || !bytes.Equal(pub, unhex(t, pubG1)) {
		t.Fatalf("Г.1: %X, %v", pub, err)
	}
	g := append(make([]byte, 32), unhex(t, "936a510418cf291e52f608c4663991785d83d651a3c9e45c9fd616fb3cfcf76b")...)
	shared, err := c.DH(priv, g, 64)
	if err != nil || !bytes.Equal(shared, pub) {
		t.Fatalf("d·G by DH: %X, %v", shared, err)
	}
	for _, c := range Curves {
		a, pa, _ := c.GenerateKey(rand.Reader)
		b, pb, _ := c.GenerateKey(rand.Reader)
		ab, _ := c.DH(a, pb, 2*c.Size())
		ba, _ := c.DH(b, pa, 2*c.Size())
		if !bytes.Equal(ab, ba) || len(ab) != 2*c.Size() {
			t.Fatalf("%s: the shared keys differ", c.Name)
		}
		bad := append([]byte(nil), pb...)
		bad[0] ^= 1
		if _, err := c.DH(a, bad, 32); err != ErrInvalidPublicKey {
			t.Fatalf("%s: a point off the curve gave %v", c.Name, err)
		}
	}
	if _, err := c.PublicKey(make([]byte, 32)); err != ErrInvalidPrivateKey {
		t.Fatal("a zero private key was taken")
	}
	if _, err := c.PublicKey(c.qBytes); err != ErrInvalidPrivateKey {
		t.Fatal("q was taken as a private key")
	}
}

func TestVerifyVectors(t *testing.T) {
	c := Curve256
	pub := unhex(t, pubG1)
	for _, v := range []struct {
		data []byte
		sig  string
	}{
		{table[:13], "E36B7F0377AE4C524027C387FADF1B20CE72F1530B71F2B5FD3A8C584FE2E1AED20082E30C8AF65011F4FB54649DFD3D"},
		{table[:48], "47A63C8B9C936E94B5FAB3D9CBD78366290F3210E163EEC8DB4E921E8479D4138F112CC23E6DCE65EC5FF21DF4231C28"},
	} {
		sig := unhex(t, v.sig)
		hash := beltHash(v.data)
		if !c.Verify(pub, hashOID, hash, sig) {
			t.Fatal("Г.2 or Г.3 does not verify")
		}
		sig[0] ^= 1
		if c.Verify(pub, hashOID, hash, sig) {
			t.Fatal("a changed signature verified")
		}
		sig[0] ^= 1
		bad := append([]byte(nil), pub...)
		bad[0] ^= 1
		if c.Verify(bad, hashOID, hash, sig) {
			t.Fatal("a changed public key verified")
		}
	}
}

func TestDeterministicSign(t *testing.T) {
	c := Curve256
	priv := unhex(t, privG1)
	pub := unhex(t, pubG1)
	q := new(big.Int).SetBytes(reverse(c.qBytes))
	le := func(b []byte) *big.Int { return new(big.Int).SetBytes(reverse(b)) }
	for _, v := range []struct {
		data, extra []byte
		k           string
	}{
		{table[:13], nil, "829614D8411DBBC4E1F2471A4004586440FD8C9553FAB6A1A45CE417AE97111E"},
		{table[:48], table[192:215], "7ADC8713283EBFA547A2AD9CDFB245AE0F7B968DF0F91CB785D1F932A3583107"},
	} {
		hash := beltHash(v.data)
		sig, err := c.SignDeterministic(priv, hashOID, hash, v.extra)
		if err != nil {
			t.Fatal(err)
		}
		// k = (S0 + 2^128)·d + S1 + H mod q gives back the one-time key of Г.6 and Г.7.
		s0 := le(sig[:16])
		s0.Add(s0, new(big.Int).Lsh(big.NewInt(1), 128))
		k := new(big.Int).Mul(s0, le(priv))
		k.Add(k, le(sig[16:]))
		k.Add(k, le(hash))
		k.Mod(k, q)
		got := reverse(k.FillBytes(make([]byte, 32)))
		if !bytes.Equal(got, unhex(t, v.k)) {
			t.Fatalf("one-time key %X, want %s", got, v.k)
		}
		if !c.Verify(pub, hashOID, hash, sig) {
			t.Fatal("the deterministic signature does not verify")
		}
	}
}

func TestSignAllCurves(t *testing.T) {
	hashes := map[int]func([]byte) []byte{
		128: beltHash,
		192: func(b []byte) []byte { h := bash.New384(); h.Write(b); return h.Sum(nil) },
		256: func(b []byte) []byte { h := bash.New512(); h.Write(b); return h.Sum(nil) },
	}
	for _, c := range Curves {
		priv, pub, err := c.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		oid := OIDDER(c.HashOID())
		for i := 0; i < 4; i++ {
			hash := hashes[c.Level](table[:10+i])
			sig, err := c.Sign(priv, oid, hash, rand.Reader)
			if err != nil || len(sig) != c.SignatureSize() {
				t.Fatalf("%s: %v", c.Name, err)
			}
			if !c.Verify(pub, oid, hash, sig) {
				t.Fatalf("%s: the signature does not verify", c.Name)
			}
			hash[0] ^= 1
			if c.Verify(pub, oid, hash, sig) {
				t.Fatalf("%s: a signature of another hash verified", c.Name)
			}
		}
		if _, err := c.Sign(priv, oid, make([]byte, c.Size()+1), rand.Reader); err != ErrInvalidHash {
			t.Fatalf("%s: a hash of another size gave %v", c.Name, err)
		}
	}
}

func TestKeyTransport(t *testing.T) {
	c := Curve256
	priv := unhex(t, privG1)
	for _, v := range []struct {
		token  string
		header []byte
		key    []byte
	}{
		{"9B4EA669DABDF100A7D4B6E6EB76EE5251912531F426750AAC8A9DBB51C54D8DEB9289B50A46952D0531861E45A8814B008FDC65DE9FF1FA2A1F16B6A280E957A814", table[32:48], table[:18]},
		{"4856093A0F6C13015FC8E15F1B23A76202D2F4BA6E5EC52B78658477F6486DE687AFAEEA0EF7BC1326A7DCE7A10BA10E3F91C0126044B22267BF30BD6F1DA29E0647CF39C1D59A56BB0194E0F4F8A2BB", table[64:80], table[:32]},
	} {
		key, err := c.KeyUnwrap(priv, unhex(t, v.token), v.header)
		if err != nil || !bytes.Equal(key, v.key) {
			t.Fatalf("Г.4 or Г.5: %X, %v", key, err)
		}
		if _, err := c.KeyUnwrap(priv, unhex(t, v.token), nil); err != ErrInvalidToken {
			t.Fatalf("a wrong header gave %v", err)
		}
	}
	for _, c := range Curves {
		priv, pub, _ := c.GenerateKey(rand.Reader)
		for _, n := range []int{16, 32, 48} {
			token, err := c.KeyWrap(pub, table[:n], nil, rand.Reader)
			if err != nil || len(token) != c.Size()+n+16 {
				t.Fatalf("%s: %d bytes, %v", c.Name, len(token), err)
			}
			key, err := c.KeyUnwrap(priv, token, nil)
			if err != nil || !bytes.Equal(key, table[:n]) {
				t.Fatalf("%s: round trip of %d bytes: %v", c.Name, n, err)
			}
		}
	}
}

func TestModulus(t *testing.T) {
	// The Montgomery arithmetic against math/big on random numbers below p.
	for _, c := range Curves {
		f := c.p
		p := new(big.Int).SetBytes(reverse(c.pBytes))
		r := new(big.Int).Lsh(big.NewInt(1), uint(64*f.n))
		rInv := new(big.Int).ModInverse(r, p)
		for i := 0; i < 200; i++ {
			xb, _ := rand.Int(rand.Reader, p)
			yb, _ := rand.Int(rand.Reader, p)
			if i == 0 {
				xb.Sub(p, big.NewInt(1))
				yb.Sub(p, big.NewInt(1))
			}
			x, y := bigToWords(xb, f.n), bigToWords(yb, f.n)
			z := f.newElement()
			check := func(name string, want *big.Int) {
				if got := new(big.Int).SetBytes(reverse(f.toLE(z))); got.Cmp(want) != 0 {
					t.Fatalf("%s %s: got %v want %v", c.Name, name, got, want)
				}
			}
			f.add(z, x, y)
			check("add", new(big.Int).Mod(new(big.Int).Add(xb, yb), p))
			f.sub(z, x, y)
			check("sub", new(big.Int).Mod(new(big.Int).Sub(xb, yb), p))
			f.mul(z, x, y)
			want := new(big.Int).Mul(xb, yb)
			want.Mul(want, rInv)
			check("mul", want.Mod(want, p))
		}
	}
}
