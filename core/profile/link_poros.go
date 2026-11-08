package profile

import (
	"encoding/json"
	"errors"
	"strings"
)

// parsePorosLink reads poros://add/<base64url of the profile's JSON>: the stored form of a profile, which says
// everything a share link of its protocol may not. The other poros:// links name no server.
func parsePorosLink(link string) (Profile, error) {
	_, rest, _ := strings.Cut(link, "://")
	command, data, _ := strings.Cut(rest, "/")
	if !strings.EqualFold(command, "add") {
		return Profile{}, ErrNotLink
	}
	raw, ok := decodeLinkBase64(data)
	if !ok {
		return Profile{}, errors.New("the poros://add/ link is damaged")
	}
	var p Profile
	if err := json.Unmarshal(raw, &p); err != nil {
		return Profile{}, errors.New("the poros://add/ link holds no server")
	}
	return p, nil
}
