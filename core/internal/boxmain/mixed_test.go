package boxmain

import (
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"
)

// freePort finds a port free on loopback, below the ports outgoing connections take theirs from (49152 and up on
// Windows, 32768 and up on Linux): between this check and the start of the box, a connection would take it
// otherwise.
func freePort(t *testing.T) int {
	t.Helper()
	var last error
	for range 200 {
		port := 10000 + rand.IntN(10000)
		listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err == nil {
			listener.Close()
			return port
		}
		last = err
	}
	t.Fatalf("no port is free: %v", last)
	return 0
}

// A program that keeps its connection to the proxy gets the next box after a reconnect, not 502 from the closed one.
func TestKeptAliveProxyConnectionsGoWithTheBox(t *testing.T) {
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "ok") }))
	defer site.Close()
	port := freePort(t)
	config := []byte(fmt.Sprintf(`{"log":{"disabled":true},"inbounds":[{"type":"mixed","tag":"in","listen":"127.0.0.1","listen_port":%d}],"outbounds":[{"type":"direct","tag":"direct"}]}`, port))
	proxy, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(proxy)}}
	get := func() (int, error) {
		response, err := client.Get(site.URL)
		if err != nil {
			return 0, err
		}
		defer response.Body.Close()
		io.Copy(io.Discard, response.Body)
		return response.StatusCode, nil
	}

	first, cancel, err := Create(config, nil)
	if err != nil {
		t.Fatal(err)
	}
	if status, err := get(); err != nil || status != http.StatusOK {
		t.Fatalf("through the first box: %d, %v", status, err)
	}
	cancel()
	first.Close()

	second, cancel, err := Create(config, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cancel()
		second.Close()
	}()
	if status, err := get(); err != nil || status != http.StatusOK {
		t.Fatalf("through the second box on the same connection pool: %d, %v", status, err)
	}
}
