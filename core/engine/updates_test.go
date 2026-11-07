package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// releaseServer stands for GitHub: latest.json of a release and its archive next to it.
type releaseServer struct {
	url       string
	mu        sync.Mutex
	status    int
	version   string
	archive   []byte
	served    []byte
	feeds     atomic.Int32
	downloads atomic.Int32
}

func newReleaseServer(t *testing.T, version string) *releaseServer {
	t.Helper()
	r := &releaseServer{status: http.StatusOK, version: version, archive: []byte("the archive of " + version)}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		defer r.mu.Unlock()
		file := "Poros-" + r.version + "-windows-amd64.zip"
		switch req.URL.Path {
		case "/releases/latest.json":
			r.feeds.Add(1)
			if r.status != http.StatusOK {
				w.WriteHeader(r.status)
				return
			}
			sum := sha256.Sum256(r.archive)
			json.NewEncoder(w).Encode(map[string]any{
				"version": r.version, "file": file, "size": len(r.archive), "sha256": hex.EncodeToString(sum[:]),
				"notes": map[string]string{"ru": "Новое", "en": "New"},
			})
		case "/releases/" + file:
			r.downloads.Add(1)
			body := r.archive
			if r.served != nil {
				body = r.served
			}
			w.Write(body)
		default:
			http.NotFound(w, req)
		}
	}))
	t.Cleanup(server.Close)
	r.url = server.URL + "/releases/latest.json"
	return r
}

func (r *releaseServer) set(change func(*releaseServer)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	change(r)
}

// openUpdatesTest opens an engine of version on the feed; withUpdater puts updater.exe next to the program.
func openUpdatesTest(t *testing.T, dir, feed, version string, withUpdater bool) *Engine {
	t.Helper()
	program := t.TempDir()
	if withUpdater {
		if err := os.WriteFile(filepath.Join(program, updaterName), []byte("updater"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	e, err := Open(Config{DataDir: dir, Version: version, UpdateFeed: feed, ProgramDir: program})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { e.Close() })
	return e
}

func waitUpdates(t *testing.T, e *Engine, done func(UpdateState) bool) UpdateState {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		state := e.UpdatesState()
		if done(state) {
			return state
		}
		if time.Now().After(deadline) {
			t.Fatalf("the update check did not settle: %+v", state)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func settled(state UpdateState) bool {
	return state.State == UpdateIdle || state.State == UpdateReady
}

func TestUpdatesDownloadAndInstall(t *testing.T) {
	releases := newReleaseServer(t, "0.1.1")
	dir := t.TempDir()
	e := openUpdatesTest(t, dir, releases.url, "0.1.0", true)
	if state := e.UpdatesState(); !state.Available || state.Current != "0.1.0" || state.State != UpdateIdle {
		t.Fatalf("a build with a version and the updater updates itself: %+v", state)
	}

	if state, err := e.CheckUpdates(); err != nil || state.State != UpdateChecking {
		t.Fatalf("a check answers at once: %+v, %v", state, err)
	}
	state := waitUpdates(t, e, settled)
	if state.State != UpdateReady || state.Version != "0.1.1" || state.CheckedAt == 0 || state.Error != "" {
		t.Fatalf("the newer release is downloaded: %+v", state)
	}
	archive := filepath.Join(dir, updateFolder, "Poros-0.1.1-windows-amd64.zip")
	install, err := e.InstallUpdate()
	if err != nil || install.Archive != archive {
		t.Fatalf("install: %+v, %v", install, err)
	}
	// A release waiting for its install is not looked for again.
	e.CheckUpdates()
	if releases.feeds.Load() != 1 {
		t.Errorf("the feed was read %d times", releases.feeds.Load())
	}
	e.Close()

	// The next run reuses the archive that is whole.
	next := openUpdatesTest(t, dir, releases.url, "0.1.0", true)
	next.CheckUpdates()
	if state := waitUpdates(t, next, settled); state.State != UpdateReady || releases.downloads.Load() != 1 {
		t.Fatalf("the archive was downloaded again: %+v, %d downloads", state, releases.downloads.Load())
	}
	// An archive changed after its check is not given to the updater.
	if err := os.WriteFile(archive, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := next.InstallUpdate(); reason(err) != ReasonUpdateNotReady {
		t.Fatalf("a changed archive: %v", err)
	}
	if state := next.UpdatesState(); state.State != UpdateIdle || state.Error != ReasonUpdateFailed {
		t.Errorf("after a changed archive: %+v", state)
	}
	if _, err := os.Stat(archive); !os.IsNotExist(err) {
		t.Errorf("the changed archive stayed: %v", err)
	}
}

func TestUpdatesWithNothingNew(t *testing.T) {
	releases := newReleaseServer(t, "0.1.0")
	for _, tc := range []struct {
		name   string
		change func(*releaseServer)
	}{
		{"no release yet", func(r *releaseServer) { r.status = http.StatusNotFound }},
		{"the same version", func(r *releaseServer) { r.status = http.StatusOK }},
		{"an older version", func(r *releaseServer) { r.version = "0.0.9" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			releases.set(tc.change)
			e := openUpdatesTest(t, t.TempDir(), releases.url, "0.1.0", true)
			e.CheckUpdates()
			if state := waitUpdates(t, e, settled); state.State != UpdateIdle || state.Version != "" || state.CheckedAt == 0 || state.Error != "" {
				t.Errorf("%+v", state)
			}
		})
	}
	if releases.downloads.Load() != 0 {
		t.Errorf("%d downloads without a newer release", releases.downloads.Load())
	}
}

func TestUpdatesThatFail(t *testing.T) {
	releases := newReleaseServer(t, "0.2.0")
	dir := t.TempDir()
	releases.set(func(r *releaseServer) { r.served = []byte("not the archive the feed describes") })
	e := openUpdatesTest(t, dir, releases.url, "0.1.0", true)
	e.CheckUpdates()
	state := waitUpdates(t, e, settled)
	if state.State != UpdateIdle || state.Error != ReasonUpdateFailed || state.CheckedAt != 0 {
		t.Fatalf("a damaged download: %+v", state)
	}
	if entries, _ := os.ReadDir(filepath.Join(dir, updateFolder)); len(entries) != 0 {
		t.Errorf("a damaged download left %v", entries)
	}
	if _, err := e.InstallUpdate(); reason(err) != ReasonUpdateNotReady {
		t.Errorf("an install with nothing downloaded: %v", err)
	}

	releases.set(func(r *releaseServer) { r.status = http.StatusInternalServerError })
	e.CheckUpdates()
	if state := waitUpdates(t, e, settled); state.Error != ReasonUpdateFailed {
		t.Errorf("a feed that fails: %+v", state)
	}

	for _, tc := range []struct{ version, why string }{{"", "a local build"}, {"dev", "a build without a release version"}} {
		local := openUpdatesTest(t, t.TempDir(), releases.url, tc.version, true)
		if _, err := local.CheckUpdates(); reason(err) != ReasonUpdateUnavailable || local.UpdatesState().Available {
			t.Errorf("%s does not update itself: %v", tc.why, err)
		}
	}
	bare := openUpdatesTest(t, t.TempDir(), releases.url, "0.1.0", false)
	if _, err := bare.CheckUpdates(); reason(err) != ReasonUpdateUnavailable {
		t.Errorf("a build without updater.exe does not update itself: %v", err)
	}
}

func TestUpdatesAfterAFailedInstall(t *testing.T) {
	releases := newReleaseServer(t, "0.1.1")
	for _, tc := range []struct{ report, reason string }{
		{"busy\ncould not replace PorosCore.exe: the file is in use", InstallBusy},
		{"damaged\nthe archive has no Poros.exe", InstallDamaged},
		{"something else went wrong", InstallFailed},
	} {
		dir := t.TempDir()
		folder := filepath.Join(dir, updateFolder)
		os.MkdirAll(folder, 0o700)
		os.WriteFile(filepath.Join(folder, "Poros-0.1.1-windows-amd64.zip"), []byte("left by the updater"), 0o600)
		os.WriteFile(filepath.Join(folder, installErrorFile), []byte(tc.report), 0o600)

		e := openUpdatesTest(t, dir, releases.url, "0.1.0", true)
		if state := e.UpdatesState(); state.InstallError != tc.reason || state.InstallVersion != "0.1.1" {
			t.Errorf("%q: %+v", tc.report, state)
		}
		if _, err := os.Stat(filepath.Join(folder, installErrorFile)); !os.IsNotExist(err) {
			t.Errorf("the report is told once: %v", err)
		}
		// The same install would fail again: the first automatic check stays quiet.
		e.checkUpdatesIfDue()
		if e.UpdatesState().State != UpdateIdle || releases.feeds.Load() != 0 {
			t.Errorf("an automatic check right after a failed install")
		}
	}
}

func TestUpdatesSchedule(t *testing.T) {
	releases := newReleaseServer(t, "0.1.0")
	e := openUpdatesTest(t, t.TempDir(), releases.url, "0.1.0", true)
	now := time.Now()
	e.now = func() time.Time { return now }
	check := func(want int32, why string) {
		t.Helper()
		e.checkUpdatesIfDue()
		waitUpdates(t, e, settled)
		if got := releases.feeds.Load(); got != want {
			t.Fatalf("%s: the feed was read %d times, want %d", why, got, want)
		}
	}
	check(1, "at launch")
	check(1, "right after a check")
	now = now.Add(updateInterval)
	check(2, "after the interval")

	settings, _ := e.Settings()
	settings.UpdateCheck = false
	if _, err := e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	now = now.Add(updateInterval)
	check(2, "with automatic checks off")
}

func TestParseFeed(t *testing.T) {
	good := `{"version":"0.1.1","file":"Poros-0.1.1-windows-amd64.zip","size":100,"sha256":"` + hex.EncodeToString(make([]byte, 32)) + `"}`
	if feed, err := parseFeed([]byte(good)); err != nil || feed.Version != "0.1.1" {
		t.Fatalf("%+v, %v", feed, err)
	}
	for _, bad := range []string{
		`not json`,
		`{"version":"v0.1.1","file":"Poros-v0.1.1-windows-amd64.zip","size":100,"sha256":"` + hex.EncodeToString(make([]byte, 32)) + `"}`,
		`{"version":"0.1.1","file":"../Poros-0.1.1-windows-amd64.zip","size":100,"sha256":"` + hex.EncodeToString(make([]byte, 32)) + `"}`,
		`{"version":"0.1.1","file":"Poros-0.1.2-windows-amd64.zip","size":100,"sha256":"` + hex.EncodeToString(make([]byte, 32)) + `"}`,
		`{"version":"0.1.1","file":"Poros-0.1.1-windows-amd64.zip","size":0,"sha256":"` + hex.EncodeToString(make([]byte, 32)) + `"}`,
		`{"version":"0.1.1","file":"Poros-0.1.1-windows-amd64.zip","size":100,"sha256":"ABC"}`,
	} {
		if _, err := parseFeed([]byte(bad)); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	for _, tc := range []struct {
		a, b  string
		newer bool
	}{{"0.1.1", "0.1.0", true}, {"0.10.0", "0.9.9", true}, {"1.0.0", "0.99.99", true}, {"0.1.0", "0.1.0", false}, {"0.1.0", "0.2.0", false}} {
		if newerVersion(tc.a, tc.b) != tc.newer {
			t.Errorf("newerVersion(%s, %s) = %v", tc.a, tc.b, !tc.newer)
		}
	}
}
