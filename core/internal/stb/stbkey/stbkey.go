// Package stbkey reads and writes bign keys: PKCS#8 private keys, plain or protected by a password as the containers
// of STB 34.101.78 (PBES2 with PBKDF2 over hmac-hbelt and belt-kwp256), and public keys of certificates.
package stbkey

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"fmt"
	"hash"
	"io"
	"strings"

	"PorosCore/internal/stb/belt"
	"PorosCore/internal/stb/bign"
)

var (
	ErrPasswordRequired = errors.New("the private key is protected by a password")
	ErrBadPassword      = errors.New("the password of the private key is wrong")
	ErrNotBign          = errors.New("not a bign key")
)

var (
	oidPBES2     = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 13}
	oidPBKDF2    = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 12}
	oidHMACHBelt = asn1.ObjectIdentifier{1, 2, 112, 0, 2, 0, 34, 101, 47, 12}
	oidHMACSHA1  = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 7}
	oidHMACSHA2  = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 9}
	oidBeltKWP   = asn1.ObjectIdentifier{1, 2, 112, 0, 2, 0, 34, 101, 31, 73}
	oidBeltCBC   = asn1.ObjectIdentifier{1, 2, 112, 0, 2, 0, 34, 101, 31, 23}
	oidBeltCFB   = asn1.ObjectIdentifier{1, 2, 112, 0, 2, 0, 34, 101, 31, 33}
	oidAES128CBC = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 2}
	oidAES256CBC = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 42}
)

// maxIterations bounds PBKDF2, so that a crafted file cannot keep the core busy for long.
const maxIterations = 1_000_000

// PrivateKey is a bign private key: d, l/4 little-endian bytes, on a standard curve.
type PrivateKey struct {
	Curve *bign.Curve
	D     []byte
}

// PublicKey returns the public key x‖y of the private key.
func (k *PrivateKey) PublicKey() ([]byte, error) {
	return k.Curve.PublicKey(k.D)
}

type privateKeyInfo struct {
	Version    int
	Algorithm  pkix.AlgorithmIdentifier
	PrivateKey []byte
	Attributes asn1.RawValue `asn1:"optional,tag:0"`
}

type encryptedPrivateKeyInfo struct {
	Algorithm     pkix.AlgorithmIdentifier
	EncryptedData []byte
}

type pbes2Params struct {
	KeyDerivationFunc pkix.AlgorithmIdentifier
	EncryptionScheme  pkix.AlgorithmIdentifier
}

type pbkdf2Params struct {
	Salt           []byte
	IterationCount int
	KeyLength      int                      `asn1:"optional"`
	PRF            pkix.AlgorithmIdentifier `asn1:"optional"`
}

// keyDER returns the DER of the first private key in a PEM text, or the data as they are when they are not PEM.
func keyDER(data []byte) []byte {
	rest := data
	for {
		block, next := pem.Decode(rest)
		if block == nil {
			break
		}
		if strings.HasSuffix(block.Type, "PRIVATE KEY") {
			return block.Bytes
		}
		rest = next
	}
	return bytes.TrimSpace(data)
}

// IsEncrypted tells a private key protected by a password.
func IsEncrypted(data []byte) bool {
	var info encryptedPrivateKeyInfo
	rest, err := asn1.Unmarshal(keyDER(data), &info)
	return err == nil && len(rest) == 0 && info.Algorithm.Algorithm.Equal(oidPBES2)
}

// ParsePrivateKey reads a bign private key from PEM or DER, with the password when it is protected.
func ParsePrivateKey(data, password []byte) (*PrivateKey, error) {
	der := keyDER(data)
	var enc encryptedPrivateKeyInfo
	if rest, err := asn1.Unmarshal(der, &enc); err == nil && len(rest) == 0 && enc.Algorithm.Algorithm.Equal(oidPBES2) {
		if len(password) == 0 {
			return nil, ErrPasswordRequired
		}
		plain, checked, err := decrypt(enc, password)
		if err != nil {
			return nil, err
		}
		defer clear(plain)
		key, err := parsePlain(plain, checked)
		if err != nil {
			// A wrong password of CBC or CFB passes the decryption and gives noise instead of a key.
			if errors.Is(err, ErrNotBign) {
				return nil, err
			}
			return nil, ErrBadPassword
		}
		return key, nil
	}
	return parsePlain(der, false)
}

// parsePlain reads PrivateKeyInfo. With bare, it also takes a bare d of 32, 48 or 64 bytes, which STB 34.101.45
// (annex Е) protects as is and whose length names the curve; only belt-kwp, which checks its header, may give one,
// as a wrong password of CBC or CFB gives noise of any length.
func parsePlain(der []byte, bare bool) (*PrivateKey, error) {
	var info privateKeyInfo
	rest, err := asn1.Unmarshal(der, &info)
	if err != nil || len(rest) != 0 {
		for _, c := range bign.Curves {
			if bare && len(der) == c.Size() {
				return newPrivateKey(c, der)
			}
		}
		return nil, errors.New("the private key is not PKCS#8")
	}
	if !info.Algorithm.Algorithm.Equal(bign.OIDPublicKey) {
		return nil, ErrNotBign
	}
	curve, err := namedCurve(info.Algorithm.Parameters)
	if err != nil {
		return nil, err
	}
	d := info.PrivateKey
	// Some writers wrap d into one more OCTET STRING.
	if len(d) != curve.Size() {
		var inner []byte
		if rest, err := asn1.Unmarshal(d, &inner); err == nil && len(rest) == 0 {
			d = inner
		}
	}
	return newPrivateKey(curve, d)
}

func newPrivateKey(c *bign.Curve, d []byte) (*PrivateKey, error) {
	if _, err := c.PublicKey(d); err != nil {
		return nil, err
	}
	return &PrivateKey{Curve: c, D: append([]byte(nil), d...)}, nil
}

func namedCurve(params asn1.RawValue) (*bign.Curve, error) {
	var oid asn1.ObjectIdentifier
	if rest, err := asn1.Unmarshal(params.FullBytes, &oid); err != nil || len(rest) != 0 {
		return nil, errors.New("the bign key has no standard curve")
	}
	c := bign.CurveByOID(oid)
	if c == nil {
		return nil, fmt.Errorf("the curve %v is not a standard one", oid)
	}
	return c, nil
}

func decrypt(enc encryptedPrivateKeyInfo, password []byte) (plain []byte, checked bool, err error) {
	var params pbes2Params
	if rest, err := asn1.Unmarshal(enc.Algorithm.Parameters.FullBytes, &params); err != nil || len(rest) != 0 {
		return nil, false, errors.New("bad PBES2 parameters")
	}
	if !params.KeyDerivationFunc.Algorithm.Equal(oidPBKDF2) {
		return nil, false, fmt.Errorf("the key derivation %v is not supported", params.KeyDerivationFunc.Algorithm)
	}
	var kdf pbkdf2Params
	if rest, err := asn1.Unmarshal(params.KeyDerivationFunc.Parameters.FullBytes, &kdf); err != nil || len(rest) != 0 {
		return nil, false, errors.New("bad PBKDF2 parameters")
	}
	if kdf.IterationCount < 1 || kdf.IterationCount > maxIterations {
		return nil, false, fmt.Errorf("PBKDF2 asks for %d iterations", kdf.IterationCount)
	}
	var prf func() hash.Hash
	switch prfOID := kdf.PRF.Algorithm; {
	case prfOID.Equal(oidHMACHBelt):
		prf = belt.NewHash
	case prfOID.Equal(oidHMACSHA2):
		prf = sha256.New
	case len(prfOID) == 0, prfOID.Equal(oidHMACSHA1):
		prf = sha1.New
	default:
		return nil, false, fmt.Errorf("the PBKDF2 function %v is not supported", prfOID)
	}
	scheme := params.EncryptionScheme
	keyLength := 32
	if scheme.Algorithm.Equal(oidAES128CBC) {
		keyLength = 16
	}
	if kdf.KeyLength != 0 && kdf.KeyLength != keyLength {
		return nil, false, errors.New("PBKDF2 gives a key of a wrong length")
	}
	key, err := pbkdf2.Key(prf, string(password), kdf.Salt, kdf.IterationCount, keyLength)
	if err != nil {
		return nil, false, err
	}
	defer clear(key)
	switch {
	case scheme.Algorithm.Equal(oidBeltKWP):
		plain, err := belt.KeyUnwrap(key, enc.EncryptedData, nil)
		if err != nil {
			return nil, false, ErrBadPassword
		}
		return plain, true, nil
	case scheme.Algorithm.Equal(oidBeltCBC), scheme.Algorithm.Equal(oidBeltCFB),
		scheme.Algorithm.Equal(oidAES128CBC), scheme.Algorithm.Equal(oidAES256CBC):
		var iv []byte
		if rest, err := asn1.Unmarshal(scheme.Parameters.FullBytes, &iv); err != nil || len(rest) != 0 || len(iv) != 16 {
			return nil, false, errors.New("bad IV of the private key")
		}
		var block cipher.Block
		if scheme.Algorithm.Equal(oidBeltCBC) || scheme.Algorithm.Equal(oidBeltCFB) {
			block, err = belt.NewCipher(key)
		} else {
			block, err = aes.NewCipher(key)
		}
		if err != nil {
			return nil, false, err
		}
		data := append([]byte(nil), enc.EncryptedData...)
		if scheme.Algorithm.Equal(oidBeltCFB) {
			//nolint:staticcheck // PBES2 with belt-cfb is a format bee2evp writes.
			cipher.NewCFBDecrypter(block, iv).XORKeyStream(data, data)
			return data, false, nil
		}
		if len(data) == 0 || len(data)%16 != 0 {
			return nil, false, ErrBadPassword
		}
		cipher.NewCBCDecrypter(block, iv).CryptBlocks(data, data)
		pad := int(data[len(data)-1])
		if pad == 0 || pad > 16 || !bytes.Equal(data[len(data)-pad:], bytes.Repeat([]byte{byte(pad)}, pad)) {
			return nil, false, ErrBadPassword
		}
		return data[:len(data)-pad], false, nil
	}
	return nil, false, fmt.Errorf("the encryption %v of the private key is not supported", scheme.Algorithm)
}

// MarshalPKCS8 returns the DER PrivateKeyInfo of the key.
func MarshalPKCS8(k *PrivateKey) ([]byte, error) {
	params, err := asn1.Marshal(k.Curve.OID)
	if err != nil {
		return nil, err
	}
	return asn1.Marshal(privateKeyInfo{
		Algorithm:  pkix.AlgorithmIdentifier{Algorithm: bign.OIDPublicKey, Parameters: asn1.RawValue{FullBytes: params}},
		PrivateKey: k.D,
	})
}

// EncodePEM returns the key as PEM "PRIVATE KEY".
func EncodePEM(k *PrivateKey) ([]byte, error) {
	der, err := MarshalPKCS8(k)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

// Encrypt protects the key with a password as a container of STB 34.101.78: PBKDF2 over hmac-hbelt with a salt of
// 8 bytes and 10000 iterations, then belt-kwp256. It returns PEM "ENCRYPTED PRIVATE KEY".
func Encrypt(k *PrivateKey, password []byte, random io.Reader) ([]byte, error) {
	if random == nil {
		random = rand.Reader
	}
	der, err := MarshalPKCS8(k)
	if err != nil {
		return nil, err
	}
	defer clear(der)
	salt := make([]byte, 8)
	if _, err := io.ReadFull(random, salt); err != nil {
		return nil, err
	}
	const iterations = 10000
	key, err := pbkdf2.Key(belt.NewHash, string(password), salt, iterations, 32)
	if err != nil {
		return nil, err
	}
	defer clear(key)
	wrapped, err := belt.KeyWrap(key, der, nil)
	if err != nil {
		return nil, err
	}
	kdf, err := asn1.Marshal(pbkdf2Params{
		Salt: salt, IterationCount: iterations,
		PRF: pkix.AlgorithmIdentifier{Algorithm: oidHMACHBelt, Parameters: asn1.NullRawValue},
	})
	if err != nil {
		return nil, err
	}
	params, err := asn1.Marshal(pbes2Params{
		KeyDerivationFunc: pkix.AlgorithmIdentifier{Algorithm: oidPBKDF2, Parameters: asn1.RawValue{FullBytes: kdf}},
		EncryptionScheme:  pkix.AlgorithmIdentifier{Algorithm: oidBeltKWP, Parameters: asn1.NullRawValue},
	})
	if err != nil {
		return nil, err
	}
	out, err := asn1.Marshal(encryptedPrivateKeyInfo{
		Algorithm:     pkix.AlgorithmIdentifier{Algorithm: oidPBES2, Parameters: asn1.RawValue{FullBytes: params}},
		EncryptedData: wrapped,
	})
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "ENCRYPTED PRIVATE KEY", Bytes: out}), nil
}

// PublicKey is a bign public key x‖y on a standard curve.
type PublicKey struct {
	Curve *bign.Curve
	Point []byte
}

type subjectPublicKeyInfo struct {
	Algorithm pkix.AlgorithmIdentifier
	PublicKey asn1.BitString
}

// ParsePublicKeyInfo reads a DER SubjectPublicKeyInfo with a bign key.
func ParsePublicKeyInfo(der []byte) (*PublicKey, error) {
	var info subjectPublicKeyInfo
	if rest, err := asn1.Unmarshal(der, &info); err != nil || len(rest) != 0 {
		return nil, errors.New("bad SubjectPublicKeyInfo")
	}
	if !info.Algorithm.Algorithm.Equal(bign.OIDPublicKey) {
		return nil, ErrNotBign
	}
	curve, err := namedCurve(info.Algorithm.Parameters)
	if err != nil {
		return nil, err
	}
	point := info.PublicKey.RightAlign()
	if !curve.ValidPublicKey(point) {
		return nil, bign.ErrInvalidPublicKey
	}
	return &PublicKey{Curve: curve, Point: point}, nil
}

// MarshalPublicKeyInfo returns the DER SubjectPublicKeyInfo of the key.
func MarshalPublicKeyInfo(k *PublicKey) ([]byte, error) {
	params, err := asn1.Marshal(k.Curve.OID)
	if err != nil {
		return nil, err
	}
	return asn1.Marshal(subjectPublicKeyInfo{
		Algorithm: pkix.AlgorithmIdentifier{Algorithm: bign.OIDPublicKey, Parameters: asn1.RawValue{FullBytes: params}},
		PublicKey: asn1.BitString{Bytes: k.Point, BitLength: 8 * len(k.Point)},
	})
}
