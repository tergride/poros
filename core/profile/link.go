package profile

import (
	"errors"
	"strings"
)

// ErrNotLink is returned for text that is not a share link of a known scheme.
var ErrNotLink = errors.New("not a server link")

type linkParser func(link string) (Profile, error)

// The schemes a share link may start with. http and https are proxies here: telling a subscription address
// from an HTTP proxy is the caller's business.
var linkParsers = map[string]linkParser{
	"vless":     parseVLESSLink,
	"vmess":     parseVMessLink,
	"trojan":    parseTrojanLink,
	"ss":        parseShadowsocksLink,
	"hysteria2": parseHysteria2Link,
	"hy2":       parseHysteria2Link,
	"tuic":      parseTUICLink,
	"socks":     parseSOCKSLink,
	"socks5":    parseSOCKSLink,
	"socks4":    parseSOCKSLink,
	"socks4a":   parseSOCKSLink,
	"http":      parseHTTPLink,
	"https":     parseHTTPLink,
	"anytls":    parseAnyTLSLink,
}

// ParseLink reads one share link into a valid profile.
func ParseLink(link string) (Profile, error) {
	link = strings.TrimSpace(link)
	scheme, _, ok := strings.Cut(link, "://")
	if !ok {
		return Profile{}, ErrNotLink
	}
	parse, ok := linkParsers[strings.ToLower(scheme)]
	if !ok {
		return Profile{}, ErrNotLink
	}
	p, err := parse(link)
	if err != nil {
		return Profile{}, err
	}
	if err = p.Validate(); err != nil {
		return Profile{}, err
	}
	return p, nil
}
