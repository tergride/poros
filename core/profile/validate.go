package profile

import (
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"
)

var shadowsocksMethods = []string{
	"2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm", "2022-blake3-chacha20-poly1305", "none",
	"aes-128-gcm", "aes-192-gcm", "aes-256-gcm", "chacha20-ietf-poly1305", "xchacha20-ietf-poly1305",
	"aes-128-ctr", "aes-192-ctr", "aes-256-ctr", "aes-128-cfb", "aes-192-cfb", "aes-256-cfb",
	"rc4-md5", "chacha20-ietf", "xchacha20",
}

var shadowsocksPlugins = []string{"", "obfs-local", "v2ray-plugin"}

// Validate checks what every core needs to start the profile: its type and part, the server and the credentials.
func (p Profile) Validate() error {
	if err := p.validatePart(); err != nil {
		return err
	}
	if strings.TrimSpace(p.Server) == "" {
		return errors.New("the server address is missing")
	}
	if p.ServerPort < 1 || p.ServerPort > 65535 {
		return fmt.Errorf("the server port %d is not from 1 to 65535", p.ServerPort)
	}
	switch p.Type {
	case TypeVLESS:
		return requiredField(p.VLESS.UUID, "the VLESS UUID")
	case TypeVMess:
		return requiredField(p.VMess.UUID, "the VMess UUID")
	case TypeTrojan:
		return requiredField(p.Trojan.Password, "the Trojan password")
	case TypeShadowsocks:
		return p.Shadowsocks.validate()
	case TypeHysteria2:
		// The password may be empty, as the Qt client allowed.
		for _, item := range p.Hysteria2.ServerPorts {
			if ranges, err := hysteriaPortRanges(item); err != nil || len(ranges) != 1 || !strings.Contains(item, ":") {
				return fmt.Errorf("the port range %q is not written as 20000:30000", item)
			}
		}
	case TypeTUIC:
		if !isUUID(strings.TrimSpace(p.TUIC.UUID)) {
			return fmt.Errorf("the TUIC UUID %q is not a UUID; only TUIC v5 servers work", p.TUIC.UUID)
		}
	case TypeAnyTLS:
		return requiredField(p.AnyTLS.Password, "the AnyTLS password")
	}
	return nil
}

func (p Profile) validatePart() error {
	parts := []struct {
		kind string
		set  bool
	}{
		{TypeVLESS, p.VLESS != nil}, {TypeVMess, p.VMess != nil}, {TypeTrojan, p.Trojan != nil},
		{TypeShadowsocks, p.Shadowsocks != nil}, {TypeHysteria2, p.Hysteria2 != nil}, {TypeTUIC, p.TUIC != nil},
		{TypeSOCKS, p.SOCKS != nil}, {TypeHTTP, p.HTTP != nil}, {TypeAnyTLS, p.AnyTLS != nil},
	}
	known := false
	for _, part := range parts {
		switch {
		case part.kind == p.Type && !part.set:
			return fmt.Errorf("the %s settings are missing", p.Type)
		case part.kind == p.Type:
			known = true
		case part.set:
			return fmt.Errorf("a %s server carries %s settings", p.Type, part.kind)
		}
	}
	if !known {
		return fmt.Errorf("unknown server type %q", p.Type)
	}
	return nil
}

func requiredField(value, what string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is missing", what)
	}
	return nil
}

func (s *Shadowsocks) validate() error {
	if s.Method == "" {
		return errors.New("the Shadowsocks method is missing")
	}
	if !slices.Contains(shadowsocksMethods, s.Method) {
		return fmt.Errorf("unknown Shadowsocks method %q", s.Method)
	}
	if !slices.Contains(shadowsocksPlugins, s.Plugin) {
		return fmt.Errorf("the Shadowsocks plugin %q is not supported; use obfs-local or v2ray-plugin", s.Plugin)
	}
	keySize := 32
	switch s.Method {
	case "none":
		return nil
	case "2022-blake3-aes-128-gcm":
		keySize = 16
	case "2022-blake3-aes-256-gcm", "2022-blake3-chacha20-poly1305":
	default:
		if s.Password == "" {
			return errors.New("the Shadowsocks password is missing")
		}
		return nil
	}
	// 2022 methods take base64 keys of the cipher's size; AES ones may join several with ':' for multiple users.
	keys := strings.Split(s.Password, ":")
	if s.Method == "2022-blake3-chacha20-poly1305" && len(keys) > 1 {
		return errors.New("2022-blake3-chacha20-poly1305 takes a single key")
	}
	for _, key := range keys {
		decoded, err := base64.StdEncoding.DecodeString(key)
		if err != nil || len(decoded) != keySize || strings.ContainsFunc(key, unicode.IsSpace) {
			return fmt.Errorf("a %s key must be %d bytes in base64", s.Method, keySize)
		}
	}
	return nil
}

// isUUID accepts the forms sing-box's UUID parser (gofrs) does: canonical, 32 hex digits, braced or urn:uuid:.
func isUUID(text string) bool {
	switch len(text) {
	case 32, 36:
	case 34, 38:
		if text[0] != '{' || text[len(text)-1] != '}' {
			return false
		}
		text = text[1 : len(text)-1]
	case 41, 45:
		var ok bool
		if text, ok = strings.CutPrefix(text, "urn:uuid:"); !ok {
			return false
		}
	default:
		return false
	}
	for i := 0; i < len(text); i++ {
		dash := len(text) == 36 && (i == 8 || i == 13 || i == 18 || i == 23)
		if c := text[i]; dash && c != '-' || !dash && !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}
