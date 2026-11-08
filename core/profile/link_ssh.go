package profile

import "strings"

// parseSSHLink reads ssh://user:password@host:port?private_key=&host_key=#name. Throne writes the user and password in
// the query and the keys in base64, a list of them comma-separated; the standard userinfo is read too.
func parseSSHLink(link string) (Profile, error) {
	l, err := parseShareLink(link)
	if err != nil {
		return Profile{}, err
	}
	q := l.query
	port, err := l.serverPort(22)
	if err != nil {
		return Profile{}, err
	}
	s := &SSH{
		User:                 l.user,
		Password:             l.password,
		PrivateKeyPassphrase: q.get("private_key_passphrase"),
		ClientVersion:        q.trimmed("client_version"),
	}
	if q.has("user") {
		s.User = q.get("user")
	}
	if q.has("password") {
		s.Password = q.get("password")
	}
	if key, ok := decodeLinkBase64(q.get("private_key")); ok {
		s.PrivateKey = strings.TrimSpace(string(key))
	}
	s.HostKey = sshLinkItems(q.get("host_key"))
	s.HostKeyAlgorithms = sshLinkItems(q.get("host_key_algorithms"))
	return Profile{Type: TypeSSH, Name: l.name, Server: l.host, ServerPort: port, SSH: s}, nil
}

// sshLinkItems decodes a comma list of base64 items; one that is not base64 is dropped.
func sshLinkItems(text string) []string {
	var items []string
	for _, item := range linkList(text) {
		if decoded, ok := decodeLinkBase64(item); ok && strings.TrimSpace(string(decoded)) != "" {
			items = append(items, strings.TrimSpace(string(decoded)))
		}
	}
	return items
}
