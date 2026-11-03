package engine

import (
	"cmp"
	"net/url"
	"slices"
	"strings"

	"github.com/sagernet/bbolt"

	"PorosCore/internal/jsonrpc"
)

// Subscriptions are listed in the order they were added.
func (e *Engine) Subscriptions() ([]Subscription, error) {
	list := []Subscription{}
	err := e.db.View(func(tx *bbolt.Tx) error {
		return tx.Bucket(bucketSubscriptions).ForEach(func(key, _ []byte) error {
			var sub Subscription
			if _, err := getJSON(tx, bucketSubscriptions, key, &sub); err != nil {
				return err
			}
			list = append(list, sub)
			return nil
		})
	})
	slices.SortStableFunc(list, func(a, b Subscription) int { return cmp.Compare(a.AddedAt, b.AddedAt) })
	return list, err
}

// AddSubscription stores the link. Its servers come with the first update.
func (e *Engine) AddSubscription(req AddSubscription) (Subscription, error) {
	link := strings.TrimSpace(req.URL)
	parsed, err := url.Parse(link)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return Subscription{}, jsonrpc.Failed(ReasonInvalidURL, "a subscription link starts with http:// or https://")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = parsed.Hostname()
	}

	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	existing, err := e.Subscriptions()
	if err != nil {
		return Subscription{}, err
	}
	if slices.ContainsFunc(existing, func(s Subscription) bool { return s.URL == link }) {
		return Subscription{}, jsonrpc.Failed(ReasonDuplicate, "this subscription is already added")
	}
	sub := Subscription{ID: newID(), Name: name, URL: link, AddedAt: e.now().UnixMilli()}
	if err = e.db.Update(func(tx *bbolt.Tx) error {
		return putJSON(tx, bucketSubscriptions, []byte(sub.ID), sub)
	}); err != nil {
		return Subscription{}, err
	}
	e.notify(EventSubscriptionsChanged, append(existing, sub))
	return sub, nil
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
		return bucket.Delete([]byte(id))
	}); err != nil {
		return err
	}
	if !found {
		return jsonrpc.Failed(ReasonNotFound, "there is no subscription %q", id)
	}
	list, err := e.Subscriptions()
	if err != nil {
		return err
	}
	e.notify(EventSubscriptionsChanged, list)
	return nil
}
