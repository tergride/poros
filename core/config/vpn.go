package config

import (
	"errors"
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json/badoption"

	"PorosCore/internal/stb/stbkey"
	"PorosCore/profile"
)

// pemLines hands a PEM block to sing-box, which takes certificates and keys as lists; an empty one is none.
func pemLines(text string) badoption.Listable[string] {
	if text == "" {
		return nil
	}
	return badoption.Listable[string]{text}
}

// openVPNDefaultCiphers are what OpenVPN offers without data-ciphers, and sing-box too.
var openVPNDefaultCiphers = []string{"AES-256-GCM", "AES-128-GCM", "CHACHA20-POLY1305"}

// openVPNDataCiphers reads the cipher of a config as OpenVPN 2.6 does in TLS mode, where sing-box refuses it: the
// fallback for a server without cipher negotiation, and, without data-ciphers, one more cipher offered. OpenVPN|BY
// offers its belt alone.
func openVPNDataCiphers(p profile.Profile) (ciphers []string, fallback string) {
	o := p.OpenVPN
	ciphers, fallback = o.DataCiphers, o.DataCiphersFallback
	if o.Cipher == "" {
		return ciphers, fallback
	}
	if fallback == "" {
		fallback = o.Cipher
	}
	if len(ciphers) == 0 {
		if p.Type != profile.TypeOpenVPNBY {
			ciphers = slices.Clone(openVPNDefaultCiphers)
		}
		if !slices.ContainsFunc(ciphers, func(c string) bool { return strings.EqualFold(c, o.Cipher) }) {
			ciphers = append(ciphers, o.Cipher)
		}
	}
	return ciphers, fallback
}

func openVPNEndpoint(p profile.Profile, tag string) (option.Endpoint, error) {
	o := p.OpenVPN
	ciphers, fallback := openVPNDataCiphers(p)
	options := &option.OpenVPNClientEndpointOptions{
		ServerOptions:          option.ServerOptions{Server: ace(p.Server), ServerPort: uint16(p.ServerPort)},
		OpenVPNEndpointOptions: option.OpenVPNEndpointOptions{MTU: uint32(o.MTU)},
		Network:                o.Network,
		RemoteRandom:           o.RemoteRandom,
		Username:               o.Username,
		Password:               o.Password,
		StaticChallenge:        o.StaticChallenge,
		StaticChallengeEcho:    o.StaticChallengeEcho,
		DataCiphers:            ciphers,
		DataCiphersFallback:    fallback,
		Auth:                   o.Auth,
		Compression:            o.Compression,
		CompressionLZO:         o.CompressionLZO,
		AllowCompression:       o.AllowCompression,
		MSSFix:                 uint32(o.MSSFix),
		Fragment:               uint32(o.Fragment),
	}
	// sing-box tries servers in turn and needs the first among them once there are several.
	if len(o.Remotes) > 0 {
		options.Servers = []option.OpenVPNRemoteOptions{{ServerOptions: options.ServerOptions}}
		for _, r := range o.Remotes {
			options.Servers = append(options.Servers, option.OpenVPNRemoteOptions{
				ServerOptions: option.ServerOptions{Server: ace(r.Server), ServerPort: uint16(r.Port)},
				Network:       r.Network,
			})
		}
		options.ServerOptions = option.ServerOptions{}
	}
	var err error
	for _, d := range []struct {
		text   string
		target *badoption.Duration
	}{{o.PingInterval, &options.PingInterval}, {o.PingRestart, &options.PingRestart}, {o.RenegotiateInterval, &options.RenegotiateInterval}} {
		if *d.target, err = duration(d.text); err != nil {
			return option.Endpoint{}, err
		}
	}
	tls := &option.OpenVPNOutboundTLSOptions{
		ServerName:           o.VerifyName,
		ServerNameType:       o.VerifyNameType,
		Certificate:          pemLines(o.CA),
		ClientCertificate:    pemLines(o.Cert),
		ClientKey:            pemLines(o.Key),
		PeerFingerprint:      o.PeerFingerprint,
		RemoteCertificateTLS: o.RemoteCertTLS,
		VersionMin:           o.TLSVersionMin,
		VersionMax:           o.TLSVersionMax,
	}
	if w := o.ControlWrap; w.Type != "" {
		tls.ControlWrap = &option.OpenVPNControlWrapOptions{Type: w.Type, Key: pemLines(w.Key), Direction: w.Direction}
	}
	if p.Type == profile.TypeOpenVPNBY {
		// BTLS is defined for TLS 1.2 alone, and the suites in tls-cipher hand the control channel to it.
		tls.Cipher, tls.VersionMin, tls.VersionMax = strings.Join(o.TLSCipher, ":"), "1.2", "1.2"
		key, err := openKey(p)
		if err != nil {
			return option.Endpoint{}, err
		}
		tls.ClientKey = pemLines(key)
	}
	options.TLS = tls
	return option.Endpoint{Type: C.TypeOpenVPNClient, Tag: tag, Options: options}, nil
}

// KeyPasswordError is a key of OpenVPN|BY that its password does not open, or one without the password it needs.
type KeyPasswordError struct {
	// Server is the name of the profile.
	Server string
	Err    error
}

func (e *KeyPasswordError) Error() string { return e.Err.Error() }
func (e *KeyPasswordError) Unwrap() error { return e.Err }

// openKey opens the bign key of an OpenVPN|BY profile with its password: sing-box takes the key as it is, so it gets
// it open, in memory only.
func openKey(p profile.Profile) (string, error) {
	o := p.OpenVPN
	if o.Key == "" {
		return "", nil
	}
	key, err := stbkey.ParsePrivateKey([]byte(o.Key), []byte(o.KeyPassword))
	switch {
	case errors.Is(err, stbkey.ErrPasswordRequired) || errors.Is(err, stbkey.ErrBadPassword):
		return "", &KeyPasswordError{Server: p.Name, Err: err}
	case err != nil:
		return "", fmt.Errorf("the client key of OpenVPN|BY: %w", err)
	}
	data, err := stbkey.EncodePEM(key)
	return string(data), err
}

func openConnectEndpoint(p profile.Profile, tag string) option.Endpoint {
	o := p.OpenConnect
	return option.Endpoint{Type: C.TypeOpenConnect, Tag: tag, Options: &option.OpenConnectEndpointOptions{
		Server:    "https://" + net.JoinHostPort(ace(p.Server), strconv.Itoa(p.ServerPort)) + o.Path,
		Flavor:    o.Flavor,
		Username:  o.Username,
		Password:  o.Password,
		AuthGroup: o.AuthGroup,
		NoUDP:     o.NoUDP,
		MTU:       uint32(o.MTU),
		TLS: option.OpenConnectTLSOptions{
			Insecure:             o.Insecure,
			PeerFingerprint:      o.PeerFingerprint,
			CertificateAuthority: pemLines(o.CA),
			ClientCertificate:    pemLines(o.Cert),
			ClientKey:            pemLines(o.Key),
			ClientKeyPassword:    o.KeyPassword,
		},
	}}
}

func masqueEndpoint(p profile.Profile, tag string, tune Tuning) (option.Endpoint, error) {
	m := p.MASQUE
	tls, err := singBoxTLS(m.TLS, p.Server, m.Version == 0 || m.Version == 3, Tuning{Insecure: tune.Insecure})
	if err != nil {
		return option.Endpoint{}, err
	}
	return option.Endpoint{Type: C.TypeMASQUEClient, Tag: tag, Options: &option.MASQUEClientEndpointOptions{
		HTTP2Options:          tune.QUIC.HTTP2Options,
		HTTP3Options:          tune.QUIC,
		ServerOptions:         option.ServerOptions{Server: ace(p.Server), ServerPort: uint16(p.ServerPort)},
		MASQUEEndpointOptions: option.MASQUEEndpointOptions{MTU: uint32(m.MTU)},
		// MASQUE refuses dials until its tunnel is up and tells no one when; on demand they wait for it instead.
		OnDemand:                    true,
		Username:                    m.Username,
		Password:                    m.Password,
		OutboundTLSOptionsContainer: option.OutboundTLSOptionsContainer{TLS: tls},
		Path:                        m.Path,
		Headers:                     httpHeader(m.Headers, ""),
		Version:                     m.Version,
	}}, nil
}
