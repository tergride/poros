package profile

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"gopkg.in/yaml.v2"
)

// parseClashDocument reads the proxies list of a Clash or mihomo config. A proxy that cannot be read is skipped on
// its own; the error is for a body that cannot be read at all.
func parseClashDocument(text []byte, d *Document) error {
	text = clashSanitize(text)
	if err := clashCheckSize(text); err != nil {
		return err
	}
	var root clashRoot
	if err := clashDecode(text, &root); err != nil {
		var typeErr *yaml.TypeError
		// A type error is left only by a root that is not a map, which has no proxies list.
		if !errors.As(err, &typeErr) {
			return fmt.Errorf("the Clash config cannot be read: %s", clashErrorText(err))
		}
	}
	if len(root.Proxies) == 0 {
		if root.ProxyProviders {
			return errors.New("the Clash config only lists proxy-providers, which are not downloaded: add each provider's URL as a subscription instead")
		}
		return errors.New("the Clash config has no proxies list")
	}
	for i, entry := range root.Proxies {
		switch {
		case !entry.read:
			d.add(i+1, "", Profile{}, errors.New("the proxy is empty"))
		case entry.profile != nil:
			d.add(i+1, entry.name, *entry.profile, nil)
		case !entry.infra:
			d.add(i+1, entry.name, Profile{}, entry.err)
		}
	}
	return nil
}

// NUL, U+FEFF, CR LF, CR, and U+0085, U+2028 and U+2029, which yaml.v2 also ends a line at.
var clashBreaks = strings.NewReplacer("\x00", "", "\xef\xbb\xbf", "", "\r\n", "\n", "\r", "\n",
	"\xc2\x85", "\n", "\xe2\x80\xa8", "\n", "\xe2\x80\xa9", "\n")

// clashSanitize does what the Qt client did before parsing: invalid UTF-8 becomes U+FFFD, NUL bytes and byte order
// marks go. Every line break yaml.v2 knows becomes \n, so that clashCheckSize sees the lines yaml.v2 does.
func clashSanitize(text []byte) []byte {
	return []byte(clashBreaks.Replace(string(bytes.ToValidUTF8(text, []byte(string(utf8.RuneError))))))
}

// clashDecode runs yaml.v2, which can panic on input nobody has tried yet, as a plain error.
func clashDecode(text []byte, root *clashRoot) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	return yaml.Unmarshal(text, root)
}

// clashErrorText keeps yaml.v2's message but not a value it quotes in backticks, which may be a password.
func clashErrorText(err error) string {
	text := strings.TrimPrefix(err.Error(), "yaml: ")
	if i := strings.IndexByte(text, '`'); i >= 0 {
		text = strings.TrimSpace(text[:i])
	}
	return text
}

// clashAliasing tells yaml.v2's alias expansion limit, which a billion-laughs body hits: it ends the document, not
// the proxy it happens in.
func clashAliasing(err error) bool {
	return err != nil && strings.Contains(err.Error(), "excessive aliasing")
}

// The config is decoded into types of a fixed depth, never into interface{}: yaml.v2 then recurses only as deep as
// the types go, whatever aliases the body holds. Each type reads leniently and never fails the document.
type clashRoot struct {
	Proxies        clashProxyList `yaml:"proxies"`
	ProxyProviders clashPresence  `yaml:"proxy-providers"`
}

// clashProxyList is empty when proxies is not a list.
type clashProxyList []clashEntry

func (l *clashProxyList) UnmarshalYAML(unmarshal func(any) error) error {
	var entries []clashEntry
	err := unmarshal(&entries)
	var typeErr *yaml.TypeError
	if errors.As(err, &typeErr) {
		return nil
	}
	*l = entries
	return err
}

// clashPresence notes that a key is there without reading its value.
type clashPresence bool

func (p *clashPresence) UnmarshalYAML(func(any) error) error {
	*p = true
	return nil
}

// clashEntry is what one item of proxies became. Each is read as it is decoded, so that only the profiles stay in
// memory, not a clashProxy of several KB for each item. read is false for a null item, which yaml.v2 leaves without
// a call; infra is an item that is not a server, such as direct or a group.
type clashEntry struct {
	read    bool
	infra   bool
	name    string
	profile *Profile
	err     error
}

func (e *clashEntry) UnmarshalYAML(unmarshal func(any) error) error {
	e.read = true
	var scalar string
	var list []clashPresence
	if unmarshal(&scalar) == nil || unmarshal(&list) == nil {
		e.err = errors.New("the proxy is not a map of settings")
		return nil
	}
	var p clashProxy
	err := unmarshal(&p)
	e.name = strings.TrimSpace(string(p.Name))
	var typeErr *yaml.TypeError
	switch {
	case err == nil, errors.As(err, &typeErr):
		// A type error in a map is a key that is itself a list or a map: only that key is lost.
		profile, proxy, err := p.profile()
		e.infra, e.err = !proxy, err
		if proxy && err == nil {
			e.profile = &profile
		}
	case clashAliasing(err):
		return err
	default:
		e.err = fmt.Errorf("the proxy cannot be read: %s", clashErrorText(err))
	}
	return nil
}

// clashString is a scalar as written: yaml.v2 gives the text even of a number or a boolean to a string, so a
// password 0123 stays 0123. A list or a map leaves it empty.
type clashString string

func (s *clashString) UnmarshalYAML(unmarshal func(any) error) error {
	var text string
	err := unmarshal(&text)
	if err == nil {
		*s = clashString(text)
	}
	if clashAliasing(err) {
		return err
	}
	return nil
}

// clashInt is a whole number in int32 range, also written as a string or as 443.0, as the Qt client read it.
// Anything else is 0.
type clashInt int

func (n *clashInt) UnmarshalYAML(unmarshal func(any) error) error {
	var text clashString
	if err := unmarshal(&text); err != nil {
		return err
	}
	value := strings.TrimSpace(string(text))
	if v, err := strconv.ParseInt(value, 10, 32); err == nil {
		*n = clashInt(v)
	} else if f, err := strconv.ParseFloat(value, 64); err == nil && f == math.Trunc(f) && f >= math.MinInt32 && f <= math.MaxInt32 {
		*n = clashInt(f)
	} else {
		*n = 0
	}
	return nil
}

// clashBool is true only for true and 1, in any case: mihomo, on YAML 1.2, reads yes and on as strings, which
// yaml.v2 would take for true.
type clashBool bool

func (b *clashBool) UnmarshalYAML(unmarshal func(any) error) error {
	var text clashString
	if err := unmarshal(&text); err != nil {
		return err
	}
	value := strings.ToLower(strings.TrimSpace(string(text)))
	*b = clashBool(value == "true" || value == "1")
	return nil
}

// clashList is a list of scalars, or one scalar as a list of one, without empty items.
type clashList struct {
	items []string
	bad   bool
}

func (l *clashList) UnmarshalYAML(unmarshal func(any) error) error {
	var items []clashString
	err := unmarshal(&items)
	if err == nil {
		for _, item := range items {
			if text := strings.TrimSpace(string(item)); text != "" {
				l.items = append(l.items, text)
			}
		}
		return nil
	}
	if clashAliasing(err) {
		return err
	}
	var one string
	if err = unmarshal(&one); err == nil {
		if text := strings.TrimSpace(one); text != "" {
			l.items = []string{text}
		}
		return nil
	}
	if clashAliasing(err) {
		return err
	}
	l.bad = true
	return nil
}

func (l clashList) check(key string) error {
	if l.bad {
		return fmt.Errorf("%s must be a list", key)
	}
	return nil
}

// clashMap is a nested map of options; bad is a list or a scalar in its place.
type clashMap[T any] struct {
	v   T
	set bool
	bad bool
}

func (m *clashMap[T]) UnmarshalYAML(unmarshal func(any) error) error {
	m.set = true
	if err := unmarshal(&m.v); err != nil {
		if clashAliasing(err) {
			return err
		}
		m.bad = true
	}
	return nil
}

func (m clashMap[T]) check(key string) error {
	if m.bad {
		return fmt.Errorf("%s must be a map", key)
	}
	return nil
}

// clashProxy holds the mihomo keys Poros reads; others are not decoded at all.
type clashProxy struct {
	Name   clashString `yaml:"name"`
	Type   clashString `yaml:"type"`
	Server clashString `yaml:"server"`
	Port   clashInt    `yaml:"port"`

	UUID           clashString `yaml:"uuid"`
	Username       clashString `yaml:"username"`
	Password       clashString `yaml:"password"`
	Cipher         clashString `yaml:"cipher"`
	AlterID        clashInt    `yaml:"alterId"`
	Flow           clashString `yaml:"flow"`
	PacketEncoding clashString `yaml:"packet-encoding"`
	Encryption     clashString `yaml:"encryption"`

	TLS               clashBool                  `yaml:"tls"`
	ServerName        clashString                `yaml:"servername"`
	SNI               clashString                `yaml:"sni"`
	SkipCertVerify    clashBool                  `yaml:"skip-cert-verify"`
	DisableSNI        clashBool                  `yaml:"disable-sni"`
	ALPN              clashList                  `yaml:"alpn"`
	ClientFingerprint clashString                `yaml:"client-fingerprint"`
	Fingerprint       clashString                `yaml:"fingerprint"`
	RealityOpts       clashMap[clashRealityOpts] `yaml:"reality-opts"`
	ECHOpts           clashMap[clashECHOpts]     `yaml:"ech-opts"`

	Network   clashString              `yaml:"network"`
	WSOpts    clashMap[clashWSOpts]    `yaml:"ws-opts"`
	HTTPOpts  clashMap[clashHTTPOpts]  `yaml:"http-opts"`
	H2Opts    clashMap[clashH2Opts]    `yaml:"h2-opts"`
	GRPCOpts  clashMap[clashGRPCOpts]  `yaml:"grpc-opts"`
	XHTTPOpts clashMap[clashXHTTPOpts] `yaml:"xhttp-opts"`
	Smux      clashMap[clashSmux]      `yaml:"smux"`

	Plugin     clashString                 `yaml:"plugin"`
	PluginOpts clashMap[clashPluginOpts]   `yaml:"plugin-opts"`
	UDPOverTCP clashBool                   `yaml:"udp-over-tcp"`
	SSOpts     clashMap[clashTrojanSSOpts] `yaml:"ss-opts"`

	Ports        clashString `yaml:"ports"`
	Up           clashString `yaml:"up"`
	Down         clashString `yaml:"down"`
	Obfs         clashString `yaml:"obfs"`
	ObfsPassword clashString `yaml:"obfs-password"`
	HopInterval  clashString `yaml:"hop-interval"`

	IP                   clashString `yaml:"ip"`
	Token                clashString `yaml:"token"`
	CongestionController clashString `yaml:"congestion-controller"`
	UDPRelayMode         clashString `yaml:"udp-relay-mode"`
	ReduceRTT            clashBool   `yaml:"reduce-rtt"`
	HeartbeatInterval    clashInt    `yaml:"heartbeat-interval"`

	IdleSessionCheckInterval clashInt `yaml:"idle-session-check-interval"`
	IdleSessionTimeout       clashInt `yaml:"idle-session-timeout"`
	MinIdleSession           clashInt `yaml:"min-idle-session"`

	Headers clashMap[map[string]clashString] `yaml:"headers"`

	PrivateKey           clashString               `yaml:"private-key"`
	PublicKey            clashString               `yaml:"public-key"`
	PreSharedKey         clashString               `yaml:"pre-shared-key"`
	IPv6                 clashString               `yaml:"ipv6"`
	Reserved             clashList                 `yaml:"reserved"`
	MTU                  clashInt                  `yaml:"mtu"`
	PersistentKeepalive  clashInt                  `yaml:"persistent-keepalive"`
	Peers                []clashWireGuardPeer      `yaml:"peers"`
	AmneziaWG            clashPresence             `yaml:"amnezia-wg-option"`
	AuthStr              clashString               `yaml:"auth-str"`
	Auth                 clashString               `yaml:"auth"`
	Protocol             clashString               `yaml:"protocol"`
	PrivateKeyPassphrase clashString               `yaml:"private-key-passphrase"`
	HostKey              clashList                 `yaml:"host-key"`
	HostKeyAlgorithms    clashList                 `yaml:"host-key-algorithms"`
	PSK                  clashString               `yaml:"psk"`
	Version              clashInt                  `yaml:"version"`
	ObfsOpts             clashMap[clashPluginOpts] `yaml:"obfs-opts"`
}

// clashWireGuardPeer is mihomo's peer list; Poros keeps one peer.
type clashWireGuardPeer struct {
	Server       clashString `yaml:"server"`
	Port         clashInt    `yaml:"port"`
	PublicKey    clashString `yaml:"public-key"`
	PreSharedKey clashString `yaml:"pre-shared-key"`
	Reserved     clashList   `yaml:"reserved"`
}

type clashRealityOpts struct {
	PublicKey clashString `yaml:"public-key"`
	ShortID   clashString `yaml:"short-id"`
}

type clashECHOpts struct {
	Enable          clashBool   `yaml:"enable"`
	Config          clashString `yaml:"config"`
	QueryServerName clashString `yaml:"query-server-name"`
}

type clashWSOpts struct {
	Path                clashString                      `yaml:"path"`
	Headers             clashMap[map[string]clashString] `yaml:"headers"`
	MaxEarlyData        clashInt                         `yaml:"max-early-data"`
	EarlyDataHeaderName clashString                      `yaml:"early-data-header-name"`
	V2rayHTTPUpgrade    clashBool                        `yaml:"v2ray-http-upgrade"`
}

type clashHTTPOpts struct {
	Method  clashString                    `yaml:"method"`
	Path    clashList                      `yaml:"path"`
	Headers clashMap[map[string]clashList] `yaml:"headers"`
}

type clashH2Opts struct {
	Host clashList   `yaml:"host"`
	Path clashString `yaml:"path"`
}

type clashGRPCOpts struct {
	ServiceName clashString `yaml:"grpc-service-name"`
}

type clashSmux struct {
	Enabled        clashBool   `yaml:"enabled"`
	Protocol       clashString `yaml:"protocol"`
	MaxConnections clashInt    `yaml:"max-connections"`
	MinStreams     clashInt    `yaml:"min-streams"`
	MaxStreams     clashInt    `yaml:"max-streams"`
	Padding        clashBool   `yaml:"padding"`
}

// clashPluginOpts holds the keys of both plugins: obfs reads mode and host, v2ray-plugin all of them.
type clashPluginOpts struct {
	Mode     clashString `yaml:"mode"`
	Host     clashString `yaml:"host"`
	Path     clashString `yaml:"path"`
	TLS      clashBool   `yaml:"tls"`
	Mux      clashBool   `yaml:"mux"`
	Password clashString `yaml:"password"`
	Version  clashInt    `yaml:"version"`
}

// clashTrojanSSOpts is Trojan-Go's Shadowsocks layer inside Trojan.
type clashTrojanSSOpts struct {
	Enabled clashBool `yaml:"enabled"`
}

type clashXHTTPPadding struct {
	ObfsMode  clashBool   `yaml:"x-padding-obfs-mode"`
	Key       clashString `yaml:"x-padding-key"`
	Header    clashString `yaml:"x-padding-header"`
	Placement clashString `yaml:"x-padding-placement"`
	Method    clashString `yaml:"x-padding-method"`
}

type clashXHTTPReuse struct {
	MaxConcurrency   clashString `yaml:"max-concurrency"`
	MaxConnections   clashString `yaml:"max-connections"`
	CMaxReuseTimes   clashString `yaml:"c-max-reuse-times"`
	HMaxRequestTimes clashString `yaml:"h-max-request-times"`
	HMaxReusableSecs clashString `yaml:"h-max-reusable-secs"`
	HKeepAlivePeriod clashString `yaml:"h-keep-alive-period"`
}

type clashXHTTPOpts struct {
	Host               clashString                  `yaml:"host"`
	Path               clashString                  `yaml:"path"`
	Mode               clashString                  `yaml:"mode"`
	Padding            clashXHTTPPadding            `yaml:",inline"`
	ScMinPostsInterval clashString                  `yaml:"sc-min-posts-interval-ms"`
	ReuseSettings      clashMap[clashXHTTPReuse]    `yaml:"reuse-settings"`
	DownloadSettings   clashMap[clashXHTTPDownload] `yaml:"download-settings"`
}

// clashXHTTPDownload is the separate download leg; a value left unset, nil here, is the proxy's own, as in mihomo.
type clashXHTTPDownload struct {
	Host              *clashString                `yaml:"host"`
	Path              *clashString                `yaml:"path"`
	Mode              clashString                 `yaml:"mode"`
	Server            *clashString                `yaml:"server"`
	Port              *clashInt                   `yaml:"port"`
	TLS               *clashBool                  `yaml:"tls"`
	ALPN              *clashList                  `yaml:"alpn"`
	ServerName        *clashString                `yaml:"servername"`
	ClientFingerprint *clashString                `yaml:"client-fingerprint"`
	Fingerprint       *clashString                `yaml:"fingerprint"`
	RealityOpts       *clashMap[clashRealityOpts] `yaml:"reality-opts"`
	Padding           clashXHTTPPadding           `yaml:",inline"`
	ReuseSettings     clashMap[clashXHTTPReuse]   `yaml:"reuse-settings"`
}

// Entries every full config carries that are not servers; leaving them out is not worth a report.
var clashInfraTypes = []string{"direct", "reject", "reject-drop", "dns", "pass", "compatible", "relay", "select",
	"url-test", "fallback", "load-balance"}

var clashBuilders = map[string]func(*clashProxy, *Profile) error{
	"vless":     (*clashProxy).vless,
	"vmess":     (*clashProxy).vmess,
	"trojan":    (*clashProxy).trojan,
	"ss":        (*clashProxy).shadowsocks,
	"hysteria2": (*clashProxy).hysteria2,
	"tuic":      (*clashProxy).tuic,
	"socks5":    (*clashProxy).socks5,
	"http":      (*clashProxy).http,
	"anytls":    (*clashProxy).anyTLS,
	"wireguard": (*clashProxy).wireGuard,
	"hysteria":  (*clashProxy).hysteria,
	"ssh":       (*clashProxy).ssh,
	"snell":     (*clashProxy).snell,
}

func (p *clashProxy) profile() (Profile, bool, error) {
	kind := strings.ToLower(strings.TrimSpace(string(p.Type)))
	if slices.Contains(clashInfraTypes, kind) {
		return Profile{}, false, nil
	}
	if kind == "" {
		return Profile{}, true, errors.New("the proxy has no type")
	}
	build, ok := clashBuilders[kind]
	if !ok {
		return Profile{}, true, unsupported("Clash proxies of type %q are not supported", kind)
	}
	server, err := clashHost(string(p.Server))
	if err != nil {
		return Profile{}, true, err
	}
	out := Profile{Server: server, ServerPort: int(p.Port)}
	if err = build(p, &out); err != nil {
		return Profile{}, true, err
	}
	return out, true, nil
}

func clashHost(raw string) (string, error) {
	return normalizeLinkHost(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(raw), "["), "]"))
}

func (p *clashProxy) vless(out *Profile) (err error) {
	v := &VLESS{
		UUID:           strings.TrimSpace(string(p.UUID)),
		Flow:           vlessLinkFlow(string(p.Flow)),
		Encryption:     strings.TrimSpace(string(p.Encryption)),
		PacketEncoding: linkPacketEncoding(string(p.PacketEncoding)),
	}
	if strings.EqualFold(v.Encryption, "none") {
		v.Encryption = ""
	}
	if v.TLS, err = p.tls(bool(p.TLS)); err != nil {
		return err
	}
	if v.Transport, err = p.transport(); err != nil {
		return err
	}
	if v.Multiplex, err = p.multiplex(); err != nil {
		return err
	}
	fitVisionFlow(v)
	out.Type, out.VLESS = TypeVLESS, v
	return nil
}

func (p *clashProxy) vmess(out *Profile) (err error) {
	v := &VMess{
		UUID:           strings.TrimSpace(string(p.UUID)),
		AlterID:        max(0, int(p.AlterID)),
		Security:       vmessLinkSecurity(string(p.Cipher)),
		PacketEncoding: linkPacketEncoding(string(p.PacketEncoding)),
	}
	if v.TLS, err = p.tls(bool(p.TLS)); err != nil {
		return err
	}
	if v.Transport, err = p.transport(); err != nil {
		return err
	}
	if v.Multiplex, err = p.multiplex(); err != nil {
		return err
	}
	out.Type, out.VMess = TypeVMess, v
	return nil
}

// trojan always runs over TLS in mihomo.
func (p *clashProxy) trojan(out *Profile) (err error) {
	if err = p.SSOpts.check("ss-opts"); err != nil {
		return err
	}
	if p.SSOpts.v.Enabled {
		return unsupported("Trojan-Go's Shadowsocks layer, ss-opts, is not supported")
	}
	t := &Trojan{Password: string(p.Password)}
	if t.TLS, err = p.tls(true); err != nil {
		return err
	}
	if t.Transport, err = p.transport(); err != nil {
		return err
	}
	if t.Multiplex, err = p.multiplex(); err != nil {
		return err
	}
	out.Type, out.Trojan = TypeTrojan, t
	return nil
}

func (p *clashProxy) shadowsocks(out *Profile) (err error) {
	s := &Shadowsocks{
		Method:     strings.ToLower(strings.TrimSpace(string(p.Cipher))),
		Password:   string(p.Password),
		UDPOverTCP: bool(p.UDPOverTCP),
	}
	if raw := strings.TrimSpace(string(p.Plugin)); raw != "" {
		if err = p.PluginOpts.check("plugin-opts"); err != nil {
			return err
		}
		o := p.PluginOpts.v
		var opts []string
		add := func(key string, value clashString) {
			if text := strings.TrimSpace(string(value)); text != "" {
				opts = append(opts, key+"="+clashPluginValue(text))
			}
		}
		if strings.EqualFold(raw, "shadow-tls") {
			s.ShadowTLS = ShadowTLS{Version: int(o.Version), Password: string(o.Password), TLS: TLS{
				Enabled:    true,
				ServerName: strings.TrimSpace(string(o.Host)),
				UTLS:       clashUTLS(string(p.ClientFingerprint)),
			}}
			// mihomo takes version 2 when none is given.
			if s.ShadowTLS.Version == 0 {
				s.ShadowTLS.Version = 2
			}
			raw = ""
		}
		switch s.Plugin = shadowsocksLinkPlugin(raw); s.Plugin {
		case "":
		case "obfs-local":
			add("obfs", o.Mode)
			add("obfs-host", o.Host)
		case "v2ray-plugin":
			if o.TLS {
				opts = append(opts, "tls")
			}
			add("host", o.Host)
			add("path", o.Path)
			add("mode", o.Mode)
			// A key alone is 1 to the plugin's option parser.
			if o.Mux {
				opts = append(opts, "mux")
			}
		default:
			return unsupported("the Shadowsocks plugin %q is not supported; only obfs and v2ray-plugin work", raw)
		}
		s.PluginOpts = strings.Join(opts, ";")
	}
	if s.Multiplex, err = p.multiplex(); err != nil {
		return err
	}
	out.Type, out.Shadowsocks = TypeShadowsocks, s
	return nil
}

// clashPluginValue escapes what ends a value in SIP003 plugin options.
func clashPluginValue(value string) string {
	return strings.NewReplacer(`\`, `\\`, ";", `\;`).Replace(value)
}

// hysteria2 takes mihomo's port hopping list, written with commas or slashes.
func (p *clashProxy) hysteria2(out *Profile) (err error) {
	h := &Hysteria2{
		Password:    string(p.Password),
		UpMbps:      hysteriaLinkMbps(string(p.Up)),
		DownMbps:    hysteriaLinkMbps(string(p.Down)),
		HopInterval: clashHopInterval(string(p.HopInterval)),
	}
	if ports := strings.ReplaceAll(strings.TrimSpace(string(p.Ports)), "/", ","); ports != "" {
		if h.ServerPorts, err = hysteriaPortRanges(ports); err != nil {
			return err
		}
	}
	if out.ServerPort == 0 && len(h.ServerPorts) > 0 {
		first, _, _ := strings.Cut(h.ServerPorts[0], ":")
		out.ServerPort, _ = strconv.Atoi(first)
	}
	if obfs := strings.ToLower(strings.TrimSpace(string(p.Obfs))); obfs != "" && obfs != "salamander" {
		return unsupported("the Hysteria2 obfuscation %q is not supported", obfs)
	}
	if password := string(p.ObfsPassword); password != "" {
		h.ObfsType, h.ObfsPassword = "salamander", password
	}
	tls, err := p.tls(true)
	if err != nil {
		return err
	}
	h.TLS = quicLinkTLS(tls)
	out.Type, out.Hysteria2 = TypeHysteria2, h
	return nil
}

// clashHopInterval reads mihomo's seconds, or a Go duration.
func clashHopInterval(raw string) string {
	text := strings.TrimSpace(raw)
	if seconds, err := strconv.Atoi(text); err == nil {
		if seconds > 0 {
			return strconv.Itoa(seconds) + "s"
		}
		return ""
	}
	if d, err := time.ParseDuration(text); err == nil && d > 0 {
		return text
	}
	return ""
}

// tuic reads TUIC v5. ip is the address to dial instead of the server's, which stays the TLS server name.
func (p *clashProxy) tuic(out *Profile) (err error) {
	uuid := strings.TrimSpace(string(p.UUID))
	if uuid == "" {
		if strings.TrimSpace(string(p.Token)) != "" {
			return unsupported("TUIC v4 servers, which take a token, are not supported; only TUIC v5 works")
		}
		return errors.New("the TUIC UUID is missing")
	}
	t := &TUIC{
		UUID:              uuid,
		Password:          string(p.Password),
		CongestionControl: tuicLinkChoice(string(p.CongestionController), tuicCongestionControls),
		UDPRelayMode:      tuicLinkChoice(string(p.UDPRelayMode), tuicRelayModes),
		ZeroRTT:           bool(p.ReduceRTT),
	}
	if ms := int(p.HeartbeatInterval); ms > 0 {
		t.Heartbeat = strconv.Itoa(ms) + "ms"
	}
	tls, err := p.tls(true)
	if err != nil {
		return err
	}
	t.TLS = quicLinkTLS(tls)
	if ip := strings.TrimSpace(string(p.IP)); ip != "" {
		if t.TLS.ServerName == "" {
			t.TLS.ServerName = out.Server
		}
		if out.Server, err = clashHost(ip); err != nil {
			return err
		}
	}
	out.Type, out.TUIC = TypeTUIC, t
	return nil
}

func (p *clashProxy) socks5(out *Profile) error {
	if p.TLS {
		return unsupported("SOCKS5 over TLS is not supported")
	}
	out.Type, out.SOCKS = TypeSOCKS, &SOCKS{Version: "5", Username: string(p.Username), Password: string(p.Password)}
	return nil
}

func (p *clashProxy) http(out *Profile) (err error) {
	if err = p.Headers.check("headers"); err != nil {
		return err
	}
	h := &HTTP{Username: string(p.Username), Password: string(p.Password)}
	for name, value := range p.Headers.v {
		if strings.TrimSpace(name) != "" {
			if h.Headers == nil {
				h.Headers = map[string]string{}
			}
			h.Headers[name] = string(value)
		}
	}
	if h.TLS, err = p.tls(bool(p.TLS)); err != nil {
		return err
	}
	out.Type, out.HTTP = TypeHTTP, h
	return nil
}

// anyTLS takes the idle settings in seconds; 0 keeps the defaults of parseAnyTLSLink.
func (p *clashProxy) anyTLS(out *Profile) (err error) {
	a := &AnyTLS{Password: string(p.Password), IdleSessionCheckInterval: "30s", IdleSessionTimeout: "30s", MinIdleSession: 5}
	if seconds := int(p.IdleSessionCheckInterval); seconds > 0 {
		a.IdleSessionCheckInterval = strconv.Itoa(seconds) + "s"
	}
	if seconds := int(p.IdleSessionTimeout); seconds > 0 {
		a.IdleSessionTimeout = strconv.Itoa(seconds) + "s"
	}
	if count := int(p.MinIdleSession); count > 0 {
		a.MinIdleSession = count
	}
	if a.TLS, err = p.tls(true); err != nil {
		return err
	}
	out.Type, out.AnyTLS = TypeAnyTLS, a
	return nil
}

// wireGuard reads mihomo's WireGuard: the server and keys at the top, or a list of peers, of which the first counts.
func (p *clashProxy) wireGuard(out *Profile) (err error) {
	if p.AmneziaWG {
		return unsupported("AmneziaWG servers are not supported")
	}
	w := &WireGuard{
		PrivateKey:          strings.TrimSpace(string(p.PrivateKey)),
		PeerPublicKey:       strings.TrimSpace(string(p.PublicKey)),
		PreSharedKey:        strings.TrimSpace(string(p.PreSharedKey)),
		MTU:                 int(p.MTU),
		PersistentKeepalive: int(p.PersistentKeepalive),
	}
	reserved := p.Reserved
	if len(p.Peers) > 0 {
		peer := p.Peers[0]
		if out.Server, err = clashHost(string(peer.Server)); err != nil {
			return err
		}
		out.ServerPort = int(peer.Port)
		w.PeerPublicKey = strings.TrimSpace(string(peer.PublicKey))
		w.PreSharedKey = strings.TrimSpace(string(peer.PreSharedKey))
		reserved = peer.Reserved
	}
	if err = reserved.check("reserved"); err != nil {
		return err
	}
	if w.Reserved, err = wireGuardReserved(strings.Join(reserved.items, ",")); err != nil {
		return err
	}
	var addresses []string
	for _, address := range []clashString{p.IP, p.IPv6} {
		if text := strings.TrimSpace(string(address)); text != "" {
			addresses = append(addresses, text)
		}
	}
	if w.Address, err = wireGuardAddresses(addresses); err != nil {
		return err
	}
	out.Type, out.WireGuard = TypeWireGuard, w
	return nil
}

// hysteria reads Hysteria v1; auth is the password in base64, auth-str in plain text.
func (p *clashProxy) hysteria(out *Profile) (err error) {
	if protocol := strings.ToLower(strings.TrimSpace(string(p.Protocol))); protocol != "" && protocol != "udp" {
		return unsupported("Hysteria over %s is not supported; sing-box runs it over UDP", protocol)
	}
	h := &Hysteria{
		Auth:        string(p.AuthStr),
		Obfs:        string(p.Obfs),
		UpMbps:      hysteriaLinkMbps(string(p.Up)),
		DownMbps:    hysteriaLinkMbps(string(p.Down)),
		HopInterval: clashHopInterval(string(p.HopInterval)),
	}
	if h.Auth == "" {
		if decoded, ok := decodeLinkBase64(string(p.Auth)); ok {
			h.Auth = string(decoded)
		}
	}
	if ports := strings.ReplaceAll(strings.TrimSpace(string(p.Ports)), "/", ","); ports != "" {
		if h.ServerPorts, err = hysteriaPortRanges(ports); err != nil {
			return err
		}
	}
	if out.ServerPort == 0 && len(h.ServerPorts) > 0 {
		first, _, _ := strings.Cut(h.ServerPorts[0], ":")
		out.ServerPort, _ = strconv.Atoi(first)
	}
	tls, err := p.tls(true)
	if err != nil {
		return err
	}
	h.TLS = quicLinkTLS(tls)
	out.Type, out.Hysteria = TypeHysteria, h
	return nil
}

// ssh reads mihomo's SSH. Its private-key may also be a file path, which a subscription cannot carry.
func (p *clashProxy) ssh(out *Profile) error {
	for _, err := range []error{p.HostKey.check("host-key"), p.HostKeyAlgorithms.check("host-key-algorithms")} {
		if err != nil {
			return err
		}
	}
	s := &SSH{
		User:                 string(p.Username),
		Password:             string(p.Password),
		PrivateKey:           strings.TrimSpace(string(p.PrivateKey)),
		PrivateKeyPassphrase: string(p.PrivateKeyPassphrase),
		HostKey:              p.HostKey.items,
		HostKeyAlgorithms:    p.HostKeyAlgorithms.items,
	}
	if s.PrivateKey != "" && !strings.Contains(s.PrivateKey, "-----BEGIN ") {
		return errors.New("the SSH private key is a file path, which Poros cannot read from a subscription")
	}
	out.Type, out.SSH = TypeSSH, s
	return nil
}

// snell reads mihomo's Snell. mihomo runs versions 1 to 3 and sing-box 4 and 6, so most of them stay unsupported.
func (p *clashProxy) snell(out *Profile) error {
	if err := p.ObfsOpts.check("obfs-opts"); err != nil {
		return err
	}
	s := &Snell{Version: int(p.Version), PSK: string(p.PSK)}
	if s.Version == 0 {
		s.Version = 1
	}
	if mode := strings.ToLower(strings.TrimSpace(string(p.ObfsOpts.v.Mode))); mode != "" && mode != "none" {
		s.ObfsMode, s.ObfsHost = mode, strings.TrimSpace(string(p.ObfsOpts.v.Host))
	}
	out.Type, out.Snell = TypeSnell, s
	return nil
}

// serverName is the TLS server name: vmess and vless write servername, the others sni.
func (p *clashProxy) serverName() string {
	if name := strings.TrimSpace(string(p.ServerName)); name != "" {
		return name
	}
	return strings.TrimSpace(string(p.SNI))
}

// tls reads the TLS keys when on. mihomo's fingerprint is the certificate pin in hex, client-fingerprint the uTLS
// hello, where none turns uTLS off.
func (p *clashProxy) tls(on bool) (TLS, error) {
	if !on {
		return TLS{}, nil
	}
	for _, err := range []error{p.ALPN.check("alpn"), p.RealityOpts.check("reality-opts"), p.ECHOpts.check("ech-opts")} {
		if err != nil {
			return TLS{}, err
		}
	}
	t := TLS{
		Enabled:    true,
		ServerName: p.serverName(),
		DisableSNI: bool(p.DisableSNI),
		Insecure:   bool(p.SkipCertVerify),
		ALPN:       clashALPN(p.ALPN.items),
		UTLS:       clashUTLS(string(p.ClientFingerprint)),
	}
	var err error
	if t.CertificateSHA256, err = linkCertificatePins(string(p.Fingerprint)); err != nil {
		return TLS{}, err
	}
	if p.RealityOpts.set {
		r := p.RealityOpts.v
		key := strings.TrimSpace(string(r.PublicKey))
		// Without the key REALITY would silently become plain TLS.
		if key == "" {
			return TLS{}, errors.New("reality-opts needs the server's public-key")
		}
		t.Reality = Reality{Enabled: true, PublicKey: linkRealityKey(key), ShortID: strings.TrimSpace(string(r.ShortID))}
	}
	if e := p.ECHOpts.v; e.Enable {
		t.ECH = ECH{Enabled: true, QueryServerName: strings.TrimSpace(string(e.QueryServerName))}
		if config := strings.TrimSpace(string(e.Config)); config != "" {
			t.ECH.Config = linkECHConfig([]string{config})
		}
	}
	return t, nil
}

// clashALPN also splits "h2,http/1.1" written as one item.
func clashALPN(items []string) []string {
	var alpn []string
	for _, item := range items {
		alpn = append(alpn, linkList(item)...)
	}
	return alpn
}

func clashUTLS(raw string) UTLS {
	if fingerprint := strings.ToLower(strings.TrimSpace(raw)); slices.Contains(linkFingerprints, fingerprint) {
		return UTLS{Enabled: true, Fingerprint: fingerprint}
	}
	return UTLS{}
}

// transport reads the options of the network mihomo uses; options of another network are ignored, as mihomo does.
func (p *clashProxy) transport() (Transport, error) {
	switch network := strings.ToLower(strings.TrimSpace(string(p.Network))); network {
	case "", "tcp":
		return Transport{}, nil
	case "ws":
		return p.webSocket()
	case "grpc":
		if err := p.GRPCOpts.check("grpc-opts"); err != nil {
			return Transport{}, err
		}
		return Transport{Type: TransportGRPC, ServiceName: string(p.GRPCOpts.v.ServiceName)}, nil
	case "h2":
		if err := p.H2Opts.check("h2-opts"); err != nil {
			return Transport{}, err
		}
		if err := p.H2Opts.v.Host.check("h2-opts.host"); err != nil {
			return Transport{}, err
		}
		o := p.H2Opts.v
		return Transport{Type: TransportHTTP, Host: strings.Join(o.Host.items, ","), Path: string(o.Path)}, nil
	case "http":
		return p.httpHeader()
	case "xhttp":
		return p.xhttp()
	default:
		return Transport{}, unsupported("the transport %q is not supported", network)
	}
}

// webSocket sends the Host header as the transport's host and falls back to the TLS server name, as the Qt client
// did. v2ray-http-upgrade is HTTPUpgrade, which has no early data.
func (p *clashProxy) webSocket() (Transport, error) {
	if err := p.WSOpts.check("ws-opts"); err != nil {
		return Transport{}, err
	}
	o := p.WSOpts.v
	if err := o.Headers.check("ws-opts.headers"); err != nil {
		return Transport{}, err
	}
	t := Transport{Type: TransportWebSocket, Path: string(o.Path)}
	for _, name := range slices.Sorted(maps.Keys(o.Headers.v)) {
		value := string(o.Headers.v[name])
		switch {
		case strings.EqualFold(strings.TrimSpace(name), "host"):
			if t.Host == "" {
				t.Host = strings.TrimSpace(value)
			}
		case strings.TrimSpace(name) != "":
			if t.Headers == nil {
				t.Headers = map[string]string{}
			}
			t.Headers[name] = value
		}
	}
	if t.Host == "" {
		t.Host = p.serverName()
	}
	if o.V2rayHTTPUpgrade {
		t.Type = TransportHTTPUpgrade
		t.Path, _, _ = cutEarlyData(t.Path)
		return t, nil
	}
	t.MaxEarlyData = max(0, int(o.MaxEarlyData))
	t.EarlyDataHeaderName = strings.TrimSpace(string(o.EarlyDataHeaderName))
	if path, ed, ok := cutEarlyData(t.Path); ok {
		t.Path, t.MaxEarlyData, t.EarlyDataHeaderName = path, ed, wsEarlyDataHeader
	}
	return t, nil
}

// httpHeader reads mihomo's http network, the HTTP header obfuscation over raw TCP; Host and Path keep the lists it
// picks from.
func (p *clashProxy) httpHeader() (Transport, error) {
	if err := p.HTTPOpts.check("http-opts"); err != nil {
		return Transport{}, err
	}
	o := p.HTTPOpts.v
	if err := o.Path.check("http-opts.path"); err != nil {
		return Transport{}, err
	}
	if err := o.Headers.check("http-opts.headers"); err != nil {
		return Transport{}, err
	}
	t := Transport{HeaderType: "http", Path: strings.Join(o.Path.items, ",")}
	for _, name := range slices.Sorted(maps.Keys(o.Headers.v)) {
		if strings.EqualFold(strings.TrimSpace(name), "host") && t.Host == "" {
			t.Host = strings.Join(o.Headers.v[name].items, ",")
		}
	}
	if method := strings.TrimSpace(string(o.Method)); method != "" && !strings.ContainsFunc(method, notHTTPTokenRune) {
		t.Method = method
	}
	return t, nil
}

// xhttp keeps what Xray's XHTTP has no field of its own for in extra, under Xray's names: padding, XMUX and the
// download leg, which Xray refuses in stream-one mode.
func (p *clashProxy) xhttp() (Transport, error) {
	if err := p.XHTTPOpts.check("xhttp-opts"); err != nil {
		return Transport{}, err
	}
	o := p.XHTTPOpts.v
	if err := o.ReuseSettings.check("xhttp-opts.reuse-settings"); err != nil {
		return Transport{}, err
	}
	if err := o.DownloadSettings.check("xhttp-opts.download-settings"); err != nil {
		return Transport{}, err
	}
	t := Transport{Type: TransportXHTTP, Host: strings.TrimSpace(string(o.Host)), Path: string(o.Path), XHTTP: XHTTP{Mode: "auto"}}
	if mode := strings.TrimSpace(string(o.Mode)); slices.Contains(xhttpModes, mode) {
		t.XHTTP.Mode = mode
	}
	extra := o.Padding.extra()
	if interval := strings.TrimSpace(string(o.ScMinPostsInterval)); clashInt32Range(interval) {
		extra["scMinPostsIntervalMs"] = interval
	}
	if xmux := o.ReuseSettings.v.xmux(); len(xmux) > 0 {
		extra["xmux"] = xmux
	}
	if o.DownloadSettings.set && t.XHTTP.Mode != "stream-one" {
		download, err := p.xhttpDownload(o.DownloadSettings.v)
		if err != nil {
			return Transport{}, err
		}
		extra["downloadSettings"] = download
	}
	if len(extra) > 0 {
		t.XHTTP.Extra = clashJSON(extra)
	}
	return t, nil
}

func (x clashXHTTPPadding) extra() map[string]any {
	extra := map[string]any{}
	if x.ObfsMode {
		extra["xPaddingObfsMode"] = true
	}
	for key, value := range map[string]clashString{"xPaddingKey": x.Key, "xPaddingHeader": x.Header} {
		if text := strings.TrimSpace(string(value)); text != "" {
			extra[key] = text
		}
	}
	// Xray refuses other values at start.
	if placement := strings.TrimSpace(string(x.Placement)); slices.Contains([]string{"queryInHeader", "cookie", "header", "query"}, placement) {
		extra["xPaddingPlacement"] = placement
	}
	if method := strings.TrimSpace(string(x.Method)); slices.Contains([]string{"repeat-x", "tokenish"}, method) {
		extra["xPaddingMethod"] = method
	}
	return extra
}

// xmux keeps the values Xray accepts: a number or a range such as 16-32, and a whole number of seconds.
func (r clashXHTTPReuse) xmux() map[string]any {
	xmux := map[string]any{}
	for key, value := range map[string]clashString{
		"maxConcurrency": r.MaxConcurrency, "maxConnections": r.MaxConnections, "cMaxReuseTimes": r.CMaxReuseTimes,
		"hMaxRequestTimes": r.HMaxRequestTimes, "hMaxReusableSecs": r.HMaxReusableSecs,
	} {
		if text := strings.TrimSpace(string(value)); clashInt32Range(text) {
			xmux[key] = text
		}
	}
	if period, err := strconv.ParseInt(strings.TrimSpace(string(r.HKeepAlivePeriod)), 10, 64); err == nil && period != 0 {
		xmux["hKeepAlivePeriod"] = period
	}
	return xmux
}

// clashInt32Range accepts Xray's Int32Range: a whole number or two joined by '-', either order.
func clashInt32Range(text string) bool {
	if text == "" {
		return false
	}
	from, to := text, text
	if i := strings.IndexByte(text[1:], '-'); i >= 0 {
		from, to = text[:i+1], text[i+2:]
	}
	_, errFrom := strconv.ParseInt(from, 10, 32)
	_, errTo := strconv.ParseInt(to, 10, 32)
	return errFrom == nil && errTo == nil
}

// xhttpDownload builds Xray's downloadSettings, a stream of its own, from mihomo's download-settings as the Qt
// client did: what it leaves out is the proxy's, and REALITY is lent only by a proxy with tls on.
func (p *clashProxy) xhttpDownload(s clashXHTTPDownload) (map[string]any, error) {
	pick := func(own *clashString, inherited string) string {
		if own != nil {
			return strings.TrimSpace(string(*own))
		}
		return strings.TrimSpace(inherited)
	}
	if err := s.ReuseSettings.check("xhttp-opts.download-settings.reuse-settings"); err != nil {
		return nil, err
	}
	// Xray's dialer takes the leg's address and port unchecked and crashes the core without them.
	address, err := clashHost(pick(s.Server, string(p.Server)))
	if err != nil {
		return nil, err
	}
	if address == "" {
		return nil, errors.New("the XHTTP download leg has no server")
	}
	port := int(p.Port)
	if s.Port != nil {
		port = int(*s.Port)
	}
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("the XHTTP download port %d is not from 1 to 65535", port)
	}
	leg := map[string]any{"address": address, "network": "xhttp", "port": port}

	reality := clashRealityOpts{}
	if s.RealityOpts != nil {
		if err = s.RealityOpts.check("xhttp-opts.download-settings.reality-opts"); err != nil {
			return nil, err
		}
		reality = s.RealityOpts.v
	} else if p.TLS {
		reality = p.RealityOpts.v
	}
	tlsOn := bool(p.TLS)
	if s.TLS != nil {
		tlsOn = bool(*s.TLS)
	}
	serverName := pick(s.ServerName, p.serverName())
	fingerprint := strings.ToLower(pick(s.ClientFingerprint, string(p.ClientFingerprint)))
	known := slices.Contains(linkFingerprints, fingerprint)

	if key := strings.TrimSpace(string(reality.PublicKey)); key != "" {
		settings := map[string]any{"show": false, "publicKey": linkRealityKey(key)}
		if serverName != "" {
			settings["serverName"] = serverName
		}
		// REALITY needs uTLS, so mihomo's none leaves Xray's default.
		if known {
			settings["fingerprint"] = fingerprint
		}
		if shortID := strings.TrimSpace(string(reality.ShortID)); shortID != "" {
			settings["shortId"] = shortID
		}
		leg["security"], leg["realitySettings"] = "reality", settings
	} else if tlsOn {
		settings := map[string]any{}
		if serverName != "" {
			settings["serverName"] = serverName
		}
		alpn := p.ALPN
		if s.ALPN != nil {
			if err = s.ALPN.check("xhttp-opts.download-settings.alpn"); err != nil {
				return nil, err
			}
			alpn = *s.ALPN
		}
		if items := clashALPN(alpn.items); len(items) > 0 {
			settings["alpn"] = items
		}
		// Xray's unsafe is Go's own TLS, which mihomo's none asks for; no fingerprint would mean Chrome.
		if fingerprint == "none" {
			settings["fingerprint"] = "unsafe"
		} else if known {
			settings["fingerprint"] = fingerprint
		}
		pins, err := linkCertificatePins(pick(s.Fingerprint, string(p.Fingerprint)))
		if err != nil {
			return nil, err
		}
		if len(pins) > 0 {
			settings["pinnedPeerCertSha256"] = clashHexPins(pins)
		}
		leg["security"], leg["tlsSettings"] = "tls", settings
	}

	xhttp := map[string]any{"mode": "auto"}
	if host := pick(s.Host, string(p.XHTTPOpts.v.Host)); host != "" {
		xhttp["host"] = host
	}
	path := string(p.XHTTPOpts.v.Path)
	if s.Path != nil {
		path = string(*s.Path)
	}
	if path != "" {
		xhttp["path"] = path
	}
	if mode := strings.TrimSpace(string(s.Mode)); slices.Contains(xhttpModes, mode) {
		xhttp["mode"] = mode
	}
	extra := s.Padding.extra()
	if xmux := s.ReuseSettings.v.xmux(); len(xmux) > 0 {
		extra["xmux"] = xmux
	}
	if len(extra) > 0 {
		xhttp["extra"] = extra
	}
	leg["xhttpSettings"] = xhttp
	return leg, nil
}

// clashHexPins writes pins as Xray's pinnedPeerCertSha256 takes them: hex, joined by commas.
func clashHexPins(pins []string) string {
	hexPins := make([]string, 0, len(pins))
	for _, pin := range pins {
		hash, _ := base64.StdEncoding.DecodeString(pin)
		hexPins = append(hexPins, hex.EncodeToString(hash))
	}
	return strings.Join(hexPins, ",")
}

// clashJSON writes compact JSON with sorted keys and without HTML escapes, which would garble paths.
func clashJSON(value any) json.RawMessage {
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	if encoder.Encode(value) != nil {
		return nil
	}
	return bytes.TrimSuffix(out.Bytes(), []byte("\n"))
}

// multiplex reads smux; sing-box refuses max-streams together with max-connections or min-streams.
func (p *clashProxy) multiplex() (Multiplex, error) {
	if err := p.Smux.check("smux"); err != nil {
		return Multiplex{}, err
	}
	o := p.Smux.v
	if !o.Enabled {
		return Multiplex{}, nil
	}
	m := Multiplex{Mode: SwitchOn, Padding: bool(o.Padding)}
	if protocol := strings.ToLower(strings.TrimSpace(string(o.Protocol))); slices.Contains(multiplexProtocols, protocol) {
		m.Protocol = protocol
	}
	if o.MaxStreams > 0 {
		m.MaxStreams = int(o.MaxStreams)
	} else {
		m.MaxConnections, m.MinStreams = max(0, int(o.MaxConnections)), max(0, int(o.MinStreams))
	}
	return m, nil
}

// Limits checked before yaml.v2 builds a tree of the config. Real configs nest about ten levels deep, and 20000
// proxies take about a million tokens. yaml.v2's tree takes some 500 bytes a token, so a 64 MiB body of tiny
// entries would take gigabytes.
const (
	clashMaxDepth  = 64
	clashMaxTokens = 2_000_000
)

var (
	clashErrTooDeep = errors.New("the Clash config nests its values too deeply to be read")
	clashErrTooMany = errors.New("the Clash config holds too many values to be read")
)

// clashCheckSize refuses a body that nests deeper than clashMaxDepth or holds more than clashMaxTokens tokens.
// yaml.v2 itself stops at 10000 levels of block and 10000 of flow nesting, which Go's stack survives; the depth
// limit gives a clear error long before that.
func clashCheckSize(text []byte) error {
	s := clashDepth{keyColumn: -1}
	for line := range bytes.Lines(text) {
		if s.line(bytes.TrimSuffix(line, []byte("\n"))) {
			return s.refused
		}
	}
	return nil
}

// clashDepth follows yaml.v2's scanner (scannerc.go) far enough to count nesting: the stack of block indentation,
// which grows at a '-', a '?' or a key's column, and flow brackets. Scalars, comments and directives hold no
// structure and are skipped where yaml.v2 skips them. An indentless sequence ("key:\n- item") opens no column of its
// own, so each column counts twice.
type clashDepth struct {
	indents []int
	flow    int
	// keyColumn is where the node in front of a ':' began on this line, the column yaml.v2 gives a key, or -1.
	keyColumn int
	// quote is the quote of a quoted scalar that goes on at the next line; plain is a plain scalar that may.
	quote byte
	plain bool
	// A block scalar holds the lines indented at least blockIndent, which its first line sets when 0. blockParent is
	// the indentation of the collection around it, blockBlank the deepest blank line before its first one.
	block                                bool
	blockParent, blockIndent, blockBlank int
	started                              bool
	count                                int
	// refused is the limit a body broke, nil when it ends or yaml.v2 fails before any limit.
	refused error
}

func (s *clashDepth) top() int {
	if len(s.indents) == 0 {
		return -1
	}
	return s.indents[len(s.indents)-1]
}

func (s *clashDepth) tooDeep() bool {
	if 2*len(s.indents)+s.flow > clashMaxDepth {
		s.refused = clashErrTooDeep
		return true
	}
	return false
}

// push opens a block collection at column, as yaml_parser_roll_indent does.
func (s *clashDepth) push(column int) bool {
	if column > s.top() {
		s.indents = append(s.indents, column)
	}
	return s.tooDeep()
}

// line reads one line. stop is a broken limit or a place yaml.v2 reads no further: the end of the first document,
// or an error.
func (s *clashDepth) line(line []byte) (stop bool) {
	indent := len(line) - len(bytes.TrimLeft(line, " "))
	rest := bytes.TrimLeft(line, " \t")
	if s.block && s.blockLine(indent, len(rest) == 0) {
		return false
	}
	if clashDocumentMarker(line) {
		if s.quote != 0 || s.started {
			return true
		}
		// Document end markers before any document are skipped.
		if line[0] == '.' {
			return false
		}
		s.started = true
		return s.tokens(line, 3)
	}
	if len(rest) == 0 || !s.started && line[0] == '%' {
		return false
	}
	at := 0
	switch {
	case s.quote != 0:
		end, closed := clashQuoteEnd(line, 0, s.quote)
		if !closed {
			return false
		}
		s.quote, at = 0, end
	case s.plain:
		s.plain = false
		// A plain scalar goes on at a line indented past the collection around it, or at any line in flow.
		if rest[0] != '#' && (s.flow > 0 || indent > s.top()) {
			at, s.plain = s.plainEnd(line, len(line)-len(rest))
		}
	}
	s.keyColumn = -1
	return s.tokens(line, at)
}

// blockLine tells whether a line belongs to the open block scalar, and closes the scalar at the first that does not.
func (s *clashDepth) blockLine(indent int, blank bool) bool {
	if blank {
		if s.blockIndent == 0 {
			s.blockBlank = max(s.blockBlank, indent)
		}
		return true
	}
	if s.blockIndent == 0 {
		s.blockIndent = max(s.blockBlank, indent, s.blockParent+1, 1)
	}
	if indent >= s.blockIndent {
		return true
	}
	s.block = false
	return false
}

// tokens reads the tokens of a line from i on.
func (s *clashDepth) tokens(line []byte, i int) (stop bool) {
	columns := clashColumns{line: line}
	for i < len(line) {
		c := line[i]
		if c == ' ' || c == '\t' {
			i++
			continue
		}
		if c == '#' {
			break
		}
		s.started = true
		if s.count++; s.count > clashMaxTokens {
			s.refused = clashErrTooMany
			return true
		}
		column := columns.at(i)
		if s.flow == 0 {
			for s.top() > column {
				s.indents = s.indents[:len(s.indents)-1]
			}
		}
		blankNext := i+1 == len(line) || line[i+1] == ' ' || line[i+1] == '\t'
		switch {
		case c == '[' || c == '{':
			s.startNode(column)
			s.flow++
			if s.tooDeep() {
				return true
			}
			i++
		case c == ']' || c == '}':
			s.flow = max(s.flow-1, 0)
			i++
		case c == ',':
			i++
		case (c == '-' || c == '?') && blankNext:
			if s.flow == 0 {
				if s.push(column) {
					return true
				}
				s.keyColumn = -1
			}
			i++
		case c == ':' && (blankNext || s.flow > 0):
			if s.flow == 0 {
				key := s.keyColumn
				if key < 0 {
					key = column
				}
				if s.push(key) {
					return true
				}
				s.keyColumn = -1
			}
			i++
		case (c == '|' || c == '>') && s.flow == 0:
			return s.openBlock(line[i+1:])
		case c == '\'' || c == '"':
			s.startNode(column)
			end, closed := clashQuoteEnd(line, i+1, c)
			if !closed {
				s.quote = c
				return false
			}
			i = end
		case c == '&' || c == '*' || c == '!':
			s.startNode(column)
			for i++; i < len(line) && line[i] != ' ' && line[i] != '\t' && !(s.flow > 0 && clashFlowIndicator(line[i])); i++ {
			}
		default:
			s.startNode(column)
			i, s.plain = s.plainEnd(line, i+1)
		}
	}
	return false
}

// startNode notes where a node began: a ':' after it makes a key at that column.
func (s *clashDepth) startNode(column int) {
	if s.flow == 0 && s.keyColumn < 0 {
		s.keyColumn = column
	}
}

// plainEnd finds where a plain scalar ends from i on: at ": ", at " #", in flow at a bracket or a comma. open is a
// scalar that reached the end of the line, where it may go on.
func (s *clashDepth) plainEnd(line []byte, i int) (end int, open bool) {
	for ; i < len(line); i++ {
		c := line[i]
		if c == ':' && (i+1 == len(line) || line[i+1] == ' ' || line[i+1] == '\t') ||
			c == '#' && i > 0 && (line[i-1] == ' ' || line[i-1] == '\t') || s.flow > 0 && clashFlowIndicator(c) {
			return i, false
		}
	}
	return i, true
}

// openBlock reads a block scalar's header; stop is a header yaml.v2 refuses, after which it reads nothing.
func (s *clashDepth) openBlock(header []byte) (stop bool) {
	increment, i := 0, 0
	for ; i < len(header) && (header[i] == '+' || header[i] == '-' || header[i] >= '0' && header[i] <= '9'); i++ {
		if header[i] >= '0' && header[i] <= '9' {
			increment = int(header[i] - '0')
		}
	}
	if rest := bytes.TrimLeft(header[i:], " \t"); len(rest) > 0 && rest[0] != '#' {
		return true
	}
	s.block, s.plain = true, false
	s.blockParent, s.blockIndent, s.blockBlank = s.top(), 0, 0
	if increment > 0 {
		s.blockIndent = max(s.blockParent, 0) + increment
	}
	return false
}

// clashQuoteEnd finds the end of a quoted scalar from i on. Single quotes take a doubled quote as one inside them,
// double quotes a backslash escape.
func clashQuoteEnd(line []byte, i int, quote byte) (int, bool) {
	for ; i < len(line); i++ {
		switch line[i] {
		case '\\':
			if quote == '"' {
				i++
			}
		case quote:
			if quote == '\'' && i+1 < len(line) && line[i+1] == '\'' {
				i++
				continue
			}
			return i + 1, true
		}
	}
	return len(line), false
}

func clashFlowIndicator(c byte) bool {
	return c == ',' || c == '[' || c == ']' || c == '{' || c == '}'
}

func clashDocumentMarker(line []byte) bool {
	return len(line) >= 3 && (bytes.HasPrefix(line, []byte("---")) || bytes.HasPrefix(line, []byte("..."))) &&
		(len(line) == 3 || line[3] == ' ' || line[3] == '\t')
}

// clashColumns counts columns as yaml.v2 does, in characters rather than bytes.
type clashColumns struct {
	line          []byte
	index, column int
}

func (c *clashColumns) at(i int) int {
	for ; c.index < i; c.index++ {
		if c.line[c.index]&0xC0 != 0x80 {
			c.column++
		}
	}
	return c.column
}
