package stbkey

import (
	"bytes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"PorosCore/internal/stb/belt"
	"PorosCore/internal/stb/bign"
)

func TestRoundTrip(t *testing.T) {
	for _, c := range bign.Curves {
		d, pub, err := c.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		key := &PrivateKey{Curve: c, D: d}
		plain, _ := EncodePEM(key)
		got, err := ParsePrivateKey(plain, nil)
		if err != nil || got.Curve != c || !bytes.Equal(got.D, d) || IsEncrypted(plain) {
			t.Fatalf("%s: plain key: %v", c.Name, err)
		}
		sealed, err := Encrypt(key, []byte("пароль"), rand.Reader)
		if err != nil || !IsEncrypted(sealed) {
			t.Fatalf("%s: %v", c.Name, err)
		}
		got, err = ParsePrivateKey(sealed, []byte("пароль"))
		if err != nil || got.Curve != c || !bytes.Equal(got.D, d) {
			t.Fatalf("%s: protected key: %v", c.Name, err)
		}
		if _, err := ParsePrivateKey(sealed, []byte("другой")); err != ErrBadPassword {
			t.Fatalf("%s: a wrong password gave %v", c.Name, err)
		}
		if _, err := ParsePrivateKey(sealed, nil); err != ErrPasswordRequired {
			t.Fatalf("%s: no password gave %v", c.Name, err)
		}
		spki, _ := MarshalPublicKeyInfo(&PublicKey{Curve: c, Point: pub})
		pk, err := ParsePublicKeyInfo(spki)
		if err != nil || pk.Curve != c || !bytes.Equal(pk.Point, pub) {
			t.Fatalf("%s: public key: %v", c.Name, err)
		}
	}
}

// TestBareKeyContainer reads a container of STB 34.101.45, annex Е.5: the private key of Г.1 as it is, under
// PBKDF2 of the password B194BAC80A08F53B with the salt H[192:200] and 10000 iterations.
func TestBareKeyContainer(t *testing.T) {
	token, _ := hex.DecodeString("4EA289D5F718087DD8EDB305BA1CE8980E5EC3E0B56C8BF9D5C3E909CF4C14F07B8204E67841A165E924945CD07F37E7")
	salt := belt.H[192:200]
	kdf, _ := asn1.Marshal(pbkdf2Params{Salt: salt, IterationCount: 10000,
		PRF: pkix.AlgorithmIdentifier{Algorithm: oidHMACHBelt, Parameters: asn1.NullRawValue}})
	params, _ := asn1.Marshal(pbes2Params{
		KeyDerivationFunc: pkix.AlgorithmIdentifier{Algorithm: oidPBKDF2, Parameters: asn1.RawValue{FullBytes: kdf}},
		EncryptionScheme:  pkix.AlgorithmIdentifier{Algorithm: oidBeltKWP, Parameters: asn1.NullRawValue},
	})
	der, _ := asn1.Marshal(encryptedPrivateKeyInfo{
		Algorithm:     pkix.AlgorithmIdentifier{Algorithm: oidPBES2, Parameters: asn1.RawValue{FullBytes: params}},
		EncryptedData: token,
	})
	key, err := ParsePrivateKey(der, []byte("B194BAC80A08F53B"))
	if err != nil {
		t.Fatal(err)
	}
	want, _ := hex.DecodeString("1F66B5B84B7339674533F0329C74F21834281FED0732429E0C79235FC273E269")
	if key.Curve != bign.Curve256 || !bytes.Equal(key.D, want) {
		t.Fatalf("got %X on %s", key.D, key.Curve.Name)
	}
}

func TestBeltCBCContainer(t *testing.T) {
	// The other PBES2 scheme bee2evp writes: belt-cbc256 with an IV and PKCS#7 padding. A wrong password must never
	// give a key, though CBC has no check of its own.
	d, _, _ := bign.Curve256.GenerateKey(rand.Reader)
	inner, _ := MarshalPKCS8(&PrivateKey{Curve: bign.Curve256, D: d})
	salt, iv := make([]byte, 16), make([]byte, 16)
	rand.Read(salt)
	rand.Read(iv)
	kek, _ := pbkdf2.Key(belt.NewHash, "zed", salt, 2048, 32)
	pad := 16 - len(inner)%16
	data := append(inner, bytes.Repeat([]byte{byte(pad)}, pad)...)
	block, _ := belt.NewCipher(kek)
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(data, data)
	kdf, _ := asn1.Marshal(pbkdf2Params{Salt: salt, IterationCount: 2048,
		PRF: pkix.AlgorithmIdentifier{Algorithm: oidHMACHBelt, Parameters: asn1.NullRawValue}})
	ivParam, _ := asn1.Marshal(iv)
	params, _ := asn1.Marshal(pbes2Params{
		KeyDerivationFunc: pkix.AlgorithmIdentifier{Algorithm: oidPBKDF2, Parameters: asn1.RawValue{FullBytes: kdf}},
		EncryptionScheme:  pkix.AlgorithmIdentifier{Algorithm: oidBeltCBC, Parameters: asn1.RawValue{FullBytes: ivParam}},
	})
	der, _ := asn1.Marshal(encryptedPrivateKeyInfo{
		Algorithm:     pkix.AlgorithmIdentifier{Algorithm: oidPBES2, Parameters: asn1.RawValue{FullBytes: params}},
		EncryptedData: data,
	})
	text := pem.EncodeToMemory(&pem.Block{Type: "ENCRYPTED PRIVATE KEY", Bytes: der})
	key, err := ParsePrivateKey(text, []byte("zed"))
	if err != nil || !bytes.Equal(key.D, d) {
		t.Fatalf("right password: %v", err)
	}
	for i := 0; i < 200; i++ {
		if _, err := ParsePrivateKey(text, []byte{byte(i), 'x'}); err != ErrBadPassword {
			t.Fatalf("wrong password %d gave %v", i, err)
		}
	}
}

// TestFixtures reads the sample keys of the BTLS test server of the standard when POROS_BTLS_FIXTURES points at
// its folder server; they are not part of the repository.
func TestFixtures(t *testing.T) {
	dir := os.Getenv("POROS_BTLS_FIXTURES")
	if dir == "" {
		t.Skip("POROS_BTLS_FIXTURES is not set")
	}
	for _, level := range []string{"256", "384", "512"} {
		keyPEM, err := os.ReadFile(filepath.Join(dir, "btls"+level, "priv"+level+".key"))
		if err != nil {
			t.Fatal(err)
		}
		key, err := ParsePrivateKey(keyPEM, nil)
		if err != nil {
			t.Fatalf("priv%s: %v", level, err)
		}
		certPEM, _ := os.ReadFile(filepath.Join(dir, "btls"+level, "cert"+level+".pem"))
		block, _ := pem.Decode(certPEM)
		var cert struct {
			TBS struct {
				Raw          asn1.RawContent
				Version      int `asn1:"optional,explicit,default:0,tag:0"`
				Serial       asn1.RawValue
				Signature    pkix.AlgorithmIdentifier
				Issuer       asn1.RawValue
				Validity     asn1.RawValue
				Subject      asn1.RawValue
				PublicKeyRaw asn1.RawValue
			}
		}
		if _, err := asn1.Unmarshal(block.Bytes, &cert); err != nil {
			t.Fatal(err)
		}
		pub, err := ParsePublicKeyInfo(cert.TBS.PublicKeyRaw.FullBytes)
		if err != nil {
			t.Fatalf("cert%s: %v", level, err)
		}
		mine, _ := key.PublicKey()
		if pub.Curve != key.Curve || !bytes.Equal(pub.Point, mine) {
			t.Fatalf("priv%s does not match cert%s", level, level)
		}
	}
}
