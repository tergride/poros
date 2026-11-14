package profile

import "strings"

// parseSOCKSLink reads socks://, socks5://, socks4:// and socks4a://user:password@host:port?version=#name.
func parseSOCKSLink(link string) (Profile, error) {
	l, err := parseShareLink(link)
	if err != nil {
		return Profile{}, err
	}
	port, err := l.serverPort(1080)
	if err != nil {
		return Profile{}, err
	}
	s := &SOCKS{Version: "5"}
	switch l.scheme {
	case "socks4":
		s.Version = "4"
	case "socks4a":
		s.Version = "4a"
	}
	if l.query.has("version") {
		// Anything but 4 and 4a is SOCKS5, the core's default.
		switch version := strings.ToLower(l.query.trimmed("version")); version {
		case "4", "4a":
			s.Version = version
		default:
			s.Version = "5"
		}
	}
	s.Username, s.Password = linkProxyAuth(l.user, l.password)
	return Profile{Type: TypeSOCKS, Name: l.name, Server: l.host, ServerPort: port, SOCKS: s}, nil
}

// linkProxyAuth undoes v2rayN's base64 of "user:pass" in the user name. A plain name such as "user" is valid
// base64 too, so only a decoded colon counts.
func linkProxyAuth(user, password string) (string, string) {
	if password != "" || user == "" {
		return user, password
	}
	if decoded, ok := decodeLinkBase64(user); ok {
		if decodedUser, decodedPassword, found := strings.Cut(string(decoded), ":"); found {
			return decodedUser, decodedPassword
		}
	}
	return user, password
}
