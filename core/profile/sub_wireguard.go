package profile

import (
	"bytes"
	"errors"
	"net"
	"slices"
	"strconv"
	"strings"
)

// isWireGuardConfig tells a wg-quick config, the file WireGuard apps and VPN providers hand out.
func isWireGuardConfig(text []byte) bool {
	return bytes.Contains(text, []byte("[Interface]")) && bytes.Contains(text, []byte("[Peer]"))
}

// amneziaKeys are AmneziaWG's: a server that needs them refuses plain WireGuard.
var amneziaKeys = []string{"jc", "jmin", "jmax", "s1", "s2", "s3", "s4", "h1", "h2", "h3", "h4", "i1", "i2", "i3", "i4", "i5"}

// parseWireGuardConfig reads a wg-quick config as one server. Only the last [Peer] counts, as Poros keeps one peer;
// DNS, AllowedIPs and the scripts are the business of wg-quick, not of a proxy.
func parseWireGuardConfig(text []byte, d *Document) {
	d.Format = FormatWireGuard
	w := &WireGuard{}
	var p Profile
	var addresses []string
	var err error
	for raw := range bytes.Lines(text) {
		line := string(raw)
		if comment := strings.IndexByte(line, '#'); comment >= 0 {
			line = line[:comment]
		}
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if !found {
			continue
		}
		key, value = strings.ToLower(strings.TrimSpace(key)), strings.TrimSpace(value)
		switch {
		case key == "privatekey":
			w.PrivateKey = value
		case key == "address":
			addresses = append(addresses, strings.Split(value, ",")...)
		case key == "mtu":
			w.MTU, _ = strconv.Atoi(value)
		case key == "publickey":
			w.PeerPublicKey = value
		case key == "presharedkey":
			w.PreSharedKey = value
		case key == "persistentkeepalive":
			w.PersistentKeepalive, _ = strconv.Atoi(value)
		case key == "endpoint":
			host, port, splitErr := net.SplitHostPort(value)
			if splitErr != nil {
				err = errors.New("the Endpoint of the [Peer] is not written as vpn.example.com:51820")
				continue
			}
			p.Server, _ = normalizeLinkHost(host)
			p.ServerPort, _ = strconv.Atoi(port)
		case key == "reserved":
			reserved, reservedErr := wireGuardReserved(value)
			if reservedErr != nil {
				err = reservedErr
			}
			w.Reserved = reserved
		case slices.Contains(amneziaKeys, key):
			err = unsupported("the config is for AmneziaWG, which Poros does not run")
		}
	}
	if err == nil {
		w.Address, err = wireGuardAddresses(addresses)
	}
	if err == nil && p.Server == "" {
		err = errors.New("the [Peer] has no Endpoint such as vpn.example.com:51820")
	}
	p.Type, p.WireGuard = TypeWireGuard, w
	d.add(1, "", p, err)
}
