package engine

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"PorosCore/config"
	"PorosCore/internal/boxmain"
)

// The statistics list the open connections through Poros with where they go and their traffic, and close them;
// without the statistics there is no list.
func TestConnectionsList(t *testing.T) {
	e, states := openConnectionTest(t)
	page := webPage(t)
	port := freePort(t)
	server, cancel, err := boxmain.Create([]byte(fmt.Sprintf(`{"log":{"level":"warn"},`+
		`"inbounds":[{"type":"trojan","listen":"127.0.0.1","listen_port":%d,"users":[{"password":"secret"}]}],`+
		`"outbounds":[{"type":"direct","tag":"direct"}]}`, port)), nil)
	if err != nil {
		t.Fatalf("the test server: %v", err)
	}
	t.Cleanup(func() { cancel(); server.Close() })
	useServer(t, e, fmt.Sprintf("trojan://secret@127.0.0.1:%d?security=none#local", port))
	if list, _ := e.Connections(); list.Tracked {
		t.Fatal("no connection, no statistics")
	}
	connectAndFetch(t, e, states, page)

	// A tunnel held open through the local proxy.
	target, _ := url.Parse(page)
	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(int(e.mixedPort))))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target.Host, target.Host)
	reader := bufio.NewReader(conn)
	if response, err := http.ReadResponse(reader, nil); err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("CONNECT: %v, %v", response, err)
	}
	var found Connection
	deadline := time.Now().Add(5 * time.Second)
	for found.ID == "" && time.Now().Before(deadline) {
		list, err := e.Connections()
		if err != nil || !list.Tracked {
			t.Fatalf("the list: %+v, %v", list, err)
		}
		for _, c := range list.Connections {
			if c.Destination == target.Host && c.Outbound == config.TagProxy {
				found = c
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if found.ID == "" || found.Network != "tcp" || found.Start == 0 || !strings.HasPrefix(found.Source, "127.0.0.1:") {
		t.Fatalf("the tunnel is not listed as going through the server: %+v", found)
	}
	if err = e.CloseConnections(ByID{ID: found.ID}); err != nil {
		t.Fatal(err)
	}
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err = reader.ReadByte(); err == nil {
		t.Error("a closed connection still reads")
	}
	if err = e.CloseConnections(ByID{ID: found.ID}); reason(err) != ReasonNotFound {
		t.Errorf("a connection closed before: %v", err)
	}

	// Without the statistics the connection runs, uncounted.
	settings, _ := e.Settings()
	settings.Stats = false
	if _, err = e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	connectAndFetch(t, e, states, page)
	if list, _ := e.Connections(); list.Tracked || len(list.Connections) != 0 {
		t.Errorf("statistics off: %+v", list)
	}
}
