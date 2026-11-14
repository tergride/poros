package jsonrpc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// hangLimit bounds one input. The fuzzing engine has no time limit of its own.
const hangLimit = 10 * time.Second

// watchHang crashes the process when an input runs past hangLimit, which keeps the input as a crasher.
func watchHang() (stop func()) {
	timer := time.AfterFunc(hangLimit, func() {
		debug.SetTraceback("all")
		panic("the input ran longer than " + hangLimit.String())
	})
	return func() { timer.Stop() }
}

// stream is the peer's side of the pipe: what it sent, then the end of the stream, and what the core wrote back.
type stream struct {
	in     io.Reader
	mu     sync.Mutex
	out    bytes.Buffer
	closed bool
}

func (s *stream) Read(p []byte) (int, error) { return s.in.Read(p) }

func (s *stream) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, io.ErrClosedPipe
	}
	return s.out.Write(p)
}

func (s *stream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

const fuzzToken = "secret"

// requestLines counts the lines Serve reads as requests: the non-blank lines of the stream.
func requestLines(data []byte) int {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64<<10), maxMessage)
	n := 0
	for scanner.Scan() {
		if len(bytes.TrimSpace(scanner.Bytes())) > 0 {
			n++
		}
	}
	return n
}

// The core reads whatever comes down its pipe: whatever the bytes, Serve ends with the stream, sends a valid reply to
// each request it read, and runs no method before hello with the token.
func FuzzServe(f *testing.F) {
	hello := `{"jsonrpc":"2.0","id":"h","method":"hello","params":{"token":"` + fuzzToken + `"}}` + "\n"
	for _, text := range []string{
		hello + `{"jsonrpc":"2.0","id":2,"method":"add","params":{"a":2,"b":3}}` + "\n",
		hello + `{"jsonrpc":"2.0","id":3,"method":"nope"}` + "\r\n" + `{"jsonrpc":"2.0","id":4,"method":"add","params":{"a":1,"c":2}}`,
		hello + `{"jsonrpc":"2.0","method":"add"}` + "\n\n  \n" + `{"jsonrpc":"2.0","id":{"x":[1,2]},"method":"fail"}` + "\n",
		hello + `{"jsonrpc":"2.0","id":null,"method":"none","params":[1,"two"]}` + "\nnot json\n" + `{"jsonrpc":"2.0","id":7,"method":"add","params":`,
		`{"jsonrpc":"2.0","id":1,"method":"hello","params":{"token":"guess"}}`,
		`{"jsonrpc":"2.0","id":1,"method":"add"}`,
		`{"jsonrpc":"2.0","id":1,"method":"hello"}` + "\n" + hello,
		"hello\n",
		hello + `{"jsonrpc":"2.0","id":5,"method":"add","params":{"a":1e400}}` + "\n" + `{"jsonrpc":"2.0","id":"` + strings.Repeat("x", 4096) + `","method":"none"}`,
		"\xef\xbb\xbf" + hello,
	} {
		f.Add([]byte(text))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		defer watchHang()()
		var calls atomic.Int32
		handlers := map[string]Handler{
			"add": func(_ context.Context, raw json.RawMessage) (any, error) {
				calls.Add(1)
				var params struct {
					A int `json:"a"`
					B int `json:"b"`
				}
				if err := DecodeParams(raw, &params); err != nil {
					return nil, err
				}
				return params.A + params.B, nil
			},
			"fail": func(context.Context, json.RawMessage) (any, error) {
				calls.Add(1)
				return nil, Failed("not_found", "no such thing")
			},
			"none": func(context.Context, json.RawMessage) (any, error) {
				calls.Add(1)
				return nil, nil
			},
		}
		s := &stream{in: bytes.NewReader(data)}
		err := NewConn(s).Serve(context.Background(), fuzzToken, handlers)

		var replies []map[string]json.RawMessage
		welcomed := false
		for line := range strings.Lines(s.out.String()) {
			var reply map[string]json.RawMessage
			if err := json.Unmarshal([]byte(line), &reply); err != nil {
				t.Fatalf("the reply %q is no JSON object: %v", line, err)
			}
			_, hasResult := reply["result"]
			_, hasError := reply["error"]
			if string(reply["jsonrpc"]) != `"2.0"` || reply["id"] == nil || hasResult == hasError {
				t.Fatalf("the reply %q is no JSON-RPC 2.0 response", line)
			}
			// Only hello answers with an empty object.
			welcomed = welcomed || string(reply["result"]) == "{}"
			replies = append(replies, reply)
		}
		requests := requestLines(data)
		switch {
		case errors.Is(err, io.EOF) && len(replies) != requests:
			t.Fatalf("%d requests got %d replies", requests, len(replies))
		case !errors.Is(err, io.EOF) && (len(replies) > requests || len(replies) > 0 && replies[len(replies)-1]["error"] == nil):
			t.Fatalf("Serve ended with %v after %d replies to %d requests", err, len(replies), requests)
		}
		if calls.Load() > 0 && !welcomed {
			t.Fatalf("a method ran without hello and its token")
		}
	})
}

// endless is a line that never ends.
type endless struct{}

func (endless) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	return len(p), nil
}

// A line longer than any message ends the connection instead of growing without bound.
func TestServeRefusesAGiantLine(t *testing.T) {
	s := &stream{in: io.MultiReader(strings.NewReader(`{"jsonrpc":"2.0","id":"h","method":"hello","params":{"token":"`+fuzzToken+`"}}`+"\n"),
		io.LimitReader(endless{}, maxMessage+1))}
	err := NewConn(s).Serve(context.Background(), fuzzToken, nil)
	if !errors.Is(err, bufio.ErrTooLong) {
		t.Fatalf("Serve ended with %v, want %v", err, bufio.ErrTooLong)
	}
	if lines := strings.Count(s.out.String(), "\n"); lines != 1 {
		t.Fatalf("%d replies, want the one to hello", lines)
	}
}
