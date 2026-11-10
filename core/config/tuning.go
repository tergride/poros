package config

import (
	"cmp"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/byteformats"

	"PorosCore/profile"
)

// Tuning is what the settings add to the servers of a connection: the defaults of profiles that leave a choice «As
// by default», the QUIC of the servers that run on it, and the split handshake of the hardened preset.
type Tuning struct {
	// Hardened splits the TLS handshake of the server connection; it comes from the preset.
	Hardened bool
	// Multiplex is on for profiles that leave it to the default; MuxProtocol and MuxStreams fill what a profile
	// leaves empty.
	Multiplex   bool
	MuxProtocol string
	MuxStreams  int
	// Fragment splits the handshake of profiles that leave it to the default.
	Fragment bool
	// Fingerprint is the uTLS fingerprint of profiles with TLS and none of their own; "" is none.
	Fingerprint string
	// Spoof sends a forged handshake with SpoofSNI before the real one for profiles that leave it to the default;
	// SpoofSNI fills the name a profile leaves empty, SpoofMethod is how the server drops the forgery.
	Spoof       bool
	SpoofSNI    string
	SpoofMethod string
	// QUIC are the options of QUIC and HTTP/2 of Hysteria, Hysteria2, TUIC and MASQUE.
	QUIC option.QUICOptions
	// Insecure skips the certificate check of servers on sing-box that do not skip it themselves; pins of a
	// profile still hold. Xray cannot skip it.
	Insecure bool
	// XraySystemCA has Xray trust the certificates of the system instead of its own list.
	XraySystemCA bool
	// noSpoof forges no handshake at all, the profile's own neither.
	noSpoof bool
}

// inner is the tuning of a server reached through another one: only the first connection meets the censor, and a
// forged or split handshake is no use inside a tunnel.
func (t Tuning) inner() Tuning {
	t.Hardened, t.Fragment, t.noSpoof = false, false, true
	return t
}

// forTest is the tuning of a test of servers: a forged handshake takes administrator rights, which a test does not
// ask for.
func (t Tuning) forTest() Tuning {
	t.noSpoof = true
	return t
}

// onXray gives a server on Xray the defaults Xray takes: the uTLS fingerprint. Xray has neither the multiplex of
// sing-box, its fragments nor a forged handshake.
func (t Tuning) onXray(p profile.Profile) profile.Profile {
	if p.VLESS != nil && p.VLESS.TLS.Enabled && p.VLESS.TLS.UTLS.Fingerprint == "" && t.Fingerprint != "" {
		vless := *p.VLESS
		vless.TLS.UTLS = profile.UTLS{Enabled: true, Fingerprint: t.Fingerprint}
		p.VLESS = &vless
	}
	return p
}

// xrayOutbound adds to an Xray outbound what the settings ask of its TLS: the certificates of the system, for the
// server and for the download leg of XHTTP.
func (t Tuning) xrayOutbound(outbound map[string]any) {
	if !t.XraySystemCA {
		return
	}
	stream, _ := outbound["streamSettings"].(map[string]any)
	if tls, ok := stream["tlsSettings"].(map[string]any); ok {
		tls["useSystemCA"] = true
	}
	xhttp, _ := stream["xhttpSettings"].(map[string]any)
	extra, _ := xhttp["extra"].(map[string]any)
	if leg, _ := nestedMap(extra, "downloadSettings"); leg != nil {
		if tls, _ := nestedMap(leg, "tlsSettings"); tls != nil {
			tls["useSystemCA"] = true
		}
	}
}

// Spoofs tells whether a connection through entry, the server that meets the network, forges a handshake, which
// takes administrator rights. Xray forges none.
func (t Tuning) Spoofs(entry profile.Profile, realityOnXray bool) bool {
	if RunsOnXray(entry, realityOnXray) {
		return false
	}
	tls, ok := tlsOf(entry)
	if !ok {
		return false
	}
	sni, err := t.spoofSNI(tls, entry.Server)
	return err == nil && sni != ""
}

// spoofSNI is the name a handshake to server is forged with, "" for none. A profile that leaves it to the default
// forges only where sing-box can: with a server name that is no address and differs from the forged one.
func (t Tuning) spoofSNI(tls profile.TLS, server string) (string, error) {
	if t.noSpoof || !tls.Enabled || tls.Reality.Enabled || tls.Spoof == profile.SwitchOff || (tls.Spoof == "" && !t.Spoof) {
		return "", nil
	}
	sni := strings.TrimSpace(cmp.Or(tls.SpoofSNI, t.SpoofSNI))
	if tls.Spoof == "" {
		name := ace(cmp.Or(tls.ServerName, server))
		if _, err := netip.ParseAddr(name); err == nil || sni == "" || name == "" || tls.DisableSNI || strings.EqualFold(sni, name) {
			return "", nil
		}
	}
	if sni == "" {
		return "", errors.New("the forged TLS handshake has no name: set the spoof SNI of the profile or of the settings")
	}
	return sni, nil
}

// tlsOf gives the TLS of a server over TCP that sing-box runs, if it has one.
func tlsOf(p profile.Profile) (profile.TLS, bool) {
	switch {
	case p.VLESS != nil && p.VLESS.Transport.Type != profile.TransportQUIC:
		return p.VLESS.TLS, true
	case p.VMess != nil && p.VMess.Transport.Type != profile.TransportQUIC:
		return p.VMess.TLS, true
	case p.Trojan != nil && p.Trojan.Transport.Type != profile.TransportQUIC:
		return p.Trojan.TLS, true
	case p.HTTP != nil:
		return p.HTTP.TLS, true
	case p.AnyTLS != nil:
		return p.AnyTLS.TLS, true
	}
	return profile.TLS{}, false
}

// ParseQUIC reads the QUIC and HTTP/2 options of the settings: durations such as 30s, windows such as 8MB; "" and
// 0 leave the core's own.
func ParseQUIC(idleTimeout, keepAlive, streamWindow, connectionWindow string, maxStreams, initialPacketSize int, noPathMTU bool) (option.QUICOptions, error) {
	var quic option.QUICOptions
	var err error
	if quic.IdleTimeout, err = duration(idleTimeout); err != nil {
		return quic, fmt.Errorf("the idle timeout: %w", err)
	}
	if quic.KeepAlivePeriod, err = duration(keepAlive); err != nil {
		return quic, fmt.Errorf("the keep-alive period: %w", err)
	}
	for _, w := range []struct {
		name   string
		text   string
		target **byteformats.MemoryBytes
	}{{"stream receive window", streamWindow, &quic.StreamReceiveWindow}, {"connection receive window", connectionWindow, &quic.ConnectionReceiveWindow}} {
		text := strings.TrimSpace(w.text)
		if text == "" {
			continue
		}
		size := new(byteformats.MemoryBytes)
		if err := size.UnmarshalJSON([]byte(strconv.Quote(text))); err != nil || size.Value() == 0 {
			return quic, fmt.Errorf("the %s %q is no size such as 8MB", w.name, text)
		}
		*w.target = size
	}
	if maxStreams < 0 || initialPacketSize < 0 || (initialPacketSize != 0 && (initialPacketSize < 1200 || initialPacketSize > 65000)) {
		return quic, errors.New("the streams are none or more, the initial packet from 1200 to 65000 bytes")
	}
	quic.MaxConcurrentStreams, quic.InitialPacketSize, quic.DisablePathMTUDiscovery = maxStreams, initialPacketSize, noPathMTU
	return quic, nil
}
