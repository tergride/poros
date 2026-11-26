package patches

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	mathrand "math/rand/v2"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"PorosCore/internal/boxmain"
	"PorosCore/internal/stb/bign"
	"PorosCore/internal/stb/stbkey"
	"PorosCore/internal/stb/stbtest"
)

// TestOpenVPNOverBTLS connects an OpenVPN client of sing-box to an OpenVPN server of sing-box on loopback with BTLS,
// a belt data cipher, HMAC on bash512 and tls-crypt-v2, and fetches a page through the tunnel. sing-openvpn without
// the points of the copy has neither BTLS nor belt nor bash.
func TestOpenVPNOverBTLS(t *testing.T) {
	for _, c := range []struct {
		name, suites, cipher string
		curve                *bign.Curve
	}{
		{"cbc", "DHE-BIGN-WITH-BELT-DWP-HBELT", "belt-cbc256", bign.Curve256},
		{"cfb", "DHT-BIGN-WITH-BELT-CTR-MAC-HBELT:DHE-BIGN-WITH-BELT-DWP-HBELT", "belt-cfb256", bign.Curve384},
	} {
		t.Run(c.name, func(t *testing.T) {
			page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				io.WriteString(w, "inside")
			}))
			t.Cleanup(page.Close)
			pagePort := page.Listener.Addr().(*net.TCPAddr).Port

			ca := issue(t, stbtest.Options{Name: "ca", Curve: c.curve, IsCA: true, KeyUsage: x509.KeyUsageCertSign}, nil)
			server := issue(t, stbtest.Options{Name: "server", Curve: c.curve, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageKeyAgreement, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}, ca)
			client := issue(t, stbtest.Options{Name: "client", Curve: c.curve, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyAgreement, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}, ca)
			serverWrap, clientWrap := tlsCryptV2Keys(t)

			serverPort, proxyPort := freePort(t), freePort(t)
			for proxyPort == serverPort {
				proxyPort = freePort(t)
			}
			runBox(t, fmt.Sprintf(`{"log":{"level":"warn"},"endpoints":[{"type":"openvpn-server","tag":"ovpn-in","listen":"127.0.0.1",`+
				`"listen_port":%d,"network":"tcp","address":["10.8.0.1/24"],"data_ciphers":[%q],"auth":"bash512",`+
				`"tls":{"certificate":%s,"key":%s,"client_certificate":%s,"verify_client_certificate":"require",`+
				`"version_min":"1.2","version_max":"1.2","cipher":%q,"control_wrap":{"type":"tls_crypt_v2","key":%s}}}],`+
				`"outbounds":[{"type":"direct","tag":"direct"}],`+
				`"route":{"rules":[{"ip_cidr":["192.0.2.80/32"],"action":"route","outbound":"direct","override_address":"127.0.0.1"}],"final":"direct"}}`,
				serverPort, c.cipher, text(t, stbtest.PEM(server.Cert)), keyText(t, server), text(t, stbtest.PEM(ca.Cert)), c.suites, text(t, serverWrap)))
			runBox(t, fmt.Sprintf(`{"log":{"level":"warn"},"inbounds":[{"type":"mixed","listen":"127.0.0.1","listen_port":%d}],`+
				`"endpoints":[{"type":"openvpn-client","tag":"ovpn","server":"127.0.0.1","server_port":%d,"network":"tcp",`+
				`"data_ciphers":[%q],"auth":"bash512","tls":{"certificate":%s,"client_certificate":%s,"client_key":%s,`+
				`"version_min":"1.2","version_max":"1.2","cipher":%q,"control_wrap":{"type":"tls_crypt_v2","key":%s}}}],`+
				`"route":{"final":"ovpn"}}`,
				proxyPort, serverPort, c.cipher, text(t, stbtest.PEM(ca.Cert)), text(t, stbtest.PEM(client.Cert)), keyText(t, client), c.suites, text(t, clientWrap)))

			proxy, _ := url.Parse("http://127.0.0.1:" + strconv.Itoa(int(proxyPort)))
			httpClient := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxy)}, Timeout: 5 * time.Second}
			target := "http://" + net.JoinHostPort("192.0.2.80", strconv.Itoa(pagePort))
			var last error
			for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
				response, err := httpClient.Get(target)
				if err != nil {
					last = err
					continue
				}
				body, err := io.ReadAll(response.Body)
				response.Body.Close()
				if err == nil && string(body) == "inside" {
					return
				}
				last = fmt.Errorf("status %d, body %q, %v", response.StatusCode, body, err)
			}
			t.Fatalf("no page through the tunnel: %v", last)
		})
	}
}

func issue(t *testing.T, options stbtest.Options, parent *stbtest.Identity) *stbtest.Identity {
	t.Helper()
	identity, err := stbtest.Issue(options, parent)
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func runBox(t *testing.T, config string) {
	t.Helper()
	instance, cancel, err := boxmain.Create([]byte(config), nil)
	if err != nil {
		t.Fatalf("box: %v\n%s", err, config)
	}
	t.Cleanup(func() {
		cancel()
		instance.Close()
	})
}

// text is a PEM text as the JSON list of lines sing-box takes.
func text(t *testing.T, pemText []byte) string {
	t.Helper()
	data, err := json.Marshal([]string{string(pemText)})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func keyText(t *testing.T, identity *stbtest.Identity) string {
	t.Helper()
	key, err := stbkey.EncodePEM(identity.Key)
	if err != nil {
		t.Fatal(err)
	}
	return text(t, key)
}

// tlsCryptV2Keys makes a server key and a client key wrapped by it as OpenVPN does: the client key, metadata of a
// timestamp, the tag HMAC-SHA256 over the length and the plaintext, and AES-256-CTR with the tag as the IV.
func tlsCryptV2Keys(t *testing.T) (server, client []byte) {
	t.Helper()
	serverKey := make([]byte, 128)
	clientKey := make([]byte, 256)
	if _, err := rand.Read(serverKey); err != nil {
		t.Fatal(err)
	}
	if _, err := rand.Read(clientKey); err != nil {
		t.Fatal(err)
	}
	plaintext := append(append([]byte{}, clientKey...), 1)
	plaintext = binary.BigEndian.AppendUint64(plaintext, uint64(time.Now().Unix()))
	length := binary.BigEndian.AppendUint16(nil, uint16(sha256.Size+len(plaintext)+2))
	mac := hmac.New(sha256.New, serverKey[64:96])
	mac.Write(length)
	mac.Write(plaintext)
	tag := mac.Sum(nil)
	block, err := aes.NewCipher(serverKey[:32])
	if err != nil {
		t.Fatal(err)
	}
	ciphertext := make([]byte, len(plaintext))
	cipher.NewCTR(block, tag[:aes.BlockSize]).XORKeyStream(ciphertext, plaintext)
	wrapped := append(append(append(append([]byte{}, clientKey...), tag...), ciphertext...), length...)
	return pem.EncodeToMemory(&pem.Block{Type: "OpenVPN tls-crypt-v2 server key", Bytes: serverKey}),
		pem.EncodeToMemory(&pem.Block{Type: "OpenVPN tls-crypt-v2 client key", Bytes: wrapped})
}

// freePort finds a port free on loopback, below the ports outgoing connections take theirs from (49152 and up on
// Windows, 32768 and up on Linux): between this check and the start of the server, a connection would take it
// otherwise.
func freePort(t *testing.T) uint16 {
	t.Helper()
	var last error
	for range 200 {
		port := 10000 + mathrand.IntN(10000)
		listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err == nil {
			listener.Close()
			return uint16(port)
		}
		last = err
	}
	t.Fatalf("no port is free: %v", last)
	return 0
}
