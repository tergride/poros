// Package stbtest issues bign certificates for tests.
package stbtest

import (
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"hash"
	"math/big"
	"time"

	"PorosCore/internal/stb/bash"
	"PorosCore/internal/stb/belt"
	"PorosCore/internal/stb/bign"
	"PorosCore/internal/stb/stbkey"
)

// Identity is a certificate with its private key.
type Identity struct {
	Cert *x509.Certificate
	Key  *stbkey.PrivateKey
}

// Options describe a certificate to issue.
type Options struct {
	Name string
	// Curve of the key; nil means bign-curve256v1.
	Curve       *bign.Curve
	IsCA        bool
	KeyUsage    x509.KeyUsage
	ExtKeyUsage []x509.ExtKeyUsage
	// MaxPathLen limits the chain below an authority when positive or when ZeroPathLen is set.
	MaxPathLen  int
	ZeroPathLen bool
	// NotBefore and NotAfter default to an hour ago and a day ahead.
	NotBefore, NotAfter time.Time
}

type tbsCertificate struct {
	Version      int `asn1:"explicit,tag:0"`
	SerialNumber *big.Int
	Signature    pkix.AlgorithmIdentifier
	Issuer       asn1.RawValue
	Validity     validity
	Subject      asn1.RawValue
	PublicKey    asn1.RawValue
	Extensions   []pkix.Extension `asn1:"optional,explicit,tag:3"`
}

type validity struct {
	NotBefore, NotAfter time.Time
}

type basicConstraints struct {
	IsCA       bool `asn1:"optional"`
	MaxPathLen int  `asn1:"optional,default:-1"`
}

type certificate struct {
	TBS       asn1.RawValue
	Algorithm pkix.AlgorithmIdentifier
	Signature asn1.BitString
}

var (
	oidBasicConstraints = asn1.ObjectIdentifier{2, 5, 29, 19}
	oidKeyUsage         = asn1.ObjectIdentifier{2, 5, 29, 15}
	oidExtKeyUsage      = asn1.ObjectIdentifier{2, 5, 29, 37}
	extKeyUsageOIDs     = map[x509.ExtKeyUsage]asn1.ObjectIdentifier{
		x509.ExtKeyUsageServerAuth: {1, 3, 6, 1, 5, 5, 7, 3, 1},
		x509.ExtKeyUsageClientAuth: {1, 3, 6, 1, 5, 5, 7, 3, 2},
	}
)

// signatureAlgorithm is the bign signature of a curve with the hash of its level.
func signatureAlgorithm(c *bign.Curve) (pkix.AlgorithmIdentifier, func() hash.Hash) {
	null := asn1.NullRawValue
	switch c.Level {
	case 192:
		return pkix.AlgorithmIdentifier{Algorithm: bign.OIDSignWithBash384, Parameters: null}, bash.New384
	case 256:
		return pkix.AlgorithmIdentifier{Algorithm: bign.OIDSignWithBash512, Parameters: null}, bash.New512
	}
	return pkix.AlgorithmIdentifier{Algorithm: bign.OIDSignWithBeltHash, Parameters: null}, belt.NewHash
}

func reverseBits(b byte) byte {
	var r byte
	for i := 0; i < 8; i++ {
		r = r<<1 | b&1
		b >>= 1
	}
	return r
}

func keyUsageExtension(ku x509.KeyUsage) (pkix.Extension, error) {
	bits := []byte{reverseBits(byte(ku)), reverseBits(byte(ku >> 8))}
	if bits[1] == 0 {
		bits = bits[:1]
	}
	length := 8 * len(bits)
	for last := bits[len(bits)-1]; length > 0 && last&1 == 0; last >>= 1 {
		length--
	}
	value, err := asn1.Marshal(asn1.BitString{Bytes: bits, BitLength: length})
	return pkix.Extension{Id: oidKeyUsage, Critical: true, Value: value}, err
}

// Issue makes a certificate signed by the issuer, or a self-signed one when the issuer is nil.
func Issue(opts Options, issuer *Identity) (*Identity, error) {
	curve := opts.Curve
	if curve == nil {
		curve = bign.Curve256
	}
	d, pub, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	key := &stbkey.PrivateKey{Curve: curve, D: d}
	spki, err := stbkey.MarshalPublicKeyInfo(&stbkey.PublicKey{Curve: curve, Point: pub})
	if err != nil {
		return nil, err
	}
	subject, err := asn1.Marshal(pkix.Name{CommonName: opts.Name}.ToRDNSequence())
	if err != nil {
		return nil, err
	}
	issuerName, signer := subject, key
	if issuer != nil {
		issuerName, signer = issuer.Cert.RawSubject, issuer.Key
	}
	now := time.Now()
	notBefore, notAfter := opts.NotBefore, opts.NotAfter
	if notBefore.IsZero() {
		notBefore = now.Add(-time.Hour)
	}
	if notAfter.IsZero() {
		notAfter = now.Add(24 * time.Hour)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 63))
	if err != nil {
		return nil, err
	}
	var extensions []pkix.Extension
	constraints := basicConstraints{IsCA: opts.IsCA, MaxPathLen: -1}
	if opts.MaxPathLen > 0 || opts.ZeroPathLen {
		constraints.MaxPathLen = opts.MaxPathLen
	}
	value, err := asn1.Marshal(constraints)
	if err != nil {
		return nil, err
	}
	extensions = append(extensions, pkix.Extension{Id: oidBasicConstraints, Critical: true, Value: value})
	if opts.KeyUsage != 0 {
		ext, err := keyUsageExtension(opts.KeyUsage)
		if err != nil {
			return nil, err
		}
		extensions = append(extensions, ext)
	}
	if len(opts.ExtKeyUsage) > 0 {
		var oids []asn1.ObjectIdentifier
		for _, u := range opts.ExtKeyUsage {
			oids = append(oids, extKeyUsageOIDs[u])
		}
		value, err := asn1.Marshal(oids)
		if err != nil {
			return nil, err
		}
		extensions = append(extensions, pkix.Extension{Id: oidExtKeyUsage, Value: value})
	}
	algorithm, newHash := signatureAlgorithm(signer.Curve)
	tbs, err := asn1.Marshal(tbsCertificate{
		Version:      2,
		SerialNumber: serial,
		Signature:    algorithm,
		Issuer:       asn1.RawValue{FullBytes: issuerName},
		Validity:     validity{notBefore.UTC().Truncate(time.Second), notAfter.UTC().Truncate(time.Second)},
		Subject:      asn1.RawValue{FullBytes: subject},
		PublicKey:    asn1.RawValue{FullBytes: spki},
		Extensions:   extensions,
	})
	if err != nil {
		return nil, err
	}
	h := newHash()
	h.Write(tbs)
	sig, err := signer.Curve.Sign(signer.D, bign.OIDDER(signer.Curve.HashOID()), h.Sum(nil), rand.Reader)
	if err != nil {
		return nil, err
	}
	der, err := asn1.Marshal(certificate{
		TBS:       asn1.RawValue{FullBytes: tbs},
		Algorithm: algorithm,
		Signature: asn1.BitString{Bytes: sig, BitLength: 8 * len(sig)},
	})
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &Identity{Cert: cert, Key: key}, nil
}

// PEM encodes certificates as PEM.
func PEM(certs ...*x509.Certificate) []byte {
	var out []byte
	for _, c := range certs {
		out = append(out, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.Raw})...)
	}
	return out
}
