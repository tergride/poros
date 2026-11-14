package main

import (
	"encoding/json"
	"net"
	"os"
	"sync"
	"time"
)

// event is one thing the lab saw. Src is who asked, as the lab's internet sees it: the user's address behind the
// provider, the provider's resolver, or a VPN server.
type event struct {
	Time   string `json:"t"`
	Kind   string `json:"kind"`
	Src    string `json:"src"`
	Dst    string `json:"dst,omitempty"`
	Name   string `json:"name,omitempty"`
	Path   string `json:"path,omitempty"`
	Detail string `json:"detail,omitempty"`
	// Via is what the source address stands for: user, isp-dns, vpn or other.
	Via string `json:"via"`
}

// eventLog appends events to a file opened with O_APPEND: the host empties it between scenarios by truncating it,
// and the next write lands at its new end.
type eventLog struct {
	mu   sync.Mutex
	file *os.File
}

func openEventLog(path string) (*eventLog, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	return &eventLog{file: file}, nil
}

func (l *eventLog) add(e event) {
	e.Time = time.Now().UTC().Format(time.RFC3339Nano)
	if e.Via == "" {
		e.Via = viaOf(e.Src)
	}
	line, err := json.Marshal(e)
	if err != nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.file.Write(append(line, '\n'))
}

// hostOf takes the address out of "host:port", or gives the text back.
func hostOf(address string) string {
	if host, _, err := net.SplitHostPort(address); err == nil {
		return host
	}
	return address
}
