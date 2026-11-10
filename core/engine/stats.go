package engine

import (
	"os"
	"path/filepath"
	"runtime"

	C "github.com/sagernet/sing-box/constant"
	"github.com/xtls/xray-core/core"

	"PorosCore/config"
	"PorosCore/internal/netmon"
)

// StatsNow is what «Statistics → Now» shows that only the core knows; the window adds the server, its address and
// the check of the IP.
func (e *Engine) StatsNow() (StatsNow, error) {
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	now := StatsNow{
		Memory:    int64(memory.Sys),
		Uptime:    e.now().Sub(e.started).Milliseconds(),
		Databases: e.databasesSize(),
	}
	if iface := netmon.DefaultInterface(); iface != nil {
		now.Online, now.Network = true, iface.Name
	}
	switch {
	case e.ConnectionState().State != ConnectionRunning:
	case e.conn.onXray.Load():
		now.Core = "Xray " + core.Version()
	default:
		now.Core = "sing-box " + C.Version
	}
	if tracker := e.tracker(); tracker != nil {
		for _, m := range tracker.Connections() {
			switch m.Outbound {
			case config.TagProxy:
				now.ThroughServer++
			case config.TagDirect:
				now.Direct++
			default:
				now.Other++
			}
		}
	}
	now.NextSubscriptionUpdate = e.nextSubscriptionUpdate()
	return now, nil
}

// databasesSize is the size of the data files of the data folder.
func (e *Engine) databasesSize() int64 {
	var size int64
	for _, name := range []string{DataFile} {
		if info, err := os.Stat(filepath.Join(e.dataDir, name)); err == nil {
			size += info.Size()
		}
	}
	return size
}

// nextSubscriptionUpdate is when the schedule updates a subscription next, Unix milliseconds; 0 is never.
func (e *Engine) nextSubscriptionUpdate() int64 {
	settings, err := e.Settings()
	if err != nil {
		return 0
	}
	list, err := e.groups()
	if err != nil {
		return 0
	}
	var next int64
	for _, rec := range list {
		if rec.manual() || !updatesItself(rec, settings) {
			continue
		}
		if due := dueAt(rec, settings).UnixMilli(); next == 0 || due < next {
			next = due
		}
	}
	return next
}
