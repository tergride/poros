package profile

import (
	"encoding/json"
	"errors"
	"strings"
)

// parseThroneLink reads throne://add/<base64url of the profile's JSON>, the deep link Throne shares a server with. Its
// JSON is a sing-box outbound with the profile's name as the tag, so the sing-box reader takes it. Throne's other
// links name no server.
func parseThroneLink(link string) (Profile, error) {
	_, rest, _ := strings.Cut(link, "://")
	command, data, _ := strings.Cut(rest, "/")
	if !strings.EqualFold(command, "add") {
		return Profile{}, ErrNotLink
	}
	raw, ok := decodeLinkBase64(data)
	if !ok || !json.Valid(raw) {
		return Profile{}, errors.New("the throne://add/ link is damaged")
	}
	p, tag, proxy, err := singBoxOutbound(raw)
	if err != nil {
		return Profile{}, err
	}
	if !proxy {
		return Profile{}, unsupported("the throne://add/ link holds no server Poros can run")
	}
	p.Name = tag
	return p, nil
}
