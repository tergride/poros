package subfetch

import (
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func serve(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server.URL
}

func TestFetchReadsHeaders(t *testing.T) {
	var gotAgent string
	url := serve(t, func(w http.ResponseWriter, r *http.Request) {
		gotAgent = r.Header.Get("User-Agent")
		w.Header().Set("Subscription-Userinfo", "upload=100; download=2000; total=107374182400; expire=1765497600000")
		w.Header().Set("Profile-Update-Interval", "12")
		w.Header().Set("Profile-Title", "base64:"+base64.StdEncoding.EncodeToString([]byte("Основная подписка")))
		w.Header().Set("Support-Url", "https://t.me/support")
		w.Write([]byte("vless://id@de1.example.com:443#DE\n"))
	})

	result, err := Fetch(t.Context(), http.DefaultClient, url, "Poros/0.1.0")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if gotAgent != "Poros/0.1.0" {
		t.Fatalf("User-Agent %q", gotAgent)
	}
	want := Info{
		Upload: 100, Download: 2000, Total: 107374182400, HasQuota: true,
		// The provider sent milliseconds.
		Expire:         1765497600,
		UpdateInterval: 12 * time.Hour,
		Title:          "Основная подписка",
		SupportURL:     "https://t.me/support",
	}
	if result.Info != want {
		t.Fatalf("info\n got %+v\nwant %+v", result.Info, want)
	}
	if !strings.HasPrefix(string(result.Body), "vless://") {
		t.Fatalf("body %q", result.Body)
	}
}

func TestBodyMetadataFillsWhatHeadersLeftOut(t *testing.T) {
	body := "#profile-title: Work\n// profile-update-interval: 2d\n#subscription-userinfo: upload=1; download=2; total=3; expire=4\n#announce: base64:" +
		base64.StdEncoding.EncodeToString([]byte("Maintenance tonight")) + "\nvless://id@a.example:443\n#profile-web-page-url: https://ignored.example\n"
	url := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Profile-Title", "From the header")
		w.Write([]byte(body))
	})
	result, err := Fetch(t.Context(), http.DefaultClient, url, "Poros")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	want := Info{Upload: 1, Download: 2, Total: 3, HasQuota: true, Expire: 4, UpdateInterval: 48 * time.Hour, Title: "From the header", Announce: "Maintenance tonight"}
	if result.Info != want {
		t.Fatalf("a header wins, comments after the first link are not metadata\n got %+v\nwant %+v", result.Info, want)
	}
}

func TestBodyMetadataInsideBase64(t *testing.T) {
	plain := "#profile-title: Encoded\nvless://id@a.example:443\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(plain))
	// Providers wrap base64 bodies into lines.
	wrapped := encoded[:10] + "\n" + encoded[10:]
	url := serve(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(wrapped)) })
	result, err := Fetch(t.Context(), http.DefaultClient, url, "Poros")
	if err != nil || result.Info.Title != "Encoded" {
		t.Fatalf("title %q, %v", result.Info.Title, err)
	}
}

func TestFetchFailures(t *testing.T) {
	notFound := serve(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "gone", http.StatusNotFound) })
	if _, err := Fetch(t.Context(), http.DefaultClient, notFound, "Poros"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("404: %v", err)
	}
	empty := serve(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(" \n ")) })
	if _, err := Fetch(t.Context(), http.DefaultClient, empty, "Poros"); !errors.Is(err, ErrEmpty) {
		t.Fatalf("empty: %v", err)
	}
	huge := serve(t, func(w http.ResponseWriter, r *http.Request) {
		chunk := []byte(strings.Repeat("a", 1<<20))
		for range MaxBody>>20 + 1 {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	})
	if _, err := Fetch(t.Context(), http.DefaultClient, huge, "Poros"); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("too large: %v", err)
	}
}

func TestParseUpdateInterval(t *testing.T) {
	for raw, want := range map[string]time.Duration{
		"12":        12 * time.Hour,
		`"24"`:      24 * time.Hour,
		"6h":        6 * time.Hour,
		"1d":        24 * time.Hour,
		"7200s":     2 * time.Hour,
		"60s":       time.Hour,
		"999999":    maxUpdateInterval,
		"99999999d": maxUpdateInterval,
		"0":         0,
		"-5":        0,
		"soon":      0,
		"":          0,
	} {
		if got := parseUpdateInterval(raw); got != want {
			t.Errorf("%q: got %v, want %v", raw, got, want)
		}
	}
}

func TestHeadersWithLineBreaksOrHugeValuesAreIgnored(t *testing.T) {
	h := http.Header{}
	h.Set("Profile-Title", strings.Repeat("x", maxHeaderValue))
	h["Support-Url"] = []string{"https://a.example\r\nX-Evil: 1"}
	info := infoFromHeaders(h)
	if info.Title != "" || info.SupportURL != "" {
		t.Fatalf("unusable values were kept: %+v", info)
	}
}
