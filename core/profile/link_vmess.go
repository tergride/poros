package profile

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
)

var vmessSecurities = []string{"auto", "aes-128-gcm", "chacha20-poly1305", "none", "zero"}

// parseVMessLink reads v2rayN's vmess://base64(JSON) and, failing that, the vmess://uuid@host:port?query#name form.
func parseVMessLink(link string) (Profile, error) {
	_, rest, _ := strings.Cut(link, "://")
	// Some generators append #name to the base64 or use the URL-safe alphabet.
	payload, fragment, _ := strings.Cut(rest, "#")
	if decoded, ok := decodeLinkBase64(payload); ok {
		var fields map[string]any
		if json.Unmarshal(decoded, &fields) == nil && len(fields) > 0 {
			return parseVMessJSON(fields, unescapeLinkText(fragment))
		}
	}
	return parseVMessURL(link)
}

// parseVMessJSON turns v2rayN's fields into link keys, so they go through the same TLS and transport rules.
func parseVMessJSON(fields map[string]any, fragment string) (Profile, error) {
	text := func(key string) string {
		value, _ := fields[key].(string)
		return value
	}
	q := parseLinkQuery("")
	network := strings.ToLower(strings.TrimSpace(text("net")))
	q.set("type", network)
	q.set("headerType", text("type"))
	q.set("host", text("host"))
	if network == "grpc" {
		q.set("serviceName", text("path"))
	} else {
		q.set("path", text("path"))
	}

	tlsMode := strings.TrimSpace(text("tls"))
	tlsOn := strings.EqualFold(tlsMode, "tls")
	if !tlsOn && tlsMode != "" && !strings.EqualFold(tlsMode, "none") {
		return Profile{}, fmt.Errorf("a VMess link can use only tls or none, not %q", tlsMode)
	}
	if tlsOn {
		q.set("security", "tls")
		q.set("sni", text("sni"))
		q.set("alpn", text("alpn"))
		q.set("fp", text("fp"))
		insecure, ok := fields["insecure"]
		if !ok {
			insecure = fields["allowInsecure"]
		}
		q.set("allowInsecure", jsonLinkText(insecure))
	}

	packetEncoding := "xudp"
	// Throne's throneExtra carries, as link keys, what the v2rayN schema has no room for: REALITY, pins, mux.
	if extra := text("throneExtra"); extra != "" {
		extraQuery := parseLinkQuery(extra)
		q.merge(extraQuery)
		if extraQuery.has("packetEncoding") {
			packetEncoding = linkPacketEncoding(extraQuery.get("packetEncoding"))
		}
	}
	// TLS off in the schema keeps it off whatever throneExtra says.
	if !tlsOn {
		q.set("security", "none")
	}

	host := strings.TrimSpace(text("add"))
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	server, err := normalizeLinkHost(host)
	if err != nil {
		return Profile{}, err
	}
	v := &VMess{
		UUID:           text("id"),
		AlterID:        jsonLinkInt(fields["aid"]),
		Security:       "auto",
		PacketEncoding: packetEncoding,
		Multiplex:      linkMultiplex(q),
	}
	if scy := text("scy"); scy != "" {
		v.Security = vmessLinkSecurity(scy)
	}
	if v.Transport, err = linkTransport(q); err != nil {
		return Profile{}, err
	}
	if v.TLS, err = linkTLS(q, false); err != nil {
		return Profile{}, err
	}
	name := text("ps")
	if name == "" {
		name = fragment
	}
	return Profile{Type: TypeVMess, Name: name, Server: server, ServerPort: jsonLinkInt(fields["port"]), VMess: v}, nil
}

func parseVMessURL(link string) (Profile, error) {
	l, err := parseShareLink(link)
	if err != nil {
		return Profile{}, err
	}
	port, err := l.serverPort(443)
	if err != nil {
		return Profile{}, err
	}
	q := l.query
	v := &VMess{
		UUID:           l.user,
		Security:       vmessLinkSecurity(q.get("encryption")),
		PacketEncoding: "xudp",
		Multiplex:      linkMultiplex(q),
	}
	v.AlterID, _ = linkInt(q, "alterId", 0, math.MaxInt32)
	if q.has("packetEncoding") {
		v.PacketEncoding = linkPacketEncoding(q.get("packetEncoding"))
	}
	if v.Transport, err = linkTransport(q); err != nil {
		return Profile{}, err
	}
	if v.TLS, err = linkTLS(q, false); err != nil {
		return Profile{}, err
	}
	return Profile{Type: TypeVMess, Name: l.name, Server: l.host, ServerPort: port, VMess: v}, nil
}

// vmessLinkSecurity keeps a known cipher; an empty or unknown one is auto.
func vmessLinkSecurity(raw string) string {
	if security := strings.ToLower(strings.TrimSpace(raw)); slices.Contains(vmessSecurities, security) {
		return security
	}
	return "auto"
}

// jsonLinkInt reads a v2rayN number, which generators write as a JSON number or as a string.
func jsonLinkInt(value any) int {
	switch v := value.(type) {
	case float64:
		if v >= math.MinInt32 && v <= math.MaxInt32 {
			return int(v)
		}
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return 0
}

// jsonLinkText reads a v2rayN flag, which generators write as a string, a number or a boolean.
func jsonLinkText(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	}
	return ""
}
