//go:build windows

package logontask

import (
	"encoding/xml"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestWaitsOnlyForItsOwnShellWithTheFlag(t *testing.T) {
	shell := `D:\Apps\Poros\Poros.exe`
	cases := map[string]bool{
		`"D:\Apps\Poros\Poros.exe" --autostart --logon-core`:             true,
		`"d:\apps\poros\poros.exe" --autostart --minimized --logon-core`: true,
		`"D:\Apps\Poros\Poros.exe" --autostart`:                          false,
		`"D:\Other\Poros.exe" --autostart --logon-core`:                  false,
	}
	for command, want := range cases {
		if got := waits(command, shell); got != want {
			t.Errorf("waits(%s) = %t, want %t", command, got, want)
		}
	}
}

func TestDefinitionIsUTF16XMLWithTheCore(t *testing.T) {
	core := `C:\Poros & Co\PorosCore.exe`
	data := utf16File(definition("S-1-5-21-1", core, `C:\Poros & Co\config`))
	if data[0] != 0xFF || data[1] != 0xFE {
		t.Fatal("no byte order mark")
	}
	units := make([]uint16, (len(data)-2)/2)
	for i := range units {
		units[i] = uint16(data[2+2*i]) | uint16(data[3+2*i])<<8
	}
	text := string(utf16.Decode(units))
	var task struct {
		Principal struct {
			UserID   string `xml:"UserId"`
			RunLevel string
		} `xml:"Principals>Principal"`
		Exec struct {
			Command   string
			Arguments string
		} `xml:"Actions>Exec"`
		AllowStartOnDemand bool `xml:"Settings>AllowStartOnDemand"`
	}
	// The declaration names UTF-16, which the text no longer is.
	if err := xml.Unmarshal([]byte(strings.Replace(text, `encoding="UTF-16"`, "", 1)), &task); err != nil {
		t.Fatal(err)
	}
	if task.Exec.Command != core || task.Exec.Arguments != `--logon --data-dir "C:\Poros & Co\config"` {
		t.Errorf("action %q %q", task.Exec.Command, task.Exec.Arguments)
	}
	if task.Principal.UserID != "S-1-5-21-1" || task.Principal.RunLevel != "HighestAvailable" || task.AllowStartOnDemand {
		t.Errorf("principal %+v, on demand %t", task.Principal, task.AllowStartOnDemand)
	}
}

func TestEveryCopyHasItsTask(t *testing.T) {
	a, b := Name(`C:\A\PorosCore.exe`), Name(`C:\B\PorosCore.exe`)
	if a == b || a != Name(`c:\a\PorosCore.exe`) {
		t.Errorf("names %q and %q", a, b)
	}
}
