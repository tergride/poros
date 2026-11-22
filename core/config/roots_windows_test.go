package config

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"slices"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/sagernet/sing-box/option"
	"golang.org/x/sys/windows"
)

// tlsOfDNS is what a DNS server over TLS of a built config verifies its certificate by.
func tlsOfDNS(t *testing.T, preset Preset, tag string) *option.OutboundTLSOptions {
	t.Helper()
	configs, err := Build(Input{Server: mustParse(t, sampleLinks(t)["trojan tcp"]), Preset: preset, MixedPort: 2090})
	if err != nil {
		t.Fatal(err)
	}
	for _, server := range configs.SingBox.DNS.Servers {
		if server.Tag != tag {
			continue
		}
		switch options := server.Options.(type) {
		case *option.RemoteHTTPSDNSServerOptions:
			return options.TLS
		case *option.RemoteTLSDNSServerOptions:
			return options.TLS
		}
		t.Fatalf("%s is no DNS over TLS: %T", tag, server.Options)
	}
	t.Fatalf("no %s", tag)
	return nil
}

func TestPublicDNSVerifiesWithoutTheSystem(t *testing.T) {
	// Windows fetches what it lacks for a chain from the internet, by the proxy of the system or the TUN: by Poros,
	// which waits for this very DNS.
	public := []string{
		"", "https://1.1.1.1/dns-query", "https://8.8.8.8/dns-query", "https://9.9.9.9/dns-query",
		"tls://1.0.0.1:853", "https://DNS.Google./dns-query", "tls://dns.quad9.net", "https://[2606:4700:4700::1111]/dns-query",
	}
	for _, text := range public {
		for _, tag := range []string{TagDNSRemote, TagDNSDirect} {
			preset := allPreset
			if tag == TagDNSRemote {
				preset.DNS = text
			} else if text == "" {
				continue
			} else {
				preset.DirectDNS = text
			}
			tls := tlsOfDNS(t, preset, tag)
			if tls == nil || len(tls.Certificate) == 0 {
				t.Errorf("%s %q asks the system about its certificate", tag, text)
				continue
			}
			if tls.Insecure || tls.DisableSNI {
				t.Errorf("%s %q does not verify: %+v", tag, text, tls)
			}
			var names []string
			for rest := []byte(strings.Join(tls.Certificate, "\n")); ; {
				var block *pem.Block
				if block, rest = pem.Decode(rest); block == nil {
					break
				}
				// An installed root may be one that Go does not read, and sing-box skips it.
				if root, err := x509.ParseCertificate(block.Bytes); err == nil {
					names = append(names, root.Subject.CommonName)
				}
			}
			if !strings.Contains(strings.Join(tls.Certificate, "\n"), installedRoots()) {
				t.Errorf("%s %q leaves out the installed roots", tag, text)
			}
			// The roots of Google, Cloudflare and Quad9.
			for _, want := range []string{"GTS Root R1", "SSL.com Root Certification Authority ECC", "DigiCert Global Root G3"} {
				if !slices.Contains(names, want) {
					t.Errorf("%s %q has no %s", tag, text, want)
				}
			}
		}
	}
}

func TestOwnDNSVerifiesWithTheSystem(t *testing.T) {
	// A DNS of the user's may have a certificate of a company, whose root only the system knows.
	for _, text := range []string{"https://10.0.0.53/dns-query", "tls://dns.corp.example", "https://dns.google.example/dns-query"} {
		for _, tag := range []string{TagDNSRemote, TagDNSDirect} {
			preset := allPreset
			if tag == TagDNSRemote {
				preset.DNS = text
			} else {
				preset.DirectDNS = text
			}
			if tls := tlsOfDNS(t, preset, tag); tls != nil && (len(tls.Certificate) > 0 || tls.Insecure) {
				t.Errorf("%s %q must leave its certificate to the system", tag, text)
			}
		}
	}
}

// memoryStore is a store of certificates of the test's own: the stores of Windows stay as they are.
func memoryStore(t *testing.T) windows.Handle {
	t.Helper()
	store, err := windows.CertOpenStore(windows.CERT_STORE_PROV_MEMORY, 0, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = windows.CertCloseStore(store, 0) })
	return store
}

func testRoot(t *testing.T, name string, usage ...x509.ExtKeyUsage) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		ExtKeyUsage:           usage,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func TestInstalledRootsAreThoseForServers(t *testing.T) {
	setUsage := windows.NewLazySystemDLL("crypt32.dll").NewProc("CertSetEnhancedKeyUsage")
	put := func(store windows.Handle, der []byte, purposes ...string) {
		cert, err := windows.CertCreateCertificateContext(windows.X509_ASN_ENCODING, &der[0], uint32(len(der)))
		if err != nil {
			t.Fatal(err)
		}
		defer windows.CertFreeCertificateContext(cert)
		var added *windows.CertContext
		if err := windows.CertAddCertificateContextToStore(store, cert, windows.CERT_STORE_ADD_ALWAYS, &added); err != nil {
			t.Fatal(err)
		}
		defer windows.CertFreeCertificateContext(added)
		if len(purposes) == 0 {
			return
		}
		// What the properties of a root in certmgr set: "" is all purposes off.
		var list []*byte
		for _, purpose := range purposes {
			if purpose != "" {
				oid, err := windows.BytePtrFromString(purpose)
				if err != nil {
					t.Fatal(err)
				}
				list = append(list, oid)
			}
		}
		usage := windows.CertEnhKeyUsage{Length: uint32(len(list))}
		if len(list) > 0 {
			usage.UsageIdentifiers = &list[0]
		}
		if ok, _, err := setUsage.Call(uintptr(unsafe.Pointer(added)), uintptr(unsafe.Pointer(&usage))); ok == 0 {
			t.Fatal(err)
		}
	}

	root, public, denied := memoryStore(t), memoryStore(t), memoryStore(t)
	const codeSigning = "1.3.6.1.5.5.7.3.3"
	company := testRoot(t, "company")
	put(root, company)
	put(root, company)
	put(root, testRoot(t, "server", x509.ExtKeyUsageServerAuth))
	put(root, testRoot(t, "server by the store"), codeSigning, oidServerAuth)
	put(root, testRoot(t, "code", x509.ExtKeyUsageCodeSigning))
	put(root, testRoot(t, "code by the store"), codeSigning)
	put(root, testRoot(t, "no purposes"), "")
	ofProgram, distrusted := testRoot(t, "public"), testRoot(t, "distrusted")
	put(root, ofProgram)
	put(public, ofProgram)
	put(root, distrusted)
	put(denied, distrusted)

	var names []string
	for rest := []byte(rootsForServers(root, public, denied)); ; {
		var block *pem.Block
		if block, rest = pem.Decode(rest); block == nil {
			break
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, cert.Subject.CommonName)
	}
	slices.Sort(names)
	if want := []string{"company", "server", "server by the store"}; !slices.Equal(names, want) {
		t.Errorf("roots for servers: %q, want %q", names, want)
	}
}
