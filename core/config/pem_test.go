package config

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"
)

// testCA is a self-signed certificate authority in PEM: OpenVPN reads its CA when the endpoint is created.
func testCA(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Poros test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})))
}

// testStaticKey is an OpenVPN static key: 256 random bytes as 16 lines of hex.
func testStaticKey(t *testing.T) string {
	t.Helper()
	key := make([]byte, 256)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	lines := []string{"-----BEGIN OpenVPN Static key V1-----"}
	for i := 0; i < len(key); i += 16 {
		lines = append(lines, hex.EncodeToString(key[i:i+16]))
	}
	return strings.Join(append(lines, "-----END OpenVPN Static key V1-----"), "\n")
}

func jsonString(t *testing.T, text string) string {
	t.Helper()
	data, err := json.Marshal(text)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
