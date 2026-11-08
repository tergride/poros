package profile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// Formats of a subscription body, as Document.Format names them.
const (
	FormatLinks   = "links"
	FormatClash   = "clash"
	FormatSingBox = "sing-box"
	FormatXray    = "xray"
	FormatSIP008  = "sip008"
	// FormatWireGuard is a wg-quick config of one server.
	FormatWireGuard = "wireguard"
)

// Document is what a subscription body held.
type Document struct {
	Format   string
	Profiles []Profile
	// Skipped lists, in document order, the first maxSkipped items that did not become servers; SkippedCount counts
	// them all.
	Skipped      []Skipped
	SkippedCount int
}

// maxSkipped bounds the reasons kept: a hostile body of a million bad lines would otherwise hold them all.
const maxSkipped = 1000

// Skipped is an item that did not become a server. It never holds the item's text: links carry passwords.
type Skipped struct {
	// Index is the item's place from 1: a line of a list, a proxy of a Clash config, an outbound of a JSON config,
	// a config of a list of Xray configs.
	Index int
	Name  string
	// Unsupported is a server neither core can run, as opposed to a broken one.
	Unsupported bool
	Reason      string
}

// maxLinkNesting bounds base64 inside base64: providers wrap a list once, and some wrap its lines again.
const maxLinkNesting = 2

// ParseDocument reads a subscription body: links one per line, plain or in base64, a Clash config, a sing-box or
// Xray config, or SIP008. A bad item only lands in Skipped; the error is for a body that cannot be read at all.
func ParseDocument(body []byte) (Document, error) {
	var d Document
	text := trimDocument(body)
	if decoded, ok := decodeDocumentBase64(text); ok {
		text = trimDocument(decoded)
	}
	jsonLike := len(text) > 0 && (text[0] == '{' || text[0] == '[')
	if jsonLike && json.Valid(text) {
		return d, parseJSONDocument(text, &d)
	}
	if isWireGuardConfig(text) {
		parseWireGuardConfig(text, &d)
		return d, nil
	}
	if bytes.Contains(text, []byte("proxies:")) {
		d.Format = FormatClash
		return d, parseClashDocument(text, &d)
	}
	if jsonLike {
		return d, jsonSyntaxError(text)
	}
	d.Format = FormatLinks
	parseLinkList(text, &d, 0, 0)
	return d, nil
}

// add keeps a valid profile or records why the item was skipped. name labels a skipped item, and a kept profile
// without a name of its own takes it.
func (d *Document) add(index int, name string, p Profile, err error) {
	if err == nil {
		err = p.Validate()
	}
	if err != nil {
		if d.SkippedCount++; len(d.Skipped) < maxSkipped {
			d.Skipped = append(d.Skipped, Skipped{Index: index, Name: name, Unsupported: errors.Is(err, ErrUnsupported), Reason: err.Error()})
		}
		return
	}
	if p.Name == "" {
		p.Name = name
	}
	d.Profiles = append(d.Profiles, p)
}

// trimDocument drops surrounding space, a byte order mark and the no-break spaces that copying from a web page adds.
func trimDocument(text []byte) []byte {
	text = bytes.TrimFunc(text, unicode.IsSpace)
	text = bytes.TrimPrefix(text, []byte("\uFEFF"))
	return bytes.TrimFunc(text, unicode.IsSpace)
}

// decodeDocumentBase64 decodes a body that is one base64 blob, on one line or wrapped at a fixed width. Several
// blobs, one per line, have ragged widths: they are left to be decoded line by line.
func decodeDocumentBase64(text []byte) ([]byte, bool) {
	if bytes.ContainsAny(text, " \t\r\n") && !isWrappedBase64(text) {
		return nil, false
	}
	compact := bytes.Join(bytes.Fields(text), nil)
	if !isBase64Text(compact) {
		return nil, false
	}
	return decodeLinkBase64(string(compact))
}

// isWrappedBase64 tells lines of one width with a shorter last one, as base64 tools wrap their output.
func isWrappedBase64(text []byte) bool {
	width, lines, short := 0, 0, false
	for raw := range bytes.Lines(text) {
		line := bytes.TrimRight(raw, " \t\r\n")
		if len(line) == 0 {
			continue
		}
		lines++
		switch {
		case short || width != 0 && len(line) > width:
			return false
		case width == 0:
			width = len(line)
		case len(line) < width:
			short = true
		}
	}
	return lines >= 2 && width%4 == 0
}

func isBase64Text(text []byte) bool {
	if len(text) < 4 {
		return false
	}
	for _, c := range text {
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '+', c == '/', c == '-', c == '_', c == '=':
		default:
			return false
		}
	}
	return true
}

// parseLinkList reads one link per line. A line that is base64 again holds a list of its own, whose items take
// the line's index.
func parseLinkList(text []byte, d *Document, depth, index int) {
	line := 0
	for raw := range bytes.Lines(text) {
		line++
		at := index
		if depth == 0 {
			at = line
		}
		item := trimDocument(raw)
		if len(item) == 0 || item[0] == '#' || bytes.HasPrefix(item, []byte("//")) {
			continue
		}
		if depth < maxLinkNesting && isBase64Text(item) {
			if decoded, ok := decodeLinkBase64(string(item)); ok && bytes.Contains(decoded, []byte("://")) {
				parseLinkList(decoded, d, depth+1, at)
				continue
			}
		}
		d.addLink(at, string(item))
	}
}

// addLink reads one line of a list. Text without a scheme is a provider's note and is left out, as is a web
// address: providers end their notes with their site, while a proxy names a port or a user.
func (d *Document) addLink(index int, line string) {
	scheme, _, ok := strings.Cut(line, "://")
	if !ok || !isSchemeName(scheme) || isWebAddress(line) {
		return
	}
	_, fragment, _ := strings.Cut(line, "#")
	name := strings.TrimSpace(unescapeLinkText(fragment))
	p, err := ParseLink(line)
	if errors.Is(err, ErrNotLink) {
		err = unsupported("%s:// links are not supported", strings.ToLower(scheme))
	}
	d.add(index, name, p, err)
}

func isSchemeName(scheme string) bool {
	for i, c := range scheme {
		letter := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
		if !letter && (i == 0 || !(c >= '0' && c <= '9' || c == '+' || c == '-' || c == '.')) {
			return false
		}
	}
	return scheme != ""
}

func isWebAddress(line string) bool {
	l, err := parseShareLink(line)
	return err == nil && (l.scheme == "http" || l.scheme == "https") && l.port == "" && l.user == "" && l.password == ""
}

// jsonOutbound reads one outbound of a JSON config into a profile without a name. proxy is false for what is not
// a server, such as direct, block, dns or a group, which is left out silently. tag is the outbound's tag, also
// next to an error.
type jsonOutbound func(raw json.RawMessage) (p Profile, tag string, proxy bool, err error)

// parseJSONDocument tells Xray from sing-box by the key that names an outbound's protocol: Xray's "protocol",
// sing-box's "type". Both wrap them into "outbounds", so Xray goes first.
func parseJSONDocument(text []byte, d *Document) error {
	if text[0] == '[' {
		var items []json.RawMessage
		if err := json.Unmarshal(text, &items); err != nil {
			return err
		}
		var first map[string]json.RawMessage
		if len(items) > 0 {
			first = jsonFields(items[0])
		}
		switch {
		case first["protocol"] != nil:
			d.Format = FormatXray
			d.addOutbounds(items, 0, xrayOutbound)
		case isXrayConfig(first):
			d.Format = FormatXray
			for i, config := range items {
				d.addXrayConfig(i+1, config)
			}
		case first["type"] != nil:
			d.Format = FormatSingBox
			shadowTLS := singBoxShadowTLSTags(items)
			d.addOutbounds(items, 0, func(raw json.RawMessage) (Profile, string, bool, error) { return singBoxConfigOutbound(raw, shadowTLS) })
		default:
			return errors.New("the JSON list holds neither sing-box nor Xray outbounds")
		}
		return nil
	}

	fields := jsonFields(text)
	switch {
	case isXrayConfig(fields):
		d.Format = FormatXray
		d.addXrayConfig(0, text)
	case fields["protocol"] != nil:
		d.Format = FormatXray
		d.addOutbounds([]json.RawMessage{text}, 0, xrayOutbound)
	case fields["outbounds"] != nil || fields["endpoints"] != nil:
		d.Format = FormatSingBox
		var outbounds, endpoints []json.RawMessage
		if err := json.Unmarshal(jsonOr(fields["outbounds"], "[]"), &outbounds); err != nil {
			return fmt.Errorf("the sing-box outbounds are not a list: %w", err)
		}
		if err := json.Unmarshal(jsonOr(fields["endpoints"], "[]"), &endpoints); err != nil {
			return fmt.Errorf("the sing-box endpoints are not a list: %w", err)
		}
		shadowTLS := singBoxShadowTLSTags(outbounds)
		read := func(raw json.RawMessage) (Profile, string, bool, error) { return singBoxConfigOutbound(raw, shadowTLS) }
		d.addOutbounds(outbounds, 0, read)
		d.addOutbounds(endpoints, len(outbounds), read)
	case fields["type"] != nil:
		d.Format = FormatSingBox
		d.addOutbounds([]json.RawMessage{text}, 0, singBoxOutbound)
	case fields["servers"] != nil:
		d.Format = FormatSIP008
		return parseSIP008(fields["servers"], d)
	case fields["proxies"] != nil:
		// Clash written as JSON, which YAML reads too.
		d.Format = FormatClash
		return parseClashDocument(text, d)
	default:
		return errors.New("the JSON is neither a sing-box, Xray nor SIP008 config")
	}
	return nil
}

// jsonFields reads the keys of a JSON object; anything else has none.
func jsonFields(raw json.RawMessage) map[string]json.RawMessage {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return nil
	}
	return fields
}

func jsonOr(raw json.RawMessage, fallback string) json.RawMessage {
	if raw == nil {
		return json.RawMessage(fallback)
	}
	return raw
}

// isXrayConfig tells a whole Xray config by an outbound with "protocol" in its outbounds.
func isXrayConfig(fields map[string]json.RawMessage) bool {
	var outbounds []json.RawMessage
	if json.Unmarshal(fields["outbounds"], &outbounds) != nil {
		return false
	}
	for _, outbound := range outbounds {
		if jsonFields(outbound)["protocol"] != nil {
			return true
		}
	}
	return false
}

// addOutbounds adds the servers among outbounds, which start at index offset+1, named by their tags.
func (d *Document) addOutbounds(outbounds []json.RawMessage, offset int, read jsonOutbound) {
	for i, raw := range outbounds {
		if p, tag, proxy, err := read(raw); proxy {
			d.add(offset+i+1, tag, p, err)
		}
	}
}

// addXrayConfig reads the servers of one Xray config, named by its remarks, or by tags where remarks are missing or
// shared by several servers. In a list of configs, as panels serve them, a config is one item at index; the
// outbounds of a lone config, index 0, are items of their own.
func (d *Document) addXrayConfig(index int, raw json.RawMessage) {
	var config struct {
		Remarks   string            `json:"remarks"`
		Outbounds []json.RawMessage `json:"outbounds"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		d.add(index, "", Profile{}, fmt.Errorf("the Xray config cannot be read: %w", err))
		return
	}
	type server struct {
		at  int
		tag string
		p   Profile
		err error
	}
	var servers []server
	direct := xrayDirectTags(config.Outbounds)
	for i, outbound := range config.Outbounds {
		p, tag, proxy, err := xrayConfigOutbound(outbound, direct)
		if !proxy {
			continue
		}
		at := index
		if at == 0 {
			at = i + 1
		}
		servers = append(servers, server{at, tag, p, err})
	}
	remarks := strings.TrimSpace(config.Remarks)
	for _, s := range servers {
		name := remarks
		switch {
		case name == "":
			name = s.tag
		case len(servers) > 1 && s.tag != "":
			name += " · " + s.tag
		}
		s.p.Name = name
		d.add(s.at, name, s.p, s.err)
	}
}

// sip008Server is one server of SIP008, Shadowsocks's JSON list.
type sip008Server struct {
	Remarks    string `json:"remarks"`
	Server     string `json:"server"`
	ServerPort int    `json:"server_port"`
	Password   string `json:"password"`
	Method     string `json:"method"`
	Plugin     string `json:"plugin"`
	PluginOpts string `json:"plugin_opts"`
}

func parseSIP008(raw json.RawMessage, d *Document) error {
	var servers []json.RawMessage
	if err := json.Unmarshal(raw, &servers); err != nil {
		return fmt.Errorf("the SIP008 servers are not a list: %w", err)
	}
	for i, item := range servers {
		var s sip008Server
		if err := json.Unmarshal(item, &s); err != nil {
			d.add(i+1, "", Profile{}, fmt.Errorf("the SIP008 server cannot be read: %w", err))
			continue
		}
		host := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(s.Server), "["), "]")
		server, err := normalizeLinkHost(host)
		ss := &Shadowsocks{Method: strings.ToLower(strings.TrimSpace(s.Method)), Password: s.Password}
		if ss.Plugin = shadowsocksLinkPlugin(s.Plugin); ss.Plugin != "" {
			ss.PluginOpts = strings.TrimSpace(s.PluginOpts)
		}
		name := strings.TrimSpace(s.Remarks)
		d.add(i+1, name, Profile{Type: TypeShadowsocks, Name: name, Server: server, ServerPort: s.ServerPort, Shadowsocks: ss}, err)
	}
	return nil
}

// jsonSyntaxError tells where a body that looks like JSON breaks.
func jsonSyntaxError(text []byte) error {
	var value any
	err := json.Unmarshal(text, &value)
	var syntax *json.SyntaxError
	if !errors.As(err, &syntax) {
		return fmt.Errorf("the body is not valid JSON: %v", err)
	}
	// Offset counts the bytes read up to and including the one that broke.
	before := text[:min(int(syntax.Offset), len(text))]
	line := bytes.Count(before, []byte("\n")) + 1
	column := len(before) - bytes.LastIndexByte(before, '\n') - 1
	return fmt.Errorf("the body is not valid JSON: %v at line %d, column %d", syntax, line, column)
}
