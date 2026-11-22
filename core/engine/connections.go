package engine

import (
	"cmp"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/trafficcontrol"

	"PorosCore/config"
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
	var programPaths []string
	if owner := meta.ProcessInfo; owner != nil && len(owner.ProcessPaths) > 0 {
		programPaths = owner.ProcessPaths
		c.ProcessPath = programPaths[0]
		c.Process = filepath.Base(c.ProcessPath)
	}
	if m.Rule != nil {
		text := m.Rule.String()
		if len(programPaths) > 1 && strings.Contains(text, "process_path_regex=") {
			programPaths = matchedFirst(m.Rule, meta, programPaths)
		}
		c.Rule = config.RuleText(text, programPaths)
	}
	return c
}

// matchedFirst puts first the path the rule matches by: a service has two, of its DLL and of the program that hosts
// it, and the text of a pattern does not tell which one it found.
func matchedFirst(rule adapter.Rule, meta adapter.InboundContext, paths []string) []string {
	for i, path := range paths {
		owner := *meta.ProcessInfo
		owner.ProcessPaths = []string{path}
		alone := meta
		alone.ProcessInfo = &owner
		alone.ResetRuleCache()
		if rule.Match(&alone) {
			return slices.Concat(paths[i:], paths[:i])
		}
	}
	return paths
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
				if err := connection.Close(); err != nil {
					return jsonrpc.Failed(ReasonNotFound, "the connection %q is gone: %v", p.ID, err)
				}
				return nil
			}
		}
	}
	return jsonrpc.Failed(ReasonNotFound, "there is no connection %q", p.ID)
}
