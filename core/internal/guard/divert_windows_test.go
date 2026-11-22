package guard

import (
	"runtime"
	"slices"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestDivertPermitsTheRawPacketsOfSystemAlone(t *testing.T) {
	var pin runtime.Pinner
	defer pin.Unpin()
	rules := divertRules(&pin)
	if len(rules) != 1 || rules[0].action != fwpActionPermit {
		t.Fatalf("one permit: %+v", rules)
	}
	rule := rules[0]
	if !slices.Equal(rule.layers, []windows.GUID{layerALEAuthConnectV4, layerALEAuthConnectV6}) {
		t.Errorf("only what goes out: %v", rule.layers)
	}
	if rule.weight <= weightBlock || rule.weight >= weightLAN {
		t.Errorf("weight %d: over the block, under the rules of the LAN", rule.weight)
	}
	var apps []string
	var protocols []uintptr
	raw := false
	for _, c := range rule.conditions {
		switch c.fieldKey {
		case conditionALEAppID:
			apps = append(apps, blobText(*(**fwpByteBlob)(unsafe.Pointer(&c.conditionValue.value))))
		case conditionIPProtocol:
			protocols = append(protocols, c.conditionValue.value)
		case conditionFlags:
			// Without it System's own connections would pass, and any program makes System dial SMB, over QUIC on
			// 443 as well, with a path to a computer. 0x10 is FWP_CONDITION_FLAG_IS_RAW_ENDPOINT of fwptypes.h.
			raw = c.matchType == fwpMatchFlagsAllSet && c.conditionValue.value == 0x10
		default:
			t.Errorf("a condition on %v", c.fieldKey)
		}
	}
	if !slices.Equal(apps, []string{"System"}) {
		t.Errorf("for %q, not System alone", apps)
	}
	slices.Sort(protocols)
	if !slices.Equal(protocols, []uintptr{ipProtoTCP, ipProtoUDP}) {
		t.Errorf("of protocols %v", protocols)
	}
	if !raw {
		t.Error("not for raw endpoints alone")
	}
}

func TestDivertDriverByItsName(t *testing.T) {
	for name, want := range map[string]bool{
		"WinDivert64.sys": true, "windivert.sys": true, "WinDivert32.sys": true, "WinDivert14.sys": true,
		"wintun.sys": false, "WinDivert.dll": false, "notwindivert.sys": false, "": false,
	} {
		if got := isDivertDriver(name); got != want {
			t.Errorf("%q: %t", name, got)
		}
	}
	drivers, err := loadedDrivers()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(drivers, func(name string) bool { return strings.EqualFold(name, "ntoskrnl.exe") }) {
		t.Errorf("no kernel among the %d drivers", len(drivers))
	}
}
