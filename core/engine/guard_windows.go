package engine

import (
	"fmt"
	"path/filepath"
	"time"

	"PorosCore/config"
	"PorosCore/internal/guard"
	"PorosCore/internal/marker"
)

// How long the marker of a block may outlive the guard of this window that turned it off.
const (
	guardGoneWait = 2 * time.Second
	guardGonePoll = 100 * time.Millisecond
)

type windowsGuard struct {
	hostPID int
	logPath string
}

// NewGuard is the kill switch of the Poros window hostPID, whose guard writes its log next to the core's; it is
// nil without a window.
func NewGuard(hostPID int, dataDir string) Guard {
	if hostPID <= 0 {
		return nil
	}
	return windowsGuard{hostPID: hostPID, logPath: filepath.Join(dataDir, "logs", "guard.log")}
}

func (g windowsGuard) Find() (GuardLink, error) {
	link, err := guard.Find(g.hostPID)
	if link == nil {
		return nil, err
	}
	return link, nil
}

// Arm lets the local network through, all but its DNS: private addresses stay out of the TUN, and a printer or a
// NAS would be cut off otherwise.
func (g windowsGuard) Arm() (GuardLink, error) {
	if link, err := g.Find(); link != nil || err != nil {
		return link, err
	}
	// This window has no block of its own, so a block there is another Poros's: asking for rights would not help. The
	// guard of this window that has just turned its block off may still be exiting, so the marker gets a moment.
	for wait := guardGoneWait; marker.Taken(marker.Block); wait -= guardGonePoll {
		if wait <= 0 {
			return nil, errGuardTaken
		}
		time.Sleep(guardGonePoll)
	}
	if !elevated() {
		return nil, errGuardRights
	}
	link, err := guard.Start(g.hostPID, g.logPath, guard.Config{AllowLAN: true, TunName: config.TunName})
	switch guard.ErrorCode(err) {
	case guard.CodePrivilege:
		return nil, fmt.Errorf("%w: %v", errGuardRights, err)
	case guard.CodeTaken:
		return nil, fmt.Errorf("%w: %v", errGuardTaken, err)
	}
	if link == nil {
		return nil, err
	}
	return link, nil
}
