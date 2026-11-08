package config

import (
	"bytes"
	"context"
	stdjson "encoding/json"

	"github.com/sagernet/sing/common/json"

	"PorosCore/profile"
)

// Export writes a server as the core that runs it reads it, for the user to copy: an Xray outbound for a server only
// Xray runs, a sing-box outbound otherwise. Names stay names, nothing is resolved and no interface is bound: those
// belong to a connection, not to the server.
func Export(p profile.Profile, realityOnXray bool) (onXray bool, data []byte, err error) {
	if err = p.Validate(); err != nil {
		return false, nil, err
	}
	if RunsOnXray(p, realityOnXray) {
		outbound, err := xrayVLESS(p, TagProxy, "", nil)
		if err != nil {
			return true, nil, err
		}
		data, err = stdjson.MarshalIndent(outbound, "", "  ")
		return true, data, err
	}
	outbound, err := singBoxOutbound(p, TagProxy, false)
	if err != nil {
		return false, nil, err
	}
	compact, err := json.MarshalContext(context.Background(), &outbound)
	if err != nil {
		return false, nil, err
	}
	var indented bytes.Buffer
	if err = stdjson.Indent(&indented, compact, "", "  "); err != nil {
		return false, nil, err
	}
	return false, indented.Bytes(), nil
}
