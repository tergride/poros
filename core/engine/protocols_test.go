package engine

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"PorosCore/internal/boxmain"
)

// The protocols beyond the first nine, each against its sing-box server on loopback. NaiveProxy checks certificates
// as a browser does, so a loopback server with its own certificate cannot stand in for one.

// runServer starts a sing-box server from inbounds, endpoints and route rules written as JSON.
func runServer(t *testing.T, config string) {
	t.Helper()
	server, cancel, err := boxmain.Create([]byte(config), nil)
	if err != nil {
		t.Fatalf("the test server: %v\n%s", err, config)
	}
	t.Cleanup(func() {
		cancel()
		server.Close()
	})
}

// testCertificate is a self-signed certificate for localhost and 127.0.0.1, as PEM.
func testCertificate(t *testing.T) (certificate, key string) {
	t.Helper()
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		// Its own authority, for the clients that take a CA rather than skip the check.
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &private.PublicKey, private)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
}

func jsonText(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func porosLink(t *testing.T, profile string) string {
	return "poros://add/" + base64.RawURLEncoding.EncodeToString([]byte(profile))
}

func TestConnectionThroughHysteria(t *testing.T) {
	e, states := openConnectionTest(t)
	page := webPage(t)
	port := freePort(t)
	certificate, key := testCertificate(t)
	runServer(t, fmt.Sprintf(`{"log":{"level":"warn"},"inbounds":[{"type":"hysteria","listen":"127.0.0.1","listen_port":%d,`+
		`"up_mbps":100,"down_mbps":100,"users":[{"auth_str":"pw"}],"obfs":"ob","tls":{"enabled":true,"certificate":%s,"key":%s}}],`+
		`"outbounds":[{"type":"direct","tag":"direct"}]}`, port, jsonText(t, certificate), jsonText(t, key)))
	useServer(t, e, fmt.Sprintf("hysteria://127.0.0.1:%d?auth=pw&upmbps=100&downmbps=100&obfsParam=ob&sni=localhost&insecure=1#local", port))
	connectAndFetch(t, e, states, page)
}

func TestConnectionThroughSnell(t *testing.T) {
	e, states := openConnectionTest(t)
	page := webPage(t)
	port := freePort(t)
	runServer(t, fmt.Sprintf(`{"log":{"level":"warn"},"inbounds":[{"type":"snell","listen":"127.0.0.1","listen_port":%d,`+
		`"version":6,"psk":"a-long-test-psk"}],"outbounds":[{"type":"direct","tag":"direct"}]}`, port))
	useServer(t, e, fmt.Sprintf("snell://a-long-test-psk@127.0.0.1:%d?version=6#local", port))
	connectAndFetch(t, e, states, page)
}

func TestConnectionThroughShadowTLS(t *testing.T) {
	e, states := openConnectionTest(t)
	page := webPage(t)
	// The site of the handshake: ShadowTLS v3 needs TLS 1.3, which Go's test server speaks.
	site := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(site.Close)
	siteURL, _ := url.Parse(site.URL)
	sitePort, _ := strconv.Atoi(siteURL.Port())
	port := freePort(t)
	const key = "AAECAwQFBgcICQoLDA0ODw=="
	runServer(t, fmt.Sprintf(`{"log":{"level":"warn"},"inbounds":[`+
		`{"type":"shadowtls","listen":"127.0.0.1","listen_port":%d,"version":3,"users":[{"password":"pw"}],`+
		`"handshake":{"server":"127.0.0.1","server_port":%d},"detour":"ss-in"},`+
		`{"type":"shadowsocks","tag":"ss-in","listen":"127.0.0.1","method":"2022-blake3-aes-128-gcm","password":"%s"}],`+
		`"outbounds":[{"type":"direct","tag":"direct"}]}`, port, sitePort, key))
	useServer(t, e, porosLink(t, fmt.Sprintf(`{"type":"shadowsocks","name":"local","server":"127.0.0.1","server_port":%d,`+
		`"shadowsocks":{"method":"2022-blake3-aes-128-gcm","password":"%s","shadowtls":{"version":3,"password":"pw",`+
		`"tls":{"enabled":true,"server_name":"example.com","insecure":true}}}}`, port, key)))
	connectAndFetch(t, e, states, page)
}

func TestConnectionThroughWireGuard(t *testing.T) {
	e, states := openConnectionTest(t)
	keys := func() (private, public string) {
		key, err := ecdh.X25519().GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		return base64.StdEncoding.EncodeToString(key.Bytes()), base64.StdEncoding.EncodeToString(key.PublicKey().Bytes())
	}
	serverPrivate, serverPublic := keys()
	clientPrivate, clientPublic := keys()
	page, err := url.Parse(webPage(t))
	if err != nil {
		t.Fatal(err)
	}
	// Loopback is no address inside a tunnel: the client asks for a documentation address, which the server sends
	// to the page.
	const inside = "192.0.2.80"
	port := freePort(t)
	runServer(t, fmt.Sprintf(`{"log":{"level":"warn"},"endpoints":[{"type":"wireguard","tag":"wg-in","address":["10.200.0.1/24"],`+
		`"private_key":"%s","listen_port":%d,"peers":[{"public_key":"%s","allowed_ips":["10.200.0.2/32"]}]}],`+
		`"outbounds":[{"type":"direct","tag":"direct"}],`+
		`"route":{"rules":[{"ip_cidr":["%s/32"],"action":"route","outbound":"direct","override_address":"127.0.0.1"}],"final":"direct"}}`,
		serverPrivate, port, clientPublic, inside))
	useServer(t, e, fmt.Sprintf("wireguard://%s@127.0.0.1:%d?publickey=%s&address=10.200.0.2/32#local",
		url.QueryEscape(clientPrivate), port, url.QueryEscape(serverPublic)))
	connectAndFetch(t, e, states, "http://"+net.JoinHostPort(inside, page.Port()))
}

// insideRoute sends the documentation address a client asks for inside a tunnel to the page on loopback.
const insideRoute = `"route":{"rules":[{"ip_cidr":["192.0.2.80/32"],"action":"route","outbound":"direct","override_address":"127.0.0.1"}],"final":"direct"}`

func insidePage(t *testing.T) string {
	page, err := url.Parse(webPage(t))
	if err != nil {
		t.Fatal(err)
	}
	return "http://" + net.JoinHostPort("192.0.2.80", page.Port())
}

func TestConnectionThroughOpenVPN(t *testing.T) {
	e, states := openConnectionTest(t)
	page := insidePage(t)
	certificate, key := testCertificate(t)
	port := freePort(t)
	runServer(t, fmt.Sprintf(`{"log":{"level":"warn"},"endpoints":[{"type":"openvpn-server","tag":"ovpn-in","listen":"127.0.0.1",`+
		`"listen_port":%d,"network":"tcp","address":["10.8.0.1/24"],"users":[{"username":"u","password":"pw"}],`+
		`"tls":{"certificate":%s,"key":%s,"verify_client_certificate":"none"}}],"outbounds":[{"type":"direct","tag":"direct"}],%s}`,
		port, jsonText(t, certificate), jsonText(t, key), insideRoute))
	useServer(t, e, porosLink(t, fmt.Sprintf(`{"type":"openvpn","name":"local","server":"127.0.0.1","server_port":%d,`+
		`"openvpn":{"network":"tcp","username":"u","password":"pw","ca":%s}}`, port, jsonText(t, certificate))))
	connectAndFetch(t, e, states, page)
}

func TestConnectionThroughMASQUE(t *testing.T) {
	e, states := openConnectionTest(t)
	page := insidePage(t)
	certificate, key := testCertificate(t)
	port := freePort(t)
	runServer(t, fmt.Sprintf(`{"log":{"level":"warn"},"endpoints":[{"type":"masque-server","tag":"masque-in","listen":"127.0.0.1",`+
		`"listen_port":%d,"users":[{"username":"u","password":"pw"}],"address":["10.9.0.1/24"],`+
		`"tls":{"enabled":true,"certificate":%s,"key":%s}}],"outbounds":[{"type":"direct","tag":"direct"}],%s}`,
		port, jsonText(t, certificate), jsonText(t, key), insideRoute))
	useServer(t, e, porosLink(t, fmt.Sprintf(`{"type":"masque","name":"local","server":"127.0.0.1","server_port":%d,`+
		`"masque":{"username":"u","password":"pw","tls":{"enabled":true,"server_name":"localhost","insecure":true}}}`, port)))
	connectAndFetch(t, e, states, page)
}

// A refused login fails the start, rather than leave a connection that only looks up.
func TestOpenVPNRefusalFailsTheStart(t *testing.T) {
	e, states := openConnectionTest(t)
	certificate, key := testCertificate(t)
	port := freePort(t)
	runServer(t, fmt.Sprintf(`{"log":{"level":"warn"},"endpoints":[{"type":"openvpn-server","tag":"ovpn-in","listen":"127.0.0.1",`+
		`"listen_port":%d,"network":"tcp","address":["10.8.0.1/24"],"users":[{"username":"u","password":"pw"}],`+
		`"tls":{"certificate":%s,"key":%s,"verify_client_certificate":"none"}}],"outbounds":[{"type":"direct","tag":"direct"}]}`,
		port, jsonText(t, certificate), jsonText(t, key)))
	useServer(t, e, porosLink(t, fmt.Sprintf(`{"type":"openvpn","name":"local","server":"127.0.0.1","server_port":%d,`+
		`"openvpn":{"network":"tcp","username":"u","password":"wrong","ca":%s}}`, port, jsonText(t, certificate))))
	e.StartConnection()
	if state := waitState(t, states); state.State != ConnectionIdle || state.Error != ReasonStartFailed {
		t.Fatalf("a refused login: %+v", state)
	}
}
