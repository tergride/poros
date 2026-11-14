package profile

import (
	"encoding/json"
	"net/netip"
	"slices"
	"strings"
)

// Security levels of a profile, as Security tells them.
const (
	SecurityOK   = ""
	SecurityWeak = "weak"
	SecurityNone = "none"
)

// Shadowsocks ciphers without authentication: anyone on the way can change the traffic.
var weakShadowsocksMethods = []string{"aes-128-ctr", "aes-192-ctr", "aes-256-ctr", "aes-128-cfb", "aes-192-cfb", "aes-256-cfb",
	"rc4-md5", "chacha20-ietf", "xchacha20"}

// Security tells how well a server protects the traffic on its way: SecurityNone is plain text to anyone on the way,
// SecurityWeak an old cipher or TLS that checks no certificate. A server on a private address, such as one at home
// or in a lab, is never insecure: its traffic does not cross the internet.
func (p Profile) Security() string {
	if addr, err := netip.ParseAddr(p.Server); err == nil && (addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast()) {
		return SecurityOK
	}
	switch p.Type {
	case TypeVLESS:
		if !p.VLESS.TLS.Enabled && p.VLESS.Encryption == "" {
			return SecurityNone
		}
		return tlsSecurity(p.VLESS.TLS)
	case TypeVMess:
		if !p.VMess.TLS.Enabled && (p.VMess.Security == "none" || p.VMess.Security == "zero") {
			return SecurityNone
		}
		if p.VMess.AlterID > 0 {
			return SecurityWeak
		}
		return tlsSecurity(p.VMess.TLS)
	case TypeTrojan:
		if !p.Trojan.TLS.Enabled {
			return SecurityNone
		}
		return tlsSecurity(p.Trojan.TLS)
	case TypeShadowsocks:
		switch {
		case p.Shadowsocks.Method == "none":
			return SecurityNone
		case slices.Contains(weakShadowsocksMethods, p.Shadowsocks.Method):
			return SecurityWeak
		}
	case TypeSOCKS:
		return SecurityNone
	case TypeHTTP:
		if !p.HTTP.TLS.Enabled {
			return SecurityNone
		}
		return tlsSecurity(p.HTTP.TLS)
	case TypeHysteria2:
		return tlsSecurity(p.Hysteria2.TLS)
	case TypeHysteria:
		return tlsSecurity(p.Hysteria.TLS)
	case TypeTUIC:
		return tlsSecurity(p.TUIC.TLS)
	case TypeAnyTLS:
		return tlsSecurity(p.AnyTLS.TLS)
	case TypeMASQUE:
		return tlsSecurity(p.MASQUE.TLS)
	case TypeOpenConnect:
		if p.OpenConnect.Insecure && len(p.OpenConnect.PeerFingerprint) == 0 {
			return SecurityWeak
		}
	}
	return SecurityOK
}

// tlsSecurity is weak for TLS that checks no certificate and pins none: whoever is on the way can pose as the server.
func tlsSecurity(t TLS) string {
	if t.Insecure && len(t.CertificateSHA256) == 0 && len(t.CertificatePublicKeySHA256) == 0 && !t.Reality.Enabled {
		return SecurityWeak
	}
	return SecurityOK
}

// WithAddress dials the server at another address, such as an IP of a CDN in front of it. The server keeps its own
// name where TLS and HTTP need it: as the TLS server name and as the Host of a WebSocket or HTTP transport.
func (p Profile) WithAddress(address string) Profile {
	address = strings.TrimSpace(address)
	if address == "" || address == p.Server || p.Server == "" {
		return p
	}
	var clone Profile
	data, _ := json.Marshal(p)
	if json.Unmarshal(data, &clone) != nil {
		return p
	}
	name := p.Server
	clone.Server = address
	keepName := func(t *TLS, transport *Transport) {
		if t != nil && t.Enabled && t.ServerName == "" && !t.DisableSNI {
			t.ServerName = name
		}
		if transport != nil && transport.Host == "" {
			switch transport.Type {
			case TransportWebSocket, TransportHTTPUpgrade, TransportHTTP, TransportXHTTP:
				transport.Host = name
			}
		}
	}
	switch {
	case clone.VLESS != nil:
		keepName(&clone.VLESS.TLS, &clone.VLESS.Transport)
	case clone.VMess != nil:
		keepName(&clone.VMess.TLS, &clone.VMess.Transport)
	case clone.Trojan != nil:
		keepName(&clone.Trojan.TLS, &clone.Trojan.Transport)
	case clone.Hysteria2 != nil:
		keepName(&clone.Hysteria2.TLS, nil)
	case clone.Hysteria != nil:
		keepName(&clone.Hysteria.TLS, nil)
	case clone.TUIC != nil:
		keepName(&clone.TUIC.TLS, nil)
	case clone.HTTP != nil:
		keepName(&clone.HTTP.TLS, nil)
	case clone.AnyTLS != nil:
		keepName(&clone.AnyTLS.TLS, nil)
	case clone.Naive != nil:
		keepName(&clone.Naive.TLS, nil)
	case clone.MASQUE != nil:
		keepName(&clone.MASQUE.TLS, nil)
	}
	return clone
}
