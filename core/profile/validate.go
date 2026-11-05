package profile

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
)

var shadowsocksMethods = []string{
	"2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm", "2022-blake3-chacha20-poly1305", "none",
	"aes-128-gcm", "aes-192-gcm", "aes-256-gcm", "chacha20-ietf-poly1305", "xchacha20-ietf-poly1305",
	"aes-128-ctr", "aes-192-ctr", "aes-256-ctr", "aes-128-cfb", "aes-192-cfb", "aes-256-cfb",
	"rc4-md5", "chacha20-ietf", "xchacha20",
}

var shadowsocksPlugins = []string{"", "obfs-local", "v2ray-plugin"}

// sing-quic refuses to hop ports more often.
const minHopInterval = 5 * time.Second

// Validate checks what every core needs to start the profile: its type and part, the server, the credentials and
// TLS settings a core would start with.
func (p Profile) Validate() error {
	if err := p.validatePart(); err != nil {
		return err
	}
	if strings.TrimSpace(p.Server) == "" {
		return errors.New("the server address is missing")
	}
	if p.ServerPort < 1 || p.ServerPort > 65535 {
		return fmt.Errorf("the server port %d is not from 1 to 65535", p.ServerPort)
	}
	switch p.Type {
	case TypeVLESS:
		if err := requiredField(p.VLESS.UUID, "the VLESS UUID"); err != nil {
			return err
		}
		if err := p.VLESS.TLS.validate(p.VLESS.Transport.Type); err != nil {
			return err
		}
		if p.VLESS.NeedsXray() {
			if t := p.VLESS.Transport.Type; t == TransportHTTP || t == TransportQUIC {
				return unsupported("Xray no longer runs the %s transport", t)
			}
			return p.VLESS.TLS.validateXray()
		}
		return nil
	case TypeVMess:
		if err := requiredField(p.VMess.UUID, "the VMess UUID"); err != nil {
			return err
		}
		if err := p.VMess.Transport.validateSingBox(p.VMess.TLS.Enabled); err != nil {
			return err
		}
		return p.VMess.TLS.validate(p.VMess.Transport.Type)
	case TypeTrojan:
		if err := requiredField(p.Trojan.Password, "the Trojan password"); err != nil {
			return err
		}
		if err := p.Trojan.Transport.validateSingBox(p.Trojan.TLS.Enabled); err != nil {
			return err
		}
		return p.Trojan.TLS.validate(p.Trojan.Transport.Type)
	case TypeShadowsocks:
		return p.Shadowsocks.validate()
	case TypeHysteria2:
		return p.Hysteria2.validate()
	case TypeTUIC:
		if !isUUID(strings.TrimSpace(p.TUIC.UUID)) {
			// The value stays out of the message: a TUIC v4 server puts its token there.
			return unsupported("the TUIC UUID is not a UUID; only TUIC v5 servers work")
		}
		return p.TUIC.TLS.validate("")
	case TypeHTTP:
		return p.HTTP.TLS.validate("")
	case TypeAnyTLS:
		if err := requiredField(p.AnyTLS.Password, "the AnyTLS password"); err != nil {
			return err
		}
		return p.AnyTLS.TLS.validate("")
	}
	return nil
}

// validate refuses TLS settings a core would not start with, or would start and then fail every connection on.
// transport is the transport type TLS runs over.
func (t TLS) validate(transport string) error {
	if transport == TransportQUIC {
		if !t.Enabled {
			return errors.New("the QUIC transport needs TLS")
		}
		if t.Reality.Enabled {
			return errors.New("REALITY does not work over the QUIC transport")
		}
	}
	if t.MinVersion != "" && t.MaxVersion != "" && t.MinVersion > t.MaxVersion {
		return errors.New("the minimum TLS version is above the maximum")
	}
	if !t.Reality.Enabled {
		return nil
	}
	if key, err := base64.RawURLEncoding.DecodeString(t.Reality.PublicKey); err != nil || len(key) != 32 {
		return errors.New("the REALITY public key is not 32 bytes in URL-safe base64")
	}
	// sing-box decodes the short ID into 8 bytes and panics on a longer one.
	if _, err := hex.DecodeString(t.Reality.ShortID); err != nil || len(t.Reality.ShortID) > 16 {
		return errors.New("the REALITY short ID must be up to 16 hex digits, an even number of them")
	}
	if t.ECH.Enabled {
		return errors.New("REALITY cannot be combined with ECH")
	}
	if t.DisableSNI && !t.Insecure {
		return errors.New("REALITY cannot run with disable_sni")
	}
	return nil
}

// NeedsXray tells whether only Xray runs the profile: VLESS encryption, XHTTP, or Xray's HTTP header obfuscation
// under TLS, which sing-box would send as HTTP/2.
func (v *VLESS) NeedsXray() bool {
	return v.Encryption != "" || v.Transport.Type == TransportXHTTP || (v.Transport.HeaderType == "http" && v.TLS.Enabled)
}

// validateSingBox refuses the Xray transports of a protocol only sing-box runs.
func (t Transport) validateSingBox(tls bool) error {
	if t.Type == TransportXHTTP {
		return unsupported("XHTTP works only with VLESS")
	}
	if t.HeaderType == "http" && tls {
		return unsupported("HTTP header obfuscation under TLS works only with VLESS")
	}
	return nil
}

// validateXray refuses TLS settings Xray has no way to express.
func (t TLS) validateXray() error {
	if !t.Enabled || t.Reality.Enabled {
		return nil
	}
	if t.Insecure && len(t.CertificateSHA256) == 0 {
		return unsupported("Xray no longer skips the certificate check; the server needs a certificate pin")
	}
	if len(t.CertificatePublicKeySHA256) > 0 {
		return unsupported("Xray pins only the certificate, not the server key")
	}
	if t.DisableSNI {
		return unsupported("Xray always sends the server name")
	}
	if t.ECH.Enabled && len(t.ECH.Config) == 0 {
		return unsupported("Xray needs the ECH config itself, not a DNS lookup for it")
	}
	return nil
}

func (h *Hysteria2) validate() error {
	// The password may be empty, as the Qt client allowed.
	for _, item := range h.ServerPorts {
		if ranges, err := hysteriaPortRanges(item); err != nil || len(ranges) != 1 || !strings.Contains(item, ":") {
			return fmt.Errorf("the port range %q is not written as 20000:30000", item)
		}
	}
	if interval, _ := time.ParseDuration(h.HopInterval); len(h.ServerPorts) > 0 && interval > 0 && interval < minHopInterval {
		return fmt.Errorf("port hopping needs at least %v between hops", minHopInterval)
	}
	return h.TLS.validate("")
}

func (p Profile) validatePart() error {
	parts := []struct {
		kind string
		set  bool
	}{
		{TypeVLESS, p.VLESS != nil}, {TypeVMess, p.VMess != nil}, {TypeTrojan, p.Trojan != nil},
		{TypeShadowsocks, p.Shadowsocks != nil}, {TypeHysteria2, p.Hysteria2 != nil}, {TypeTUIC, p.TUIC != nil},
		{TypeSOCKS, p.SOCKS != nil}, {TypeHTTP, p.HTTP != nil}, {TypeAnyTLS, p.AnyTLS != nil},
	}
	known := false
	for _, part := range parts {
		switch {
		case part.kind == p.Type && !part.set:
			return fmt.Errorf("the %s settings are missing", p.Type)
		case part.kind == p.Type:
			known = true
		case part.set:
			return fmt.Errorf("a %s server carries %s settings", p.Type, part.kind)
		}
	}
	if !known {
		return fmt.Errorf("unknown server type %q", p.Type)
	}
	return nil
}

func requiredField(value, what string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is missing", what)
	}
	return nil
}

func (s *Shadowsocks) validate() error {
	if s.Method == "" {
		return errors.New("the Shadowsocks method is missing")
	}
	if !slices.Contains(shadowsocksMethods, s.Method) {
		return unsupported("unknown Shadowsocks method %q", s.Method)
	}
	if !slices.Contains(shadowsocksPlugins, s.Plugin) {
		return unsupported("the Shadowsocks plugin %q is not supported; use obfs-local or v2ray-plugin", s.Plugin)
	}
	keySize := 32
	switch s.Method {
	case "none":
		return nil
	case "2022-blake3-aes-128-gcm":
		keySize = 16
	case "2022-blake3-aes-256-gcm", "2022-blake3-chacha20-poly1305":
	default:
		if s.Password == "" {
			return errors.New("the Shadowsocks password is missing")
		}
		return nil
	}
	// 2022 methods take base64 keys of the cipher's size; AES ones may join several with ':' for multiple users.
	keys := strings.Split(s.Password, ":")
	if s.Method == "2022-blake3-chacha20-poly1305" && len(keys) > 1 {
		return errors.New("2022-blake3-chacha20-poly1305 takes a single key")
	}
	for _, key := range keys {
		decoded, err := base64.StdEncoding.DecodeString(key)
		if err != nil || len(decoded) != keySize || strings.ContainsFunc(key, unicode.IsSpace) {
			return fmt.Errorf("a %s key must be %d bytes in base64", s.Method, keySize)
		}
	}
	return nil
}

// isUUID accepts the forms sing-box's UUID parser (gofrs) does: canonical, 32 hex digits, braced or urn:uuid:.
func isUUID(text string) bool {
	switch len(text) {
	case 32, 36:
	case 34, 38:
		if text[0] != '{' || text[len(text)-1] != '}' {
			return false
		}
		text = text[1 : len(text)-1]
	case 41, 45:
		var ok bool
		if text, ok = strings.CutPrefix(text, "urn:uuid:"); !ok {
			return false
		}
	default:
		return false
	}
	for i := 0; i < len(text); i++ {
		dash := len(text) == 36 && (i == 8 || i == 13 || i == 18 || i == 23)
		if c := text[i]; dash && c != '-' || !dash && !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}
