package config

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/netip"
	"strconv"
	"strings"

	"PorosCore/profile"
)

// Bridge is the loopback SOCKS5 hop from sing-box to Xray: Xray listens on the port, sing-box dials it with the
// credentials. The engine picks them for each start.
type Bridge struct {
	Port     uint16
	Username string
	Password string
}

var errDownloadLeg = errors.New("the XHTTP download leg needs an address, a port and the xhttp network")

// RunsOnXray tells which core runs a server: VLESS that needs Xray, and VLESS with REALITY over TCP or gRPC while
// realityOnXray holds. sing-box runs everything else.
func RunsOnXray(p profile.Profile, realityOnXray bool) bool {
	if p.Type != profile.TypeVLESS || p.VLESS == nil {
		return false
	}
	v := p.VLESS
	if v.NeedsXray() {
		return true
	}
	return realityOnXray && v.TLS.Reality.Enabled &&
		(v.Transport.Type == profile.TransportTCP || v.Transport.Type == profile.TransportGRPC)
}

// XrayHosts lists the host names Xray would resolve for a server: the engine resolves them before the start, since
// upstream Xray would ask the system, whose DNS goes into the TUN and back out through Xray itself.
func XrayHosts(p profile.Profile, realityOnXray bool) []string {
	if !RunsOnXray(p, realityOnXray) {
		return nil
	}
	var hosts []string
	if host := ace(p.Server); !isIP(host) {
		hosts = append(hosts, host)
	}
	if leg := downloadLeg(p.VLESS.Transport.XHTTP.Extra); leg != nil {
		if address, _ := leg[findKey(leg, "address")].(string); address != "" && !isIP(ace(address)) {
			hosts = append(hosts, ace(address))
		}
	}
	return hosts
}

func downloadLeg(extra json.RawMessage) map[string]any {
	if len(extra) == 0 {
		return nil
	}
	var object map[string]any
	if json.Unmarshal(extra, &object) != nil {
		return nil
	}
	leg, _ := object[findKey(object, "downloadSettings")].(map[string]any)
	return leg
}

// buildXray writes the Xray config of a VLESS server behind the bridge.
func buildXray(in Input) ([]byte, error) {
	outbound, err := xrayVLESS(in.Server, in.Interface, in.Resolved)
	if err != nil {
		return nil, err
	}
	config := map[string]any{
		"log": map[string]any{"loglevel": "warning", "access": "none"},
		"inbounds": []any{map[string]any{
			"tag":      "bridge",
			"listen":   "127.0.0.1",
			"port":     in.Bridge.Port,
			"protocol": "socks",
			"settings": map[string]any{
				"auth":     "password",
				"udp":      true,
				"ip":       "127.0.0.1",
				"accounts": []any{map[string]any{"user": in.Bridge.Username, "pass": in.Bridge.Password}},
			},
		}},
		"outbounds": []any{outbound},
	}
	return json.Marshal(config)
}

func xrayVLESS(p profile.Profile, iface string, resolved map[string]netip.Addr) (map[string]any, error) {
	v := p.VLESS
	host := ace(p.Server)
	address := resolve(host, resolved)
	encryption := v.Encryption
	if encryption == "" {
		encryption = "none"
	}
	settings := map[string]any{"address": address, "port": p.ServerPort, "id": v.UUID, "encryption": encryption}
	if v.Flow != "" {
		settings["flow"] = v.Flow
	}
	stream, err := xrayStream(v.TLS, v.Transport, host, address != host, iface, resolved)
	if err != nil {
		return nil, err
	}
	return map[string]any{"tag": TagProxy, "protocol": "vless", "settings": settings, "streamSettings": stream}, nil
}

func resolve(host string, resolved map[string]netip.Addr) string {
	if addr, ok := resolved[host]; ok && addr.IsValid() {
		return addr.Unmap().String()
	}
	return host
}

var xrayNetworks = map[string]string{
	profile.TransportTCP:         "raw",
	profile.TransportWebSocket:   "ws",
	profile.TransportHTTPUpgrade: "httpupgrade",
	profile.TransportGRPC:        "grpc",
	profile.TransportXHTTP:       "xhttp",
}

// xrayStream writes the streamSettings. host is the server's name; when it was replaced by an address, it becomes
// the default server name and Host, as Xray would have taken them from the address.
func xrayStream(t profile.TLS, tr profile.Transport, host string, replaced bool, iface string, resolved map[string]netip.Addr) (map[string]any, error) {
	network, ok := xrayNetworks[tr.Type]
	if !ok {
		return nil, fmt.Errorf("Xray has no %q transport", tr.Type)
	}
	stream := map[string]any{"network": network}
	serverName := ace(t.ServerName)
	if serverName == "" && replaced {
		serverName = host
	}
	switch {
	case t.Reality.Enabled:
		stream["security"] = "reality"
		fingerprint := t.UTLS.Fingerprint
		if fingerprint == "" {
			fingerprint = realityFingerprint
		}
		reality := map[string]any{
			"serverName":  serverName,
			"fingerprint": fingerprint,
			"password":    t.Reality.PublicKey,
			"shortId":     t.Reality.ShortID,
		}
		if spider := t.Reality.SpiderX; spider != "" {
			if !strings.HasPrefix(spider, "/") {
				spider = "/" + spider
			}
			reality["spiderX"] = spider
		}
		stream["realitySettings"] = reality
	case t.Enabled:
		stream["security"] = "tls"
		tls, err := xrayTLS(t, serverName)
		if err != nil {
			return nil, err
		}
		stream["tlsSettings"] = tls
	default:
		stream["security"] = "none"
	}

	hostHeader := ace(tr.Host)
	headers := map[string]string{}
	for name, value := range tr.Headers {
		if strings.EqualFold(name, "Host") {
			if hostHeader == "" {
				hostHeader = ace(value)
			}
			continue
		}
		headers[name] = value
	}
	if hostHeader == "" && replaced {
		hostHeader = serverName
	}
	switch tr.Type {
	case profile.TransportTCP:
		if tr.HeaderType == "http" {
			paths := splitList(tr.Path)
			if len(paths) == 0 {
				paths = []string{"/"}
			}
			request := map[string]any{"path": paths}
			if hosts := aceList(splitList(tr.Host)); len(hosts) > 0 {
				request["headers"] = map[string]any{"Host": hosts}
			} else if hostHeader != "" {
				request["headers"] = map[string]any{"Host": []string{hostHeader}}
			}
			stream["rawSettings"] = map[string]any{"header": map[string]any{"type": "http", "request": request}}
		}
	case profile.TransportWebSocket, profile.TransportHTTPUpgrade:
		settings := map[string]any{"path": earlyDataPath(tr.Path, tr.MaxEarlyData)}
		if hostHeader != "" {
			settings["host"] = hostHeader
		}
		if len(headers) > 0 {
			settings["headers"] = headers
		}
		stream[network+"Settings"] = settings
	case profile.TransportGRPC:
		stream["grpcSettings"] = map[string]any{"serviceName": tr.ServiceName}
	case profile.TransportXHTTP:
		mode := tr.XHTTP.Mode
		if mode == "" {
			mode = "auto"
		}
		settings := map[string]any{"path": tr.Path, "mode": mode}
		if hostHeader != "" {
			settings["host"] = hostHeader
		}
		extra, err := xrayExtra(tr.XHTTP.Extra, mode, iface, resolved)
		if err != nil {
			return nil, err
		}
		if len(headers) > 0 {
			if extra == nil {
				extra = map[string]any{}
			}
			if _, set := extra[findKey(extra, "headers")]; !set {
				extra["headers"] = headers
			}
		}
		if extra != nil {
			settings["extra"] = extra
		}
		stream["xhttpSettings"] = settings
	}
	if iface != "" {
		stream["sockopt"] = map[string]any{"interface": iface}
	}
	return stream, nil
}

// earlyDataPath appends ?ed=, Xray's way to ask for WebSocket early data in the Sec-WebSocket-Protocol header.
func earlyDataPath(path string, earlyData int) string {
	if earlyData <= 0 {
		return path
	}
	if strings.Contains(path, "?") {
		return path + "&ed=" + strconv.Itoa(earlyData)
	}
	return path + "?ed=" + strconv.Itoa(earlyData)
}

func xrayTLS(t profile.TLS, serverName string) (map[string]any, error) {
	tls := map[string]any{"serverName": serverName}
	if len(t.ALPN) > 0 {
		tls["alpn"] = t.ALPN
	}
	if t.UTLS.Fingerprint != "" {
		tls["fingerprint"] = t.UTLS.Fingerprint
	}
	if t.MinVersion != "" {
		tls["minVersion"] = t.MinVersion
	}
	if t.MaxVersion != "" {
		tls["maxVersion"] = t.MaxVersion
	}
	if len(t.CertificateSHA256) > 0 {
		pins := make([]string, len(t.CertificateSHA256))
		for i, pin := range t.CertificateSHA256 {
			value, err := base64.StdEncoding.DecodeString(pin)
			if err != nil || len(value) != 32 {
				return nil, fmt.Errorf("the certificate pin %q is not a base64 SHA-256", pin)
			}
			pins[i] = hex.EncodeToString(value)
		}
		tls["pinnedPeerCertSha256"] = strings.Join(pins, ",")
	}
	if t.ECH.Enabled {
		list, err := echConfigList(t.ECH.Config)
		if err != nil {
			return nil, err
		}
		tls["echConfigList"] = list
	}
	return tls, nil
}

// echConfigList turns the PEM lines of an ECH config into the base64 ECHConfigList Xray reads.
func echConfigList(lines []string) (string, error) {
	var body strings.Builder
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "-----") {
			continue
		}
		body.WriteString(line)
	}
	decoded, err := base64.StdEncoding.DecodeString(body.String())
	if err != nil || len(decoded) == 0 {
		return "", errors.New("the ECH config is not PEM")
	}
	return base64.StdEncoding.EncodeToString(decoded), nil
}

// xrayExtra cleans the XHTTP extra object of a link, which nobody checked: stream-one has no download leg, and a
// leg carries no files, no dialer of its own and no skipped certificate check.
func xrayExtra(raw json.RawMessage, mode, iface string, resolved map[string]netip.Addr) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var extra map[string]any
	if err := decoder.Decode(&extra); err != nil || extra == nil {
		return nil, errors.New("the XHTTP extra is not a JSON object")
	}
	key := findKey(extra, "downloadSettings")
	if key == "" {
		return extra, nil
	}
	if mode == "stream-one" {
		delete(extra, key)
		return extra, nil
	}
	leg, ok := extra[key].(map[string]any)
	if !ok {
		return nil, errDownloadLeg
	}
	cleaned, err := xrayDownloadLeg(leg, iface, resolved)
	if err != nil {
		return nil, err
	}
	extra[key] = cleaned
	return extra, nil
}

func xrayDownloadLeg(leg map[string]any, iface string, resolved map[string]netip.Addr) (map[string]any, error) {
	leg = maps.Clone(leg)
	addressKey := findKey(leg, "address")
	address, _ := leg[addressKey].(string)
	address = strings.TrimSpace(address)
	port, err := jsonInt(leg[findKey(leg, "port")])
	network, _ := leg[findKey(leg, "network")].(string)
	if network == "" {
		network, _ = leg[findKey(leg, "method")].(string)
	}
	network = strings.ToLower(network)
	if address == "" || err != nil || port < 1 || port > 65535 || (network != "xhttp" && network != "splithttp") {
		return nil, errDownloadLeg
	}

	sockopt, sockoptKey := nestedMap(leg, "sockopt")
	if sockopt != nil {
		deleteKey(sockopt, "dialerProxy")
		deleteKey(sockopt, "domainStrategy")
	}
	for _, name := range []string{"tlsSettings", "realitySettings"} {
		settings, _ := nestedMap(leg, name)
		if settings == nil {
			continue
		}
		deleteKey(settings, "masterKeyLog")
		if insecure, _ := settings[findKey(settings, "allowInsecure")].(bool); insecure {
			return nil, profile.Unsupported("Xray no longer skips the certificate check of the XHTTP download leg")
		}
		deleteKey(settings, "allowInsecure")
	}
	xhttp, _ := nestedMap(leg, "xhttpSettings")
	if xhttp == nil {
		xhttp, _ = nestedMap(leg, "splithttpSettings")
	}
	if xhttp != nil {
		if inner, _ := nestedMap(xhttp, "extra"); inner != nil {
			deleteKey(inner, "downloadSettings")
		}
	}

	host := ace(address)
	if ip := resolve(host, resolved); ip != host {
		leg[addressKey] = ip
		security, _ := leg[findKey(leg, "security")].(string)
		serverName := host
		if strings.EqualFold(security, "tls") {
			tls, _ := nestedMap(leg, "tlsSettings")
			if tls == nil {
				tls = map[string]any{}
				leg["tlsSettings"] = tls
			}
			if name, _ := tls[findKey(tls, "serverName")].(string); name != "" {
				serverName = name
			} else {
				tls["serverName"] = host
			}
		}
		if xhttp == nil {
			xhttp = map[string]any{}
			leg["xhttpSettings"] = xhttp
		}
		if name, _ := xhttp[findKey(xhttp, "host")].(string); name == "" {
			xhttp["host"] = serverName
		}
	}
	if iface != "" {
		if sockopt == nil {
			sockopt, sockoptKey = map[string]any{}, "sockopt"
			leg[sockoptKey] = sockopt
		}
		if _, set := sockopt[findKey(sockopt, "interface")]; !set {
			sockopt["interface"] = iface
		}
	}
	return leg, nil
}

// findKey finds a key the way Xray reads them, ignoring case; "" when absent.
func findKey(object map[string]any, name string) string {
	if _, ok := object[name]; ok {
		return name
	}
	for key := range object {
		if strings.EqualFold(key, name) {
			return key
		}
	}
	return ""
}

func deleteKey(object map[string]any, name string) {
	for key := range object {
		if strings.EqualFold(key, name) {
			delete(object, key)
		}
	}
}

// nestedMap returns a copy of an object under a key, stored back in place so that edits stay in the parent.
func nestedMap(object map[string]any, name string) (map[string]any, string) {
	key := findKey(object, name)
	nested, ok := object[key].(map[string]any)
	if !ok {
		return nil, ""
	}
	nested = maps.Clone(nested)
	object[key] = nested
	return nested, key
}

func jsonInt(value any) (int, error) {
	switch number := value.(type) {
	case json.Number:
		n, err := number.Int64()
		return int(n), err
	case float64:
		if number != float64(int(number)) {
			return 0, errors.New("not an integer")
		}
		return int(number), nil
	}
	return 0, errors.New("not a number")
}
