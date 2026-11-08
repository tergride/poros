package config

import (
	"net"
	"strconv"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json/badoption"

	"PorosCore/profile"
)

// pemLines hands a PEM block to sing-box, which takes certificates and keys as lists; an empty one is none.
func pemLines(text string) badoption.Listable[string] {
	if text == "" {
		return nil
	}
	return badoption.Listable[string]{text}
}

func openVPNEndpoint(p profile.Profile, tag string) (option.Endpoint, error) {
	o := p.OpenVPN
	options := &option.OpenVPNClientEndpointOptions{
		ServerOptions:          option.ServerOptions{Server: ace(p.Server), ServerPort: uint16(p.ServerPort)},
		OpenVPNEndpointOptions: option.OpenVPNEndpointOptions{MTU: uint32(o.MTU)},
		Network:                o.Network,
		RemoteRandom:           o.RemoteRandom,
		Username:               o.Username,
		Password:               o.Password,
		StaticChallenge:        o.StaticChallenge,
		StaticChallengeEcho:    o.StaticChallengeEcho,
		Cipher:                 o.Cipher,
		DataCiphers:            o.DataCiphers,
		DataCiphersFallback:    o.DataCiphersFallback,
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
	options.TLS = tls
	return option.Endpoint{Type: C.TypeOpenVPNClient, Tag: tag, Options: options}, nil
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

func masqueEndpoint(p profile.Profile, tag string) (option.Endpoint, error) {
	m := p.MASQUE
	tls, err := singBoxTLS(m.TLS, m.Version == 0 || m.Version == 3, false)
	if err != nil {
		return option.Endpoint{}, err
	}
	return option.Endpoint{Type: C.TypeMASQUEClient, Tag: tag, Options: &option.MASQUEClientEndpointOptions{
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
