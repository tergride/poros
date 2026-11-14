package engine

import (
	"encoding/base64"
	"net/url"
	"strings"

	"PorosCore/internal/jsonrpc"
	"PorosCore/profile"
)

// Links from a browser or a messenger: poros://addsub/<base64 of the URL, with the name as its fragment>,
// poros://route/<preset>, and poros://add/<server>, which only the advanced mode adds. Throne's throne:// links
// of a subscription and a server look the same. Nothing is added here: the interface asks the user first.

// ParseLink tells what a link asks to add.
func (e *Engine) ParseLink(p OutsideLink) (LinkInfo, error) {
	text := strings.TrimSpace(p.Link)
	scheme, rest, ok := strings.Cut(text, "://")
	scheme = strings.ToLower(scheme)
	if !ok || (scheme != "poros" && scheme != "throne") {
		return LinkInfo{}, jsonrpc.Failed(ReasonInvalidLink, "not a poros:// link")
	}
	command, data, _ := strings.Cut(rest, "/")
	switch strings.ToLower(command) {
	case "addsub":
		return parseSubscriptionLink(data)
	case "add":
		p, err := profile.ParseLink(text)
		if err != nil {
			return LinkInfo{}, jsonrpc.Failed(ReasonInvalidLink, "%s", err.Error())
		}
		server := serverRecord{Profile: p}.view("")
		return LinkInfo{Kind: LinkServer, Server: &server, Profile: &p}, nil
	case "route":
		if scheme != "poros" {
			return LinkInfo{}, jsonrpc.Failed(ReasonInvalidLink, "Throne routing profiles are not Poros presets")
		}
		link, err := decodePresetLink("poros://route/" + data)
		if err != nil {
			return LinkInfo{}, err
		}
		preset := link.copy(link.Name)
		return LinkInfo{Kind: LinkPreset, Preset: &preset}, nil
	}
	return LinkInfo{}, jsonrpc.Failed(ReasonInvalidLink, "unknown poros:// link")
}

func parseSubscriptionLink(data string) (LinkInfo, error) {
	encoded := strings.TrimRight(strings.TrimSpace(data), "=")
	var decoded []byte
	var err error
	for _, encoding := range []*base64.Encoding{base64.RawURLEncoding, base64.RawStdEncoding} {
		if decoded, err = encoding.DecodeString(encoded); err == nil {
			break
		}
	}
	if err != nil {
		return LinkInfo{}, jsonrpc.Failed(ReasonInvalidLink, "the subscription link is damaged")
	}
	link := strings.TrimSpace(string(decoded))
	name := ""
	if base, fragment, found := strings.Cut(link, "#"); found {
		link = base
		if name, err = url.PathUnescape(fragment); err != nil {
			name = fragment
		}
	}
	if err := checkSubscriptionURL(link); err != nil {
		return LinkInfo{}, err
	}
	return LinkInfo{Kind: LinkSubscription, URL: link, Name: strings.TrimSpace(name)}, nil
}
