package profile

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
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
	// The compound profiles have no address of their own.
	switch p.Type {
	case TypeChain:
		return p.Chain.validate()
	case TypeCustom:
		return p.Custom.validate()
	case TypeDirect:
		return nil
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
	case TypeWireGuard:
		return p.WireGuard.validate()
	case TypeHysteria:
		return p.Hysteria.validate()
	case TypeNaive:
		return p.Naive.validate()
	case TypeSSH:
		return p.SSH.validate()
	case TypeSnell:
		return p.Snell.validate()
	case TypeOpenVPN:
		return p.OpenVPN.validate()
	case TypeOpenConnect:
		return p.OpenConnect.validate()
	case TypeMASQUE:
		return p.MASQUE.validate()
	}
	return nil
}

var openVPNNetworks = []string{"", "udp", "udp4", "udp6", "tcp", "tcp4", "tcp6"}

// validate checks the choices against what sing-box's OpenVPN takes and that the keys are PEM blocks.
func (o *OpenVPN) validate() error {
	choices := []struct {
		value, what string
		allowed     []string
	}{
		{o.Network, "network", openVPNNetworks},
		{o.ControlWrap.Type, "control channel protection", []string{"", "tls_auth", "tls_crypt", "tls_crypt_v2"}},
		{o.ControlWrap.Direction, "tls-auth direction", []string{"", "server", "client"}},
		{o.VerifyNameType, "verify-x509-name type", []string{"", "subject", "name", "name-prefix"}},
		{o.RemoteCertTLS, "remote-cert-tls", []string{"", "server", "client", "none"}},
		{o.TLSVersionMin, "minimum TLS version", []string{"", "1.0", "1.1", "1.2", "1.3"}},
		{o.TLSVersionMax, "maximum TLS version", []string{"", "1.0", "1.1", "1.2", "1.3"}},
		{o.Compression, "compression", []string{"", "none", "no", "lz4", "lz4-v2", "stub", "stub-v2", "disabled", "off"}},
		{o.CompressionLZO, "LZO compression", []string{"", "none", "no", "yes", "adaptive", "asym", "disabled", "off"}},
		{o.AllowCompression, "allow-compression", []string{"", "no", "asym", "yes"}},
	}
	for _, c := range choices {
		if !slices.Contains(c.allowed, c.value) {
			return unsupported("OpenVPN's %s %q is not supported", c.what, c.value)
		}
	}
	for _, r := range o.Remotes {
		if strings.TrimSpace(r.Server) == "" || r.Port < 1 || r.Port > 65535 || !slices.Contains(openVPNNetworks, r.Network) {
			return fmt.Errorf("the OpenVPN remote %q is not a server with a port from 1 to 65535", r.Server)
		}
	}
	for _, d := range []string{o.PingInterval, o.PingRestart, o.RenegotiateInterval} {
		if d != "" && goDuration(d) == "" {
			return fmt.Errorf("%q is not a duration such as 10s", d)
		}
	}
	pems := []struct{ text, marker, what string }{
		{o.CA, "CERTIFICATE-----", "the OpenVPN CA"},
		{o.Cert, "CERTIFICATE-----", "the OpenVPN client certificate"},
		{o.Key, "PRIVATE KEY-----", "the OpenVPN client key"},
		{o.ControlWrap.Key, "-----BEGIN OpenVPN", "the key of " + strings.ReplaceAll(o.ControlWrap.Type, "_", "-")},
	}
	for _, p := range pems {
		if p.text != "" && !strings.Contains(p.text, p.marker) {
			return fmt.Errorf("%s is not a PEM block", p.what)
		}
	}
	switch {
	case (o.Cert == "") != (o.Key == ""):
		return errors.New("the OpenVPN client certificate and its key go together")
	case o.ControlWrap.Type != "" && o.ControlWrap.Key == "":
		return errors.New("the key of the control channel protection is missing")
	case o.CA == "" && len(o.PeerFingerprint) == 0:
		return errors.New("OpenVPN needs the CA of the server's certificate or its fingerprint")
	}
	return nil
}

var openConnectFlavors = []string{"", "anyconnect", "gp", "fortinet", "f5", "pulse", "nc"}

func (o *OpenConnect) validate() error {
	switch {
	case !slices.Contains(openConnectFlavors, o.Flavor):
		return unsupported("the OpenConnect gateway %q is not supported", o.Flavor)
	case o.Path != "" && !strings.HasPrefix(o.Path, "/"):
		return errors.New("the gateway's path starts with /")
	case (o.Cert == "") != (o.Key == ""):
		return errors.New("the OpenConnect client certificate and its key go together")
	case o.CA != "" && !strings.Contains(o.CA, "CERTIFICATE-----"):
		return errors.New("the OpenConnect CA is not a PEM block")
	case o.MTU != 0 && (o.MTU < 576 || o.MTU > 65535):
		return fmt.Errorf("the MTU %d is not from 576 to 65535", o.MTU)
	}
	return nil
}

func (m *MASQUE) validate() error {
	switch {
	case !m.TLS.Enabled:
		return errors.New("MASQUE needs TLS")
	case m.Version < 0 || m.Version > 3:
		return unsupported("MASQUE over HTTP/%d is not supported", m.Version)
	case m.Path != "" && !strings.HasPrefix(m.Path, "/"):
		return errors.New("the MASQUE path starts with /")
	case m.MTU != 0 && (m.MTU < 576 || m.MTU > 65535):
		return fmt.Errorf("the MTU %d is not from 576 to 65535", m.MTU)
	}
	return m.TLS.validate("")
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
	if err := validatePortHopping(h.ServerPorts, h.HopInterval); err != nil {
		return err
	}
	if err := h.Realm.validate(len(h.ServerPorts) > 0); err != nil {
		return err
	}
	return h.TLS.validate("")
}

func validatePortHopping(ports []string, hopInterval string) error {
	for _, item := range ports {
		if ranges, err := hysteriaPortRanges(item); err != nil || len(ranges) != 1 || !strings.Contains(item, ":") {
			return fmt.Errorf("the port range %q is not written as 20000:30000", item)
		}
	}
	if interval, _ := time.ParseDuration(hopInterval); len(ports) > 0 && interval > 0 && interval < minHopInterval {
		return fmt.Errorf("port hopping needs at least %v between hops", minHopInterval)
	}
	return nil
}

// On reports a realm in use: it takes the place of the server's address.
func (r HysteriaRealm) On() bool { return r.ServerURL != "" || r.RealmID != "" }

func (r HysteriaRealm) validate(hopping bool) error {
	if !r.On() {
		return nil
	}
	if u, err := url.Parse(r.ServerURL); err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return errors.New("the realm service must be an https:// address")
	}
	if strings.TrimSpace(r.RealmID) == "" {
		return errors.New("the realm ID is missing")
	}
	if hopping {
		return errors.New("a realm cannot be combined with port hopping")
	}
	return nil
}

func (h *Hysteria) validate() error {
	if h.UpMbps <= 0 || h.DownMbps <= 0 {
		return errors.New("Hysteria v1 needs the upload and download speeds")
	}
	if err := validatePortHopping(h.ServerPorts, h.HopInterval); err != nil {
		return err
	}
	return h.TLS.validate("")
}

func (w *WireGuard) validate() error {
	keys := []struct {
		key, what string
		optional  bool
	}{{w.PrivateKey, "private key", false}, {w.PeerPublicKey, "peer's public key", false}, {w.PreSharedKey, "pre-shared key", true}}
	for _, k := range keys {
		if k.key == "" && k.optional {
			continue
		}
		if decoded, err := base64.StdEncoding.DecodeString(k.key); err != nil || len(decoded) != 32 {
			return fmt.Errorf("the WireGuard %s must be 32 bytes in base64", k.what)
		}
	}
	if len(w.Address) == 0 {
		return errors.New("the WireGuard tunnel needs its own address, such as 172.16.0.2/32")
	}
	for _, address := range w.Address {
		if _, err := netip.ParsePrefix(address); err != nil {
			return fmt.Errorf("the WireGuard address %q is not written as 172.16.0.2/32", address)
		}
	}
	if len(w.Reserved) != 0 && len(w.Reserved) != 3 {
		return errors.New("WireGuard's reserved bytes are three numbers")
	}
	for _, b := range w.Reserved {
		if b < 0 || b > 255 {
			return errors.New("WireGuard's reserved bytes are numbers from 0 to 255")
		}
	}
	if w.MTU != 0 && (w.MTU < 576 || w.MTU > 65535) {
		return fmt.Errorf("the MTU %d is not from 576 to 65535", w.MTU)
	}
	if w.PersistentKeepalive < 0 || w.PersistentKeepalive > 65535 {
		return errors.New("the keepalive interval is not from 0 to 65535 seconds")
	}
	return nil
}

var naiveCongestionControls = []string{"", "bbr", "bbr2", "cubic", "reno"}

// validate refuses the TLS settings Chromium's stack has no room for: it checks the certificate as a browser does.
func (n *Naive) validate() error {
	t := n.TLS
	switch {
	case !t.Enabled:
		return errors.New("NaiveProxy needs TLS")
	case t.Insecure || len(t.CertificateSHA256) > 0 || len(t.CertificatePublicKeySHA256) > 0:
		return unsupported("NaiveProxy checks the server certificate as a browser does: it cannot skip the check or pin a certificate")
	case t.DisableSNI || len(t.ALPN) > 0 || t.MinVersion != "" || t.MaxVersion != "" || t.UTLS.Enabled || t.Reality.Enabled:
		return unsupported("NaiveProxy takes only the server name and ECH of the TLS settings")
	case !slices.Contains(naiveCongestionControls, n.QUICCongestionControl):
		return unsupported("unknown QUIC congestion control %q", n.QUICCongestionControl)
	case n.InsecureConcurrency < 0:
		return errors.New("the number of connections cannot be negative")
	}
	return nil
}

func (s *SSH) validate() error {
	if key := strings.TrimSpace(s.PrivateKey); key != "" && !(strings.Contains(key, "-----BEGIN ") && strings.Contains(key, "PRIVATE KEY-----")) {
		return errors.New("the SSH private key must be the whole block from -----BEGIN to the PRIVATE KEY----- line")
	}
	for _, key := range s.HostKey {
		if len(strings.Fields(key)) < 2 {
			return fmt.Errorf("the host key %q is not written as ssh-ed25519 AAAA...", key)
		}
	}
	return nil
}

func (s *Snell) validate() error {
	switch s.Version {
	case 4:
		if !slices.Contains([]string{"", "none", "http", "tls"}, s.ObfsMode) {
			return unsupported("unknown Snell obfuscation %q", s.ObfsMode)
		}
		if s.Mode != "" {
			return errors.New("the Snell mode is a setting of version 6")
		}
	case 6:
		if !slices.Contains([]string{"", "default", "unshaped", "unsafe-raw"}, s.Mode) {
			return unsupported("unknown Snell mode %q", s.Mode)
		}
		if s.ObfsMode != "" || s.ObfsHost != "" {
			return errors.New("Snell obfuscation is a setting of version 4")
		}
	default:
		return unsupported("Snell v%d is not supported; sing-box runs versions 4 and 6", s.Version)
	}
	return requiredField(s.PSK, "the Snell PSK")
}

func (s ShadowTLS) validate(plugin string) error {
	switch {
	case s.Version == 0:
		return nil
	case s.Version < 1 || s.Version > 3:
		return unsupported("ShadowTLS v%d is not supported; versions 1 to 3 are", s.Version)
	case plugin != "":
		return errors.New("ShadowTLS cannot be combined with a Shadowsocks plugin")
	case s.Version > 1 && s.Password == "":
		return errors.New("the ShadowTLS password is missing")
	case strings.TrimSpace(s.TLS.ServerName) == "":
		return errors.New("ShadowTLS needs the site of its handshake as the server name")
	case s.TLS.Reality.Enabled || s.TLS.ECH.Enabled:
		return unsupported("ShadowTLS runs neither REALITY nor ECH")
	}
	return s.TLS.validate("")
}

func (p Profile) validatePart() error {
	parts := []struct {
		kind string
		set  bool
	}{
		{TypeVLESS, p.VLESS != nil}, {TypeVMess, p.VMess != nil}, {TypeTrojan, p.Trojan != nil},
		{TypeShadowsocks, p.Shadowsocks != nil}, {TypeHysteria2, p.Hysteria2 != nil}, {TypeTUIC, p.TUIC != nil},
		{TypeSOCKS, p.SOCKS != nil}, {TypeHTTP, p.HTTP != nil}, {TypeAnyTLS, p.AnyTLS != nil},
		{TypeWireGuard, p.WireGuard != nil}, {TypeHysteria, p.Hysteria != nil}, {TypeNaive, p.Naive != nil},
		{TypeSSH, p.SSH != nil}, {TypeSnell, p.Snell != nil},
		{TypeOpenVPN, p.OpenVPN != nil}, {TypeOpenConnect, p.OpenConnect != nil}, {TypeMASQUE, p.MASQUE != nil},
		{TypeChain, p.Chain != nil}, {TypeCustom, p.Custom != nil}, {TypeDirect, p.Direct != nil},
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
	if err := s.ShadowTLS.validate(s.Plugin); err != nil {
		return err
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

func (c *Chain) validate() error {
	if len(c.Servers) < 2 {
		return errors.New("a chain needs at least two servers")
	}
	seen := map[string]bool{}
	for _, id := range c.Servers {
		if strings.TrimSpace(id) == "" {
			return errors.New("a server of the chain is missing")
		}
		if seen[id] {
			return errors.New("a server comes twice in the chain")
		}
		seen[id] = true
	}
	return nil
}

// Outbounds that are not servers, which a custom profile cannot be.
var customInfraTypes = []string{"direct", "block", "dns", "selector", "urltest"}

// validate checks the shape of a custom outbound; whether sing-box takes it, its check tells when it is saved.
func (c *Custom) validate() error {
	var head struct {
		Type   any    `json:"type"`
		Detour string `json:"detour"`
	}
	if trimmed := bytes.TrimSpace(c.JSON); len(trimmed) == 0 || trimmed[0] != '{' || json.Unmarshal(c.JSON, &head) != nil {
		return errors.New("the custom outbound is not a JSON object")
	}
	kind, _ := head.Type.(string)
	switch {
	case strings.TrimSpace(kind) == "":
		return errors.New("the custom outbound has no type")
	case slices.Contains(customInfraTypes, kind):
		return fmt.Errorf("a %s outbound is no server", kind)
	case strings.TrimSpace(head.Detour) != "":
		return errors.New("Poros dials the custom outbound itself; to go through another server, make a chain")
	}
	return nil
}
