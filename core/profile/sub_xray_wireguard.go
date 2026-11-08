package profile

import (
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"strings"
)

type xrayWireGuardJSON struct {
	SecretKey string         `json:"secretKey"`
	Address   xrayStringList `json:"address"`
	Peers     []struct {
		PublicKey    string  `json:"publicKey"`
		PreSharedKey string  `json:"preSharedKey"`
		Endpoint     string  `json:"endpoint"`
		KeepAlive    xrayInt `json:"keepAlive"`
	} `json:"peers"`
	MTU      xrayInt `json:"mtu"`
	Reserved []int   `json:"reserved"`
}

// xrayWireGuard reads Xray's WireGuard outbound; its first peer is the server, which sing-box runs.
func xrayWireGuard(settings json.RawMessage) (Profile, error) {
	var s xrayWireGuardJSON
	if xrayPresent(settings) {
		if err := json.Unmarshal(settings, &s); err != nil {
			return Profile{}, xrayReadError("settings", err)
		}
	}
	if len(s.Peers) == 0 {
		return Profile{}, errors.New("the WireGuard outbound has no peer")
	}
	peer := s.Peers[0]
	host, portText, err := net.SplitHostPort(strings.TrimSpace(peer.Endpoint))
	if err != nil {
		return Profile{}, errors.New("the WireGuard peer's endpoint is not written as vpn.example.com:51820")
	}
	server, err := normalizeLinkHost(host)
	if err != nil {
		return Profile{}, err
	}
	port, _ := strconv.Atoi(portText)
	w := &WireGuard{
		PrivateKey:          strings.TrimSpace(s.SecretKey),
		PeerPublicKey:       strings.TrimSpace(peer.PublicKey),
		PreSharedKey:        strings.TrimSpace(peer.PreSharedKey),
		MTU:                 int(s.MTU),
		PersistentKeepalive: int(peer.KeepAlive),
		Reserved:            s.Reserved,
	}
	if w.Address, err = wireGuardAddresses(s.Address); err != nil {
		return Profile{}, err
	}
	return Profile{Type: TypeWireGuard, Server: server, ServerPort: port, WireGuard: w}, nil
}
