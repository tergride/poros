package engine

import (
	"context"
	"errors"
	"log"
	"net/url"
	"slices"
	"strings"
	"sync"

	"PorosCore/internal/jsonrpc"
	"PorosCore/internal/subfetch"
	"PorosCore/profile"
)

// updateFailure is an update that did not work, with the reason the interface translates.
type updateFailure struct {
	reason string
	text   string
}

func (f *updateFailure) Error() string { return f.text }

func (f *updateFailure) rpc() error { return jsonrpc.Failed(f.reason, "%s", f.text) }

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
	doc, info, failure := e.download(ctx, groupRecord{Kind: GroupSubscription, URL: link})
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
	rec := groupRecord{ID: newID(), Kind: GroupSubscription, URL: link, Name: strings.TrimSpace(req.Name), AddedAt: now}
	servers, update := rec.apply(doc, info, nil, now)
	if err := e.storeGroup(rec, servers); err != nil {
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
	list, err := e.groups()
	return err == nil && slices.ContainsFunc(list, func(r groupRecord) bool { return r.URL == link })
}

// download fetches and reads a subscription as its group asks. Its errors leave out the URL, which carries the
// user's token.
func (e *Engine) download(ctx context.Context, rec groupRecord) (profile.Document, subfetch.Info, *updateFailure) {
	settings, err := e.Settings()
	if err != nil {
		return profile.Document{}, subfetch.Info{}, &updateFailure{ReasonFetchFailed, err.Error()}
	}
	request := e.requestOf(rec, settings)
	client, done := e.fetchClient()
	defer done()
	result, err := subfetch.Fetch(ctx, request.client(client), rec.URL, request.userAgent, request.device)
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
	doc.Profiles = afterDownload(rec.Settings, doc.Profiles)
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
func (r *groupRecord) apply(doc profile.Document, info subfetch.Info, old []serverRecord, now int64) ([]serverRecord, SubscriptionUpdate) {
	servers, update := reconcile(old, doc.Profiles)
	r.Info = info
	r.UpdatedAt, r.CheckedAt = now, now
	r.Servers, r.Skipped = len(servers), doc.SkippedCount
	r.Error, r.ErrorText = "", ""
	update.Skips = skipReasons(doc.Skipped)
	update.Group = r.view()
	return servers, update
}

// subscription is the stored subscription id; a group of the user's is none.
func (e *Engine) subscription(id string) (groupRecord, error) {
	rec, found, err := e.group(id)
	switch {
	case err != nil:
		return rec, err
	case !found:
		return rec, groupNotFound(id)
	case rec.manual():
		return rec, jsonrpc.Failed(ReasonInvalidValue, "the group %q is no subscription", id)
	}
	return rec, nil
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
	// A lock only for a subscription that exists: the locks are never let go.
	if _, err := e.subscription(id); err != nil {
		return SubscriptionUpdate{}, err
	}
	lock := e.subscriptionLock(id)
	lock.Lock()
	defer lock.Unlock()
	rec, err := e.subscription(id)
	if err != nil {
		return SubscriptionUpdate{}, err
	}
	doc, info, failure := e.download(ctx, rec)
	// Poros is closing: that is no failure of the provider's.
	if err = ctx.Err(); err != nil {
		return SubscriptionUpdate{}, err
	}

	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	// The subscription may have been removed or renamed while it downloaded.
	if rec, err = e.subscription(id); err != nil {
		return SubscriptionUpdate{}, err
	}
	now := e.now().UnixMilli()
	if failure != nil {
		log.Printf("subscription %s: the update failed (%s): %s", rec.ID, failure.reason, failure.text)
		rec.CheckedAt, rec.Error, rec.ErrorText = now, failure.reason, failure.text
		if err = e.storeGroup(rec, nil); err != nil {
			return SubscriptionUpdate{}, err
		}
		e.announce()
		return SubscriptionUpdate{}, failure.rpc()
	}
	old, err := e.serverRecords(id)
	if err != nil {
		return SubscriptionUpdate{}, err
	}
	servers, update := rec.apply(doc, info, old, now)
	servers = e.keepWorking(rec.Settings, old, servers)
	if err = e.storeGroup(rec, servers); err != nil {
		return SubscriptionUpdate{}, err
	}
	e.announce(id)
	e.keepServerChosen(id)
	if rec.Settings.Test {
		// The test runs on its own: it reports through servers.tested, and what it decides lands when it ends.
		if _, err := e.testServers(ServersQuery{GroupID: id}, func(results map[string]int) { e.afterTest(id, results) }); err != nil {
			log.Printf("subscription %s: the test after the update did not start: %v", id, err)
		}
	}
	return update, nil
}

// maxSkipGroups bounds the reasons an update reports; the items beyond still count in Group.Skipped.
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
