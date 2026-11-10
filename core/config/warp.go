package config

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json/badoption"
)

// OutboundWARP sends a rule's traffic out through Cloudflare WARP after the server; without WARP it goes through the
// server.
const OutboundWARP = "warp"

// TagWARP is the endpoint of WARP.
const TagWARP = "warp"

// The kinds of WARP, the versions of HTTP of MASQUE, and what Cloudflare names MASQUE's server.
const (
	WARPWireGuard = "wireguard"
	WARPMASQUE    = "masque"
	// WARPHTTP3 keeps MASQUE on HTTP/3, WARPHTTP2 on HTTP/2; "" tries HTTP/3, then HTTP/2.
	WARPHTTP3      = "h3"
	WARPHTTP2      = "h2"
	warpServerName = "consumer-masque.cloudflareclient.com"
	// Cloudflare's WARP takes no larger packets.
	warpMTU = 1280
)

// WARP is a device of Cloudflare WARP, the exit of what rules send through it. It connects through the server.
type WARP struct {
	Mode   string
	Server string
	Port   uint16
	// PrivateKey is WireGuard's key in base64, or MASQUE's ECDSA key in base64 DER. PeerKey is the server's:
	// WireGuard's public key, or MASQUE's public key in base64 DER.
	PrivateKey string
	PeerKey    string
	Addresses  []netip.Prefix
	// Reserved are the bytes of WireGuard's reserved field.
	Reserved []uint8
	// ServerName is MASQUE's SNI, "" for Cloudflare's; HTTP is WARPHTTP3, WARPHTTP2 or "".
	ServerName string
	HTTP       string
}

// WARPDevice is a device of WARP as it is written: the endpoint as host:port, addresses and reserved bytes split by
// commas.
type WARPDevice struct {
	Endpoint, PrivateKey, PeerKey, Addresses, Reserved, ServerName, HTTP string
}

// Empty tells a device that has nothing written in.
func (d WARPDevice) Empty() bool {
	return d == WARPDevice{HTTP: d.HTTP}
}

// ParseWARP reads a device of mode. A part written wrong is named; a part missing is an error only when whole is set.
func ParseWARP(mode string, d WARPDevice, whole bool) (*WARP, error) {
	if mode != WARPWireGuard && mode != WARPMASQUE {
		return nil, fmt.Errorf("unknown WARP mode %q", mode)
	}
	w := &WARP{Mode: mode, PrivateKey: strings.TrimSpace(d.PrivateKey), PeerKey: strings.TrimSpace(d.PeerKey), ServerName: strings.TrimSpace(d.ServerName), HTTP: d.HTTP}
	if whole {
		for name, value := range map[string]string{"endpoint": d.Endpoint, "private key": w.PrivateKey, "peer key": w.PeerKey, "addresses": d.Addresses} {
			if strings.TrimSpace(value) == "" {
				return nil, fmt.Errorf("the %s of WARP is missing", name)
			}
		}
	}
	if text := strings.TrimSpace(d.Endpoint); text != "" {
		host, portText, err := net.SplitHostPort(text)
		port, perr := strconv.ParseUint(portText, 10, 16)
		if err != nil || perr != nil || host == "" || port == 0 {
			return nil, fmt.Errorf("the endpoint of WARP %q is no host:port", text)
		}
		w.Server, w.Port = host, uint16(port)
	}
	for _, field := range splitList(d.Addresses) {
		prefix, err := parsePrefix(field)
		if err != nil {
			return nil, fmt.Errorf("the address of WARP %q is no address", field)
		}
		w.Addresses = append(w.Addresses, prefix)
	}
	if mode == WARPWireGuard {
		for _, field := range splitList(d.Reserved) {
			value, err := strconv.ParseUint(field, 10, 8)
			if err != nil {
				return nil, fmt.Errorf("a reserved byte of WARP %q is no number from 0 to 255", field)
			}
			w.Reserved = append(w.Reserved, uint8(value))
		}
		if len(w.Reserved) != 0 && len(w.Reserved) != 3 {
			return nil, errors.New("WARP takes three reserved bytes")
		}
		for name, key := range map[string]string{"private key": w.PrivateKey, "peer key": w.PeerKey} {
			if raw, err := base64.StdEncoding.DecodeString(key); key != "" && (err != nil || len(raw) != 32) {
				return nil, fmt.Errorf("the %s of WARP is no WireGuard key", name)
			}
		}
		return w, nil
	}
	if w.HTTP != "" && w.HTTP != WARPHTTP3 && w.HTTP != WARPHTTP2 {
		return nil, fmt.Errorf("unknown HTTP version %q of MASQUE", w.HTTP)
	}
	if w.PrivateKey != "" {
		if _, err := masqueKey(w.PrivateKey); err != nil {
			return nil, err
		}
	}
	if w.PeerKey != "" {
		if raw, err := base64.StdEncoding.DecodeString(w.PeerKey); err != nil || !isPublicKey(raw) {
			return nil, errors.New("the peer key of WARP is no public key in base64 DER")
		}
	}
	return w, nil
}

func masqueKey(text string) (*ecdsa.PrivateKey, error) {
	raw, err := base64.StdEncoding.DecodeString(text)
	if err == nil {
		var key *ecdsa.PrivateKey
		if key, err = x509.ParseECPrivateKey(raw); err == nil {
			return key, nil
		}
	}
	return nil, errors.New("the private key of WARP is no ECDSA key in base64 DER")
}

func isPublicKey(raw []byte) bool {
	_, err := x509.ParsePKIXPublicKey(raw)
	return err == nil
}

// warpEndpoint builds WARP behind the server: its connection to Cloudflare goes through the proxy outbound.
func warpEndpoint(w *WARP) (option.Endpoint, error) {
	dialer := option.DialerOptions{Detour: TagProxy}
	if w.Mode == WARPWireGuard {
		return option.Endpoint{Type: C.TypeWireGuard, Tag: TagWARP, Options: &option.WireGuardEndpointOptions{
			MTU:        warpMTU,
			Address:    w.Addresses,
			PrivateKey: w.PrivateKey,
			Peers: []option.WireGuardPeer{{
				Address:                     w.Server,
				Port:                        w.Port,
				PublicKey:                   w.PeerKey,
				AllowedIPs:                  option.LegacyListable[netip.Prefix]{netip.MustParsePrefix("0.0.0.0/0"), netip.MustParsePrefix("::/0")},
				PersistentKeepaliveInterval: 25,
				Reserved:                    w.Reserved,
			}},
			DialerOptions: dialer,
		}}, nil
	}
	key, err := masqueKey(w.PrivateKey)
	if err != nil {
		return option.Endpoint{}, err
	}
	certificate, keyPEM, err := selfSigned(key)
	if err != nil {
		return option.Endpoint{}, err
	}
	peer, _ := base64.StdEncoding.DecodeString(w.PeerKey)
	pin := sha256.Sum256(peer)
	serverName := w.ServerName
	if serverName == "" {
		serverName = warpServerName
	}
	options := &option.MASQUEClientEndpointOptions{
		DialerOptions:         dialer,
		ServerOptions:         option.ServerOptions{Server: w.Server, ServerPort: w.Port},
		MASQUEEndpointOptions: option.MASQUEEndpointOptions{MTU: warpMTU},
		OutboundTLSOptionsContainer: option.OutboundTLSOptionsContainer{TLS: &option.OutboundTLSOptions{
			Enabled:                    true,
			ServerName:                 serverName,
			ClientCertificate:          badoption.Listable[string]{certificate},
			ClientKey:                  badoption.Listable[string]{keyPEM},
			CertificatePublicKeySHA256: badoption.Listable[[]byte]{pin[:]},
		}},
		Path: "/",
		// MASQUE refuses dials until its tunnel is up and tells no one when; on demand they wait for it instead.
		OnDemand: true,
	}
	switch w.HTTP {
	case WARPHTTP3:
		options.Version, options.DisableVersionFallback = 3, true
	case WARPHTTP2:
		options.Version = 2
	}
	return option.Endpoint{Type: C.TypeMASQUEClient, Tag: TagWARP, Options: options}, nil
}

// selfSigned is the certificate MASQUE shows Cloudflare: Cloudflare knows the device by its key, not by a CA.
func selfSigned(key *ecdsa.PrivateKey) (certificate, keyPEM string, err error) {
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "WARP"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(1, 0, 0),
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return "", "", err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return "", "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})), nil
}
