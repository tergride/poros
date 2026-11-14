package engine

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// A child process of the durability test writes to poros.db until it is killed; dbWriterEnv names its data folder.
// Each write it committed goes into progressFile as its number on a line of its own. Write n does by n%4: 0 the
// settings, with n in the user agent of subscriptions; 1 two servers in one change; 2 the removal of the oldest such
// pair, while three are there; 3 the name of its group.
const (
	dbWriterEnv  = "POROS_ROBUST_DB_WRITER"
	progressFile = "progress.log"
	writerGroup  = "durability "
)

// committedWrites reads the numbers of the writes the children committed; a line still being written is left out.
func committedWrites(dir string) []int {
	data, _ := os.ReadFile(filepath.Join(dir, progressFile))
	var writes []int
	for line := range strings.Lines(string(data)) {
		if !strings.HasSuffix(line, "\n") {
			break
		}
		if n, err := strconv.Atoi(strings.TrimSpace(line)); err == nil {
			writes = append(writes, n)
		}
	}
	return writes
}

// writerPairs gives the pairs of servers of the writer's group by the number of their write, and how many of each
// pair are there.
func writerPairs(servers []Server) map[int]int {
	pairs := map[int]int{}
	for _, s := range servers {
		number, _, ok := strings.Cut(strings.TrimPrefix(s.Name, "s"), "-")
		if n, err := strconv.Atoi(number); ok && err == nil {
			pairs[n]++
		}
	}
	return pairs
}

func writerGroupOf(e *Engine) (Group, bool) {
	groups, _ := e.Groups()
	for _, g := range groups {
		if strings.HasPrefix(g.Name, writerGroup) {
			return g, true
		}
	}
	return Group{}, false
}

// The child process of TestRobustDBSurvivesKills.
func TestRobustDBWriter(t *testing.T) {
	dir := os.Getenv(dbWriterEnv)
	if dir == "" {
		t.Skip("the child process of TestRobustDBSurvivesKills")
	}
	fail := func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, format+"\n", args...)
		os.Exit(3)
	}
	e, err := Open(Config{DataDir: dir, ForeignUser: true})
	if err != nil {
		fail("open: %v", err)
	}
	progress, err := os.OpenFile(filepath.Join(dir, progressFile), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		fail("%v", err)
	}
	// The write after the last one logged may have committed: its number is not used again.
	n := 1
	if writes := committedWrites(dir); len(writes) > 0 {
		n = writes[len(writes)-1] + 2
	}
	group, found := writerGroupOf(e)
	if !found {
		if group, err = e.AddGroup(AddGroup{Name: writerGroup + "0"}); err != nil {
			fail("%v", err)
		}
	}
	for ; ; n++ {
		switch n % 4 {
		case 0:
			var settings Settings
			if settings, err = e.Settings(); err == nil {
				settings.SubscriptionUserAgent = "w" + strconv.Itoa(n)
				_, err = e.SetSettings(settings)
			}
		case 1:
			text := trojanTo("127.0.0.1:9", fmt.Sprintf("s%d-a", n)) + "\n" + trojanTo("127.0.0.1:9", fmt.Sprintf("s%d-b", n))
			_, err = e.AddServers(AddServers{GroupID: group.ID, Text: text})
		case 2:
			var servers []Server
			if servers, err = e.Servers(ServersQuery{GroupID: group.ID}); err == nil && len(writerPairs(servers)) >= 3 {
				oldest := slices.Min(slices.Collect(maps.Keys(writerPairs(servers))))
				var ids []string
				for _, s := range servers {
					if strings.HasPrefix(s.Name, fmt.Sprintf("s%d-", oldest)) {
						ids = append(ids, s.ID)
					}
				}
				err = e.RemoveServers(ServerIDs{IDs: ids})
			}
		case 3:
			_, err = e.RenameGroup(RenameGroup{ID: group.ID, Name: writerGroup + strconv.Itoa(n)})
		}
		if err != nil {
			fail("write %d: %v", n, err)
		}
		if _, err = fmt.Fprintf(progress, "%d\n", n); err != nil {
			fail("%v", err)
		}
	}
}

// checkWrites checks the data after a killed writer against the writes it committed: each is there, the one it may
// have been in the middle of is there whole or not at all, and nothing is half written.
func checkWrites(t *testing.T, e *Engine, writes []int) {
	t.Helper()
	last := writes[len(writes)-1]
	lastOf := func(kind int) int {
		for i := len(writes) - 1; i >= 0; i-- {
			if writes[i]%4 == kind {
				return writes[i]
			}
		}
		return 0
	}
	settings, err := e.Settings()
	if err != nil {
		t.Fatalf("the settings: %v", err)
	}
	if written, _ := strconv.Atoi(strings.TrimPrefix(settings.SubscriptionUserAgent, "w")); written < lastOf(0) || written > last+1 {
		t.Errorf("the settings are of write %d; write %d committed, and %d may have", written, lastOf(0), last+1)
	}
	group, found := writerGroupOf(e)
	if !found {
		t.Fatal("the group of the writer is gone")
	}
	if renamed, _ := strconv.Atoi(strings.TrimPrefix(group.Name, writerGroup)); renamed < lastOf(3) || renamed > last+1 {
		t.Errorf("the group has the name of write %d; write %d committed, and %d may have", renamed, lastOf(3), last+1)
	}
	servers, err := e.Servers(ServersQuery{GroupID: group.ID})
	if err != nil {
		t.Fatalf("the servers: %v", err)
	}
	pairs := writerPairs(servers)
	for n, count := range pairs {
		if count != 2 {
			t.Errorf("the servers of write %d are there by half: %d of 2", n, count)
		}
	}
	if added := lastOf(1); added > 0 && pairs[added] == 0 {
		t.Errorf("the servers of write %d committed but are not there", added)
	}
}

// A writer killed at random moments, dozens of times, leaves poros.db that opens, and its data is as before or after
// each write, never in between. A damaged file would be told apart as ErrDataDamaged, which the shell offers to
// start over from; it counts as a failure here, since only a power cut should leave one.
func TestRobustDBSurvivesKills(t *testing.T) {
	kills := 50
	if !robustStress() {
		t.Skip("50 killed writers: POROS_STRESS=1")
	}
	robustOnHost(t)
	r := rand.New(rand.NewPCG(robustSeed(t), 4))
	dir := t.TempDir()
	var waits []time.Duration
	for round := range kills {
		before := len(committedWrites(dir))
		var output bytes.Buffer
		cmd := exec.Command(os.Args[0], "-test.run=^TestRobustDBWriter$")
		cmd.Env = append(os.Environ(), dbWriterEnv+"="+dir)
		cmd.Stdout, cmd.Stderr = &output, &output
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		exited := make(chan error, 1)
		go func() { exited <- cmd.Wait() }()
		// The kill comes after the writer committed once, at a random moment of the writes that follow.
		deadline := time.Now().Add(60 * time.Second)
		for len(committedWrites(dir)) == before {
			select {
			case err := <-exited:
				t.Fatalf("round %d: the writer ended by itself: %v\n%s", round, err, output.String())
			case <-time.After(5 * time.Millisecond):
			}
			if time.Now().After(deadline) {
				cmd.Process.Kill()
				<-exited
				t.Fatalf("round %d: the writer committed nothing in 60 s:\n%s", round, output.String())
			}
		}
		wait := time.Duration(r.IntN(300)) * time.Millisecond
		waits = append(waits, wait)
		time.Sleep(wait)
		if err := cmd.Process.Kill(); err != nil {
			t.Fatalf("round %d: kill: %v", round, err)
		}
		if err := <-exited; err == nil {
			t.Fatalf("round %d: the writer ended by itself:\n%s", round, output.String())
		}

		e, err := Open(Config{DataDir: dir, ForeignUser: true})
		if errors.Is(err, ErrDataDamaged) {
			t.Fatalf("round %d: a killed writer left the data file damaged: %v", round, err)
		}
		if err != nil {
			t.Fatalf("round %d: open after the kill: %v", round, err)
		}
		writes := committedWrites(dir)
		checkWrites(t, e, writes)
		robustCheckData(t, e)
		closeWithin(t, e, 30*time.Second)
		if t.Failed() {
			t.Fatalf("round %d, %d writes committed in all:\n%s", round, len(writes), output.String())
		}
	}
	writes := committedWrites(dir)
	t.Logf("%d writers killed 0-300 ms into their writes (%v), %d writes committed in all, the last %d", kills, waits, len(writes),
		writes[len(writes)-1])
}
