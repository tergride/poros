package engine

import (
	"fmt"

	"github.com/sagernet/bbolt"

	"PorosCore/config"
	"PorosCore/internal/boxmain"
	"PorosCore/internal/jsonrpc"
	"PorosCore/profile"
)

// missingLink is a server of a chain that is no longer in its group.
type missingLink string

func (m missingLink) Error() string {
	return fmt.Sprintf("the server %q of the chain is no longer in its group", string(m))
}

// serverWithLinks reads a server and, for a chain, its servers in its order, in one transaction. found is false for
// a missing server; a missing server of the chain is a missingLink error.
func (e *Engine) serverWithLinks(id string) (p profile.Profile, links []profile.Profile, found bool, err error) {
	err = e.db.View(func(tx *bbolt.Tx) error {
		place, err := findServer(tx, id)
		if err != nil || !place.found() {
			return err
		}
		p, found = place.servers[place.index].Profile, true
		if p.Type != profile.TypeChain {
			return nil
		}
		for _, linkID := range p.Chain.Servers {
			link, err := findServer(tx, linkID)
			if err != nil {
				return err
			}
			if !link.found() {
				return missingLink(linkID)
			}
			links = append(links, link.servers[link.index].Profile)
		}
		return nil
	})
	return p, links, found, err
}

// checkChain refuses a chain through a server that is gone, through itself, or through a server that is no server
// of its own: another chain or Direct.
func checkChain(s *serverStore, id string, p profile.Profile) error {
	if p.Type != profile.TypeChain {
		return nil
	}
	for _, linkID := range p.Chain.Servers {
		if linkID == id && id != "" {
			return jsonrpc.Failed(ReasonInvalidValue, "a chain cannot go through itself")
		}
		g, i := s.locate(linkID)
		if g < 0 {
			return serverNotFound(linkID)
		}
		switch s.servers[s.groups[g].ID][i].Profile.Type {
		case profile.TypeChain:
			return jsonrpc.Failed(ReasonInvalidValue, "a chain cannot go through another chain")
		case profile.TypeDirect:
			return jsonrpc.Failed(ReasonInvalidValue, "a chain goes through servers, and Direct is none")
		}
	}
	return nil
}

// checkCustom has sing-box read a custom outbound in a connection of its own, as a start would.
func checkCustom(p profile.Profile) error {
	if p.Type != profile.TypeCustom {
		return nil
	}
	configs, err := config.Build(config.Input{Server: p, Preset: config.Preset{Name: "check", DefaultOutbound: config.OutboundProxy}, MixedPort: 1})
	if err == nil {
		var data []byte
		if data, err = configs.SingBoxJSON(); err == nil {
			err = boxmain.Check(data)
		}
	}
	if err != nil {
		return jsonrpc.Failed(ReasonInvalidValue, "sing-box refuses the custom outbound: %v", err)
	}
	return nil
}
