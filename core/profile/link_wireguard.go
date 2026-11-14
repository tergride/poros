package profile

import (
	"errors"
	"net/netip"
	"strconv"
	"strings"
)

// parseWireGuardLink reads wireguard://privatekey@host:port?publickey=&address=&reserved=&mtu=#name, as v2rayN and
// Throne write it, also as wg://. Throne's key names are read too.
func parseWireGuardLink(link string) (Profile, error) {
	l, err := parseShareLink(link)
	if err != nil {
		return Profile{}, err
	}
	q := l.query
	port, err := l.serverPort(51820)
	if err != nil {
		return Profile{}, err
	}
	w := &WireGuard{PrivateKey: firstLinkValue(q, "private_key", "privatekey", "secretKey")}
	if w.PrivateKey == "" {
		w.PrivateKey = l.userinfo()
	}
	w.PeerPublicKey = firstLinkValue(q, "public_key", "publickey", "peer_public_key")
	w.PreSharedKey = firstLinkValue(q, "pre_shared_key", "preshared_key", "presharedkey", "psk")
	addresses := firstLinkValue(q, "address", "local_address", "ip")
	if w.Address, err = wireGuardAddresses(strings.FieldsFunc(addresses, func(r rune) bool { return r == ',' || r == ' ' })); err != nil {
		return Profile{}, err
	}
	if q.has("reserved") {
		if w.Reserved, err = wireGuardReserved(q.get("reserved")); err != nil {
			return Profile{}, err
		}
	}
	if mtu, ok := linkInt(q, "mtu", 576, 65535); ok {
		w.MTU = mtu
	}
	for _, key := range []string{"persistent_keepalive_interval", "persistent_keepalive", "keepalive"} {
		if keepalive, ok := linkInt(q, key, 0, 65535); ok {
			w.PersistentKeepalive = keepalive
			break
		}
	}
	return Profile{Type: TypeWireGuard, Name: l.name, Server: l.host, ServerPort: port, WireGuard: w}, nil
}

// firstLinkValue is the value of the first of keys the query has, trimmed.
func firstLinkValue(q linkQuery, keys ...string) string {
	for _, key := range keys {
		if q.has(key) {
			return q.trimmed(key)
		}
	}
	return ""
}

// wireGuardAddresses reads the tunnel's own addresses; a bare address is a single host, /32 or /128.
func wireGuardAddresses(items []string) ([]string, error) {
	var addresses []string
	for _, item := range items {
		if item = strings.TrimSpace(item); item == "" {
			continue
		}
		if !strings.Contains(item, "/") {
			addr, err := netip.ParseAddr(item)
			if err != nil {
				return nil, errors.New("the WireGuard address " + strconv.Quote(item) + " is not an IP address")
			}
			item = netip.PrefixFrom(addr, addr.BitLen()).String()
		}
		prefix, err := netip.ParsePrefix(item)
		if err != nil {
			return nil, errors.New("the WireGuard address " + strconv.Quote(item) + " is not written as 172.16.0.2/32")
		}
		addresses = append(addresses, prefix.String())
	}
	return addresses, nil
}

// wireGuardReserved reads the three reserved bytes: numbers split by commas or dashes, or their base64.
func wireGuardReserved(text string) ([]int, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	if !strings.ContainsAny(text, ",-") {
		if decoded, ok := decodeLinkBase64(text); ok && len(decoded) == 3 {
			return []int{int(decoded[0]), int(decoded[1]), int(decoded[2])}, nil
		}
	}
	var bytes []int
	for item := range strings.SplitSeq(strings.ReplaceAll(text, "-", ","), ",") {
		b, err := strconv.Atoi(strings.TrimSpace(item))
		if err != nil || b < 0 || b > 255 {
			return nil, errors.New("WireGuard's reserved bytes are three numbers from 0 to 255")
		}
		bytes = append(bytes, b)
	}
	if len(bytes) != 3 {
		return nil, errors.New("WireGuard's reserved bytes are three numbers from 0 to 255")
	}
	return bytes, nil
}
