package engine

import (
	"context"
	"net"
	"sync/atomic"
	"time"

	"github.com/sagernet/sing-box/adapter"
	tun "github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common/bufio"
	N "github.com/sagernet/sing/common/network"

	"PorosCore/config"
)

// trafficCounter counts the bytes that go through the server: what the window shows as the connection's speed.
// Direct traffic is not the connection's.
type trafficCounter struct {
	upload   atomic.Int64
	download atomic.Int64
}

var _ adapter.ConnectionTracker = (*trafficCounter)(nil)

func throughServer(outbound adapter.Outbound) bool {
	return outbound != nil && outbound.Tag() == config.TagProxy
}

// RoutedConnection counts what the client writes as upload and what it reads as download.
func (t *trafficCounter) RoutedConnection(_ context.Context, conn net.Conn, _ adapter.InboundContext, _ adapter.Rule, outbound adapter.Outbound) net.Conn {
	if !throughServer(outbound) {
		return conn
	}
	return bufio.NewInt64CounterConn(conn, []*atomic.Int64{&t.upload}, []*atomic.Int64{&t.download})
}

func (t *trafficCounter) RoutedPacketConnection(_ context.Context, conn N.PacketConn, _ adapter.InboundContext, _ adapter.Rule, outbound adapter.Outbound) N.PacketConn {
	if !throughServer(outbound) {
		return conn
	}
	return bufio.NewInt64CounterPacketConn(conn, []*atomic.Int64{&t.upload}, nil, []*atomic.Int64{&t.download}, nil)
}

func (t *trafficCounter) RoutedFlow(context.Context, adapter.InboundContext, adapter.Rule, adapter.Outbound) tun.FlowTracker {
	return nil
}

// trafficSaveTicks is how often, in seconds, the traffic of the server in use is kept and shown in its group.
const trafficSaveTicks = 60

// reportTraffic sends the speed every second until ctx ends, and counts it to the server in use.
func (e *Engine) reportTraffic(ctx context.Context, counter *trafficCounter, serverID string) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var lastUpload, lastDownload int64
	ticks := 0
	save := func() {
		upload, download := counter.upload.Load(), counter.download.Load()
		e.addTraffic(serverID, upload-lastUpload, download-lastDownload)
		lastUpload, lastDownload = upload, download
		e.saveTraffic()
		if groups, err := e.queryGroups(ServersQuery{IDs: []string{serverID}}); err == nil {
			for groupID := range groups {
				e.notify(EventServersChanged, ServersChanged{GroupID: groupID})
			}
		}
	}
	defer save()
	var shownUpload, shownDownload int64
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			upload, download := counter.upload.Load(), counter.download.Load()
			e.notify(EventConnectionTraffic, Traffic{
				Upload: upload - shownUpload, Download: download - shownDownload, TotalUpload: upload, TotalDownload: download,
			})
			shownUpload, shownDownload = upload, download
			if ticks++; ticks%trafficSaveTicks == 0 {
				save()
			}
		}
	}
}
