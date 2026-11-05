package profile

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"time"
)

// shareLink is a share link split as QUrl split it for the Qt client. net/url is not used: it refuses a Hysteria2
// port range and a stray % in a name, and its query parser turns '+' into a space, which QUrl never did.
type shareLink struct {
	scheme   string
	user     string
	password string
	host     string
	// port is the text after the host as written; Hysteria2 puts port ranges there.
	port  string
	query linkQuery
	name  string
}

func parseShareLink(link string) (shareLink, error) {
	var l shareLink
	link, fragment, hasFragment := strings.Cut(link, "#")
	if hasFragment {
		l.name = unescapeLinkText(fragment)
	}
	scheme, rest, ok := strings.Cut(link, "://")
	if !ok {
		return l, ErrNotLink
	}
	l.scheme = strings.ToLower(scheme)
	authority := rest
	rest = ""
	if i := strings.IndexAny(authority, "/?"); i >= 0 {
		authority, rest = authority[:i], authority[i:]
	}
	_, rawQuery, _ := strings.Cut(rest, "?")
	l.query = parseLinkQuery(rawQuery)

	hostPort := authority
	if at := strings.LastIndexByte(authority, '@'); at >= 0 {
		user, password, _ := strings.Cut(authority[:at], ":")
		l.user, l.password = unescapeLinkText(user), unescapeLinkText(password)
		hostPort = authority[at+1:]
	}
	var err error
	l.host, l.port, err = splitLinkHostPort(hostPort)
	return l, err
}

// userinfo is the whole decoded userinfo, for passwords that may contain a ':'.
func (l shareLink) userinfo() string {
	if l.password == "" {
		return l.user
	}
	return l.user + ":" + l.password
}

// serverPort reads a single port; a missing one, or 0, takes the protocol's default as in the Qt client.
func (l shareLink) serverPort(fallback int) (int, error) {
	if l.port == "" {
		return fallback, nil
	}
	port, ok := linkPortNumber(l.port)
	if !ok {
		return 0, fmt.Errorf("the port %q is not a number from 1 to 65535", l.port)
	}
	if port == 0 {
		return fallback, nil
	}
	return port, nil
}

func linkPortNumber(text string) (int, bool) {
	if text == "" || strings.ContainsFunc(text, func(r rune) bool { return r < '0' || r > '9' }) {
		return 0, false
	}
	port, err := strconv.Atoi(text)
	return port, err == nil && port <= 65535
}

func splitLinkHostPort(hostPort string) (host, port string, err error) {
	if rest, ok := strings.CutPrefix(hostPort, "["); ok {
		literal, after, closed := strings.Cut(rest, "]")
		if !closed {
			return "", "", errors.New("the IPv6 address has no closing bracket")
		}
		if after != "" {
			if port, ok = strings.CutPrefix(after, ":"); !ok {
				return "", "", fmt.Errorf("unexpected %q after the IPv6 address", after)
			}
		}
		addr, err := netip.ParseAddr(unescapeLinkText(literal))
		if err != nil || !addr.Is6() {
			return "", "", fmt.Errorf("%q is not an IPv6 address", literal)
		}
		return addr.String(), port, nil
	}
	if strings.Count(hostPort, ":") > 1 {
		return "", "", errors.New("an IPv6 address must be written in brackets")
	}
	host, port, _ = strings.Cut(hostPort, ":")
	host, err = normalizeLinkHost(unescapeLinkText(host))
	return host, port, err
}

// normalizeLinkHost lower-cases a host name, as QUrl did, and writes an IP address in its canonical form.
func normalizeLinkHost(host string) (string, error) {
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr.String(), nil
	}
	if strings.ContainsFunc(host, func(r rune) bool { return r <= ' ' || r == 0x7f || strings.ContainsRune(`/\?#@[]%"<>`, r) }) {
		return "", fmt.Errorf("%q is not a host name", host)
	}
	return strings.ToLower(host), nil
}

// unescapeLinkText decodes %XX escapes and keeps a stray '%' as written, as QUrl did; '+' stays '+'.
func unescapeLinkText(text string) string {
	if !strings.Contains(text, "%") {
		return text
	}
	out := make([]byte, 0, len(text))
	for i := 0; i < len(text); i++ {
		if text[i] == '%' && i+2 < len(text) {
			if value, err := strconv.ParseUint(text[i+1:i+3], 16, 8); err == nil {
				out = append(out, byte(value))
				i += 2
				continue
			}
		}
		out = append(out, text[i])
	}
	return string(out)
}

// linkQuery keeps the first value of each key, as QUrlQuery returned it.
type linkQuery struct {
	values map[string]string
	raw    map[string]string
}

func parseLinkQuery(rawQuery string) linkQuery {
	q := linkQuery{values: map[string]string{}, raw: map[string]string{}}
	for item := range strings.SplitSeq(rawQuery, "&") {
		if item == "" {
			continue
		}
		key, value, _ := strings.Cut(item, "=")
		key = unescapeLinkText(key)
		if _, seen := q.values[key]; seen {
			continue
		}
		q.values[key] = unescapeLinkText(value)
		q.raw[key] = value
	}
	return q
}

func (q linkQuery) has(key string) bool {
	_, ok := q.values[key]
	return ok
}

func (q linkQuery) get(key string) string {
	return q.values[key]
}

func (q linkQuery) trimmed(key string) string {
	return strings.TrimSpace(q.values[key])
}

// formValue reads a value from urlencode()-style panels, which write '+' for a space.
func (q linkQuery) formValue(key string) string {
	return unescapeLinkText(strings.ReplaceAll(q.raw[key], "+", "%20"))
}

func (q linkQuery) set(key, value string) {
	q.values[key] = value
	q.raw[key] = value
}

// merge lets every key of other replace this query's value.
func (q linkQuery) merge(other linkQuery) {
	for key, value := range other.values {
		q.values[key] = value
		q.raw[key] = other.raw[key]
	}
}

// linkFlag is a share link boolean: Qt read "true", and "1" for most keys.
func linkFlag(value string) bool {
	return value == "true" || value == "1"
}

// linkInt reads a whole number within [low, high]; anything else is dropped, leaving the default.
func linkInt(q linkQuery, key string, low, high int) (int, bool) {
	if !q.has(key) {
		return 0, false
	}
	value, err := strconv.Atoi(q.trimmed(key))
	if err != nil || value < low || value > high {
		return 0, false
	}
	return value, true
}

// linkDuration reads a Go duration such as 30s; anything else is dropped.
func linkDuration(q linkQuery, key string) string {
	return goDuration(q.get(key))
}

// goDuration keeps a Go duration such as 30s; anything else is dropped.
func goDuration(value string) string {
	value = strings.TrimSpace(value)
	if d, err := time.ParseDuration(value); err != nil || d < 0 {
		return ""
	}
	return value
}

// linkList splits a comma list, trimmed and without empty items: an empty ALPN entry makes Go abort every handshake.
func linkList(value string) []string {
	var items []string
	for item := range strings.SplitSeq(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			items = append(items, item)
		}
	}
	return items
}

// linkHeaders reads alternating names and values; an odd count drops them all, an empty name drops its pair.
func linkHeaders(text, separator string) map[string]string {
	if text == "" {
		return nil
	}
	parts := strings.Split(text, separator)
	if len(parts)%2 != 0 {
		return nil
	}
	headers := map[string]string{}
	for i := 0; i < len(parts); i += 2 {
		if strings.TrimSpace(parts[i]) != "" {
			headers[parts[i]] = parts[i+1]
		}
	}
	if len(headers) == 0 {
		return nil
	}
	return headers
}

// decodeLinkBase64 reads base64 in either alphabet, padded or not, as share links carry it.
func decodeLinkBase64(text string) ([]byte, bool) {
	text = strings.TrimRight(strings.TrimSpace(text), "=")
	if text == "" {
		return nil, false
	}
	for _, encoding := range []*base64.Encoding{base64.RawStdEncoding, base64.RawURLEncoding} {
		if decoded, err := encoding.DecodeString(text); err == nil {
			return decoded, true
		}
	}
	return nil, false
}

// linkPacketEncoding maps a packetEncoding value; Poros and Throne links write none for no encoding.
func linkPacketEncoding(value string) string {
	switch value = strings.TrimSpace(value); value {
	case "packetaddr", "xudp":
		return value
	}
	return ""
}

var linkFingerprints = []string{"chrome", "firefox", "edge", "safari", "360", "qq", "ios", "android", "random", "randomized"}

const echPEMBegin, echPEMEnd = "-----BEGIN ECH CONFIGS-----", "-----END ECH CONFIGS-----"

// linkTLS reads the TLS keys of a link. TLS is on when forced by the protocol, for security=tls or reality, or for
// an sni unless security turns it off. Off returns the zero TLS: the other keys mean nothing then.
func linkTLS(q linkQuery, forced bool) (TLS, error) {
	var t TLS
	security := strings.ToLower(q.trimmed("security"))
	off := security == "none" || security == "false" || security == "0"
	// An unknown security connects without TLS, as in the Qt client.
	t.Enabled = forced || security == "tls" || security == "true" || security == "1" || security == "reality"
	if q.has("sni") {
		t.ServerName = q.get("sni")
	}
	if q.has("peer") {
		t.ServerName = q.get("peer")
	}
	if q.has("server_name") {
		t.ServerName = q.get("server_name")
	}
	if !off && t.ServerName != "" {
		t.Enabled = true
	}
	// Without the key REALITY would silently become plain TLS.
	if security == "reality" && !q.has("pbk") {
		return TLS{}, errors.New("a REALITY link needs the server's public key, pbk")
	}
	if !t.Enabled {
		return TLS{}, nil
	}

	t.DisableSNI = linkFlag(q.get("disable_sni"))
	for _, key := range []string{"allowInsecure", "allow_insecure", "insecure"} {
		if q.has(key) {
			t.Insecure = linkFlag(q.get(key))
		}
	}
	t.ALPN = linkList(q.get("alpn"))
	for key, target := range map[string]*string{"tls_min_version": &t.MinVersion, "tls_max_version": &t.MaxVersion} {
		if version := q.trimmed(key); slices.Contains([]string{"1.0", "1.1", "1.2", "1.3"}, version) {
			*target = version
		}
	}
	t.CertificatePublicKeySHA256 = linkList(q.get("tls_certificate_public_key_sha256"))
	pins, err := linkCertificatePins(q.get("pcs") + "," + q.get("pinSHA256"))
	if err != nil {
		return TLS{}, err
	}
	t.CertificateSHA256 = pins

	if fingerprint := strings.ToLower(q.trimmed("fp")); slices.Contains(linkFingerprints, fingerprint) {
		t.UTLS = UTLS{Enabled: true, Fingerprint: fingerprint}
	}
	if q.has("pbk") {
		t.Reality = Reality{Enabled: true, PublicKey: linkRealityKey(q.get("pbk")), ShortID: q.trimmed("sid")}
		if spiderX := q.get("spx"); strings.TrimSpace(spiderX) == "" || strings.HasPrefix(strings.TrimSpace(spiderX), "/") {
			t.Reality.SpiderX = spiderX
		}
	}
	t.ECH = linkECH(q)
	return t, nil
}

// linkCertificatePins reads Xray's pcs and Hysteria2's pinSHA256: SHA-256 hashes of the whole certificate in hex,
// colons and dashes allowed, or in base64. A bad pin is an error, as these links usually also say insecure and
// would connect with no check at all. The official Hysteria2 client accepts a pin of any certificate in the
// chain, the cores only the server's own.
func linkCertificatePins(text string) ([]string, error) {
	var pins []string
	for _, item := range linkList(text) {
		var hash []byte
		if decoded, err := hex.DecodeString(strings.NewReplacer(":", "", "-", "").Replace(item)); err == nil && len(decoded) == sha256.Size {
			hash = decoded
		} else if decoded, ok := decodeLinkBase64(item); ok && len(decoded) == sha256.Size {
			hash = decoded
		} else {
			return nil, fmt.Errorf("the certificate pin %q is not a SHA-256 hash in hex or base64", item)
		}
		if pin := base64.StdEncoding.EncodeToString(hash); !slices.Contains(pins, pin) {
			pins = append(pins, pin)
		}
	}
	return pins, nil
}

// linkRealityKey drops the = padding some panels add to the 43-character key, which the cores refuse.
func linkRealityKey(raw string) string {
	key := strings.TrimSpace(raw)
	unpadded := strings.TrimRight(key, "=")
	if unpadded == key {
		return key
	}
	if decoded, err := base64.RawURLEncoding.DecodeString(unpadded); err == nil && len(decoded) == 32 {
		return unpadded
	}
	return key
}

// linkECH reads ech, which is 1, a base64 ECHConfigList or a DNS name to query ("name+https://resolver"), and
// Poros's own ech_* keys. The resolver has no field in the profile, so only the name is kept.
func linkECH(q linkQuery) ECH {
	var e ECH
	if q.has("ech") {
		value := q.trimmed("ech")
		e.Enabled = value != "" && value != "0" && value != "false"
		if e.Enabled && value != "1" && value != "true" {
			if scheme := strings.Index(value, "://"); scheme >= 0 {
				if plus := strings.IndexByte(value, '+'); plus > 0 && plus < scheme {
					e.QueryServerName = strings.TrimSpace(value[:plus])
				}
			} else if strings.Contains(value, ".") {
				e.QueryServerName = value
			} else {
				e.Config = linkECHConfig([]string{value})
			}
		}
	}
	if q.has("ech_enabled") {
		e.Enabled = q.get("ech_enabled") == "true"
	}
	if q.has("ech_config") {
		e.Config = linkECHConfig(linkList(q.get("ech_config")))
	}
	if q.has("ech_server_name") {
		e.QueryServerName = q.get("ech_server_name")
	}
	if !e.Enabled {
		return ECH{}
	}
	return e
}

// linkECHConfig wraps bare base64 in the PEM block sing-box reads.
func linkECHConfig(items []string) []string {
	for _, item := range items {
		if strings.Contains(item, "-----BEGIN") {
			return items
		}
	}
	joined := strings.TrimSpace(strings.Join(items, ""))
	if joined == "" {
		return nil
	}
	return []string{echPEMBegin, joined, echPEMEnd}
}

// quicLinkTLS drops what a QUIC protocol cannot use: sing-box has no uTLS or REALITY over QUIC.
func quicLinkTLS(t TLS) TLS {
	t.UTLS = UTLS{}
	t.Reality = Reality{}
	return t
}

var xhttpModes = []string{"auto", "packet-up", "stream-up", "stream-one"}

// The header Xray puts WebSocket early data in; sing-box sends it in the path unless a header is named.
const wsEarlyDataHeader = "Sec-WebSocket-Protocol"

// linkTransport reads the transport keys of a link: type, host, path, serviceName, headers, early data and XHTTP.
func linkTransport(q linkQuery) (Transport, error) {
	var t Transport
	switch network := strings.ToLower(q.trimmed("type")); network {
	case "", "tcp", "raw":
		// Xray's HTTP header obfuscation; Host and Path keep the comma lists it picks from.
		if q.get("headerType") == "http" {
			t.HeaderType = "http"
			t.Host, t.Path = q.get("host"), q.get("path")
		}
		return t, nil
	case "grpc":
		return Transport{Type: TransportGRPC, ServiceName: q.get("serviceName")}, nil
	case "quic":
		return Transport{Type: TransportQUIC}, nil
	case "ws", "websocket":
		t.Type = TransportWebSocket
	case "httpupgrade":
		t.Type = TransportHTTPUpgrade
	case "http", "h2":
		t.Type = TransportHTTP
	case "xhttp", "splithttp":
		t.Type = TransportXHTTP
	default:
		return Transport{}, unsupported("the transport %q is not supported", network)
	}
	t.Host, t.Path = q.get("host"), q.get("path")
	t.Headers = linkHeaders(q.get("headers"), "|")

	switch t.Type {
	case TransportWebSocket:
		if ed, ok := linkInt(q, "ed", 1, math.MaxInt32); ok {
			t.MaxEarlyData, t.EarlyDataHeaderName = ed, wsEarlyDataHeader
		}
		if value, ok := linkInt(q, "max_early_data", 0, math.MaxInt32); ok {
			t.MaxEarlyData = value
		}
		if q.has("early_data_header_name") {
			t.EarlyDataHeaderName = q.get("early_data_header_name")
		}
		if path, ed, ok := cutEarlyData(t.Path); ok {
			t.Path, t.MaxEarlyData, t.EarlyDataHeaderName = path, ed, wsEarlyDataHeader
		}
	case TransportHTTPUpgrade:
		// sing-box HTTPUpgrade has no early data, so a ?ed= only leaves the path.
		if path, _, ok := cutEarlyData(t.Path); ok {
			t.Path = path
		}
	case TransportHTTP:
		if method := q.trimmed("method"); method != "" && !strings.ContainsFunc(method, notHTTPTokenRune) {
			t.Method = method
		}
	case TransportXHTTP:
		t.XHTTP = linkXHTTP(q)
	}
	return t, nil
}

// cutEarlyData splits the "?ed=2048" v2rayN links append to a WebSocket path.
func cutEarlyData(path string) (string, int, bool) {
	before, ed, ok := strings.Cut(path, "?ed=")
	if !ok {
		return path, 0, false
	}
	value, err := strconv.Atoi(ed)
	if err != nil || value < 0 {
		return path, 0, false
	}
	return before, value, true
}

func notHTTPTokenRune(r rune) bool {
	return r >= 0x80 || !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", r))
}

func linkXHTTP(q linkQuery) XHTTP {
	x := XHTTP{Mode: "auto"}
	if mode := q.trimmed("mode"); slices.Contains(xhttpModes, mode) {
		x.Mode = mode
	}
	if q.has("extra") {
		if x.Extra = linkXHTTPExtra(q.get("extra")); x.Extra == nil {
			x.Extra = linkXHTTPExtra(q.formValue("extra"))
		}
	}
	return x
}

// linkXHTTPExtra keeps the extra object compacted; a Python dict repr ('key': True), which some panels write, is
// rewritten only when strict JSON fails. Anything but a non-empty object is dropped.
func linkXHTTPExtra(text string) json.RawMessage {
	python := strings.NewReplacer("'", `"`, "True", "true", "False", "false")
	for _, candidate := range []string{text, python.Replace(text)} {
		var object map[string]json.RawMessage
		if json.Unmarshal([]byte(candidate), &object) != nil || len(object) == 0 {
			continue
		}
		var compact bytes.Buffer
		if json.Compact(&compact, []byte(candidate)) == nil {
			return compact.Bytes()
		}
	}
	return nil
}

var multiplexProtocols = []string{"smux", "yamux", "h2mux"}

// linkMultiplex reads sing-box multiplex keys; off returns the zero value.
func linkMultiplex(q linkQuery) Multiplex {
	if !linkFlag(q.get("mux")) {
		return Multiplex{}
	}
	m := Multiplex{Enabled: true, Padding: linkFlag(q.get("mux_padding"))}
	if protocol := q.trimmed("mux_protocol"); slices.Contains(multiplexProtocols, protocol) {
		m.Protocol = protocol
	}
	m.MaxConnections, _ = linkInt(q, "mux_max_connections", 0, math.MaxInt32)
	m.MinStreams, _ = linkInt(q, "mux_min_streams", 0, math.MaxInt32)
	m.MaxStreams, _ = linkInt(q, "mux_max_streams", 0, math.MaxInt32)
	return m
}
