package engine

import (
	"cmp"
	"context"
	"encoding/binary"
	"encoding/json"
	"log"
	"net"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sagernet/bbolt"
	"github.com/sagernet/sing-box/adapter"
	tun "github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common/bufio"
	N "github.com/sagernet/sing/common/network"

	"PorosCore/internal/jsonrpc"
)

const (
	// HistoryFile keeps the traffic by the hour, apart from the data: it only grows with use and may be lost.
	HistoryFile  = "traffic.db"
	historyKeep  = 90 * 24 * time.Hour
	historyFlush = time.Minute
)

var bucketHours = []byte("hours")

// trafficHistory is the file of the history, opened at its first use.
type trafficHistory struct {
	mu sync.Mutex
	db *bbolt.DB
}

func (e *Engine) historyDB() (*bbolt.DB, error) {
	e.history.mu.Lock()
	defer e.history.mu.Unlock()
	if e.history.db != nil {
		return e.history.db, nil
	}
	db, err := bbolt.Open(filepath.Join(e.dataDir, HistoryFile), 0o600, &bbolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, err
	}
	if err = db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucketHours)
		return err
	}); err != nil {
		db.Close()
		return nil, err
	}
	e.history.db = db
	return db, nil
}

func (e *Engine) closeHistory() error {
	e.history.mu.Lock()
	defer e.history.mu.Unlock()
	if e.history.db == nil {
		return nil
	}
	err := e.history.db.Close()
	e.history.db = nil
	return err
}

// counterPair counts the bytes of one profile or one program.
type counterPair struct {
	upload, download atomic.Int64
}

// historyTracker counts the traffic of a connection by profile and by program, until the next record takes it.
type historyTracker struct {
	serverID string
	mu       sync.Mutex
	profiles map[string]*counterPair
	apps     map[string]*counterPair
}

var _ adapter.ConnectionTracker = (*historyTracker)(nil)

func newHistoryTracker(serverID string) *historyTracker {
	return &historyTracker{serverID: serverID, profiles: map[string]*counterPair{}, apps: map[string]*counterPair{}}
}

func (h *historyTracker) pair(list map[string]*counterPair, key string) *counterPair {
	pair := list[key]
	if pair == nil {
		pair = &counterPair{}
		list[key] = pair
	}
	return pair
}

// counters are those a connection counts to: its program, and the profile when it goes through the server.
func (h *historyTracker) counters(metadata adapter.InboundContext, outbound adapter.Outbound) (uploads, downloads []*atomic.Int64) {
	app := ""
	if owner := metadata.ProcessInfo; owner != nil && len(owner.ProcessPaths) > 0 {
		app = filepath.Base(owner.ProcessPaths[0])
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	pairs := []*counterPair{h.pair(h.apps, app)}
	if throughServer(outbound) {
		pairs = append(pairs, h.pair(h.profiles, h.serverID))
	}
	for _, pair := range pairs {
		uploads, downloads = append(uploads, &pair.upload), append(downloads, &pair.download)
	}
	return uploads, downloads
}

func (h *historyTracker) RoutedConnection(_ context.Context, conn net.Conn, metadata adapter.InboundContext, _ adapter.Rule, outbound adapter.Outbound) net.Conn {
	uploads, downloads := h.counters(metadata, outbound)
	return bufio.NewInt64CounterConn(conn, uploads, downloads)
}

func (h *historyTracker) RoutedPacketConnection(_ context.Context, conn N.PacketConn, metadata adapter.InboundContext, _ adapter.Rule, outbound adapter.Outbound) N.PacketConn {
	uploads, downloads := h.counters(metadata, outbound)
	return bufio.NewInt64CounterPacketConn(conn, uploads, nil, downloads, nil)
}

func (h *historyTracker) RoutedFlow(context.Context, adapter.InboundContext, adapter.Rule, adapter.Outbound) tun.FlowTracker {
	return nil
}

// hourTraffic is one hour of the history: bytes up and down by profile and by program.
type hourTraffic struct {
	Profiles map[string][2]int64 `json:"p,omitempty"`
	Apps     map[string][2]int64 `json:"a,omitempty"`
}

// take moves what was counted since the last take into hour.
func (h *historyTracker) take(hour *hourTraffic) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	moved := false
	for _, list := range []struct {
		from map[string]*counterPair
		to   *map[string][2]int64
	}{{h.profiles, &hour.Profiles}, {h.apps, &hour.Apps}} {
		for key, pair := range list.from {
			up, down := pair.upload.Swap(0), pair.download.Swap(0)
			if up == 0 && down == 0 {
				continue
			}
			if *list.to == nil {
				*list.to = map[string][2]int64{}
			}
			sum := (*list.to)[key]
			(*list.to)[key] = [2]int64{sum[0] + up, sum[1] + down}
			moved = true
		}
	}
	return moved
}

func hourKey(t time.Time) []byte {
	return binary.BigEndian.AppendUint64(nil, uint64(t.Unix()/3600))
}

// recordHistory writes what h counted into the hour of now, and drops hours older than the history keeps.
func (e *Engine) recordHistory(h *historyTracker) {
	if h == nil {
		return
	}
	var counted hourTraffic
	if !h.take(&counted) {
		return
	}
	db, err := e.historyDB()
	if err != nil {
		log.Printf("traffic history: %v", err)
		return
	}
	now := e.now()
	err = db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(bucketHours)
		key := hourKey(now)
		var hour hourTraffic
		if data := bucket.Get(key); data != nil {
			_ = json.Unmarshal(data, &hour)
		}
		for _, part := range []struct {
			from map[string][2]int64
			to   *map[string][2]int64
		}{{counted.Profiles, &hour.Profiles}, {counted.Apps, &hour.Apps}} {
			for name, bytes := range part.from {
				if *part.to == nil {
					*part.to = map[string][2]int64{}
				}
				sum := (*part.to)[name]
				(*part.to)[name] = [2]int64{sum[0] + bytes[0], sum[1] + bytes[1]}
			}
		}
		data, err := json.Marshal(hour)
		if err != nil {
			return err
		}
		if err = bucket.Put(key, data); err != nil {
			return err
		}
		cutoff := hourKey(now.Add(-historyKeep))
		cursor := bucket.Cursor()
		for k, _ := cursor.First(); k != nil && string(k) < string(cutoff); k, _ = cursor.Next() {
			if err = cursor.Delete(); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Printf("traffic history: %v", err)
	}
}

// keepHistory records the history of a connection every historyFlush until ctx ends; the stop records the rest.
func (e *Engine) keepHistory(ctx context.Context, h *historyTracker) {
	ticker := time.NewTicker(historyFlush)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.recordHistory(h)
		}
	}
}

// TrafficHistory sums the history of the last q.Hours hours by profile and by program, the most first.
func (e *Engine) TrafficHistory(q TrafficQuery) (TrafficHistory, error) {
	if q.Hours < 1 || q.Hours > int(historyKeep/time.Hour) {
		return TrafficHistory{}, jsonrpc.Failed(ReasonInvalidValue, "hours must be from 1 to %d, not %d", int(historyKeep/time.Hour), q.Hours)
	}
	e.recordHistory(e.conn.history.Load())
	result := TrafficHistory{Profiles: []TrafficItem{}, Apps: []TrafficItem{}}
	db, err := e.historyDB()
	if err != nil {
		return result, err
	}
	profiles, apps := map[string][2]int64{}, map[string][2]int64{}
	from := hourKey(e.now().Add(-time.Duration(q.Hours-1) * time.Hour))
	err = db.View(func(tx *bbolt.Tx) error {
		cursor := tx.Bucket(bucketHours).Cursor()
		for k, v := cursor.Seek(from); k != nil; k, v = cursor.Next() {
			var hour hourTraffic
			if json.Unmarshal(v, &hour) != nil {
				continue
			}
			for _, part := range []struct {
				from map[string][2]int64
				to   map[string][2]int64
			}{{hour.Profiles, profiles}, {hour.Apps, apps}} {
				for name, bytes := range part.from {
					sum := part.to[name]
					part.to[name] = [2]int64{sum[0] + bytes[0], sum[1] + bytes[1]}
				}
			}
		}
		return nil
	})
	result.Profiles, result.Apps = trafficItems(profiles), trafficItems(apps)
	return result, err
}

func trafficItems(sums map[string][2]int64) []TrafficItem {
	items := make([]TrafficItem, 0, len(sums))
	for key, bytes := range sums {
		items = append(items, TrafficItem{Key: key, Upload: bytes[0], Download: bytes[1]})
	}
	slices.SortFunc(items, func(a, b TrafficItem) int {
		return cmp.Or(cmp.Compare(b.Upload+b.Download, a.Upload+a.Download), strings.Compare(a.Key, b.Key))
	})
	return items
}

// historyOn tells whether a connection with these settings keeps the history; it needs the statistics.
func historyOn(s Settings) bool {
	return s.Stats && s.TrafficHistory
}
