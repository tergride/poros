package profile

import (
	"fmt"
	"strings"
	"testing"
)

// A subscription of 50 000 servers, the largest the plan budgets for.
func BenchmarkParseDocument50k(b *testing.B) {
	var text strings.Builder
	for i := range 50_000 {
		fmt.Fprintf(&text, "trojan://pw%d@s%d.example.com:443?security=tls&sni=s%d.example.com&type=ws&path=%%2Fws#Server%%20%d\n", i, i, i, i)
	}
	body := []byte(text.String())
	b.SetBytes(int64(len(body)))
	b.ReportAllocs()
	for b.Loop() {
		document, err := ParseDocument(body)
		if err != nil || len(document.Profiles) != 50_000 {
			b.Fatalf("%d profiles, %v", len(document.Profiles), err)
		}
	}
}
