// Package patches checks the changes Poros carries in its copies of other projects in third_party: each test fails on
// the code as it was before the change.
package patches

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"
	"time"

	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

// pipeDialer answers each dial with one end of a pipe, or waits for the end of the dial with block, and hands over the
// context of every dial.
type pipeDialer struct {
	contexts chan context.Context
	block    bool
}

func (d *pipeDialer) DialContext(ctx context.Context, _ string, _ M.Socksaddr) (net.Conn, error) {
	d.contexts <- ctx
	if d.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	client, server := net.Pipe()
	server.Close()
	return client, nil
}

func (d *pipeDialer) ListenPacket(context.Context, M.Socksaddr) (net.PacketConn, error) {
	return nil, errors.New("no packets here")
}

// Two addresses, as a name with A and AAAA has: DialParallel races them.
var twoAddresses = []netip.Addr{netip.MustParseAddr("192.0.2.1"), netip.MustParseAddr("2001:db8::1")}

// The context of the attempt that won is let go once the dial is over. As a child of the dial's context it stayed in
// memory as long as that one, and the context of the TUN inbound lives as long as the connection of Poros: a little
// memory for every connection through the TUN.
func TestDialParallelLetsGoOfTheWinner(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	d := &pipeDialer{contexts: make(chan context.Context, 2)}
	conn, err := N.DialParallel(parent, d, N.NetworkTCP, M.ParseSocksaddrHostPort("example.com", 443), twoAddresses, false, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	winner := <-d.contexts
	cancel()
	if winner.Err() != nil {
		t.Fatal("the context of the winning attempt is still a child of the dial's context")
	}
}

// A dial still under way stops with its context, as before the change.
func TestDialParallelStopsWithItsContext(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	d := &pipeDialer{contexts: make(chan context.Context, 2), block: true}
	done := make(chan error, 1)
	go func() {
		_, err := N.DialParallel(parent, d, N.NetworkTCP, M.ParseSocksaddrHostPort("example.com", 443), twoAddresses, false, time.Hour)
		done <- err
	}()
	<-d.contexts
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("the dial succeeded after its context ended")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the dial did not stop with its context")
	}
}
