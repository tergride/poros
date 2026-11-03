package jsonrpc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

type peer struct {
	t     *testing.T
	conn  net.Conn
	lines chan map[string]any
}

func startServer(t *testing.T, handlers map[string]Handler) (*peer, *Conn, chan error) {
	t.Helper()
	serverSide, clientSide := net.Pipe()
	conn := NewConn(serverSide)
	done := make(chan error, 1)
	go func() { done <- conn.Serve(context.Background(), "secret", handlers) }()

	p := &peer{t: t, conn: clientSide, lines: make(chan map[string]any, 16)}
	go func() {
		scanner := bufio.NewScanner(clientSide)
		for scanner.Scan() {
			var msg map[string]any
			if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
				t.Errorf("server sent invalid JSON %q: %v", scanner.Text(), err)
				continue
			}
			p.lines <- msg
		}
		close(p.lines)
	}()
	t.Cleanup(func() { clientSide.Close() })
	return p, conn, done
}

func (p *peer) send(line string) {
	p.t.Helper()
	if _, err := p.conn.Write([]byte(line + "\n")); err != nil {
		p.t.Fatalf("write: %v", err)
	}
}

func (p *peer) next() map[string]any {
	p.t.Helper()
	select {
	case msg, ok := <-p.lines:
		if !ok {
			p.t.Fatal("server closed the stream")
		}
		return msg
	case <-time.After(5 * time.Second):
		p.t.Fatal("no message from the server")
		return nil
	}
}

func errorCode(t *testing.T, msg map[string]any) float64 {
	t.Helper()
	e, ok := msg["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected an error, got %v", msg)
	}
	return e["code"].(float64)
}

func waitDone(t *testing.T, done chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return")
		return nil
	}
}

func TestRequiresHelloWithToken(t *testing.T) {
	for name, first := range map[string]string{
		"wrong token":   `{"jsonrpc":"2.0","id":1,"method":"hello","params":{"token":"guess"}}`,
		"other method":  `{"jsonrpc":"2.0","id":1,"method":"app.info"}`,
		"not json":      `hello`,
		"missing token": `{"jsonrpc":"2.0","id":1,"method":"hello"}`,
	} {
		t.Run(name, func(t *testing.T) {
			called := false
			p, _, done := startServer(t, map[string]Handler{"app.info": func(context.Context, json.RawMessage) (any, error) {
				called = true
				return nil, nil
			}})
			p.send(first)
			if code := errorCode(t, p.next()); code >= 0 {
				t.Fatalf("expected a protocol error code, got %v", code)
			}
			if err := waitDone(t, done); err == nil || errors.Is(err, io.EOF) {
				t.Fatalf("Serve should fail on a peer that does not authenticate, got %v", err)
			}
			if called {
				t.Fatal("a handler ran before hello")
			}
		})
	}
}

func TestDispatch(t *testing.T) {
	type addParams struct {
		A int `json:"a"`
		B int `json:"b"`
	}
	p, conn, done := startServer(t, map[string]Handler{
		"add": func(_ context.Context, raw json.RawMessage) (any, error) {
			var params addParams
			if err := DecodeParams(raw, &params); err != nil {
				return nil, err
			}
			return params.A + params.B, nil
		},
		"fail":  func(context.Context, json.RawMessage) (any, error) { return nil, Failed("not_found", "no such thing") },
		"boom":  func(context.Context, json.RawMessage) (any, error) { panic("boom") },
		"plain": func(context.Context, json.RawMessage) (any, error) { return nil, errors.New("disk full") },
		"none":  func(context.Context, json.RawMessage) (any, error) { return nil, nil },
	})

	p.send(`{"jsonrpc":"2.0","id":"h","method":"hello","params":{"token":"secret"}}`)
	if msg := p.next(); msg["id"] != "h" || msg["error"] != nil {
		t.Fatalf("hello failed: %v", msg)
	}

	p.send(`{"jsonrpc":"2.0","id":2,"method":"add","params":{"a":2,"b":3}}`)
	if msg := p.next(); msg["id"].(float64) != 2 || msg["result"].(float64) != 5 {
		t.Fatalf("add: %v", msg)
	}

	cases := []struct {
		line string
		code float64
	}{
		{`{"jsonrpc":"2.0","id":3,"method":"nope"}`, CodeMethodNotFound},
		{`{"jsonrpc":"2.0","id":4,"method":"add","params":{"a":1,"c":2}}`, CodeInvalidParams},
		{`{"jsonrpc":"2.0","id":5,"method":"boom"}`, CodeInternal},
		{`{"jsonrpc":"2.0","id":6,"method":"plain"}`, CodeInternal},
		{`{"jsonrpc":"2.0","method":"add"}`, CodeInvalidRequest},
	}
	for _, c := range cases {
		p.send(c.line)
		if code := errorCode(t, p.next()); code != c.code {
			t.Errorf("%s: code %v, want %v", c.line, code, c.code)
		}
	}

	p.send(`{"jsonrpc":"2.0","id":7,"method":"fail"}`)
	msg := p.next()
	if errorCode(t, msg) != CodeFailed || msg["error"].(map[string]any)["data"].(map[string]any)["reason"] != "not_found" {
		t.Fatalf("fail: %v", msg)
	}

	p.send(`{"jsonrpc":"2.0","id":8,"method":"none"}`)
	msg = p.next()
	if result, present := msg["result"]; !present || result != nil {
		t.Fatalf("a nil result must still be sent as null: %v", msg)
	}

	go conn.Notify("settings.changed", map[string]bool{"tun": true})
	msg = p.next()
	if msg["method"] != "settings.changed" || msg["id"] != nil {
		t.Fatalf("notification: %v", msg)
	}

	p.conn.Close()
	if err := waitDone(t, done); !errors.Is(err, io.EOF) {
		t.Fatalf("Serve should end with io.EOF when the peer leaves, got %v", err)
	}
}
