package config

import (
	"bytes"
	"context"
	stdjson "encoding/json"

	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json"

	"PorosCore/profile"
)

// Export writes a server as the core that runs it reads it, for the user to copy: an Xray outbound for a server only
// Xray runs, a sing-box outbound or endpoint otherwise. Names stay names, nothing is resolved and no interface is
// bound: those belong to a connection, not to the server.
func Export(p profile.Profile, links []profile.Profile, realityOnXray bool) (onXray bool, data []byte, err error) {
	if err = validateServer(p, links); err != nil {
		return false, nil, err
	}
	if p.Type == profile.TypeChain && RunsOnXray(Entry(p, links), realityOnXray) {
		return false, nil, profile.Unsupported("a chain that starts on Xray takes both cores: export its servers one by one")
	}
	if RunsOnXray(p, realityOnXray) {
		outbound, err := xrayVLESS(p, TagProxy, "", nil)
		if err != nil {
			return true, nil, err
		}
		data, err = stdjson.MarshalIndent(outbound, "", "  ")
		return true, data, err
	}
	built, err := serverParts(p, links, TagProxy, nil, Tuning{})
	if err != nil {
		return false, nil, err
	}
	// One outbound or endpoint stands alone; several, as a chain or Shadowsocks over ShadowTLS, come as the part of a
	// config.
	var value any = &option.Options{Outbounds: built.outbounds, Endpoints: built.endpoints}
	switch {
	case len(built.outbounds) == 1 && len(built.endpoints) == 0:
		value = &built.outbounds[0]
	case len(built.outbounds) == 0 && len(built.endpoints) == 1:
		value = &built.endpoints[0]
	}
	compact, err := json.MarshalContext(context.Background(), value)
	if err != nil {
		return false, nil, err
	}
	var indented bytes.Buffer
	if err = stdjson.Indent(&indented, compact, "", "  "); err != nil {
		return false, nil, err
	}
	return false, indented.Bytes(), nil
}
