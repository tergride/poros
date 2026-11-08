package profile

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// isOpenVPNConfig tells a .ovpn file: a remote and the marks of a client config.
func isOpenVPNConfig(text []byte) bool {
	hasRemote, isClient := false, false
	for raw := range bytes.Lines(text) {
		fields := strings.Fields(string(raw))
		if len(fields) == 0 {
			continue
		}
		switch strings.ToLower(fields[0]) {
		case "remote":
			hasRemote = true
		case "client", "tls-client", "dev", "<ca>", "<connection>":
			isClient = true
		}
	}
	return hasRemote && isClient
}

// openVPNLine is a directive of a .ovpn file, or an inline block such as <ca>...</ca>, whose text is in body.
type openVPNLine struct {
	args []string
	body string
}

// splitOpenVPNConfig reads the directives as OpenVPN does: '#' and ';' start comments, quotes group an argument.
func splitOpenVPNConfig(text []byte) ([]openVPNLine, error) {
	var lines []openVPNLine
	var block *openVPNLine
	var body []string
	for raw := range bytes.Lines(text) {
		line := strings.TrimSpace(string(raw))
		if block != nil {
			if strings.EqualFold(line, "</"+block.args[0][1:len(block.args[0])-1]+">") {
				block.body = strings.Join(body, "\n")
				lines = append(lines, *block)
				block, body = nil, nil
				continue
			}
			body = append(body, line)
			continue
		}
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		if strings.HasPrefix(line, "<") && strings.HasSuffix(line, ">") && !strings.HasPrefix(line, "</") {
			block = &openVPNLine{args: []string{strings.ToLower(line)}}
			continue
		}
		lines = append(lines, openVPNLine{args: openVPNArgs(line)})
	}
	if block != nil {
		return nil, fmt.Errorf("the block %s has no end", block.args[0])
	}
	return lines, nil
}

func openVPNArgs(line string) []string {
	var args []string
	var current strings.Builder
	quote, inArg := byte(0), false
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case quote != 0 && c == quote:
			quote = 0
		case quote != 0:
			current.WriteByte(c)
		case c == '"' || c == '\'':
			quote, inArg = c, true
		case c == '\\' && i+1 < len(line):
			i++
			current.WriteByte(line[i])
			inArg = true
		case c == ' ' || c == '\t':
			if inArg {
				args = append(args, current.String())
				current.Reset()
				inArg = false
			}
		case (c == '#' || c == ';') && !inArg:
			i = len(line)
		default:
			current.WriteByte(c)
			inArg = true
		}
	}
	if inArg {
		args = append(args, current.String())
	}
	if len(args) > 0 {
		args[0] = strings.ToLower(args[0])
	}
	return args
}

// openVPNNetwork reads proto: tcp-client is tcp, and the server forms are no client's.
func openVPNNetwork(value string) (string, bool) {
	value = strings.ToLower(value)
	value = strings.TrimSuffix(value, "-client")
	return value, value != "" && slices.Contains(openVPNNetworks, value)
}

// openVPNDirection reads the direction argument of tls-auth, secret and key-direction.
func openVPNDirection(value string) string {
	switch value {
	case "1":
		return "client"
	case "0":
		return "server"
	}
	return ""
}

// parseOpenVPNConfig reads a .ovpn file as one server. Keys and certificates must be inline: a subscription or a
// pasted file has no folder to read the files it names from.
func parseOpenVPNConfig(text []byte, d *Document) {
	d.Format = FormatOpenVPN
	p, err := readOpenVPNConfig(text)
	d.add(1, "", p, err)
}

func readOpenVPNConfig(text []byte) (Profile, error) {
	lines, err := splitOpenVPNConfig(text)
	if err != nil {
		return Profile{}, err
	}
	o := &OpenVPN{}
	var remotes []OpenVPNRemote
	port, name, keyDirection := 1194, "", ""
	fileError := func(directive, file string) error {
		return fmt.Errorf("the config reads %s from the file %q: put the file inline as <%s>...</%s>", directive, file, directive, directive)
	}
	for _, line := range lines {
		args := line.args
		value := ""
		if len(args) > 1 {
			value = args[1]
		}
		switch args[0] {
		case "<connection>":
			sub, err := splitOpenVPNConfig([]byte(line.body))
			if err != nil {
				return Profile{}, err
			}
			r, network := OpenVPNRemote{}, ""
			for _, item := range sub {
				switch item.args[0] {
				case "remote":
					r = openVPNRemote(item.args)
				case "proto":
					if len(item.args) > 1 {
						network, _ = openVPNNetwork(item.args[1])
					}
				case "port", "rport":
					if len(item.args) > 1 {
						r.Port, _ = strconv.Atoi(item.args[1])
					}
				}
			}
			if r.Network == "" {
				r.Network = network
			}
			if r.Server != "" {
				remotes = append(remotes, r)
			}
		case "<ca>":
			o.CA = line.body
		case "<cert>":
			o.Cert = line.body
		case "<key>":
			o.Key = line.body
		case "<tls-auth>", "<tls-crypt>", "<tls-crypt-v2>":
			o.ControlWrap.Type = strings.ReplaceAll(strings.Trim(args[0], "<>"), "-", "_")
			o.ControlWrap.Key = line.body
		case "<auth-user-pass>":
			credentials := strings.Split(line.body, "\n")
			o.Username = credentials[0]
			if len(credentials) > 1 {
				o.Password = credentials[1]
			}
		case "<peer-fingerprint>":
			o.PeerFingerprint = append(o.PeerFingerprint, strings.Fields(line.body)...)
		case "<pkcs12>", "pkcs12":
			return Profile{}, unsupported("PKCS#12 keys are not supported; export the certificate and key as PEM")
		case "mode":
			if !strings.EqualFold(value, "p2p") {
				return Profile{}, errors.New("this is the config of an OpenVPN server, not of a client")
			}
		case "dev", "dev-type":
			if strings.HasPrefix(strings.ToLower(value), "tap") {
				return Profile{}, unsupported("OpenVPN over a TAP device is not supported; only TUN works")
			}
		case "remote":
			remotes = append(remotes, openVPNRemote(args))
		case "proto":
			network, ok := openVPNNetwork(value)
			if !ok {
				return Profile{}, unsupported("the OpenVPN protocol %q is not supported", value)
			}
			o.Network = network
		case "port", "rport":
			if number, err := strconv.Atoi(value); err == nil {
				port = number
			}
		case "remote-random":
			o.RemoteRandom = true
		case "auth-user-pass":
			if value != "" {
				return Profile{}, fileError("auth-user-pass", value)
			}
		case "static-challenge":
			o.StaticChallenge = value
			o.StaticChallengeEcho = len(args) > 2 && args[2] == "1"
		case "secret", "<secret>":
			return Profile{}, unsupported("OpenVPN's static key mode, which OpenVPN itself is retiring, is not supported")
		case "ca", "cert", "key", "tls-auth", "tls-crypt", "tls-crypt-v2":
			if value != "" && value != "[inline]" {
				return Profile{}, fileError(args[0], value)
			}
			if args[0] == "tls-auth" && len(args) > 2 {
				keyDirection = openVPNDirection(args[2])
			}
		case "key-direction":
			keyDirection = openVPNDirection(value)
		case "peer-fingerprint":
			if value != "" {
				o.PeerFingerprint = append(o.PeerFingerprint, value)
			}
		case "verify-x509-name":
			o.VerifyName = value
			o.VerifyNameType = "subject"
			if len(args) > 2 {
				o.VerifyNameType = args[2]
			}
		case "remote-cert-tls":
			o.RemoteCertTLS = value
		case "tls-version-min":
			o.TLSVersionMin = value
		case "tls-version-max":
			o.TLSVersionMax = value
		case "cipher":
			o.Cipher = value
		case "data-ciphers", "ncp-ciphers":
			o.DataCiphers = strings.Split(value, ":")
		case "data-ciphers-fallback":
			o.DataCiphersFallback = value
		case "auth":
			o.Auth = value
		case "compress":
			o.Compression = value
			if value == "" {
				o.Compression = "stub"
			}
		case "comp-lzo":
			o.CompressionLZO = strings.ToLower(value)
			if value == "" {
				o.CompressionLZO = "adaptive"
			}
		case "allow-compression":
			o.AllowCompression = value
		case "tun-mtu":
			o.MTU, _ = strconv.Atoi(value)
		case "mssfix":
			o.MSSFix, _ = strconv.Atoi(value)
		case "fragment":
			o.Fragment, _ = strconv.Atoi(value)
		case "keepalive":
			if len(args) > 2 {
				o.PingInterval, o.PingRestart = value+"s", args[2]+"s"
			}
		case "ping":
			o.PingInterval = value + "s"
		case "ping-restart":
			o.PingRestart = value + "s"
		case "reneg-sec":
			o.RenegotiateInterval = value + "s"
		case "setenv":
			if strings.EqualFold(value, "FRIENDLY_NAME") && len(args) > 2 {
				name = args[2]
			}
		}
	}
	if o.ControlWrap.Type == "tls_auth" {
		o.ControlWrap.Direction = keyDirection
	}
	if name == "" {
		name = openVPNFriendlyName(text)
	}
	if len(remotes) == 0 {
		return Profile{}, errors.New("the config names no remote server")
	}
	// The first remote is the profile's server, and its protocol the profile's; the others keep their own where it
	// differs.
	effective := func(network string) string {
		if network == "" {
			return "udp"
		}
		return network
	}
	global := o.Network
	for i := range remotes {
		if remotes[i].Port == 0 {
			remotes[i].Port = port
		}
		if remotes[i].Network == "" {
			remotes[i].Network = global
		}
	}
	first := remotes[0]
	o.Network = first.Network
	for _, r := range remotes[1:] {
		if effective(r.Network) == effective(o.Network) {
			r.Network = ""
		} else {
			r.Network = effective(r.Network)
		}
		o.Remotes = append(o.Remotes, r)
	}
	return Profile{Type: TypeOpenVPN, Name: name, Server: first.Server, ServerPort: first.Port, OpenVPN: o}, nil
}

// openVPNRemote reads remote host [port] [proto].
func openVPNRemote(args []string) OpenVPNRemote {
	var r OpenVPNRemote
	if len(args) > 1 {
		r.Server, _ = normalizeLinkHost(args[1])
	}
	if len(args) > 2 {
		r.Port, _ = strconv.Atoi(args[2])
	}
	if len(args) > 3 {
		r.Network, _ = openVPNNetwork(args[3])
	}
	return r
}

// openVPNFriendlyName reads the name OpenVPN Connect profiles carry in a comment.
func openVPNFriendlyName(text []byte) string {
	for raw := range bytes.Lines(text) {
		line := strings.TrimSpace(string(raw))
		if name, ok := strings.CutPrefix(line, "# OVPN_FRIENDLY_PROFILE_NAME="); ok {
			return strings.TrimSpace(name)
		}
	}
	return ""
}
