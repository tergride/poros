package engine

import (
	"errors"

	"github.com/sagernet/bbolt"

	"PorosCore/config"
	"PorosCore/internal/jsonrpc"
	"PorosCore/profile"
)

// profilesOf reads the profiles of servers in the order of the IDs.
func (e *Engine) profilesOf(ids []string) ([]profile.Profile, error) {
	profiles := make([]profile.Profile, 0, len(ids))
	err := e.db.View(func(tx *bbolt.Tx) error {
		store, err := loadServers(tx)
		if err != nil {
			return err
		}
		for _, id := range ids {
			g, i := store.locate(id)
			if g < 0 {
				return serverNotFound(id)
			}
			p := store.servers[store.groups[g].ID][i].Profile
			if p.Type == profile.TypeChain {
				return jsonrpc.Failed(ReasonInvalidValue, "a chain is made of the servers of this Poros and has no link: share its servers")
			}
			profiles = append(profiles, p)
		}
		return nil
	})
	return profiles, err
}

// ShareLinks gives each server as the share link of its protocol, which other clients read too; a server such a link
// cannot carry whole comes as a poros://add/ link.
func (e *Engine) ShareLinks(req ServerIDs) (ServerLinks, error) {
	return e.linksOf(req.IDs, profile.ShareLink)
}

// DeepLinks gives each server as a poros://add/ link, which carries all of its settings.
func (e *Engine) DeepLinks(req ServerIDs) (ServerLinks, error) {
	return e.linksOf(req.IDs, profile.PorosLink)
}

func (e *Engine) linksOf(ids []string, write func(profile.Profile) string) (ServerLinks, error) {
	profiles, err := e.profilesOf(ids)
	if err != nil {
		return ServerLinks{}, err
	}
	links := ServerLinks{Links: make([]string, len(profiles))}
	for i, p := range profiles {
		links.Links[i] = write(p)
	}
	return links, nil
}

// ExportServer gives a server as the outbound of the core that runs it with the current settings.
func (e *Engine) ExportServer(req ByID) (ServerConfig, error) {
	settings, err := e.Settings()
	if err != nil {
		return ServerConfig{}, err
	}
	p, links, found, err := e.serverWithLinks(req.ID)
	var missing missingLink
	switch {
	case errors.As(err, &missing):
		return ServerConfig{}, jsonrpc.Failed(ReasonNoServer, "%v", err)
	case err != nil:
		return ServerConfig{}, err
	case !found:
		return ServerConfig{}, serverNotFound(req.ID)
	}
	onXray, data, err := config.Export(p, links, settings.RealityOnXray)
	switch {
	case errors.Is(err, profile.ErrUnsupported):
		return ServerConfig{}, jsonrpc.Failed(ReasonServerUnsupported, "%s", err.Error())
	case err != nil:
		return ServerConfig{}, err
	}
	out := ServerConfig{Core: CoreSingBox, JSON: string(data)}
	if onXray {
		out.Core = CoreXray
	}
	return out, nil
}
