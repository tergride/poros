package boxmain

import (
	"context"
	"net"
	"sync"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/adapter/inbound"
	"github.com/sagernet/sing-box/common/listener"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/protocol/mixed"
	N "github.com/sagernet/sing/common/network"
)

// inboundRegistry is sing-box's, with the mixed inbound that closes its connections along with the box.
func inboundRegistry() *inbound.Registry {
	registry := include.InboundRegistry()
	inbound.Register[option.HTTPMixedInboundOptions](registry, C.TypeMixed, newClosingMixed)
	return registry
}

// closingMixed is sing-box's mixed inbound behind a listener of its own, which remembers the connections it took.
// sing-box leaves an accepted HTTP proxy connection to its own loop: once the box is closed, that loop answers every
// further request on a kept-alive connection with 502 Bad Gateway. A program that keeps its connection to the
// proxy of Windows, Windows checking the network among them, would stay without a connection after every reconnect.
type closingMixed struct {
	*mixed.Inbound
	listener *listener.Listener
	access   sync.Mutex
	conns    map[net.Conn]struct{}
	closed   bool
}

func newClosingMixed(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.HTTPMixedInboundOptions) (adapter.Inbound, error) {
	created, err := mixed.NewInbound(ctx, router, logger, tag, options)
	if err != nil {
		return nil, err
	}
	inner, ok := created.(*mixed.Inbound)
	// Poros sets no TLS on its inbound; the inbound with TLS starts its own as it is.
	if !ok || options.TLS != nil {
		return created, nil
	}
	in := &closingMixed{Inbound: inner, conns: map[net.Conn]struct{}{}}
	// The inner inbound's own listener is never started: SOCKS UDP still takes its sockets from it.
	in.listener = listener.New(listener.Options{
		Context:           ctx,
		Logger:            logger,
		Network:           []string{N.NetworkTCP},
		Listen:            options.ListenOptions,
		ConnectionHandler: in,
		SetSystemProxy:    options.SetSystemProxy,
		SystemProxySOCKS:  true,
	})
	return in, nil
}

func (m *closingMixed) Start(stage adapter.StartStage, scope *adapter.Scope) error {
	if stage != adapter.StartStateStart {
		return nil
	}
	if err := m.listener.Start(); err != nil {
		return err
	}
	scope.Add(m.listener.Close)
	scope.Add(m.closeConnections)
	return nil
}

func (m *closingMixed) NewConnection(ctx context.Context, conn net.Conn, metadata adapter.InboundContext, onClose N.CloseHandlerFunc) {
	m.access.Lock()
	if m.closed {
		m.access.Unlock()
		conn.Close()
		return
	}
	m.conns[conn] = struct{}{}
	m.access.Unlock()
	m.Inbound.NewConnection(ctx, conn, metadata, func(err error) {
		m.access.Lock()
		delete(m.conns, conn)
		m.access.Unlock()
		if onClose != nil {
			onClose(err)
		}
	})
}

func (m *closingMixed) closeConnections() error {
	m.access.Lock()
	conns := m.conns
	m.conns, m.closed = nil, true
	m.access.Unlock()
	for conn := range conns {
		conn.Close()
	}
	return nil
}
