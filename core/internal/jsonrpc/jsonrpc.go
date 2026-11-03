// Package jsonrpc serves JSON-RPC 2.0 over a byte stream, one message per line.
package jsonrpc

import (
	"bufio"
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"runtime/debug"
	"sync"
)

// A subscription can carry thousands of servers, so a message may be large.
const maxMessage = 64 << 20

const (
	CodeParse          = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternal       = -32603
	// CodeFailed is a valid request whose action did not succeed; Data says why.
	CodeFailed = 1
)

// MethodHello must come first with the token the host passed to the core.
const MethodHello = "hello"

// Handler answers one method. A *Error keeps its code, any other error becomes CodeInternal.
type Handler func(ctx context.Context, params json.RawMessage) (any, error)

// Error is sent back as the error object of the response.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *Error) Error() string { return e.Message }

// Failed reports an action that did not succeed. The reason is a stable token the interface translates.
func Failed(reason, format string, args ...any) error {
	return &Error{Code: CodeFailed, Message: fmt.Sprintf(format, args...), Data: map[string]string{"reason": reason}}
}

// DecodeParams reads params strictly: unknown fields are a client bug, not something to ignore.
func DecodeParams(raw json.RawMessage, v any) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = json.RawMessage("{}")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return &Error{Code: CodeInvalidParams, Message: err.Error()}
	}
	return nil
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

type notification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type helloParams struct {
	Token string `json:"token"`
}

// Conn is one peer. Writes are serialized, so handlers and Notify can run at the same time.
type Conn struct {
	rw      io.ReadWriteCloser
	writeMu sync.Mutex
}

func NewConn(rw io.ReadWriteCloser) *Conn {
	return &Conn{rw: rw}
}

// Notify sends an event. A failed write is not reported: Serve sees the broken stream on its next read.
func (c *Conn) Notify(method string, params any) {
	if err := c.write(notification{JSONRPC: "2.0", Method: method, Params: params}); err != nil {
		log.Printf("jsonrpc: notify %s: %v", method, err)
	}
}

func (c *Conn) write(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err = c.rw.Write(data)
	return err
}

// Serve answers requests until the peer closes the stream, then closes it and returns io.EOF.
// Nothing but hello with the right token is answered first; anything else ends the connection.
func (c *Conn) Serve(ctx context.Context, token string, handlers map[string]Handler) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer c.rw.Close()

	scanner := bufio.NewScanner(c.rw)
	scanner.Buffer(make([]byte, 0, 64<<10), maxMessage)

	authenticated := false
	var running sync.WaitGroup
	defer running.Wait()

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			c.reply(nil, nil, &Error{Code: CodeParse, Message: err.Error()})
			if !authenticated {
				return err
			}
			continue
		}
		if req.JSONRPC != "2.0" || req.Method == "" || len(req.ID) == 0 {
			c.reply(req.ID, nil, &Error{Code: CodeInvalidRequest, Message: "a request needs jsonrpc 2.0, an id and a method"})
			if !authenticated {
				return errors.New("jsonrpc: invalid first request")
			}
			continue
		}

		if !authenticated {
			var hello helloParams
			if req.Method != MethodHello || DecodeParams(req.Params, &hello) != nil ||
				subtle.ConstantTimeCompare([]byte(hello.Token), []byte(token)) != 1 {
				c.reply(req.ID, nil, &Error{Code: CodeInvalidRequest, Message: "hello with the token must come first"})
				return errors.New("jsonrpc: peer did not authenticate")
			}
			authenticated = true
			c.reply(req.ID, struct{}{}, nil)
			continue
		}

		handler, ok := handlers[req.Method]
		if !ok {
			c.reply(req.ID, nil, &Error{Code: CodeMethodNotFound, Message: "unknown method " + req.Method})
			continue
		}
		// The scanner reuses its buffer, so the request keeps its own copy of the params.
		params := append(json.RawMessage(nil), req.Params...)
		id := append(json.RawMessage(nil), req.ID...)
		running.Add(1)
		go func() {
			defer running.Done()
			c.handle(ctx, id, req.Method, params, handler)
		}()
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return io.EOF
}

func (c *Conn) handle(ctx context.Context, id json.RawMessage, method string, params json.RawMessage, handler Handler) {
	defer func() {
		// One bad request must not take the core down.
		if r := recover(); r != nil {
			log.Printf("jsonrpc: %s panicked: %v\n%s", method, r, debug.Stack())
			c.reply(id, nil, &Error{Code: CodeInternal, Message: fmt.Sprint(r)})
		}
	}()
	result, err := handler(ctx, params)
	if err != nil {
		var rpcErr *Error
		if !errors.As(err, &rpcErr) {
			rpcErr = &Error{Code: CodeInternal, Message: err.Error()}
		}
		c.reply(id, nil, rpcErr)
		return
	}
	c.reply(id, result, nil)
}

func (c *Conn) reply(id json.RawMessage, result any, rpcErr *Error) {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	resp := response{JSONRPC: "2.0", ID: id, Error: rpcErr}
	if rpcErr == nil {
		data, err := json.Marshal(result)
		if err != nil {
			resp.Error = &Error{Code: CodeInternal, Message: err.Error()}
		} else {
			resp.Result = data
		}
	}
	if err := c.write(resp); err != nil {
		log.Printf("jsonrpc: reply: %v", err)
	}
}
