package btls

import (
	"bytes"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"fmt"
	"hash"
	"time"

	"PorosCore/internal/stb/bash"
	"PorosCore/internal/stb/belt"
	"PorosCore/internal/stb/bign"
	"PorosCore/internal/stb/stbkey"
)

const maxChainLength = 10

// ErrUnknownAuthority means that no chain leads from the certificate to a trusted authority.
var ErrUnknownAuthority = errors.New("btls: the certificate is not signed by a trusted authority")

// CertificateInvalidError tells why a certificate of a chain does not fit.
type CertificateInvalidError struct {
	Cert   *x509.Certificate
	Reason string
}

func (e *CertificateInvalidError) Error() string {
	return fmt.Sprintf("btls: certificate %q %s", e.Cert.Subject.String(), e.Reason)
}

// VerifyChain builds a chain from the leaf certs[0] through the other certificates to one of the roots. It checks
// the bign signatures, the validity periods at now and that the issuers may issue: a basic constraint of an
// authority or a version 1 root, the key usage for signing certificates when present, and the path length. It
// returns the chain, leaf first, root last. Purposes and names of the leaf are left to the caller.
func VerifyChain(certs []*x509.Certificate, roots []*x509.Certificate, now time.Time) ([]*x509.Certificate, error) {
	if len(certs) == 0 {
		return nil, errors.New("btls: no certificate")
	}
	if len(roots) == 0 {
		return nil, errors.New("btls: no trusted authority to check the certificate with")
	}
	leaf := certs[0]
	if len(leaf.UnhandledCriticalExtensions) > 0 {
		return nil, &CertificateInvalidError{leaf, "has an unknown critical extension"}
	}
	b := &chainBuilder{intermediates: certs[1:], roots: roots, now: now}
	return b.build([]*x509.Certificate{leaf})
}

// maxSignatureChecks bounds the work a peer can cause with many certificates of the same name.
const maxSignatureChecks = 100

type chainBuilder struct {
	intermediates, roots []*x509.Certificate
	now                  time.Time
	checks               int
}

func (b *chainBuilder) build(chain []*x509.Certificate) ([]*x509.Certificate, error) {
	current := chain[len(chain)-1]
	if err := checkValidity(current, b.now); err != nil {
		return nil, err
	}
	for _, root := range b.roots {
		if bytes.Equal(current.Raw, root.Raw) {
			return chain, nil
		}
	}
	if len(chain) >= maxChainLength {
		return nil, errors.New("btls: the certificate chain is too long")
	}
	err := ErrUnknownAuthority
	try := func(issuer *x509.Certificate) error {
		if !bytes.Equal(issuer.RawSubject, current.RawIssuer) {
			return ErrUnknownAuthority
		}
		for _, c := range chain {
			if bytes.Equal(c.Raw, issuer.Raw) {
				return ErrUnknownAuthority
			}
		}
		if e := checkIssuer(issuer, len(chain)-1); e != nil {
			return e
		}
		if b.checks++; b.checks > maxSignatureChecks {
			return errors.New("btls: too many certificates to check")
		}
		return CheckSignature(current, issuer)
	}
	for _, root := range b.roots {
		e := try(root)
		if e == nil {
			if e = checkValidity(root, b.now); e == nil {
				return append(chain, root), nil
			}
		}
		if !errors.Is(e, ErrUnknownAuthority) {
			err = e
		}
	}
	for _, issuer := range b.intermediates {
		e := try(issuer)
		if e == nil {
			var found []*x509.Certificate
			if found, e = b.build(append(chain[:len(chain):len(chain)], issuer)); e == nil {
				return found, nil
			}
		}
		if !errors.Is(e, ErrUnknownAuthority) {
			err = e
		}
	}
	return nil, err
}

func checkValidity(cert *x509.Certificate, now time.Time) error {
	if now.Before(cert.NotBefore) {
		return &CertificateInvalidError{cert, "is not valid yet"}
	}
	if now.After(cert.NotAfter) {
		return &CertificateInvalidError{cert, "has expired"}
	}
	return nil
}

// checkIssuer follows check_ca of OpenSSL; below is the number of authorities between the issuer and the leaf.
func checkIssuer(cert *x509.Certificate, below int) error {
	if cert.KeyUsage != 0 && cert.KeyUsage&x509.KeyUsageCertSign == 0 {
		return &CertificateInvalidError{cert, "may not sign certificates"}
	}
	switch {
	case cert.BasicConstraintsValid:
		if !cert.IsCA {
			return &CertificateInvalidError{cert, "is not an authority"}
		}
		if cert.MaxPathLen >= 0 && below > cert.MaxPathLen {
			return &CertificateInvalidError{cert, "allows a shorter chain"}
		}
	case cert.Version == 1 && bytes.Equal(cert.RawSubject, cert.RawIssuer), cert.KeyUsage != 0:
	default:
		return &CertificateInvalidError{cert, "is not an authority"}
	}
	return nil
}

type signedCertificate struct {
	TBS       asn1.RawValue
	Algorithm pkix.AlgorithmIdentifier
	Signature asn1.BitString
}

// CheckSignature checks the bign signature of a certificate under the key of its issuer.
func CheckSignature(cert, issuer *x509.Certificate) error {
	key, err := stbkey.ParsePublicKeyInfo(issuer.RawSubjectPublicKeyInfo)
	if err != nil {
		return &CertificateInvalidError{issuer, "has no bign key: " + err.Error()}
	}
	var signed signedCertificate
	if rest, err := asn1.Unmarshal(cert.Raw, &signed); err != nil || len(rest) != 0 {
		return &CertificateInvalidError{cert, "is malformed"}
	}
	newHash, hashOID, err := signatureHash(signed.Algorithm)
	if err != nil {
		return &CertificateInvalidError{cert, err.Error()}
	}
	if signed.Signature.BitLength%8 != 0 {
		return &CertificateInvalidError{cert, "has a malformed signature"}
	}
	h := newHash()
	h.Write(cert.RawTBSCertificate)
	if !key.Curve.Verify(key.Point, bign.OIDDER(hashOID), h.Sum(nil), signed.Signature.Bytes) {
		return &CertificateInvalidError{cert, "has a bad signature"}
	}
	return nil
}

var nullParameters = []byte{5, 0}

// signatureHash reads the algorithm of a bign signature of STB 34.101.45, annex Д: the hash comes with the
// identifier, or as the parameter of bign-with-hspec.
func signatureHash(alg pkix.AlgorithmIdentifier) (func() hash.Hash, asn1.ObjectIdentifier, error) {
	params := alg.Parameters.FullBytes
	noParams := len(params) == 0 || bytes.Equal(params, nullParameters)
	oid := alg.Algorithm
	switch {
	case oid.Equal(bign.OIDSignWithBeltHash) && noParams:
		oid = bign.OIDBeltHash
	case oid.Equal(bign.OIDSignWithBash256) && noParams:
		oid = bign.OIDBash256
	case oid.Equal(bign.OIDSignWithBash384) && noParams:
		oid = bign.OIDBash384
	case oid.Equal(bign.OIDSignWithBash512) && noParams:
		oid = bign.OIDBash512
	case oid.Equal(bign.OIDSignWithHashSpec):
		oid = nil
		if rest, err := asn1.Unmarshal(params, &oid); err != nil || len(rest) != 0 {
			return nil, nil, errors.New("has a malformed signature algorithm")
		}
	default:
		return nil, nil, fmt.Errorf("is signed by %v, not by bign", alg.Algorithm)
	}
	switch {
	case oid.Equal(bign.OIDBeltHash):
		return belt.NewHash, oid, nil
	case oid.Equal(bign.OIDBash256):
		return bash.New256, oid, nil
	case oid.Equal(bign.OIDBash384):
		return bash.New384, oid, nil
	case oid.Equal(bign.OIDBash512):
		return bash.New512, oid, nil
	}
	return nil, nil, fmt.Errorf("is signed with the hash %v", oid)
}
