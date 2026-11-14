package config

import (
	"net/netip"
	"strings"

	"golang.org/x/net/idna"
)

// ace writes a host name in ASCII (punycode), as cores and DNS expect it; addresses and names it cannot convert
// stay as they are.
func ace(host string) string {
	if host == "" || isASCII(host) {
		return host
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return host
	}
	dot := strings.HasPrefix(host, ".")
	converted, err := idna.Lookup.ToASCII(strings.TrimPrefix(host, "."))
	if err != nil {
		return host
	}
	if dot {
		return "." + converted
	}
	return converted
}

func aceList(hosts []string) []string {
	if len(hosts) == 0 {
		return nil
	}
	result := make([]string, len(hosts))
	for i, host := range hosts {
		result[i] = ace(host)
	}
	return result
}

func isASCII(text string) bool {
	for i := 0; i < len(text); i++ {
		if text[i] >= 0x80 {
			return false
		}
	}
	return true
}

// splitList reads a comma-separated value, as links carry hosts and paths, without empty items.
func splitList(text string) []string {
	var items []string
	for item := range strings.SplitSeq(text, ",") {
		if item = strings.TrimSpace(item); item != "" {
			items = append(items, item)
		}
	}
	return items
}

// isIP tells an address from a host name.
func isIP(host string) bool {
	_, err := netip.ParseAddr(host)
	return err == nil
}
