package engine

import (
	"errors"
	"net/url"
	"slices"
	"strings"

	"github.com/sagernet/bbolt"

	"PorosCore/internal/jsonrpc"
	"PorosCore/profile"
)

// serverPlace is where a server is stored: its group, the group's servers and its index among them, -1 for none.
type serverPlace struct {
	group   groupRecord
	servers []serverRecord
	index   int
}

func (p serverPlace) found() bool { return p.index >= 0 }

// findServer looks for a server in every group.
func findServer(tx *bbolt.Tx, id string) (serverPlace, error) {
	list, err := groups(tx)
	if err != nil {
		return serverPlace{index: -1}, err
	}
	for _, group := range list {
		var servers []serverRecord
		if _, err := getJSON(tx, bucketServers, []byte(group.ID), &servers); err != nil {
			return serverPlace{index: -1}, err
		}
		if i := slices.IndexFunc(servers, func(r serverRecord) bool { return r.ID == id }); i >= 0 {
			return serverPlace{group, servers, i}, nil
		}
	}
	return serverPlace{index: -1}, nil
}

func serverNotFound(id string) error {
	return jsonrpc.Failed(ReasonNotFound, "there is no server %q", id)
}

// serverStore is every group with its servers, read in one transaction so that several of them change at once.
type serverStore struct {
	groups  []groupRecord
	servers map[string][]serverRecord
	changed map[string]bool
}

func loadServers(tx *bbolt.Tx) (*serverStore, error) {
	list, err := groups(tx)
	if err != nil {
		return nil, err
	}
	store := &serverStore{groups: list, servers: map[string][]serverRecord{}, changed: map[string]bool{}}
	for _, group := range list {
		var servers []serverRecord
		if _, err := getJSON(tx, bucketServers, []byte(group.ID), &servers); err != nil {
			return nil, err
		}
		store.servers[group.ID] = servers
	}
	return store, nil
}

// locate finds a server: the index of its group and its own index in that group, or -1 and -1.
func (s *serverStore) locate(id string) (int, int) {
	for g, group := range s.groups {
		if i := slices.IndexFunc(s.servers[group.ID], func(r serverRecord) bool { return r.ID == id }); i >= 0 {
			return g, i
		}
	}
	return -1, -1
}

// manualGroup is the index of a group of the user's.
func (s *serverStore) manualGroup(id string) (int, error) {
	g := slices.IndexFunc(s.groups, func(r groupRecord) bool { return r.ID == id })
	switch {
	case g < 0:
		return -1, groupNotFound(id)
	case !s.groups[g].manual():
		return -1, jsonrpc.Failed(ReasonNotManual, "servers go only to a group of the user's, and %q is a subscription", id)
	}
	return g, nil
}

// changedIDs are the groups whose servers changed, in the order of the groups.
func (s *serverStore) changedIDs() []string {
	var ids []string
	for _, group := range s.groups {
		if s.changed[group.ID] {
			ids = append(ids, group.ID)
		}
	}
	return ids
}

func (s *serverStore) save(tx *bbolt.Tx) error {
	for i := range s.groups {
		if group := &s.groups[i]; s.changed[group.ID] {
			if err := storeGroup(tx, group, s.servers[group.ID]); err != nil {
				return err
			}
		}
	}
	return nil
}

// changeServers runs change on every group and its servers under writeMu, stores what it changed and tells the
// interface. A chosen server that went away passes the choice on within its group.
func (e *Engine) changeServers(change func(s *serverStore) error) error {
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	settings, err := e.Settings()
	if err != nil {
		return err
	}
	var changed []string
	var chosenGroup string
	err = e.db.Update(func(tx *bbolt.Tx) error {
		store, err := loadServers(tx)
		if err != nil {
			return err
		}
		if g, _ := store.locate(settings.Server); g >= 0 {
			chosenGroup = store.groups[g].ID
		}
		if err = change(store); err != nil {
			return err
		}
		changed = store.changedIDs()
		return store.save(tx)
	})
	if err != nil {
		return err
	}
	if len(changed) > 0 {
		e.announce(changed...)
		e.keepServerChosen(chosenGroup)
	}
	return nil
}

// Server gives a server with its whole profile.
func (e *Engine) Server(req ByID) (ServerProfile, error) {
	var place serverPlace
	err := e.db.View(func(tx *bbolt.Tx) error {
		var err error
		place, err = findServer(tx, req.ID)
		return err
	})
	if err != nil {
		return ServerProfile{}, err
	}
	if !place.found() {
		return ServerProfile{}, serverNotFound(req.ID)
	}
	r := place.servers[place.index]
	view := r.view(place.group.ID)
	view.Latency = e.latencyOf(r.ID)
	return ServerProfile{Server: view, Profile: r.Profile}, nil
}

// AddServers adds the servers of a text to a group of the user's, or to a new one.
func (e *Engine) AddServers(req AddServers) (ServersAdded, error) {
	text := strings.TrimSpace(req.Text)
	if isSubscriptionURL(text) {
		return ServersAdded{}, jsonrpc.Failed(ReasonSubscriptionURL, "this is a subscription's link: add it as a subscription")
	}
	doc, err := profile.ParseDocument([]byte(text))
	if err != nil {
		return ServersAdded{}, jsonrpc.Failed(ReasonNoServers, "%s", err.Error())
	}
	if len(doc.Profiles) == 0 {
		message := "the text holds no servers"
		if doc.SkippedCount > 0 {
			message = "none of the servers in the text can run in Poros"
		}
		return ServersAdded{}, jsonrpc.Failed(ReasonNoServers, "%s", message)
	}
	newGroup := strings.TrimSpace(req.NewGroup)
	if req.GroupID == "" && newGroup == "" {
		return ServersAdded{}, jsonrpc.Failed(ReasonInvalidValue, "a group needs a name")
	}

	added := ServersAdded{IDs: []string{}, Skips: skipReasons(doc.Skipped)}
	err = e.changeServers(func(s *serverStore) error {
		g := -1
		if req.GroupID != "" {
			var err error
			if g, err = s.manualGroup(req.GroupID); err != nil {
				return err
			}
		} else {
			s.groups = append(s.groups, groupRecord{ID: newID(), Kind: GroupManual, Name: newGroup, AddedAt: e.now().UnixMilli()})
			g = len(s.groups) - 1
		}
		id := s.groups[g].ID
		for _, p := range doc.Profiles {
			r := serverRecord{ID: newID(), Profile: p}
			s.servers[id] = append(s.servers[id], r)
			added.IDs = append(added.IDs, r.ID)
		}
		s.changed[id] = true
		s.groups[g].Servers = len(s.servers[id])
		added.Group = s.groups[g].view()
		return nil
	})
	if err != nil {
		return ServersAdded{}, err
	}
	return added, nil
}

// isSubscriptionURL is an http or https address without a port or a login: a subscription, not an HTTP proxy.
func isSubscriptionURL(text string) bool {
	if strings.ContainsAny(text, " \t\r\n") {
		return false
	}
	parsed, err := url.Parse(text)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" &&
		parsed.Port() == "" && parsed.User == nil
}

// SaveServer stores a changed server, or a new one in a group of the user's, once its profile is valid.
func (e *Engine) SaveServer(req SaveServer) (Server, error) {
	if err := req.Profile.Validate(); err != nil {
		reason := ReasonInvalidValue
		if errors.Is(err, profile.ErrUnsupported) {
			reason = ReasonServerUnsupported
		}
		return Server{}, jsonrpc.Failed(reason, "%s", err.Error())
	}
	var saved Server
	err := e.changeServers(func(s *serverStore) error {
		r := serverRecord{ID: req.ID, Profile: req.Profile}
		var groupID string
		if req.ID != "" {
			g, i := s.locate(req.ID)
			if g < 0 {
				return serverNotFound(req.ID)
			}
			groupID = s.groups[g].ID
			s.servers[groupID][i] = r
		} else {
			g, err := s.manualGroup(req.GroupID)
			if err != nil {
				return err
			}
			groupID, r.ID = s.groups[g].ID, newID()
			s.servers[groupID] = append(s.servers[groupID], r)
		}
		s.changed[groupID] = true
		saved = r.view(groupID)
		return nil
	})
	if err != nil {
		return Server{}, err
	}
	saved.Latency = e.latencyOf(saved.ID)
	return saved, nil
}

// RemoveServers removes servers from any group; a subscription brings its own back with its next update.
func (e *Engine) RemoveServers(req ServerIDs) error {
	return e.changeServers(func(s *serverStore) error {
		for _, id := range req.IDs {
			g, i := s.locate(id)
			if g < 0 {
				continue
			}
			groupID := s.groups[g].ID
			s.servers[groupID] = slices.Delete(s.servers[groupID], i, i+1)
			s.changed[groupID] = true
		}
		return nil
	})
}

// CloneServers puts a copy of each server right after it, in its own group.
func (e *Engine) CloneServers(req ServerIDs) (ServerIDs, error) {
	copies := ServerIDs{IDs: []string{}}
	err := e.changeServers(func(s *serverStore) error {
		for _, id := range req.IDs {
			g, i := s.locate(id)
			if g < 0 {
				return serverNotFound(id)
			}
			groupID := s.groups[g].ID
			clone := serverRecord{ID: newID(), Profile: s.servers[groupID][i].Profile}
			s.servers[groupID] = slices.Insert(s.servers[groupID], i+1, clone)
			s.changed[groupID] = true
			copies.IDs = append(copies.IDs, clone.ID)
		}
		return nil
	})
	return copies, err
}

// MoveServers moves servers to the end of a group of the user's; those of a subscription are copied there.
func (e *Engine) MoveServers(req MoveServers) (ServerIDs, error) {
	moved := ServerIDs{IDs: []string{}}
	err := e.changeServers(func(s *serverStore) error {
		target, err := s.manualGroup(req.GroupID)
		if err != nil {
			return err
		}
		targetID := s.groups[target].ID
		for _, id := range req.IDs {
			g, i := s.locate(id)
			if g < 0 {
				return serverNotFound(id)
			}
			sourceID := s.groups[g].ID
			r := s.servers[sourceID][i]
			switch {
			case sourceID == targetID:
			case s.groups[g].manual():
				s.servers[sourceID] = slices.Delete(s.servers[sourceID], i, i+1)
				s.servers[targetID] = append(s.servers[targetID], r)
				s.changed[sourceID], s.changed[targetID] = true, true
			default:
				r.ID = newID()
				s.servers[targetID] = append(s.servers[targetID], r)
				s.changed[targetID] = true
			}
			moved.IDs = append(moved.IDs, r.ID)
		}
		return nil
	})
	return moved, err
}
