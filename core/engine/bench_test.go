package engine

import (
	"fmt"
	"strings"
	"testing"
)

// The servers of a group of 10 000, as the interface lists them, and adding them from text.
func BenchmarkServers10k(b *testing.B) {
	e, err := Open(Config{DataDir: b.TempDir(), Version: "0.1.0"})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { e.Close() })
	var text strings.Builder
	for i := range 10_000 {
		fmt.Fprintf(&text, "trojan://pw%d@s%d.example.com:443?security=tls&sni=s%d.example.com#Server%%20%d\n", i, i, i, i)
	}
	var group string
	b.Run("add", func(b *testing.B) {
		n := 0
		for b.Loop() {
			n++
			added, err := e.AddServers(AddServers{NewGroup: fmt.Sprint("Big ", n), Text: text.String()})
			if err != nil || len(added.IDs) != 10_000 {
				b.Fatalf("%d added, %v", len(added.IDs), err)
			}
			group = added.Group.ID
		}
	})
	b.Run("list", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			servers, err := e.Servers(ServersQuery{GroupID: group})
			if err != nil || len(servers) != 10_000 {
				b.Fatalf("%d servers, %v", len(servers), err)
			}
		}
	})
}
