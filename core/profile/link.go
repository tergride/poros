package profile

import (
	"errors"
	"fmt"
	"strings"
)

// ErrNotLink is returned for text that is not a share link of a known scheme.
var ErrNotLink = errors.New("not a server link")

// ErrUnsupported matches the errors about a server neither core can run, as opposed to a broken one.
var ErrUnsupported = errors.New("not supported")

type unsupportedError string

func (e unsupportedError) Error() string { return string(e) }

func (unsupportedError) Is(target error) bool { return target == ErrUnsupported }

// Unsupported formats an error that matches ErrUnsupported; the config builder reports with it too.
func Unsupported(format string, args ...any) error {
	return unsupportedError(fmt.Sprintf(format, args...))
}

func unsupported(format string, args ...any) error { return Unsupported(format, args...) }

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
	"poros":     parsePorosLink,
	"throne":    parseThroneLink,
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
