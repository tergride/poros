package engine

import (
	"bufio"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"PorosCore/internal/jsonrpc"
)

const (
	// releasesFeed is the newest release of tergride/poros: GitHub redirects to it without an API limit, and it is
	// 404 until the first release. Updates never come from Throne.
	releasesFeed     = "https://github.com/tergride/poros/releases/latest/download/latest.json"
	releasesDownload = "https://github.com/tergride/poros/releases/download/v%s/%s"
	// releasesAtom lists the releases of tergride/poros, pre-releases too, newest first; a beta version is a
	// pre-release of the usual X.Y.Z with the same files.
	releasesAtom    = "https://github.com/tergride/poros/releases.atom"
	releasesGitHub  = "https://github.com/tergride/poros/releases/"
	updateInterval  = 4 * time.Hour
	feedMaxBytes    = 1 << 20
	archiveMaxBytes = 512 << 20
	downloadTimeout = 30 * time.Minute
	updateFolder    = "update"
	// installErrorFile is what updater.exe leaves next to the archive when an install failed.
	installErrorFile = "install-error.txt"
	updaterName      = "updater.exe"
	// readyFile keeps the release whose archive was downloaded and checked, so that a core started again offers it
	// at once instead of after its first check.
	readyFile = "ready.json"
)

var (
	versionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	sha256Pattern  = regexp.MustCompile(`^[0-9a-f]{64}$`)
	archivePattern = regexp.MustCompile(`^Poros-(\d+\.\d+\.\d+)-windows-amd64\.zip$`)
)

// releaseFeed is latest.json of a release.
type releaseFeed struct {
	Version string `json:"version"`
	File    string `json:"file"`
	Size    int64  `json:"size"`
	SHA256  string `json:"sha256"`
}

func parseFeed(data []byte) (releaseFeed, error) {
	var feed releaseFeed
	if err := json.Unmarshal(data, &feed); err != nil {
		return feed, fmt.Errorf("the update information could not be read: %w", err)
	}
	switch {
	case !versionPattern.MatchString(feed.Version):
		return feed, fmt.Errorf("the update information has no version: %q", feed.Version)
	case feed.File != "Poros-"+feed.Version+"-windows-amd64.zip":
		return feed, fmt.Errorf("the update information names an unexpected file: %q", feed.File)
	case !sha256Pattern.MatchString(feed.SHA256):
		return feed, fmt.Errorf("the update information has no SHA-256")
	case feed.Size <= 0 || feed.Size > archiveMaxBytes:
		return feed, fmt.Errorf("the update information has an unexpected size: %d", feed.Size)
	}
	return feed, nil
}

// newerVersion compares X.Y.Z versions by their numbers.
func newerVersion(candidate, current string) bool {
	a, b := strings.Split(candidate, "."), strings.Split(current, ".")
	for i := range max(len(a), len(b)) {
		var x, y int
		if i < len(a) {
			x, _ = strconv.Atoi(a[i])
		}
		if i < len(b) {
			y, _ = strconv.Atoi(b[i])
		}
		if x != y {
			return x > y
		}
	}
	return false
}

type updates struct {
	feedURL     string
	releasesURL string
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup

	// mu guards the fields below; events leave under it, in the order of the changes.
	mu      sync.Mutex
	state   UpdateState
	ready   releaseFeed
	running bool
	closed  bool
	// lastTry is when a check last started; an automatic one waits updateInterval after it.
	lastTry time.Time
}

// openUpdates finds out whether this build updates itself and reads the install that failed before this start.
func (e *Engine) openUpdates(cfg Config) {
	programDir := cfg.ProgramDir
	if programDir == "" {
		if self, err := os.Executable(); err == nil {
			programDir = filepath.Dir(self)
		}
	}
	_, err := os.Stat(filepath.Join(programDir, updaterName))
	e.upd.state = UpdateState{
		Available: versionPattern.MatchString(cfg.Version) && err == nil,
		Current:   cfg.Version,
		State:     UpdateIdle,
	}
	e.upd.feedURL = cmp.Or(cfg.UpdateFeed, releasesFeed)
	e.upd.releasesURL = cmp.Or(cfg.UpdateReleases, releasesAtom)
	e.upd.ctx, e.upd.cancel = context.WithCancel(context.Background())
	e.readInstallError()
	e.readReady()
}

// readReady takes back a release downloaded before this start, while its archive is whole and still newer. One
// that failed to install waits for the next check, as readInstallError says.
func (e *Engine) readReady() {
	path := filepath.Join(e.updateDir(), readyFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	feed, err := parseFeed(data)
	if err != nil || e.upd.state.InstallError != "" || !e.upd.state.Available ||
		!newerVersion(feed.Version, e.upd.state.Current) ||
		archiveIntact(filepath.Join(e.updateDir(), feed.File), feed) != nil {
		_ = os.Remove(path)
		return
	}
	e.upd.ready = feed
	e.upd.state.State, e.upd.state.Version = UpdateReady, feed.Version
}

// writeReady remembers the release that is ready to install; without the file only the next check finds it again.
func (e *Engine) writeReady(feed releaseFeed) {
	data, err := json.Marshal(feed)
	if err == nil {
		err = os.WriteFile(filepath.Join(e.updateDir(), readyFile), data, 0o600)
	}
	if err != nil {
		log.Printf("update: remember the release: %v", err)
	}
}

func (e *Engine) updateDir() string {
	return filepath.Join(e.dataDir, updateFolder)
}

// readInstallError takes the updater's report of a failed install: its reason, and the version from the archive it
// left. The first automatic check after it stays quiet, since the same install would only fail again.
func (e *Engine) readInstallError() {
	path := filepath.Join(e.updateDir(), installErrorFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	_ = os.Remove(path)
	reason, details, _ := strings.Cut(string(data), "\n")
	reason = strings.TrimSpace(reason)
	if reason != InstallBusy && reason != InstallDamaged {
		reason, details = InstallFailed, string(data)
	}
	log.Printf("update: the install failed (%s): %s", reason, strings.TrimSpace(details))
	e.upd.state.InstallError = reason
	if entries, err := os.ReadDir(e.updateDir()); err == nil {
		for _, entry := range entries {
			if match := archivePattern.FindStringSubmatch(entry.Name()); match != nil {
				e.upd.state.InstallVersion = match[1]
			}
		}
	}
	e.upd.lastTry = e.now()
}

func (e *Engine) UpdatesState() UpdateState {
	e.upd.mu.Lock()
	defer e.upd.mu.Unlock()
	return e.upd.state
}

// setUpdates changes the state and announces it; the caller holds upd.mu.
func (e *Engine) setUpdates(change func(*UpdateState)) {
	change(&e.upd.state)
	e.notify(EventUpdatesChanged, e.upd.state)
}

// CheckUpdates looks for a new release at the user's request and answers at once.
func (e *Engine) CheckUpdates() (UpdateState, error) {
	if !e.UpdatesState().Available {
		return UpdateState{}, jsonrpc.Failed(ReasonUpdateUnavailable, "this build does not update itself: it has no version or no updater.exe")
	}
	e.startUpdateCheck()
	return e.UpdatesState(), nil
}

// checkUpdatesIfDue is the schedule's: at launch, then updateInterval after the last check, while the user wants it.
func (e *Engine) checkUpdatesIfDue() {
	settings, err := e.Settings()
	if err != nil || !settings.UpdateCheck {
		return
	}
	e.upd.mu.Lock()
	due := e.upd.lastTry.IsZero() || e.now().Sub(e.upd.lastTry) >= updateInterval
	e.upd.mu.Unlock()
	if due && e.UpdatesState().Available {
		e.startUpdateCheck()
	}
}

// startUpdateCheck runs one check at a time; a release already downloaded waits for its install.
func (e *Engine) startUpdateCheck() {
	e.upd.mu.Lock()
	defer e.upd.mu.Unlock()
	if e.upd.running || e.upd.closed || e.upd.state.State == UpdateReady {
		return
	}
	e.upd.running = true
	e.upd.lastTry = e.now()
	e.setUpdates(func(s *UpdateState) { s.State, s.Error, s.ErrorText = UpdateChecking, "", "" })
	e.upd.wg.Add(1)
	go func() {
		defer e.upd.wg.Done()
		e.runUpdateCheck(e.upd.ctx)
	}()
}

func (e *Engine) runUpdateCheck(ctx context.Context) {
	current := e.UpdatesState().Current
	feedURL, err := e.feedURL(ctx)
	var feed *releaseFeed
	if err == nil && feedURL != "" {
		feed, err = e.fetchFeed(ctx, feedURL)
	}
	newer := err == nil && feed != nil && newerVersion(feed.Version, current)
	if newer {
		err = e.fetchArchive(ctx, *feed, feedURL)
	}
	e.upd.mu.Lock()
	defer e.upd.mu.Unlock()
	e.upd.running = false
	if err != nil {
		log.Printf("update: %v", err)
	}
	e.setUpdates(func(s *UpdateState) {
		s.State, s.Version = UpdateIdle, ""
		switch {
		case err != nil:
			s.Error, s.ErrorText = ReasonUpdateFailed, err.Error()
		case newer:
			s.State, s.Version, s.CheckedAt = UpdateReady, feed.Version, e.now().UnixMilli()
		default:
			s.CheckedAt = e.now().UnixMilli()
		}
	})
	if newer && err == nil {
		e.upd.ready = *feed
		e.writeReady(*feed)
	}
}

// feedURL is latest.json of the release to check: the newest full one, or with beta versions the newest of all;
// "" is no release yet.
func (e *Engine) feedURL(ctx context.Context) (string, error) {
	settings, err := e.Settings()
	if err != nil || !settings.UpdateBeta {
		return e.upd.feedURL, nil
	}
	version, err := e.newestRelease(ctx)
	if err != nil || version == "" {
		return "", err
	}
	if e.upd.releasesURL == releasesAtom {
		return releasesGitHub + "download/v" + version + "/latest.json", nil
	}
	base, err := url.Parse(e.upd.releasesURL)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(&url.URL{Path: "download/v" + version + "/latest.json"}).String(), nil
}

// releasesList is the part of the Atom list of releases a check reads: the page of each release, which ends with
// its tag.
type releasesList struct {
	Entries []struct {
		Links []struct {
			Href string `xml:"href,attr"`
		} `xml:"link"`
	} `xml:"entry"`
}

// newestRelease finds the highest X.Y.Z among the releases, pre-releases too; "" is none.
func (e *Engine) newestRelease(ctx context.Context) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, e.upd.releasesURL, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("User-Agent", e.userAgent)
	response, err := e.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("check for updates: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("check for updates: %s", response.Status)
	}
	var list releasesList
	if err = xml.NewDecoder(io.LimitReader(response.Body, feedMaxBytes)).Decode(&list); err != nil {
		return "", fmt.Errorf("the list of releases could not be read: %w", err)
	}
	newest := ""
	for _, entry := range list.Entries {
		for _, link := range entry.Links {
			_, tag, found := strings.Cut(link.Href, "/releases/tag/v")
			if found && versionPattern.MatchString(tag) && (newest == "" || newerVersion(tag, newest)) {
				newest = tag
			}
		}
	}
	return newest, nil
}

// fetchFeed reads the latest.json at link; nil is no release yet.
func (e *Engine) fetchFeed(ctx context.Context, link string) (*releaseFeed, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", e.userAgent)
	response, err := e.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("check for updates: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("check for updates: %s", response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, feedMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("check for updates: %w", err)
	}
	if len(data) > feedMaxBytes {
		return nil, errors.New("the update information is too large")
	}
	feed, err := parseFeed(data)
	return &feed, err
}

// archiveURL is the release's archive: on GitHub by its tag, elsewhere next to the feed at feedURL.
func archiveURL(feed releaseFeed, feedURL string) (string, error) {
	if strings.HasPrefix(feedURL, releasesGitHub) {
		return fmt.Sprintf(releasesDownload, feed.Version, feed.File), nil
	}
	base, err := url.Parse(feedURL)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(&url.URL{Path: feed.File}).String(), nil
}

// fetchArchive downloads the release into the update folder of the data, unless an earlier run left it whole, and
// checks its size and SHA-256.
func (e *Engine) fetchArchive(ctx context.Context, feed releaseFeed, feedURL string) error {
	dir := e.updateDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	// The core may run with rights: it writes only into a plain folder, never through a link someone planted.
	if info, err := os.Lstat(dir); err != nil || !info.IsDir() {
		return fmt.Errorf("%s is not a plain folder", dir)
	}
	path := filepath.Join(dir, feed.File)
	dropOtherArchives(dir, feed.File)
	if archiveIntact(path, feed) == nil {
		return nil
	}
	e.upd.mu.Lock()
	e.setUpdates(func(s *UpdateState) { s.State, s.Version = UpdateDownloading, feed.Version })
	e.upd.mu.Unlock()

	link, err := archiveURL(feed, feedURL)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", e.userAgent)
	// No limit for the whole download, only the context's: the archive is large.
	response, err := (&http.Client{Transport: e.client.Transport}).Do(request)
	if err != nil {
		return fmt.Errorf("download the update: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download the update: %s", response.Status)
	}
	temp := path + ".tmp"
	file, err := os.OpenFile(temp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, err = io.Copy(file, io.LimitReader(response.Body, feed.Size+1))
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = archiveIntact(temp, feed)
	}
	if err == nil {
		err = os.Rename(temp, path)
	}
	if err != nil {
		_ = os.Remove(temp)
		return fmt.Errorf("download the update: %w", err)
	}
	return nil
}

func archiveIntact(path string, feed releaseFeed) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, bufio.NewReader(file))
	if err != nil {
		return err
	}
	if size != feed.Size || hex.EncodeToString(hash.Sum(nil)) != feed.SHA256 {
		return errors.New("the update is damaged: its size or SHA-256 differs from the release's")
	}
	return nil
}

// dropOtherArchives removes older releases and broken downloads.
func dropOtherArchives(dir, keep string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if name != keep && (strings.HasSuffix(name, ".zip") || strings.HasSuffix(name, ".tmp")) {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
}

// InstallUpdate checks the downloaded release once more and gives its archive to the shell, which starts the
// updater and exits.
func (e *Engine) InstallUpdate() (UpdateInstall, error) {
	e.upd.mu.Lock()
	defer e.upd.mu.Unlock()
	if e.upd.state.State != UpdateReady {
		return UpdateInstall{}, jsonrpc.Failed(ReasonUpdateNotReady, "no update is downloaded")
	}
	path := filepath.Join(e.updateDir(), e.upd.ready.File)
	if err := archiveIntact(path, e.upd.ready); err != nil {
		_ = os.Remove(path)
		_ = os.Remove(filepath.Join(e.updateDir(), readyFile))
		e.setUpdates(func(s *UpdateState) {
			s.State, s.Version, s.Error, s.ErrorText = UpdateIdle, "", ReasonUpdateFailed, err.Error()
		})
		// The release is still there to download: the damaged archive comes again at once, not hours later.
		// The check takes upd.mu, which this call holds until it returns.
		go e.startUpdateCheck()
		return UpdateInstall{}, jsonrpc.Failed(ReasonUpdateNotReady, "%v", err)
	}
	return UpdateInstall{Archive: path}, nil
}

// closeUpdates stops a check or download in flight.
func (e *Engine) closeUpdates() {
	e.upd.mu.Lock()
	e.upd.closed = true
	e.upd.mu.Unlock()
	e.upd.cancel()
	e.upd.wg.Wait()
}
