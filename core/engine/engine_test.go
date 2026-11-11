package engine

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/sagernet/bbolt"

	"PorosCore/internal/jsonrpc"
	"PorosCore/internal/logontask"
)

type event struct {
	method string
	params any
}

func openTest(t *testing.T, dir string) (*Engine, *[]event) {
	t.Helper()
	events := &[]event{}
	e, err := Open(Config{DataDir: dir, Version: "0.1.0", Notify: func(method string, params any) {
		*events = append(*events, event{method, params})
	}})
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
	switch data := rpcErr.Data.(type) {
	case map[string]string:
		return data["reason"]
	case RuleProblem:
		return data.Reason
	case OTPFailure:
		return data.Reason
	}
	return ""
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
	want.Tun, want.SystemProxy, want.Preset, want.Language, want.SubscriptionUpdateHours = true, true, "hardened", "ru", 24
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
		"ui mode":          func(s *Settings) { s.UIMode = "expert" },
		"language":         func(s *Settings) { s.Language = "de" },
		"preset":           func(s *Settings) { s.Preset = "nope" },
		"negative hours":   func(s *Settings) { s.SubscriptionUpdateHours = -1 },
		"more than weekly": func(s *Settings) { s.SubscriptionUpdateHours = maxUpdateHours + 1 },
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

func TestBuiltinPresets(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	presets, err := e.Presets()
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, len(presets))
	for i, p := range presets {
		ids[i] = p.ID
		if !p.Builtin || p.Name == "" {
			t.Errorf("preset %+v must be builtin and named", p)
		}
	}
	if !slices.Equal(ids, []string{"all", "hardened"}) {
		t.Fatalf("builtin presets: %v", ids)
	}
}

func TestOneEnginePerDataFolder(t *testing.T) {
	dir := t.TempDir()
	openTest(t, dir)
	started := time.Now()
	if second, err := Open(Config{DataDir: dir}); err == nil {
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

// Rights at sign-in take rights to turn on; off, a core without rights leaves the task a note to remove itself.
func TestRightsAtSignInTakeRights(t *testing.T) {
	if elevated() {
		t.Skip("the core runs with administrator rights here")
	}
	dir := t.TempDir()
	e, _ := openTest(t, dir)
	settings := DefaultSettings()
	settings.AdminAutostart = true
	if _, err := e.SetSettings(settings); reason(err) != ReasonLogonTaskNeedsElevation {
		t.Fatalf("on without rights: %v", err)
	}
	if got, _ := e.Settings(); got.AdminAutostart {
		t.Fatal("the setting was saved without its task")
	}
	e.db.Update(func(tx *bbolt.Tx) error { return putJSON(tx, bucketSettings, keySettings, settings) })
	settings.AdminAutostart = false
	if _, err := e.SetSettings(settings); err != nil {
		t.Fatalf("off without rights: %v", err)
	}
	if !logontask.RemovalMarked(dir) {
		t.Error("the task was not asked to remove itself")
	}
}
