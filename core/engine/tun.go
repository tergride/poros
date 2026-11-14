package engine

import (
	"net/netip"
	"strings"

	"PorosCore/config"
	"PorosCore/internal/jsonrpc"
)

// The MTU of the TUN as it comes and its bounds: IPv6 takes no less than 1280.
const (
	defaultTunMTU = 1500
	minTunMTU     = 1280
	maxTunMTU     = 9000
)

// validateTun checks the TUN of the settings: its MTU, an address of each family that is no network's own, and
// ranges to keep out that leave something in the TUN.
func validateTun(s Settings) error {
	if s.TunMTU < minTunMTU || s.TunMTU > maxTunMTU {
		return jsonrpc.Failed(ReasonInvalidValue, "tunMtu must be from %d to %d, not %d", minTunMTU, maxTunMTU, s.TunMTU)
	}
	for name, family := range map[string]struct {
		text   string
		is4    bool
		maxLen int
	}{"tunIpv4": {s.TunIPv4, true, 30}, "tunIpv6": {s.TunIPv6, false, 126}} {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(family.text))
		if err != nil || prefix.Addr().Is4() != family.is4 || prefix.Bits() < 1 || prefix.Bits() > family.maxLen || prefix.Addr() == prefix.Masked().Addr() {
			return jsonrpc.Failed(ReasonInvalidValue, "%s: %q is no address of the TUN with its subnet", name, family.text)
		}
	}
	if _, err := bypassOf(s.TunBypass); err != nil {
		return err
	}
	return nil
}

// bypassOf reads the ranges the TUN keeps out, a line each; empty lines and lines starting with # are skipped.
func bypassOf(text string) ([]netip.Prefix, error) {
	ranges := []netip.Prefix{}
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		prefix, err := netip.ParsePrefix(line)
		if err != nil {
			addr, aerr := netip.ParseAddr(line)
			if aerr != nil {
				return nil, jsonrpc.Failed(ReasonInvalidValue, "tunBypass, line %d: %q is no subnet", i+1, line)
			}
			prefix = netip.PrefixFrom(addr, addr.BitLen())
		}
		if prefix.Bits() == 0 {
			return nil, jsonrpc.Failed(ReasonInvalidValue, "tunBypass, line %d: %s covers every address, and nothing would go into the TUN", i+1, line)
		}
		ranges = append(ranges, prefix.Masked())
	}
	return ranges, nil
}

// tunOf is the TUN of a connection by the settings, which validateTun has checked.
func tunOf(s Settings) config.TunOptions {
	bypass, _ := bypassOf(s.TunBypass)
	address4, _ := netip.ParsePrefix(strings.TrimSpace(s.TunIPv4))
	address6, _ := netip.ParsePrefix(strings.TrimSpace(s.TunIPv6))
	return config.TunOptions{
		MTU: uint32(s.TunMTU), LooseRoute: !s.TunStrictRoute, Routing: s.TunRouting,
		Address4: address4, Address6: address6, Bypass: bypass,
	}
}

// tunChanged tells whether two settings make the TUN differently.
func tunChanged(a, b Settings) bool {
	return a.TunMTU != b.TunMTU || a.TunStrictRoute != b.TunStrictRoute || a.TunRouting != b.TunRouting || a.TunIPv4 != b.TunIPv4 ||
		a.TunIPv6 != b.TunIPv6 || a.TunBypass != b.TunBypass
}
