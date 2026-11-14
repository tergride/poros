package engine

import (
	"github.com/sagernet/bbolt"

	"PorosCore/config"
	"PorosCore/internal/boxmain"
	"PorosCore/internal/xray"
	"PorosCore/profile"
)

// CheckServer tells the editor of a profile not saved yet which core would run it, why it would not run, and its
// share link: the core reads the profile as a connection would, chain links and Xray included.
func (e *Engine) CheckServer(req SaveServer) (ServerCheck, error) {
	settings, err := e.Settings()
	if err != nil {
		return ServerCheck{}, err
	}
	p := req.Profile
	check := ServerCheck{Core: CoreSingBox}
	var links []profile.Profile
	err = p.Validate()
	if err == nil {
		links, err = e.chainLinks(req.ID, p)
	}
	if err == nil {
		if config.RunsOnXray(config.Entry(p, links), settings.RealityOnXray) {
			check.Core = CoreXray
		}
		err = checkBuild(p, links, settings.RealityOnXray)
	}
	switch {
	case err != nil:
		check.Problem = err.Error()
	case p.Type != profile.TypeChain:
		check.Link = profile.ShareLink(p)
	}
	return check, nil
}

// chainLinks are the servers of a chain as they are stored, once the chain may go through them.
func (e *Engine) chainLinks(id string, p profile.Profile) ([]profile.Profile, error) {
	if p.Type != profile.TypeChain {
		return nil, nil
	}
	var links []profile.Profile
	err := e.db.View(func(tx *bbolt.Tx) error {
		store, err := loadServers(tx)
		if err != nil {
			return err
		}
		if err := checkChain(store, id, p); err != nil {
			return err
		}
		for _, linkID := range p.Chain.Servers {
			g, i := store.locate(linkID)
			links = append(links, store.servers[store.groups[g].ID][i].Profile)
		}
		return nil
	})
	return links, err
}

// checkBuild has the cores read a server in a connection of its own, as a start would, without starting anything.
func checkBuild(p profile.Profile, links []profile.Profile, realityOnXray bool) error {
	configs, err := config.Build(config.Input{
		Server:        p,
		Links:         links,
		Preset:        config.Preset{Name: "check", DefaultOutbound: config.OutboundProxy},
		MixedPort:     1,
		RealityOnXray: realityOnXray,
		Bridge:        config.Bridge{Port: 1, Username: "check", Password: "check"},
	})
	if err != nil {
		return err
	}
	data, err := configs.SingBoxJSON()
	if err == nil {
		err = boxmain.Check(data)
	}
	if err == nil && configs.Xray != nil {
		err = xray.CheckXrayConfig(string(configs.Xray))
	}
	return err
}

// serversThrough are the servers a connection to id goes through: id itself, the links of its chain and the
// proxies of its group.
func (e *Engine) serversThrough(id string) []string {
	ids := []string{id}
	_ = e.db.View(func(tx *bbolt.Tx) error {
		store, err := loadServers(tx)
		if err != nil {
			return err
		}
		g, i := store.locate(id)
		if g < 0 {
			return nil
		}
		group := store.groups[g]
		if chain := store.servers[group.ID][i].Profile.Chain; chain != nil {
			ids = append(ids, chain.Servers...)
		}
		for _, proxy := range []string{group.Settings.FrontProxy, group.Settings.LandingProxy} {
			if proxy != "" {
				ids = append(ids, proxy)
			}
		}
		return nil
	})
	return ids
}
