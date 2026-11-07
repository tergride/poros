package engine

import (
	"cmp"
	"context"
	"errors"
	"net/url"
	"slices"
	"strings"
	"sync"

	"github.com/sagernet/bbolt"

	"PorosCore/internal/jsonrpc"
	"PorosCore/internal/subfetch"
	"PorosCore/profile"
)

// subscriptionRecord is a subscription as stored.
type subscriptionRecord struct {
	ID  string `json:"id"`
	URL string `json:"url"`
	// Name is the user's name for it, "" for none.
	Name      string `json:"name,omitempty"`
	AddedAt   int64  `json:"addedAt"`
	UpdatedAt int64  `json:"updatedAt"`
	// CheckedAt is the last attempt to update, whether it worked or not.
	CheckedAt int64         `json:"checkedAt"`
	Servers   int           `json:"servers"`
	Skipped   int           `json:"skipped"`
	Info      subfetch.Info `json:"info"`
	Error     string        `json:"error,omitempty"`
	ErrorText string        `json:"errorText,omitempty"`
}

func (r subscriptionRecord) view() Subscription {
	name := r.Name
	if name == "" {
		name = r.Info.Title
	}
	if name == "" {
		if parsed, err := url.Parse(r.URL); err == nil {
			name = parsed.Hostname()
		}
	}
	return Subscription{
		ID:         r.ID,
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
	}
}

// updateFailure is an update that did not work, with the reason the interface translates.
type updateFailure struct {
	reason string
	text   string
}

func (f *updateFailure) Error() string { return f.text }

func (f *updateFailure) rpc() error { return jsonrpc.Failed(f.reason, "%s", f.text) }

// records lists the stored subscriptions in the order they were added.
func records(tx *bbolt.Tx) ([]subscriptionRecord, error) {
	list := []subscriptionRecord{}
	err := tx.Bucket(bucketSubscriptions).ForEach(func(key, _ []byte) error {
		var rec subscriptionRecord
		if _, err := getJSON(tx, bucketSubscriptions, key, &rec); err != nil {
			return err
		}
		list = append(list, rec)
		return nil
	})
	slices.SortStableFunc(list, func(a, b subscriptionRecord) int { return cmp.Compare(a.AddedAt, b.AddedAt) })
	return list, err
}

func (e *Engine) records() (list []subscriptionRecord, err error) {
	err = e.db.View(func(tx *bbolt.Tx) error {
		list, err = records(tx)
		return err
	})
	return list, err
}

func (e *Engine) record(id string) (rec subscriptionRecord, found bool, err error) {
	err = e.db.View(func(tx *bbolt.Tx) error {
		found, err = getJSON(tx, bucketSubscriptions, []byte(id), &rec)
		return err
	})
	return rec, found, err
}

// Subscriptions are listed in the order they were added.
func (e *Engine) Subscriptions() ([]Subscription, error) {
	list, err := e.records()
	if err != nil {
		return nil, err
	}
	views := make([]Subscription, len(list))
	for i, rec := range list {
		views[i] = rec.view()
	}
	return views, nil
}

// announce tells the interface about the subscriptions and, for id, about its servers. It runs under writeMu.
func (e *Engine) announce(id string) {
	if list, err := e.Subscriptions(); err == nil {
		e.notify(EventSubscriptionsChanged, list)
	}
	if id != "" {
		e.notify(EventServersChanged, ServersChanged{SubscriptionID: id})
	}
}

// RenameSubscription gives a subscription the user's name; an empty one goes back to the provider's title.
func (e *Engine) RenameSubscription(req RenameSubscription) (Subscription, error) {
	e.writeMu.Lock()
	var rec subscriptionRecord
	var found bool
	err := e.db.Update(func(tx *bbolt.Tx) error {
		var err error
		if found, err = getJSON(tx, bucketSubscriptions, []byte(req.ID), &rec); err != nil || !found {
			return err
		}
		rec.Name = strings.TrimSpace(req.Name)
		return putJSON(tx, bucketSubscriptions, []byte(req.ID), rec)
	})
	e.writeMu.Unlock()
	if err != nil {
		return Subscription{}, err
	}
	if !found {
		return Subscription{}, jsonrpc.Failed(ReasonNotFound, "there is no subscription %q", req.ID)
	}
	e.announce("")
	return rec.view(), nil
}

// AddSubscription downloads the subscription and stores it with its servers; one that does not download, or holds
// no server Poros can run, is not added.
func (e *Engine) AddSubscription(ctx context.Context, req AddSubscription) (SubscriptionUpdate, error) {
	link := strings.TrimSpace(req.URL)
	if err := checkSubscriptionURL(link); err != nil {
		return SubscriptionUpdate{}, err
	}
	if e.hasURL(link) {
		return SubscriptionUpdate{}, jsonrpc.Failed(ReasonDuplicate, "this subscription is already added")
	}
	doc, info, failure := e.download(ctx, link)
	if failure != nil {
		return SubscriptionUpdate{}, failure.rpc()
	}

	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	// The same link may have been added while this one downloaded.
	if e.hasURL(link) {
		return SubscriptionUpdate{}, jsonrpc.Failed(ReasonDuplicate, "this subscription is already added")
	}
	now := e.now().UnixMilli()
	rec := subscriptionRecord{ID: newID(), URL: link, Name: strings.TrimSpace(req.Name), AddedAt: now}
	servers, update := rec.apply(doc, info, nil, now)
	if err := e.store(rec, servers); err != nil {
		return SubscriptionUpdate{}, err
	}
	e.announce(rec.ID)
	return update, nil
}

// checkSubscriptionURL takes an http or https link; a link to one server is told apart, as the simple mode adds
// none by hand.
func checkSubscriptionURL(link string) error {
	parsed, err := url.Parse(link)
	if err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") {
		if parsed.Host == "" {
			return jsonrpc.Failed(ReasonInvalidURL, "the subscription link has no host")
		}
		return nil
	}
	lower := strings.ToLower(link)
	if strings.HasPrefix(lower, "poros://add/") || strings.HasPrefix(lower, "throne://add/") {
		return jsonrpc.Failed(ReasonSingleServer, "this is a link to one server, not a subscription")
	}
	if _, err := profile.ParseLink(link); !errors.Is(err, profile.ErrNotLink) {
		return jsonrpc.Failed(ReasonSingleServer, "this is a link to one server, not a subscription")
	}
	return jsonrpc.Failed(ReasonInvalidURL, "a subscription link starts with http:// or https://")
}

func (e *Engine) hasURL(link string) bool {
	list, err := e.records()
	return err == nil && slices.ContainsFunc(list, func(r subscriptionRecord) bool { return r.URL == link })
}

// download fetches and reads a subscription. Its errors leave out the URL, which carries the user's token.
func (e *Engine) download(ctx context.Context, link string) (profile.Document, subfetch.Info, *updateFailure) {
	client, done := e.fetchClient()
	defer done()
	result, err := subfetch.Fetch(ctx, client, link, e.userAgent, e.deviceHeaders())
	if err != nil {
		if errors.Is(err, subfetch.ErrEmpty) {
			return profile.Document{}, subfetch.Info{}, &updateFailure{ReasonNoServers, err.Error()}
		}
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return profile.Document{}, subfetch.Info{}, &updateFailure{ReasonFetchFailed, err.Error()}
	}
	doc, err := profile.ParseDocument(result.Body)
	if err != nil {
		return profile.Document{}, subfetch.Info{}, &updateFailure{ReasonInvalidSubscription, err.Error()}
	}
	if len(doc.Profiles) == 0 {
		text := "the subscription holds no servers"
		if doc.SkippedCount > 0 {
			text = "none of the servers in the subscription can run in Poros"
		}
		return profile.Document{}, subfetch.Info{}, &updateFailure{ReasonNoServers, text}
	}
	return doc, result.Info, nil
}

// apply takes a downloaded subscription into the record and returns the servers to store and what changed.
func (r *subscriptionRecord) apply(doc profile.Document, info subfetch.Info, old []serverRecord, now int64) ([]serverRecord, SubscriptionUpdate) {
	servers, update := reconcile(old, doc.Profiles)
	r.Info = info
	r.UpdatedAt, r.CheckedAt = now, now
	r.Servers, r.Skipped = len(servers), doc.SkippedCount
	r.Error, r.ErrorText = "", ""
	update.Skips = skipReasons(doc.Skipped)
	update.Subscription = r.view()
	return servers, update
}

func (e *Engine) store(rec subscriptionRecord, servers []serverRecord) error {
	return e.db.Update(func(tx *bbolt.Tx) error {
		if err := putJSON(tx, bucketSubscriptions, []byte(rec.ID), rec); err != nil {
			return err
		}
		if servers == nil {
			return nil
		}
		return putJSON(tx, bucketServers, []byte(rec.ID), servers)
	})
}

// subscriptionLock serializes the updates of one subscription.
func (e *Engine) subscriptionLock(id string) *sync.Mutex {
	e.updatingMu.Lock()
	defer e.updatingMu.Unlock()
	lock, ok := e.updating[id]
	if !ok {
		lock = &sync.Mutex{}
		e.updating[id] = lock
	}
	return lock
}

// UpdateSubscription downloads the subscription again. A failed update keeps the servers and records why.
func (e *Engine) UpdateSubscription(ctx context.Context, id string) (SubscriptionUpdate, error) {
	lock := e.subscriptionLock(id)
	lock.Lock()
	defer lock.Unlock()
	rec, found, err := e.record(id)
	if err != nil {
		return SubscriptionUpdate{}, err
	}
	if !found {
		return SubscriptionUpdate{}, jsonrpc.Failed(ReasonNotFound, "there is no subscription %q", id)
	}
	doc, info, failure := e.download(ctx, rec.URL)
	// Poros is closing: that is no failure of the provider's.
	if err = ctx.Err(); err != nil {
		return SubscriptionUpdate{}, err
	}

	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	// The subscription may have been removed or renamed while it downloaded.
	if rec, found, err = e.record(id); err != nil {
		return SubscriptionUpdate{}, err
	}
	if !found {
		return SubscriptionUpdate{}, jsonrpc.Failed(ReasonNotFound, "there is no subscription %q", id)
	}
	now := e.now().UnixMilli()
	if failure != nil {
		rec.CheckedAt, rec.Error, rec.ErrorText = now, failure.reason, failure.text
		if err = e.store(rec, nil); err != nil {
			return SubscriptionUpdate{}, err
		}
		e.announce("")
		return SubscriptionUpdate{}, failure.rpc()
	}
	old, err := e.serverRecords(id)
	if err != nil {
		return SubscriptionUpdate{}, err
	}
	servers, update := rec.apply(doc, info, old, now)
	if err = e.store(rec, servers); err != nil {
		return SubscriptionUpdate{}, err
	}
	e.announce(id)
	e.keepServerChosen(id)
	return update, nil
}

func (e *Engine) RemoveSubscription(id string) error {
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	found := false
	if err := e.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(bucketSubscriptions)
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
		return jsonrpc.Failed(ReasonNotFound, "there is no subscription %q", id)
	}
	e.announce(id)
	e.keepServerChosen("")
	return nil
}

// keepServerChosen moves the choice on when the chosen server went away with its subscription or an update: to the
// first server left in that subscription, else to the first of all, else to none. It runs under writeMu.
func (e *Engine) keepServerChosen(subscriptionID string) {
	settings, err := e.Settings()
	if err != nil || settings.Server == "" {
		return
	}
	if _, found, err := e.serverProfile(settings.Server); err != nil || found {
		return
	}
	subs, err := e.records()
	if err != nil {
		return
	}
	slices.SortStableFunc(subs, func(a, b subscriptionRecord) int {
		return cmp.Compare(boolRank(a.ID != subscriptionID), boolRank(b.ID != subscriptionID))
	})
	settings.Server = ""
	for _, sub := range subs {
		if list, err := e.serverRecords(sub.ID); err == nil && len(list) > 0 {
			settings.Server = list[0].ID
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

// maxSkipGroups bounds the reasons an update reports; the items beyond still count in Subscription.Skipped.
const maxSkipGroups = 20

// skipReasons groups skipped items by reason, in the order the reasons first came.
func skipReasons(skipped []profile.Skipped) []SkipReason {
	reasons := []SkipReason{}
	for _, item := range skipped {
		i := slices.IndexFunc(reasons, func(r SkipReason) bool { return r.Reason == item.Reason })
		if i < 0 {
			if len(reasons) == maxSkipGroups {
				continue
			}
			reasons = append(reasons, SkipReason{Reason: item.Reason, Unsupported: item.Unsupported, Names: []string{}})
			i = len(reasons) - 1
		}
		reasons[i].Count++
		if item.Name != "" && len(reasons[i].Names) < 3 {
			reasons[i].Names = append(reasons[i].Names, item.Name)
		}
	}
	return reasons
}
