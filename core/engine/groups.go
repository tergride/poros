package engine

import (
	"cmp"
	"net/url"
	"slices"
	"strings"

	"github.com/sagernet/bbolt"

	"PorosCore/internal/jsonrpc"
	"PorosCore/internal/subfetch"
)

// groupRecord is a group as stored. The fields after Servers are a subscription's.
type groupRecord struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	// Name is the user's name for it, "" for none; a group of the user's always has one.
	Name    string `json:"name,omitempty"`
	AddedAt int64  `json:"addedAt"`
	Servers int    `json:"servers"`

	URL       string `json:"url,omitempty"`
	UpdatedAt int64  `json:"updatedAt,omitempty"`
	// CheckedAt is the last attempt to update, whether it worked or not.
	CheckedAt int64         `json:"checkedAt,omitempty"`
	Skipped   int           `json:"skipped,omitempty"`
	Info      subfetch.Info `json:"info,omitzero"`
	Error     string        `json:"error,omitempty"`
	ErrorText string        `json:"errorText,omitempty"`
	Settings  GroupSettings `json:"settings,omitzero"`
}

func (r groupRecord) manual() bool { return r.Kind == GroupManual }

func (r groupRecord) view() Group {
	name := r.Name
	if name == "" {
		name = r.Info.Title
	}
	if name == "" {
		if parsed, err := url.Parse(r.URL); err == nil {
			name = parsed.Hostname()
		}
	}
	return Group{
		ID:         r.ID,
		Kind:       r.Kind,
		Name:       name,
		URL:        r.URL,
		AddedAt:    r.AddedAt,
		UpdatedAt:  r.UpdatedAt,
		Servers:    r.Servers,
		Skipped:    r.Skipped,
		Upload:     r.Info.Upload,
		Download:   r.Info.Download,
		Total:      r.Info.Total,
		Expire:     r.Info.Expire * 1000,
		WebPageURL: r.Info.WebPageURL,
		SupportURL: r.Info.SupportURL,
		Announce:   r.Info.Announce,
		Error:      r.Error,
		ErrorText:  r.ErrorText,
		Settings:   r.Settings,
	}
}

// groups lists the stored groups: subscriptions first, each kind in the order it was added.
func groups(tx *bbolt.Tx) ([]groupRecord, error) {
	list := []groupRecord{}
	err := tx.Bucket(bucketGroups).ForEach(func(key, _ []byte) error {
		var rec groupRecord
		if _, err := getJSON(tx, bucketGroups, key, &rec); err != nil {
			return err
		}
		list = append(list, rec)
		return nil
	})
	slices.SortStableFunc(list, func(a, b groupRecord) int {
		return cmp.Or(cmp.Compare(boolRank(a.manual()), boolRank(b.manual())), cmp.Compare(a.AddedAt, b.AddedAt))
	})
	return list, err
}

func (e *Engine) groups() (list []groupRecord, err error) {
	err = e.db.View(func(tx *bbolt.Tx) error {
		list, err = groups(tx)
		return err
	})
	return list, err
}

func (e *Engine) group(id string) (rec groupRecord, found bool, err error) {
	err = e.db.View(func(tx *bbolt.Tx) error {
		found, err = getJSON(tx, bucketGroups, []byte(id), &rec)
		return err
	})
	return rec, found, err
}

func groupNotFound(id string) error {
	return jsonrpc.Failed(ReasonNotFound, "there is no group %q", id)
}

// Groups lists the subscriptions, then the user's groups.
func (e *Engine) Groups() ([]Group, error) {
	list, err := e.groups()
	if err != nil {
		return nil, err
	}
	views := make([]Group, len(list))
	for i, rec := range list {
		views[i] = rec.view()
	}
	return views, nil
}

// announce tells the interface about the groups and about the servers of each of ids. It runs under writeMu.
func (e *Engine) announce(ids ...string) {
	if list, err := e.Groups(); err == nil {
		e.notify(EventGroupsChanged, list)
	}
	for _, id := range ids {
		e.notify(EventServersChanged, ServersChanged{GroupID: id})
	}
}

// storeGroup writes the group and, unless servers is nil, its servers, which it counts.
func storeGroup(tx *bbolt.Tx, rec *groupRecord, servers []serverRecord) error {
	if servers != nil {
		rec.Servers = len(servers)
		if err := putJSON(tx, bucketServers, []byte(rec.ID), servers); err != nil {
			return err
		}
	}
	return putJSON(tx, bucketGroups, []byte(rec.ID), rec)
}

func (e *Engine) storeGroup(rec groupRecord, servers []serverRecord) error {
	return e.db.Update(func(tx *bbolt.Tx) error { return storeGroup(tx, &rec, servers) })
}

// AddGroup creates a group of the user's, empty.
func (e *Engine) AddGroup(req AddGroup) (Group, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return Group{}, jsonrpc.Failed(ReasonInvalidValue, "a group needs a name")
	}
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	rec := groupRecord{ID: newID(), Kind: GroupManual, Name: name, AddedAt: e.now().UnixMilli()}
	if err := e.storeGroup(rec, []serverRecord{}); err != nil {
		return Group{}, err
	}
	e.announce()
	return rec.view(), nil
}

// RenameGroup gives a group the user's name; an empty one takes a subscription back to the provider's title.
func (e *Engine) RenameGroup(req RenameGroup) (Group, error) {
	e.writeMu.Lock()
	var rec groupRecord
	var found bool
	err := e.db.Update(func(tx *bbolt.Tx) error {
		var err error
		if found, err = getJSON(tx, bucketGroups, []byte(req.ID), &rec); err != nil || !found {
			return err
		}
		name := strings.TrimSpace(req.Name)
		if name == "" && rec.manual() {
			return jsonrpc.Failed(ReasonInvalidValue, "a group needs a name")
		}
		rec.Name = name
		return putJSON(tx, bucketGroups, []byte(req.ID), rec)
	})
	e.writeMu.Unlock()
	if err != nil {
		return Group{}, err
	}
	if !found {
		return Group{}, groupNotFound(req.ID)
	}
	e.announce()
	return rec.view(), nil
}

// RemoveGroup removes a group with its servers.
func (e *Engine) RemoveGroup(id string) error {
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	found := false
	if err := e.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(bucketGroups)
		if bucket.Get([]byte(id)) == nil {
			return nil
		}
		found = true
		if err := tx.Bucket(bucketServers).Delete([]byte(id)); err != nil {
			return err
		}
		return bucket.Delete([]byte(id))
	}); err != nil {
		return err
	}
	if !found {
		return groupNotFound(id)
	}
	e.announce(id)
	e.keepServerChosen("")
	return nil
}

// keepServerChosen moves the choice on when the chosen server went away with its group, an update or a removal: to
// the first server left in groupID, else to the first of all, else to none. It runs under writeMu.
func (e *Engine) keepServerChosen(groupID string) {
	settings, err := e.Settings()
	if err != nil || settings.Server == "" {
		return
	}
	if _, found, err := e.serverProfile(settings.Server); err != nil || found {
		return
	}
	list, err := e.groups()
	if err != nil {
		return
	}
	slices.SortStableFunc(list, func(a, b groupRecord) int {
		return cmp.Compare(boolRank(a.ID != groupID), boolRank(b.ID != groupID))
	})
	settings.Server = ""
	for _, rec := range list {
		if servers, err := e.serverRecords(rec.ID); err == nil && len(servers) > 0 {
			settings.Server = servers[0].ID
			break
		}
	}
	if err = e.db.Update(func(tx *bbolt.Tx) error { return putJSON(tx, bucketSettings, keySettings, settings) }); err != nil {
		return
	}
	e.notify(EventSettingsChanged, settings)
}

func boolRank(b bool) int {
	if b {
		return 1
	}
	return 0
}
