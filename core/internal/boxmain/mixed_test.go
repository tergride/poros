package boxmain

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// A program that keeps its connection to the proxy gets the next box after a reconnect, not 502 from the closed one.
func TestKeptAliveProxyConnectionsGoWithTheBox(t *testing.T) {
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "ok") }))
	defer site.Close()
	free, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := free.Addr().(*net.TCPAddr).Port
	free.Close()
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
