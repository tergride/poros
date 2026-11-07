package config

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"PorosCore/profile"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json/badoption"
)

// realityFingerprint stands in for a REALITY server without one: sing-box runs REALITY only over uTLS, and chrome
// is what Xray picks in the same case.
const realityFingerprint = "chrome"

// singBoxOutbound turns a server into a sing-box outbound under tag. hardened fragments the TLS handshake.
func singBoxOutbound(p profile.Profile, tag string, hardened bool) (option.Outbound, error) {
	server := option.ServerOptions{Server: ace(p.Server), ServerPort: uint16(p.ServerPort)}
	var options any
	switch p.Type {
	case profile.TypeVLESS:
		v := p.VLESS
		tls, transport, err := singBoxStream(v.TLS, v.Transport, hardened)
		if err != nil {
			return option.Outbound{}, err
		}
		// Without the key sing-box picks xudp; the model's "" is no packet encoding.
		encoding := v.PacketEncoding
		options = &option.VLESSOutboundOptions{
			ServerOptions:               server,
			UUID:                        v.UUID,
			Flow:                        v.Flow,
			OutboundTLSOptionsContainer: option.OutboundTLSOptionsContainer{TLS: tls},
			Multiplex:                   singBoxMultiplex(v.Multiplex),
			Transport:                   transport,
			PacketEncoding:              &encoding,
		}
	case profile.TypeVMess:
		v := p.VMess
		tls, transport, err := singBoxStream(v.TLS, v.Transport, hardened)
		if err != nil {
			return option.Outbound{}, err
		}
		security := v.Security
		if security == "auto" {
			security = ""
		}
		options = &option.VMessOutboundOptions{
			ServerOptions:               server,
			UUID:                        v.UUID,
			Security:                    security,
			AlterId:                     v.AlterID,
			OutboundTLSOptionsContainer: option.OutboundTLSOptionsContainer{TLS: tls},
			PacketEncoding:              v.PacketEncoding,
			Multiplex:                   singBoxMultiplex(v.Multiplex),
			Transport:                   transport,
		}
	case profile.TypeTrojan:
		t := p.Trojan
		tls, transport, err := singBoxStream(t.TLS, t.Transport, hardened)
		if err != nil {
			return option.Outbound{}, err
		}
		options = &option.TrojanOutboundOptions{
			ServerOptions:               server,
			Password:                    t.Password,
			OutboundTLSOptionsContainer: option.OutboundTLSOptionsContainer{TLS: tls},
			Multiplex:                   singBoxMultiplex(t.Multiplex),
			Transport:                   transport,
		}
	case profile.TypeShadowsocks:
		s := p.Shadowsocks
		ss := &option.ShadowsocksOutboundOptions{
			ServerOptions: server,
			Method:        s.Method,
			Password:      s.Password,
			Plugin:        s.Plugin,
			PluginOptions: s.PluginOpts,
			Multiplex:     singBoxMultiplex(s.Multiplex),
		}
		if s.UDPOverTCP {
			ss.UDPOverTCP = &option.UDPOverTCPOptions{Enabled: true}
		}
		options = ss
	case profile.TypeHysteria2:
		h := p.Hysteria2
		tls, err := singBoxTLS(h.TLS, true, false)
		if err != nil {
			return option.Outbound{}, err
		}
		hop, err := duration(h.HopInterval)
		if err != nil {
			return option.Outbound{}, err
		}
		hy := &option.Hysteria2OutboundOptions{
			ServerOptions:               server,
			ServerPorts:                 h.ServerPorts,
			HopInterval:                 hop,
			UpMbps:                      h.UpMbps,
			DownMbps:                    h.DownMbps,
			Password:                    h.Password,
			OutboundTLSOptionsContainer: option.OutboundTLSOptionsContainer{TLS: tls},
		}
		if h.ObfsPassword != "" {
			hy.Obfs = &option.Hysteria2Obfs{Type: C.Hysteria2ObfsTypeSalamander, Password: h.ObfsPassword}
		}
		options = hy
	case profile.TypeTUIC:
		t := p.TUIC
		tls, err := singBoxTLS(t.TLS, true, false)
		if err != nil {
			return option.Outbound{}, err
		}
		heartbeat, err := duration(t.Heartbeat)
		if err != nil {
			return option.Outbound{}, err
		}
		options = &option.TUICOutboundOptions{
			ServerOptions:               server,
			UUID:                        t.UUID,
			Password:                    t.Password,
			CongestionControl:           t.CongestionControl,
			UDPRelayMode:                t.UDPRelayMode,
			ZeroRTTHandshake:            t.ZeroRTT,
			Heartbeat:                   heartbeat,
			OutboundTLSOptionsContainer: option.OutboundTLSOptionsContainer{TLS: tls},
		}
	case profile.TypeSOCKS:
		s := p.SOCKS
		version := s.Version
		if version == "5" {
			version = ""
		}
		options = &option.SOCKSOutboundOptions{ServerOptions: server, Version: version, Username: s.Username, Password: s.Password}
	case profile.TypeHTTP:
		h := p.HTTP
		tls, err := singBoxTLS(h.TLS, false, hardened)
		if err != nil {
			return option.Outbound{}, err
		}
		options = &option.HTTPOutboundOptions{
			ServerOptions:               server,
			Username:                    h.Username,
			Password:                    h.Password,
			OutboundTLSOptionsContainer: option.OutboundTLSOptionsContainer{TLS: tls},
			Path:                        h.Path,
			Headers:                     httpHeader(h.Headers, ""),
		}
	case profile.TypeAnyTLS:
		a := p.AnyTLS
		tls, err := singBoxTLS(a.TLS, false, hardened)
		if err != nil {
			return option.Outbound{}, err
		}
		check, err := duration(a.IdleSessionCheckInterval)
		if err != nil {
			return option.Outbound{}, err
		}
		timeout, err := duration(a.IdleSessionTimeout)
		if err != nil {
			return option.Outbound{}, err
		}
		options = &option.AnyTLSOutboundOptions{
			ServerOptions:               server,
			OutboundTLSOptionsContainer: option.OutboundTLSOptionsContainer{TLS: tls},
			Password:                    a.Password,
			IdleSessionCheckInterval:    check,
			IdleSessionTimeout:          timeout,
			MinIdleSession:              a.MinIdleSession,
		}
	default:
		return option.Outbound{}, fmt.Errorf("unknown server type %q", p.Type)
	}
	return option.Outbound{Type: p.Type, Tag: tag, Options: options}, nil
}

func singBoxStream(t profile.TLS, transport profile.Transport, hardened bool) (*option.OutboundTLSOptions, *option.V2RayTransportOptions, error) {
	tls, err := singBoxTLS(t, transport.Type == profile.TransportQUIC, hardened)
	if err != nil {
		return nil, nil, err
	}
	v2ray, err := singBoxTransport(transport)
	return tls, v2ray, err
}

// singBoxTLS builds the TLS of a server; quic is TLS inside QUIC, where neither uTLS nor fragments exist.
func singBoxTLS(t profile.TLS, quic, hardened bool) (*option.OutboundTLSOptions, error) {
	if !t.Enabled {
		return nil, nil
	}
	options := &option.OutboundTLSOptions{
		Enabled:    true,
		DisableSNI: t.DisableSNI,
		ServerName: ace(t.ServerName),
		Insecure:   t.Insecure,
		ALPN:       t.ALPN,
		MinVersion: t.MinVersion,
		MaxVersion: t.MaxVersion,
		Fragment:   hardened && !quic && !t.Reality.Enabled,
	}
	var err error
	if options.CertificateSHA256, err = decodePins(t.CertificateSHA256); err != nil {
		return nil, err
	}
	if options.CertificatePublicKeySHA256, err = decodePins(t.CertificatePublicKeySHA256); err != nil {
		return nil, err
	}
	if t.ECH.Enabled {
		options.ECH = &option.OutboundECHOptions{Enabled: true, Config: t.ECH.Config, QueryServerName: ace(t.ECH.QueryServerName)}
	}
	if !quic {
		switch {
		case t.UTLS.Enabled || t.UTLS.Fingerprint != "":
			options.UTLS = &option.OutboundUTLSOptions{Enabled: true, Fingerprint: t.UTLS.Fingerprint}
		case t.Reality.Enabled:
			options.UTLS = &option.OutboundUTLSOptions{Enabled: true, Fingerprint: realityFingerprint}
		}
	}
	if t.Reality.Enabled {
		options.Reality = &option.OutboundRealityOptions{Enabled: true, PublicKey: t.Reality.PublicKey, ShortID: t.Reality.ShortID}
	}
	return options, nil
}

func decodePins(pins []string) (badoption.Listable[[]byte], error) {
	var decoded badoption.Listable[[]byte]
	for _, pin := range pins {
		value, err := base64.StdEncoding.DecodeString(pin)
		if err != nil || len(value) != 32 {
			return nil, fmt.Errorf("the certificate pin %q is not a base64 SHA-256", pin)
		}
		decoded = append(decoded, value)
	}
	return decoded, nil
}

func singBoxTransport(t profile.Transport) (*option.V2RayTransportOptions, error) {
	switch t.Type {
	case profile.TransportTCP:
		if t.HeaderType != "http" {
			return nil, nil
		}
		// Xray's HTTP header obfuscation over raw TCP: in plain text sing-box's HTTP transport sends the same request.
		path := "/"
		if paths := splitList(t.Path); len(paths) > 0 {
			path = paths[0]
		}
		return &option.V2RayTransportOptions{Type: C.V2RayTransportTypeHTTP, HTTPOptions: option.V2RayHTTPOptions{
			Host: aceList(splitList(t.Host)), Path: path, Method: "GET", Headers: httpHeader(t.Headers, ""),
		}}, nil
	case profile.TransportWebSocket:
		return &option.V2RayTransportOptions{Type: C.V2RayTransportTypeWebsocket, WebsocketOptions: option.V2RayWebsocketOptions{
			Path:                t.Path,
			Headers:             httpHeader(t.Headers, ace(t.Host)),
			MaxEarlyData:        uint32(t.MaxEarlyData),
			EarlyDataHeaderName: t.EarlyDataHeaderName,
		}}, nil
	case profile.TransportGRPC:
		return &option.V2RayTransportOptions{Type: C.V2RayTransportTypeGRPC, GRPCOptions: option.V2RayGRPCOptions{ServiceName: t.ServiceName}}, nil
	case profile.TransportHTTP:
		return &option.V2RayTransportOptions{Type: C.V2RayTransportTypeHTTP, HTTPOptions: option.V2RayHTTPOptions{
			Host: aceList(splitList(t.Host)), Path: t.Path, Method: t.Method, Headers: httpHeader(t.Headers, ""),
		}}, nil
	case profile.TransportHTTPUpgrade:
		return &option.V2RayTransportOptions{Type: C.V2RayTransportTypeHTTPUpgrade, HTTPUpgradeOptions: option.V2RayHTTPUpgradeOptions{
			Host: ace(t.Host), Path: t.Path, Headers: httpHeader(t.Headers, ""),
		}}, nil
	case profile.TransportQUIC:
		return &option.V2RayTransportOptions{Type: C.V2RayTransportTypeQUIC}, nil
	}
	return nil, fmt.Errorf("sing-box has no %q transport", t.Type)
}

func singBoxMultiplex(m profile.Multiplex) *option.OutboundMultiplexOptions {
	if !m.Enabled {
		return nil
	}
	return &option.OutboundMultiplexOptions{
		Enabled:        true,
		Protocol:       m.Protocol,
		MaxConnections: m.MaxConnections,
		MinStreams:     m.MinStreams,
		MaxStreams:     m.MaxStreams,
		Padding:        m.Padding,
	}
}

// httpHeader copies headers; a non-empty host becomes the Host header.
func httpHeader(headers map[string]string, host string) badoption.HTTPHeader {
	if len(headers) == 0 && host == "" {
		return nil
	}
	result := make(badoption.HTTPHeader, len(headers)+1)
	for name, value := range headers {
		result[name] = badoption.Listable[string]{value}
	}
	if host != "" {
		for name := range result {
			if strings.EqualFold(name, "Host") {
				delete(result, name)
			}
		}
		result["Host"] = badoption.Listable[string]{host}
	}
	return result
}

func duration(text string) (badoption.Duration, error) {
	if text == "" {
		return 0, nil
	}
	value, err := time.ParseDuration(text)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%q is not a duration", text)
	}
	return badoption.Duration(value), nil
}
