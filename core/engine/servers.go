package engine

import (
	"encoding/json"
	"net"
	"strconv"
	"strings"

	"github.com/sagernet/bbolt"

	"PorosCore/profile"
)

// serverRecord is a server as stored, in the list of its subscription.
type serverRecord struct {
	ID      string          `json:"id"`
	Profile profile.Profile `json:"profile"`
}

func (r serverRecord) view(subscriptionID string) Server {
	p := r.Profile
	name := strings.TrimSpace(p.Name)
	if name == "" {
		name = net.JoinHostPort(p.Server, strconv.Itoa(p.ServerPort))
	}
	return Server{
		ID:             r.ID,
		SubscriptionID: subscriptionID,
		Name:           name,
		Protocol:       p.Type,
		Host:           p.Server,
		Port:           p.ServerPort,
		Country:        flagCountry(name),
	}
}

func (e *Engine) serverRecords(subscriptionID string) (list []serverRecord, err error) {
	err = e.db.View(func(tx *bbolt.Tx) error {
		_, err = getJSON(tx, bucketServers, []byte(subscriptionID), &list)
		return err
	})
	return list, err
}

// Servers lists the servers of one subscription, or of all in the order of the subscriptions.
func (e *Engine) Servers(query ServersQuery) ([]Server, error) {
	servers := []Server{}
	err := e.db.View(func(tx *bbolt.Tx) error {
		subs, err := records(tx)
		if err != nil {
			return err
		}
		for _, sub := range subs {
			if query.SubscriptionID != "" && sub.ID != query.SubscriptionID {
				continue
			}
			var list []serverRecord
			if _, err = getJSON(tx, bucketServers, []byte(sub.ID), &list); err != nil {
				return err
			}
			for _, r := range list {
				servers = append(servers, r.view(sub.ID))
			}
		}
		return nil
	})
	return servers, err
}

// maxChangeNames bounds the names an update reports; the counts cover the rest.
const maxChangeNames = 50

// reconcile gives the incoming servers the IDs of the stored ones they continue, so that a chosen server survives
// an update. A server identical to a stored one keeps its ID; then one with the same address and credentials does,
// counted as changed. As in the Qt client, each stored server is claimed once, in order.
func reconcile(old []serverRecord, incoming []profile.Profile) ([]serverRecord, SubscriptionUpdate) {
	update := SubscriptionUpdate{AddedNames: []string{}, RemovedNames: []string{}}
	servers := make([]serverRecord, len(incoming))
	claimed := make([]bool, len(old))
	matched := make([]bool, len(incoming))

	byContent := map[string][]int{}
	for i, r := range old {
		key := contentKey(r.Profile)
		byContent[key] = append(byContent[key], i)
	}
	for i, p := range incoming {
		servers[i].Profile = p
		key := contentKey(p)
		if candidates := byContent[key]; len(candidates) > 0 {
			servers[i].ID, claimed[candidates[0]], matched[i] = old[candidates[0]].ID, true, true
			byContent[key] = candidates[1:]
		}
	}

	byIdentity := map[string][]int{}
	for i, r := range old {
		if !claimed[i] {
			key := identityKey(r.Profile)
			byIdentity[key] = append(byIdentity[key], i)
		}
	}
	for i, p := range incoming {
		if matched[i] {
			continue
		}
		key := identityKey(p)
		if candidates := byIdentity[key]; len(candidates) > 0 {
			servers[i].ID, claimed[candidates[0]] = old[candidates[0]].ID, true
			byIdentity[key] = candidates[1:]
			update.Changed++
			continue
		}
		servers[i].ID = newID()
		update.Added++
		if len(update.AddedNames) < maxChangeNames {
			update.AddedNames = append(update.AddedNames, servers[i].view("").Name)
		}
	}
	for i, r := range old {
		if claimed[i] {
			continue
		}
		update.Removed++
		if len(update.RemovedNames) < maxChangeNames {
			update.RemovedNames = append(update.RemovedNames, r.view("").Name)
		}
	}
	return servers, update
}

func contentKey(p profile.Profile) string {
	data, _ := json.Marshal(p)
	return string(data)
}

// identityKey is the server a profile reaches and the account it uses there.
func identityKey(p profile.Profile) string {
	var account string
	switch {
	case p.VLESS != nil:
		account = p.VLESS.UUID
	case p.VMess != nil:
		account = p.VMess.UUID
	case p.Trojan != nil:
		account = p.Trojan.Password
	case p.Shadowsocks != nil:
		account = p.Shadowsocks.Method + ":" + p.Shadowsocks.Password
	case p.Hysteria2 != nil:
		account = p.Hysteria2.Password
	case p.TUIC != nil:
		account = p.TUIC.UUID + ":" + p.TUIC.Password
	case p.SOCKS != nil:
		account = p.SOCKS.Username + ":" + p.SOCKS.Password
	case p.HTTP != nil:
		account = p.HTTP.Username + ":" + p.HTTP.Password
	case p.AnyTLS != nil:
		account = p.AnyTLS.Password
	}
	return strings.Join([]string{p.Type, p.Server, strconv.Itoa(p.ServerPort), account}, "\x00")
}

// flagCountry reads the first flag in a name, two regional indicator letters, as an ISO 3166 code.
func flagCountry(name string) string {
	const first, last = 0x1F1E6, 0x1F1FF
	runes := []rune(name)
	for i := 0; i+1 < len(runes); i++ {
		a, b := runes[i], runes[i+1]
		if a >= first && a <= last && b >= first && b <= last {
			return string([]rune{'A' + a - first, 'A' + b - first})
		}
	}
	return ""
}
