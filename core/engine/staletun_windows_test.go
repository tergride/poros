package engine

import "testing"

// The device of Poros's TUN as Windows names it: SWD\WINTUN\{4F594090-…}, the one a user's Windows kept after a run.
func TestTunGUIDIsSingTuns(t *testing.T) {
	if got := tunGUID("poros-net").String(); got != "{4F594090-EC0E-1B39-2480-6AC5136350DF}" {
		t.Errorf("the GUID of poros-net is %s", got)
	}
}
