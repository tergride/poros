package engine

import (
	"cmp"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sagernet/sing-box/common/trafficcontrol"

	"PorosCore/internal/jsonrpc"
)

// tracker is the statistics of the running connection, nil while there is none or the settings turn them off.
func (e *Engine) tracker() *trafficcontrol.Manager {
	return e.conn.tracker.Load()
}

// Connections lists the open connections through Poros, the oldest first.
func (e *Engine) Connections() (ConnectionList, error) {
	tracker := e.tracker()
	list := ConnectionList{Connections: []Connection{}, Tracked: tracker != nil}
	if tracker == nil {
		return list, nil
	}
	for _, m := range tracker.Connections() {
		list.Connections = append(list.Connections, connectionOf(m))
	}
	slices.SortFunc(list.Connections, func(a, b Connection) int {
		return cmp.Or(cmp.Compare(a.Start, b.Start), strings.Compare(a.ID, b.ID))
	})
	return list, nil
}

func connectionOf(m *trafficcontrol.TrackerMetadata) Connection {
	meta := m.Metadata
	c := Connection{
		ID:       m.ID.String(),
		Start:    m.CreatedAt.UnixMilli(),
		Network:  meta.Network,
		Protocol: meta.Protocol,
		Inbound:  meta.Inbound,
		Outbound: m.Outbound,
		Source:   meta.Source.String(),
		Upload:   m.Upload.Load(),
		Download: m.Download.Load(),
	}
	destination := meta.Destination
	if meta.Domain != "" {
		destination.Fqdn = meta.Domain
	}
	c.Destination = destination.String()
	if m.Rule != nil {
		c.Rule = m.Rule.String()
	}
	if owner := meta.ProcessInfo; owner != nil && len(owner.ProcessPaths) > 0 {
		c.ProcessPath = owner.ProcessPaths[0]
		c.Process = filepath.Base(c.ProcessPath)
	}
	return c
}

// CloseConnections closes one connection, or all of them without an ID.
func (e *Engine) CloseConnections(p ByID) error {
	tracker := e.tracker()
	if tracker == nil {
		return nil
	}
	if p.ID == "" {
		tracker.CloseAllConnections()
		return nil
	}
	for _, m := range tracker.Connections() {
		if m.ID.String() == p.ID {
			if connection := tracker.Connection(m.ID); connection != nil {
				return connection.Close()
			}
		}
	}
	return jsonrpc.Failed(ReasonNotFound, "there is no connection %q", p.ID)
}
