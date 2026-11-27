package engine

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"PorosCore/config"
	"PorosCore/internal/logontask"
	"PorosCore/internal/release"
	"PorosCore/internal/wincom"
)

const (
	firewallRulesKey = `SYSTEM\CurrentControlSet\Services\SharedAccess\Parameters\FirewallPolicy\FirewallRules`
	networkListKey   = `SOFTWARE\Microsoft\Windows NT\CurrentVersion\NetworkList`
	// shortcutDepth is how deep the Start menu is looked through: Poros's shortcut is at the top, a user may have
	// moved it into a folder.
	shortcutDepth = 3
	netshTimeout  = 10 * time.Second
)

// platformTraces are the traces of Poros in Windows, in the order they go: the system proxy before its entry for
// the next sign-in, which goes with it.
func (e *Engine) platformTraces() []trace {
	core := filepath.Join(e.programDir, "PorosCore.exe")
	return []trace{
		e.systemProxyTrace(),
		proxyRecoveryTrace(e.dataDir),
		shortcutsTrace(filepath.Join(e.programDir, release.Program)),
		filesTrace(CleanupTempFiles, false, os.TempDir(), "poros-core-heap-*.pprof"),
		e.logonTaskTrace(core),
		systemTempTrace(),
		tunDeviceTrace(),
		firewallTrace(e.programDir),
		e.networksTrace(),
	}
}

func proxyRecoveryTrace(dataDir string) trace {
	name := proxyRecoveryName(dataDir)
	return trace{
		kind:    CleanupProxyRecovery,
		profile: true,
		find: func() (int, error) {
			key, err := registry.OpenKey(registry.CURRENT_USER, runOnceKey, registry.QUERY_VALUE)
			if errors.Is(err, registry.ErrNotExist) {
				return 0, nil
			}
			if err != nil {
				return 0, err
			}
			defer key.Close()
			if _, _, err = key.GetStringValue(name); errors.Is(err, registry.ErrNotExist) {
				return 0, nil
			}
			return 1, err
		},
		remove: func() error {
			key, err := registry.OpenKey(registry.CURRENT_USER, runOnceKey, registry.SET_VALUE)
			if err != nil {
				return err
			}
			defer key.Close()
			if err = key.DeleteValue(name); errors.Is(err, registry.ErrNotExist) {
				return nil
			}
			return err
		},
	}
}

// shortcutsTrace finds the shortcuts that start program on the desktop and in the Start menu, those the archive's
// window made among them.
func shortcutsTrace(program string) trace {
	find := func() ([]string, error) {
		var found []string
		err := withCOM(func() error {
			for _, place := range []struct {
				folder *windows.KNOWNFOLDERID
				depth  int
			}{{windows.FOLDERID_Desktop, 0}, {windows.FOLDERID_Programs, shortcutDepth}} {
				dir, err := windows.KnownFolderPath(place.folder, windows.KF_FLAG_DEFAULT)
				if err != nil {
					continue
				}
				links, err := shortcutsIn(dir, place.depth, program)
				if err != nil {
					return err
				}
				found = append(found, links...)
			}
			return nil
		})
		return found, err
	}
	return trace{
		kind:    CleanupShortcuts,
		profile: true,
		find: func() (int, error) {
			found, err := find()
			return len(found), err
		},
		remove: func() error {
			found, err := find()
			if err != nil {
				return err
			}
			var errs []error
			for _, link := range found {
				errs = append(errs, os.Remove(link))
			}
			return errors.Join(errs...)
		},
	}
}

// shortcutsIn lists the shortcut files in dir and its folders down to depth that start program.
func shortcutsIn(dir string, depth int, program string) ([]string, error) {
	var found []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			// A folder that cannot be read holds no shortcut Poros or the user put there.
			if path != dir {
				return fs.SkipDir
			}
			return err
		}
		if entry.IsDir() {
			if rel, _ := filepath.Rel(dir, path); path != dir && strings.Count(rel, string(filepath.Separator)) >= depth {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".lnk") {
			return nil
		}
		if link, err := wincom.ReadShortcut(path); err == nil && strings.EqualFold(filepath.Clean(link.Target), filepath.Clean(program)) {
			found = append(found, path)
		}
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return found, err
}

// withCOM runs f on a thread of its own with COM initialized. A thread that a library of the core already set up for
// COM in the other mode stays in it: the objects here work in both.
func withCOM(f func() error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	switch err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); {
	case err == nil, errors.Is(err, syscall.Errno(1)):
		defer windows.CoUninitialize()
	case errors.Is(err, syscall.Errno(windows.RPC_E_CHANGED_MODE)):
	default:
		return fmt.Errorf("COM: %w", err)
	}
	return f()
}

// filesTrace finds the files of pattern in dir.
func filesTrace(kind string, rights bool, dir, pattern string) trace {
	return trace{
		kind:    kind,
		rights:  rights,
		profile: !rights,
		find: func() (int, error) {
			found, err := filepath.Glob(filepath.Join(dir, pattern))
			return len(found), err
		},
		remove: func() error { return removeFiles(filepath.Join(dir, pattern)) },
	}
}

func removeFiles(pattern string) error {
	found, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}
	var errs []error
	for _, file := range found {
		errs = append(errs, os.Remove(file))
	}
	return errors.Join(errs...)
}

// systemTempTrace finds the definitions of the sign-in task that a core with rights writes into the Temp folder
// of Windows and removes at once: a crash in between leaves one. Only an administrator sees that folder.
func systemTempTrace() trace {
	pattern := ""
	if dir, err := windows.GetSystemWindowsDirectory(); err == nil {
		pattern = filepath.Join(dir, "Temp", "poros-task-*.xml")
	}
	return trace{
		kind:   CleanupTempFiles,
		rights: true,
		find: func() (int, error) {
			if pattern == "" || !elevated() {
				return 0, nil
			}
			found, err := filepath.Glob(pattern)
			return len(found), err
		},
		remove: func() error { return removeFiles(pattern) },
	}
}

// logonTaskTrace is the task of rights at sign-in. A user without rights may not see it: the setting, or the mark
// for its removal that turning the setting off without rights leaves, tells of it then.
func (e *Engine) logonTaskTrace(core string) trace {
	return trace{
		kind:   CleanupLogonTask,
		rights: true,
		find: func() (int, error) {
			settings, err := e.Settings()
			if err != nil {
				return 0, err
			}
			if settings.AdminAutostart || logontask.RemovalMarked(e.dataDir) || logontask.Exists(core) {
				return 1, nil
			}
			return 0, nil
		},
		remove: func() error { return logontask.Remove(core, e.dataDir) },
	}
}

func tunDeviceTrace() trace {
	return trace{
		kind:   CleanupTunDevice,
		rights: true,
		find: func() (int, error) {
			left, err := staleTunLeft()
			if left {
				return 1, err
			}
			return 0, err
		},
		remove: func() error {
			_, err := removeStaleTun()
			return err
		},
	}
}

// firewallTrace finds the rules of the Windows firewall for the programs of the folder: Windows makes them when the
// user answers its question about a program that listens on the network. Reading them takes no rights.
func firewallTrace(folder string) trace {
	return trace{
		kind:   CleanupFirewall,
		rights: true,
		find: func() (int, error) {
			rules, err := firewallRules(folder)
			return len(rules), err
		},
		remove: func() error {
			rules, err := firewallRules(folder)
			if err != nil {
				return err
			}
			apps := map[string]bool{}
			for _, app := range rules {
				apps[strings.ToLower(app)] = true
			}
			var errs []error
			for app := range apps {
				errs = append(errs, netsh("advfirewall", "firewall", "delete", "rule", "name=all", "program="+app))
			}
			if rules, err = firewallRules(folder); err == nil && len(rules) > 0 {
				err = errStillThere
			}
			// netsh fails on a program whose rules another call removed; what counts is that none stay.
			if err != nil {
				return errors.Join(append(errs, err)...)
			}
			return nil
		},
	}
}

// firewallRules gives the program of each rule for a program inside folder, its variables of the environment
// expanded.
func firewallRules(folder string) ([]string, error) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, firewallRulesKey, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer key.Close()
	names, err := key.ReadValueNames(0)
	if err != nil {
		return nil, err
	}
	var apps []string
	for _, name := range names {
		rule, _, err := key.GetStringValue(name)
		if err != nil {
			continue
		}
		app := firewallApp(rule)
		if expanded, err := registry.ExpandString(app); err == nil {
			app = expanded
		}
		if insideFolder(app, folder) {
			apps = append(apps, app)
		}
	}
	return apps, nil
}

func netsh(args ...string) error {
	system, err := windows.GetSystemDirectory()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), netshTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(system, "netsh.exe"), args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("netsh %s: %w: %s", strings.Join(args[:3], " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

// networksTrace finds the networks Windows remembers for the TUN adapter. A core with rights reads them from the
// registry. The list of networks of Windows, which needs no rights, forgets a network once its adapter is gone, so a
// core without rights goes by the mark of a TUN that ran. Only one TUN of Poros runs on the computer: its adapter
// without a TUN of this core is another copy's, whose connected network stays. The list may still call a network
// connected for a moment after its adapter went, so without the adapter none counts as connected.
func (e *Engine) networksTrace() trace {
	find := func() ([]windows.GUID, error) {
		_, up := net.InterfaceByName(config.TunName)
		others := !e.ConnectionState().Tun && up == nil
		connected := map[windows.GUID]bool{}
		var listed []windows.GUID
		err := withCOM(func() error {
			networks, err := wincom.Networks()
			for _, network := range networks {
				if !tunNetwork(network.Name, config.TunName) {
					continue
				}
				connected[network.ID] = network.Connected
				if !network.Connected || !others {
					listed = append(listed, network.ID)
				}
			}
			return err
		})
		if !elevated() {
			return listed, err
		}
		// Without the list a connected network of another copy cannot be told apart; the registry still tells the rest.
		if err != nil {
			log.Printf("cleanup: the networks of Windows: %v", err)
		}
		profiles, err := tunProfiles()
		var found []windows.GUID
		for _, id := range profiles {
			if !connected[id] || !others {
				found = append(found, id)
			}
		}
		return found, err
	}
	return trace{
		kind:   CleanupNetworks,
		rights: true,
		find: func() (int, error) {
			found, err := find()
			if err == nil && len(found) == 0 && !elevated() && e.tunRan() {
				return 1, nil
			}
			return len(found), err
		},
		remove: func() error {
			found, err := find()
			if err != nil {
				return err
			}
			var errs []error
			for _, id := range found {
				errs = append(errs, forgetNetwork(id))
			}
			return errors.Join(errs...)
		},
	}
}

// tunProfiles gives the profiles of the networks Windows remembers under the name of the TUN. Only an administrator
// may read them.
func tunProfiles() ([]windows.GUID, error) {
	path := networkListKey + `\Profiles`
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.ENUMERATE_SUB_KEYS)
	if errors.Is(err, registry.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	names, err := key.ReadSubKeyNames(0)
	key.Close()
	if err != nil {
		return nil, err
	}
	var found []windows.GUID
	for _, name := range names {
		profile, err := registry.OpenKey(registry.LOCAL_MACHINE, path+`\`+name, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		title, _, err := profile.GetStringValue("ProfileName")
		profile.Close()
		if err != nil || !tunNetwork(title, config.TunName) {
			continue
		}
		if id, err := windows.GUIDFromString(name); err == nil {
			found = append(found, id)
		}
	}
	return found, nil
}

// forgetNetwork removes the profile of a network and the signatures that lead to it, as forgetting a network in
// the settings of Windows does.
func forgetNetwork(id windows.GUID) error {
	profile := id.String()
	var errs []error
	for _, kind := range []string{"Managed", "Unmanaged"} {
		path := networkListKey + `\Signatures\` + kind
		key, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.ENUMERATE_SUB_KEYS)
		if errors.Is(err, registry.ErrNotExist) {
			continue
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}
		signatures, err := key.ReadSubKeyNames(0)
		key.Close()
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, signature := range signatures {
			entry, err := registry.OpenKey(registry.LOCAL_MACHINE, path+`\`+signature, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			guid, _, err := entry.GetStringValue("ProfileGuid")
			entry.Close()
			if err == nil && strings.EqualFold(guid, profile) {
				errs = append(errs, deleteKey(path+`\`+signature))
			}
		}
	}
	errs = append(errs, deleteKey(networkListKey+`\Profiles\`+profile))
	return errors.Join(errs...)
}

var procRegDeleteTreeW = windows.NewLazySystemDLL("advapi32.dll").NewProc("RegDeleteTreeW")

// deleteKey removes a key of HKEY_LOCAL_MACHINE with what it holds; one that is not there is fine.
func deleteKey(path string) error {
	status, _, _ := procRegDeleteTreeW.Call(uintptr(registry.LOCAL_MACHINE), uintptr(unsafe.Pointer(wincom.Wide(path))))
	if status != 0 && syscall.Errno(status) != windows.ERROR_FILE_NOT_FOUND {
		return fmt.Errorf("remove %s: %w", path, syscall.Errno(status))
	}
	return nil
}

// insideFolder tells whether path names a file inside folder, in any case and with any separators.
func insideFolder(path, folder string) bool {
	if path == "" || folder == "" {
		return false
	}
	path, folder = strings.ToLower(filepath.Clean(path)), strings.ToLower(filepath.Clean(folder))
	return strings.HasPrefix(path, strings.TrimSuffix(folder, string(filepath.Separator))+string(filepath.Separator))
}

// firewallApp gives the program of a rule as the firewall of Windows stores it, "v2.30|Action=Allow|…|App=…|".
func firewallApp(rule string) string {
	for field := range strings.SplitSeq(rule, "|") {
		if name, value, ok := strings.Cut(field, "="); ok && strings.EqualFold(name, "App") {
			return value
		}
	}
	return ""
}
