package config

import (
	"context"
	stdjson "encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"sync"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json"

	"PorosCore/profile"
)

// Entry is the server a connection dials first: the first link of a chain, or the server itself. Only it may run on
// Xray, which dials the network itself.
func Entry(p profile.Profile, links []profile.Profile) profile.Profile {
	if p.Type == profile.TypeChain && len(links) > 0 {
		return links[0]
	}
	return p
}

func linkTag(tag string, i int) string {
	return tag + "-" + strconv.Itoa(i+1)
}

// dialer is the part of a server that dials the network: its endpoint, or its last outbound, as ShadowTLS carries
// the Shadowsocks outbound before it.
func (b boxServer) dialer() any {
	if len(b.endpoints) > 0 {
		return b.endpoints[0].Options
	}
	return b.outbounds[len(b.outbounds)-1].Options
}

// setDetour dials through another outbound: every sing-box option struct embeds DialerOptions.
func setDetour(options any, detour string) error {
	value := reflect.ValueOf(options)
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	field := value.FieldByName("DialerOptions")
	if !field.IsValid() {
		return fmt.Errorf("a %T cannot be dialled through another server", options)
	}
	field.FieldByName("Detour").SetString(detour)
	return nil
}

// chainServer builds the links of a chain, each under its own tag and dialled through the one before; the last takes
// tag. entry is the bridge to the first link when Xray runs it.
func chainServer(links []profile.Profile, tag string, entry *option.Outbound, hardened bool) (boxServer, error) {
	if len(links) < 2 {
		return boxServer{}, errors.New("a chain needs at least two servers")
	}
	var built boxServer
	previous := ""
	for i, link := range links {
		current := linkTag(tag, i)
		if i == len(links)-1 {
			current = tag
		}
		var part boxServer
		var err error
		switch {
		case i == 0 && entry != nil:
			part = boxServer{outbounds: []option.Outbound{*entry}}
		case link.Type == profile.TypeChain:
			return boxServer{}, profile.Unsupported("a chain cannot hold another chain")
		case link.Type == profile.TypeDirect:
			return boxServer{}, profile.Unsupported("a chain goes through servers, and Direct is none")
		case link.VLESS != nil && link.VLESS.NeedsXray():
			return boxServer{}, profile.Unsupported("a server only Xray runs can be only the first in a chain: Xray dials the network itself")
		default:
			// Only the first link meets the network, where fragments of the TLS handshake help.
			part, err = singBoxServer(link, current, hardened && i == 0)
		}
		if err != nil {
			return boxServer{}, err
		}
		if previous != "" {
			if err = setDetour(part.dialer(), previous); err != nil {
				return boxServer{}, err
			}
		}
		built.outbounds = append(built.outbounds, part.outbounds...)
		built.endpoints = append(built.endpoints, part.endpoints...)
		previous = current
	}
	return built, nil
}

var endpointTypes = sync.OnceValue(func() map[string]bool {
	types := map[string]bool{}
	for _, kind := range include.EndpointRegistry().OptionTypes() {
		types[kind] = true
	}
	return types
})

// customServer reads a custom outbound or endpoint as sing-box does, under tag.
func customServer(raw stdjson.RawMessage, tag string) (boxServer, error) {
	var head struct {
		Type string `json:"type"`
	}
	if err := stdjson.Unmarshal(raw, &head); err != nil {
		return boxServer{}, errors.New("the custom outbound is not a JSON object")
	}
	ctx := include.Context(context.Background())
	if endpointTypes()[head.Type] {
		var endpoint option.Endpoint
		if err := json.UnmarshalContext(ctx, raw, &endpoint); err != nil {
			return boxServer{}, fmt.Errorf("sing-box cannot read the custom endpoint: %w", err)
		}
		endpoint.Tag = tag
		return boxServer{endpoints: []option.Endpoint{endpoint}}, nil
	}
	var outbound option.Outbound
	if err := json.UnmarshalContext(ctx, raw, &outbound); err != nil {
		return boxServer{}, fmt.Errorf("sing-box cannot read the custom outbound: %w", err)
	}
	outbound.Tag = tag
	return boxServer{outbounds: []option.Outbound{outbound}}, nil
}

func directServer(tag string) boxServer {
	return boxServer{outbounds: []option.Outbound{{Type: C.TypeDirect, Tag: tag, Options: &option.DirectOutboundOptions{}}}}
}

// entryTag is the tag of the server a connection dials first: the first link of a chain, or the server itself.
func entryTag(p profile.Profile, tag string) string {
	if p.Type == profile.TypeChain {
		return linkTag(tag, 0)
	}
	return tag
}

// serverParts builds a server under tag: a chain from its links, the bridge to Xray, or what sing-box runs.
func serverParts(p profile.Profile, links []profile.Profile, tag string, bridge *option.Outbound, hardened bool) (boxServer, error) {
	switch {
	case p.Type == profile.TypeChain:
		return chainServer(links, tag, bridge, hardened)
	case bridge != nil:
		return boxServer{outbounds: []option.Outbound{*bridge}}, nil
	}
	return singBoxServer(p, tag, hardened)
}

// validateServer checks a server and, for a chain, its links: the servers it names, in its order.
func validateServer(p profile.Profile, links []profile.Profile) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if p.Type != profile.TypeChain {
		return nil
	}
	if len(links) != len(p.Chain.Servers) {
		return errors.New("a server of the chain is gone")
	}
	for _, link := range links {
		if err := link.Validate(); err != nil {
			return err
		}
	}
	return nil
}
