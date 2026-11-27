package engine

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/sys/windows"

	"PorosCore/internal/wincom"
)

func TestCleanupMatchesOnlyThisFolder(t *testing.T) {
	for path, inside := range map[string]bool{
		`C:\Apps\Poros\PorosCore.exe`:          true,
		`c:\apps\poros\SUB\updater.old`:        true,
		`C:/Apps/Poros/Poros.exe`:              true,
		`C:\Apps\Poros`:                        false,
		`C:\Apps\Poros2\Poros.exe`:             false,
		`C:\Apps\Throne\Throne.exe`:            false,
		``:                                     false,
		`C:\Apps\Poros\..\Throne\nekobox_core`: false,
		`C:\Apps\Poros\.\config\..\Poros.exe`:  true,
		`D:\Apps\Poros\PorosCore.exe`:          false,
	} {
		if got := insideFolder(path, `C:\Apps\Poros\`); got != inside {
			t.Errorf("%q inside the folder: %v", path, got)
		}
	}
	if insideFolder(`C:\Apps\Poros\Poros.exe`, "") {
		t.Error("no folder holds nothing")
	}

	rule := `v2.30|Action=Allow|Active=TRUE|Dir=In|Protocol=6|Profile=Private|App=C:\Apps\Poros\PorosCore.exe|Name=poroscore|`
	if got := firewallApp(rule); got != `C:\Apps\Poros\PorosCore.exe` {
		t.Errorf("the program of the rule: %q", got)
	}
	if got := firewallApp(`v2.30|Action=Block|Dir=Out|Name=Core Networking|`); got != "" {
		t.Errorf("a rule without a program: %q", got)
	}
}

func TestCleanupFindsTheShortcutsOfThisPoros(t *testing.T) {
	folder := tempDir(t)
	program := filepath.Join(folder, "Poros.exe")
	menu := tempDir(t)
	deep := filepath.Join(menu, "A", "B", "C", "D")
	if err := os.MkdirAll(filepath.Join(menu, "Tools"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	links := map[string]string{
		filepath.Join(menu, "Poros.lnk"):          program,
		filepath.Join(menu, "Tools", "VPN.lnk"):   strings.ToUpper(program),
		filepath.Join(menu, "Other Poros.lnk"):    filepath.Join(folder, "Other", "Poros.exe"),
		filepath.Join(menu, "Throne.lnk"):         filepath.Join(folder, "Throne", "Throne.exe"),
		filepath.Join(deep, "Too deep.lnk"):       program,
		filepath.Join(menu, "Not a shortcut.txt"): "",
	}
	err := withCOM(func() error {
		for link, target := range links {
			if target == "" {
				if err := os.WriteFile(link, []byte(program), 0o644); err != nil {
					return err
				}
				continue
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(target, nil, 0o644); err != nil {
				return err
			}
			if err := wincom.CreateShortcut(link, target, filepath.Dir(target)); err != nil {
				return err
			}
		}
		top, err := shortcutsIn(menu, 0, program)
		if err != nil {
			return err
		}
		if want := []string{filepath.Join(menu, "Poros.lnk")}; !slices.Equal(top, want) {
			t.Errorf("the desktop holds %v", top)
		}
		all, err := shortcutsIn(menu, shortcutDepth, program)
		if err != nil {
			return err
		}
		slices.Sort(all)
		if want := []string{filepath.Join(menu, "Poros.lnk"), filepath.Join(menu, "Tools", "VPN.lnk")}; !slices.Equal(all, want) {
			t.Errorf("the Start menu holds %v", all)
		}
		missing, err := shortcutsIn(filepath.Join(menu, "none"), shortcutDepth, program)
		if err != nil || missing != nil {
			t.Errorf("a folder that is not there holds %v, %v", missing, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Looking takes no rights and changes nothing: the networks of Windows and the rules of its firewall.
func TestCleanupLooksWithoutRights(t *testing.T) {
	if err := withCOM(func() error {
		networks, err := wincom.Networks()
		for _, network := range networks {
			if network.ID == (windows.GUID{}) {
				t.Error("a network without the ID of its profile")
			}
		}
		return err
	}); err != nil {
		t.Errorf("the networks of Windows: %v", err)
	}
	if rules, err := firewallRules(tempDir(t)); err != nil || len(rules) != 0 {
		t.Errorf("a new folder has rules of the firewall: %v, %v", rules, err)
	}
}
