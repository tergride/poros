package engine

import (
	"fmt"

	"github.com/sagernet/bbolt"

	"PorosCore/internal/jsonrpc"
	"PorosCore/profile"
)

// missingLink is a server of a chain that is no longer in its group.
type missingLink string

func (m missingLink) Error() string {
	return fmt.Sprintf("the server %q of the chain is no longer in its group", string(m))
}

// serverWithLinks reads what a server connects with, as resolveServer gives it, in one transaction.
func (e *Engine) serverWithLinks(id string) (p profile.Profile, links []profile.Profile, found bool, err error) {
	err = e.db.View(func(tx *bbolt.Tx) error {
		store, err := loadServers(tx)
		if err != nil {
			return err
		}
		p, links, found, err = resolveServer(store, id)
		return err
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
	if err := checkBuild(p, nil, false); err != nil {
		return jsonrpc.Failed(ReasonInvalidValue, "sing-box refuses the custom outbound: %v", err)
	}
	return nil
}
