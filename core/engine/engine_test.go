package engine

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"PorosCore/internal/jsonrpc"
)

type event struct {
	method string
	params any
}

func openTest(t *testing.T, dir string) (*Engine, *[]event) {
	t.Helper()
	events := &[]event{}
	e, err := Open(dir, func(method string, params any) { *events = append(*events, event{method, params}) })
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { e.Close() })
	return e, events
}

func reason(err error) string {
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.CodeFailed {
		return ""
	}
	return rpcErr.Data.(map[string]string)["reason"]
}

func TestSettingsStartFromDefaultsAndPersist(t *testing.T) {
	dir := t.TempDir()
	e, events := openTest(t, dir)

	got, err := e.Settings()
	if err != nil || got != DefaultSettings() {
		t.Fatalf("a new folder must start from the defaults, got %+v, %v", got, err)
	}
	if got.Tun || got.SystemProxy {
		t.Fatal("TUN and System must both start off: the first Start asks")
	}

	want := DefaultSettings()
	want.Tun, want.SystemProxy, want.Preset, want.Language = true, true, "except-ru", "ru"
	if _, err = e.SetSettings(want); err != nil {
		t.Fatalf("SetSettings: %v", err)
	}
	if len(*events) != 1 || (*events)[0].method != EventSettingsChanged || (*events)[0].params != want {
		t.Fatalf("SetSettings must announce the new settings, got %+v", *events)
	}

	e.Close()
	reopened, _ := openTest(t, dir)
	if got, _ = reopened.Settings(); got != want {
		t.Fatalf("settings did not survive a restart: %+v", got)
	}
}

func TestSettingsRejectUnknownValues(t *testing.T) {
	e, events := openTest(t, t.TempDir())
	for name, change := range map[string]func(*Settings){
		"ui mode":  func(s *Settings) { s.UIMode = "expert" },
		"language": func(s *Settings) { s.Language = "de" },
		"preset":   func(s *Settings) { s.Preset = "nope" },
	} {
		settings := DefaultSettings()
		change(&settings)
		if _, err := e.SetSettings(settings); reason(err) == "" {
			t.Errorf("%s: expected a failed request, got %v", name, err)
		}
	}
	if len(*events) != 0 {
		t.Fatalf("a rejected change must not be announced: %+v", *events)
	}
}

func TestSubscriptions(t *testing.T) {
	dir := t.TempDir()
	e, events := openTest(t, dir)
	clock := time.UnixMilli(1_000)
	e.now = func() time.Time { clock = clock.Add(time.Second); return clock }

	for _, link := range []string{"", "vless://id@example.com:443", "https://", "sub.example.com/u/1"} {
		if _, err := e.AddSubscription(AddSubscription{URL: link}); reason(err) != ReasonInvalidURL {
			t.Errorf("%q: expected %s, got %v", link, ReasonInvalidURL, err)
		}
	}

	first, err := e.AddSubscription(AddSubscription{URL: "  https://sub.example.com/u/8f3a  "})
	if err != nil {
		t.Fatalf("AddSubscription: %v", err)
	}
	if first.Name != "sub.example.com" || first.URL != "https://sub.example.com/u/8f3a" || first.UpdatedAt != 0 {
		t.Fatalf("unexpected subscription %+v", first)
	}
	second, err := e.AddSubscription(AddSubscription{URL: "http://vpn.example.org/s", Name: " Work "})
	if err != nil || second.Name != "Work" {
		t.Fatalf("named subscription: %+v, %v", second, err)
	}
	if _, err = e.AddSubscription(AddSubscription{URL: "https://sub.example.com/u/8f3a"}); reason(err) != ReasonDuplicate {
		t.Fatalf("the same link twice: %v", err)
	}

	list, _ := e.Subscriptions()
	if len(list) != 2 || list[0].ID != first.ID || list[1].ID != second.ID {
		t.Fatalf("subscriptions must keep the order they were added in: %+v", list)
	}
	last := (*events)[len(*events)-1]
	if last.method != EventSubscriptionsChanged || len(last.params.([]Subscription)) != 2 {
		t.Fatalf("adding must announce the whole list: %+v", last)
	}

	if err = e.RemoveSubscription("missing"); reason(err) != ReasonNotFound {
		t.Fatalf("removing an unknown id: %v", err)
	}
	if err = e.RemoveSubscription(first.ID); err != nil {
		t.Fatalf("RemoveSubscription: %v", err)
	}

	e.Close()
	reopened, _ := openTest(t, dir)
	if list, _ = reopened.Subscriptions(); len(list) != 1 || list[0].ID != second.ID {
		t.Fatalf("subscriptions did not survive a restart: %+v", list)
	}
}

func TestBuiltinPresets(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	presets := e.Presets()
	ids := make([]string, len(presets))
	for i, p := range presets {
		ids[i] = p.ID
		if !p.Builtin || p.Name == "" {
			t.Errorf("preset %+v must be builtin and named", p)
		}
	}
	if !slices.Equal(ids, []string{"all", "except-ru", "blocked-only", "hardened"}) {
		t.Fatalf("builtin presets: %v", ids)
	}
}

func TestOneEnginePerDataFolder(t *testing.T) {
	dir := t.TempDir()
	openTest(t, dir)
	started := time.Now()
	if second, err := Open(dir, nil); err == nil {
		second.Close()
		t.Fatal("a second engine on the same folder must not open")
	}
	if waited := time.Since(started); waited > 5*time.Second {
		t.Fatalf("the second Open waited %v instead of giving up", waited)
	}
}

func TestHandlersDecodeParamsStrictly(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	_, err := e.Handlers()[MethodSubscriptionsAdd](t.Context(), json.RawMessage(`{"link":"https://a.example"}`))
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Fatalf("an unknown field must be rejected, got %v", err)
	}
}
