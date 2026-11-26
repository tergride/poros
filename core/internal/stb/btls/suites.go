// Package btls is TLS 1.2 with the cipher suites of STB 34.101.65 (BTLS): the key exchange by bign, ephemeral (DHE)
// or by the transport of the key under the certificate of the server (DHT), records under belt-dwp or under belt-ctr
// with belt-mac, and the PRF over hmac-hbelt. It follows the BTLS of OpenSSL with the bee2evp engine.
package btls

import (
	"hash"
	"strings"

	"PorosCore/internal/stb/bash"
	"PorosCore/internal/stb/belt"
	"PorosCore/internal/stb/bign"
)

// The cipher suites of STB 34.101.65 for TLS 1.2.
const (
	TLS_DHE_BIGN_WITH_BELT_CTR_MAC_HBELT uint16 = 0xff15
	TLS_DHE_BIGN_WITH_BELT_DWP_HBELT     uint16 = 0xff16
	TLS_DHT_BIGN_WITH_BELT_CTR_MAC_HBELT uint16 = 0xff17
	TLS_DHT_BIGN_WITH_BELT_DWP_HBELT     uint16 = 0xff18
)

type cipherSuite struct {
	id        uint16
	name      string
	ephemeral bool
	dwp       bool
}

// cipherSuites is in the default order: belt-dwp before belt-ctr with belt-mac, ephemeral keys first.
var cipherSuites = []*cipherSuite{
	{TLS_DHE_BIGN_WITH_BELT_DWP_HBELT, "DHE-BIGN-WITH-BELT-DWP-HBELT", true, true},
	{TLS_DHT_BIGN_WITH_BELT_DWP_HBELT, "DHT-BIGN-WITH-BELT-DWP-HBELT", false, true},
	{TLS_DHE_BIGN_WITH_BELT_CTR_MAC_HBELT, "DHE-BIGN-WITH-BELT-CTR-MAC-HBELT", true, false},
	{TLS_DHT_BIGN_WITH_BELT_CTR_MAC_HBELT, "DHT-BIGN-WITH-BELT-CTR-MAC-HBELT", false, false},
}

func cipherSuiteByID(id uint16) *cipherSuite {
	for _, s := range cipherSuites {
		if s.id == id {
			return s
		}
	}
	return nil
}

// DefaultCipherSuites returns all suites, belt-dwp first.
func DefaultCipherSuites() []uint16 {
	ids := make([]uint16, len(cipherSuites))
	for i, s := range cipherSuites {
		ids[i] = s.id
	}
	return ids
}

// CipherSuiteName returns the OpenSSL name of a suite, such as DHE-BIGN-WITH-BELT-DWP-HBELT, or "" for another one.
func CipherSuiteName(id uint16) string {
	if s := cipherSuiteByID(id); s != nil {
		return s.name
	}
	return ""
}

// CipherSuiteByName finds a suite by its OpenSSL name or by the name of the standard, such as
// TLS_DHE_BIGN_WITH_BELT_DWP_HBELT.
func CipherSuiteByName(name string) (uint16, bool) {
	for _, s := range cipherSuites {
		if name == s.name || name == "TLS_"+strings.ReplaceAll(s.name, "-", "_") {
			return s.id, true
		}
	}
	return 0, false
}

func (s *cipherSuite) keyLengths() (macLen, keyLen, ivLen int) {
	if s.dwp {
		return 0, 32, 8
	}
	return 32, 32, 0
}

// The signature algorithms of bign. OpenSSL with bee2evp named them 0xE7E7-0xE9E9 first and added 0xFE01-0xFE03
// later; both mean the same.
const (
	signatureBign128         uint16 = 0xe7e7
	signatureBign192         uint16 = 0xe8e8
	signatureBign256         uint16 = 0xe9e9
	signatureBignWithHBelt   uint16 = 0xfe01
	signatureBignWithBash384 uint16 = 0xfe02
	signatureBignWithBash512 uint16 = 0xfe03
)

var supportedSignatureAlgorithms = []uint16{
	signatureBign128, signatureBign192, signatureBign256,
	signatureBignWithHBelt, signatureBignWithBash384, signatureBignWithBash512,
}

// signatureCurve is the curve a signature algorithm signs on: each one fixes the curve and its hash.
func signatureCurve(alg uint16) *bign.Curve {
	switch alg {
	case signatureBign128, signatureBignWithHBelt:
		return bign.Curve256
	case signatureBign192, signatureBignWithBash384:
		return bign.Curve384
	case signatureBign256, signatureBignWithBash512:
		return bign.Curve512
	}
	return nil
}

// signatureFor picks the first algorithm of the peer's list that signs on the curve.
func signatureFor(curve *bign.Curve, peer []uint16) (uint16, bool) {
	for _, alg := range peer {
		if signatureCurve(alg) == curve {
			return alg, true
		}
	}
	return 0, false
}

// signatureAlgorithmsFor is the list to offer with the algorithms of the own curve first, or in the usual order
// without a curve. OpenSSL with bee2evp signs ServerKeyExchange and CertificateVerify with the first algorithm of the
// peer's list for its key type whatever the curve, so a peer on the same curve then signs with one that fits its key.
func signatureAlgorithmsFor(curve *bign.Curve) []uint16 {
	algs := make([]uint16, 0, len(supportedSignatureAlgorithms))
	for _, alg := range supportedSignatureAlgorithms {
		if signatureCurve(alg) == curve {
			algs = append(algs, alg)
		}
	}
	for _, alg := range supportedSignatureAlgorithms {
		if signatureCurve(alg) != curve {
			algs = append(algs, alg)
		}
	}
	return algs
}

func isSupportedSignature(alg uint16) bool {
	return signatureCurve(alg) != nil
}

// curveHash is the hash a curve signs with: belt-hash at level 128, bash384 and bash512 above.
func curveHash(curve *bign.Curve) hash.Hash {
	switch curve.Level {
	case 192:
		return bash.New384()
	case 256:
		return bash.New512()
	}
	return belt.NewHash()
}

var hashOIDDER = map[*bign.Curve][]byte{}

func init() {
	for _, c := range bign.Curves {
		hashOIDDER[c] = bign.OIDDER(c.HashOID())
	}
}

func signDigest(curve *bign.Curve, parts ...[]byte) []byte {
	h := curveHash(curve)
	for _, p := range parts {
		h.Write(p)
	}
	return h.Sum(nil)
}
