// Package sysproxy points the system proxy at Poros and gives the user's settings back: after Clear, and after a
// crash, at the next start, from a state file. The settings of another client or the user that replaced Poros's
// stay theirs.
package sysproxy

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Connection flags, as WinINet stores them.
const (
	flagDirect uint32 = 1
	flagProxy  uint32 = 2
)

// ErrUnsupported is returned where the platform has no system proxy Poros sets.
var ErrUnsupported = errors.New("the system proxy is not supported here")

// backend reaches the platform's settings. Connections are named; "" is the default one, the LAN on Windows.
type backend interface {
	// connections lists the connections, the default one first. An error still comes with the default one.
	connections() ([]string, error)
	flags(connection string) uint32
	server(connection string) string
	set(connection string, flags uint32, server string) error
	// announce tells running programs the settings changed.
	announce()
}

type saved struct {
	Name   string `json:"name"`
	Flags  uint32 `json:"flags"`
	Server string `json:"server"`
}

type stateFile struct {
	Applied     string  `json:"applied"`
	Connections []saved `json:"connections"`
}

// Proxy is the system proxy of one data folder.
type Proxy struct {
	mu      sync.Mutex
	path    string
	backend backend
	// applied is the server Poros set; saved holds what each connection had before the first Apply.
	applied string
	saved   map[string]saved
	// serves tells whether a proxy server still answers; a test replaces it.
	serves func(server string) bool
}

// New keeps the state in path, a file of the data folder.
func New(path string) *Proxy {
	return newProxy(path, platformBackend())
}

func newProxy(path string, b backend) *Proxy {
	return &Proxy{path: path, backend: b, saved: map[string]saved{}, serves: stillServes}
}

// Apply points every connection at host:port, remembering what each had the first time.
func (p *Proxy) Apply(host string, port uint16) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.backend == nil {
		return ErrUnsupported
	}
	server := net.JoinHostPort(host, strconv.Itoa(int(port)))
	p.applied = server
	names, err := p.backend.connections()
	var errs []error
	if err != nil {
		errs = append(errs, err)
	}
	for _, name := range names {
		// Only the first Apply still sees the user's settings; a later one would record Poros's own.
		if _, ok := p.saved[name]; !ok {
			p.saved[name] = saved{Name: name, Flags: p.backend.flags(name), Server: p.backend.server(name)}
		}
		if err := p.backend.set(name, flagDirect|flagProxy, server); err != nil {
			errs = append(errs, err)
		}
	}
	errs = append(errs, p.save())
	p.backend.announce()
	return errors.Join(errs...)
}

// Clear gives every connection Poros changed its settings back, unless someone changed it since.
func (p *Proxy) Clear() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.clear()
}

// Recover gives the settings back after a crash, from the state file. It must run before the first Apply, and only
// in the core that holds the data folder: another one's proxy is not this one's to clear.
func (p *Proxy) Recover() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.backend == nil || len(p.saved) > 0 {
		return nil
	}
	data, err := os.ReadFile(p.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var state stateFile
	// A damaged file names nothing to give back; it goes either way.
	if json.Unmarshal(data, &state) != nil || state.Applied == "" || len(state.Connections) == 0 {
		return os.Remove(p.path)
	}
	p.applied = state.Applied
	for _, connection := range state.Connections {
		p.saved[connection.Name] = connection
	}
	return p.clear()
}

func (p *Proxy) clear() error {
	if p.backend == nil || len(p.saved) == 0 {
		return nil
	}
	var errs []error
	for name, connection := range p.saved {
		// Another client or the user changed it since Apply: what is there now is theirs.
		if current := p.backend.server(name); (current != "" && current != p.applied) || p.backend.flags(name)&flagProxy == 0 {
			delete(p.saved, name)
			continue
		}
		flags := connection.Flags | flagDirect
		// A proxy of this computer that has gone away would leave the browsers without a connection.
		if flags&flagProxy != 0 && !p.serves(connection.Server) {
			flags &^= flagProxy
		}
		if err := p.backend.set(name, flags, connection.Server); err != nil {
			errs = append(errs, err)
			continue
		}
		delete(p.saved, name)
	}
	p.backend.announce()
	errs = append(errs, p.save())
	return errors.Join(errs...)
}

// save writes the state, so that a start after a crash can still give the settings back.
func (p *Proxy) save() error {
	if len(p.saved) == 0 {
		if err := os.Remove(p.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	state := stateFile{Applied: p.applied}
	for _, connection := range p.saved {
		state.Connections = append(state.Connections, connection)
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	temp := p.path + ".tmp"
	if err = os.WriteFile(temp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(temp, p.path)
}

// stillServes checks the loopback servers of a proxy list ("host:port" or "http=host:port;https=host:port"):
// other addresses are taken as working.
func stillServes(server string) bool {
	entries := strings.FieldsFunc(server, func(r rune) bool { return r == ';' || r == ' ' || r == '\t' })
	for _, entry := range entries {
		if _, value, found := strings.Cut(entry, "="); found {
			entry = value
		}
		if _, value, found := strings.Cut(entry, "://"); found {
			entry = value
		}
		host, port, err := net.SplitHostPort(entry)
		if err != nil {
			host, port = strings.Trim(entry, "[]"), "80"
		}
		if !strings.EqualFold(host, "localhost") {
			if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
				return true
			}
		}
		if conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 300*time.Millisecond); err == nil {
			conn.Close()
			return true
		}
	}
	return len(entries) == 0
}
