package engine

import (
	"log"
	"sync"

	"github.com/sagernet/bbolt"
)

// bucketTraffic holds the bytes that went through each server, by its ID.
var bucketTraffic = []byte("traffic")

// trafficTotal is what went through one server: upload and download in bytes.
type trafficTotal struct {
	Upload   int64 `json:"upload"`
	Download int64 `json:"download"`
}

// serverTraffic counts the bytes of each server in memory and keeps them in bucketTraffic now and then.
type serverTraffic struct {
	mu     sync.Mutex
	totals map[string]trafficTotal
	dirty  map[string]bool
}

func (e *Engine) loadTraffic() error {
	e.traffic.totals, e.traffic.dirty = map[string]trafficTotal{}, map[string]bool{}
	return e.db.View(func(tx *bbolt.Tx) error {
		return tx.Bucket(bucketTraffic).ForEach(func(key, _ []byte) error {
			var total trafficTotal
			if _, err := getJSON(tx, bucketTraffic, key, &total); err != nil {
				return err
			}
			e.traffic.totals[string(key)] = total
			return nil
		})
	})
}

// addTraffic counts bytes that went through a server; saveTraffic keeps them.
func (e *Engine) addTraffic(id string, upload, download int64) {
	if upload <= 0 && download <= 0 {
		return
	}
	e.traffic.mu.Lock()
	defer e.traffic.mu.Unlock()
	total := e.traffic.totals[id]
	total.Upload += max(upload, 0)
	total.Download += max(download, 0)
	e.traffic.totals[id] = total
	e.traffic.dirty[id] = true
}

func (e *Engine) trafficOf(id string) trafficTotal {
	e.traffic.mu.Lock()
	defer e.traffic.mu.Unlock()
	return e.traffic.totals[id]
}

// saveTraffic writes the totals that changed since the last save.
func (e *Engine) saveTraffic() {
	e.traffic.mu.Lock()
	changed := map[string]trafficTotal{}
	for id := range e.traffic.dirty {
		changed[id] = e.traffic.totals[id]
	}
	e.traffic.dirty = map[string]bool{}
	e.traffic.mu.Unlock()
	if len(changed) == 0 {
		return
	}
	err := e.db.Update(func(tx *bbolt.Tx) error {
		for id, total := range changed {
			if total == (trafficTotal{}) {
				if err := tx.Bucket(bucketTraffic).Delete([]byte(id)); err != nil {
					return err
				}
				continue
			}
			if err := putJSON(tx, bucketTraffic, []byte(id), total); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Printf("traffic of the servers: %v", err)
	}
}

// ResetTraffic sets the traffic of servers back to nothing.
func (e *Engine) ResetTraffic(query ServersQuery) error {
	groups, err := e.queryGroups(query)
	if err != nil {
		return err
	}
	e.traffic.mu.Lock()
	for _, ids := range groups {
		for _, id := range ids {
			if _, ok := e.traffic.totals[id]; ok {
				e.traffic.totals[id] = trafficTotal{}
				e.traffic.dirty[id] = true
			}
		}
	}
	e.traffic.mu.Unlock()
	e.saveTraffic()
	for groupID := range groups {
		e.notify(EventServersChanged, ServersChanged{GroupID: groupID})
	}
	return nil
}

// serverView is a server with what Poros measured of it: delay, traffic, speed and IP.
func (e *Engine) serverView(r serverRecord, groupID string) Server {
	view := r.view(groupID)
	view.Latency = e.latencyOf(r.ID)
	traffic := e.trafficOf(r.ID)
	view.Upload, view.Download = traffic.Upload, traffic.Download
	speed := e.results.speedOf(r.ID)
	view.SpeedDown, view.SpeedUp = speed.Download, speed.Upload
	ip := e.results.ipOf(r.ID)
	view.ExitIP, view.ExitCountry = ip.IP, ip.Country
	return view
}
