package engine

import (
	"net/netip"
	"slices"
	"strings"

	"PorosCore/internal/jsonrpc"
)

// listActions are the tabs of the lists window in the order it shows them; a value on two tabs is a problem of the
// later one.
var listActions = []string{ActionProxy, ActionDirect, ActionBlock}

// Prefixes of Throne's simple rules and of Xray's domain lists that say what a line is. Poros has no exact domains:
// domain:, full: and suffix: all take the domain with its subdomains. Keywords, patterns and rule sets it cannot read.
var (
	sitePrefixes       = []string{"domain", "full", "suffix", "ip"}
	appPrefixes        = []string{"processname", "processpath"}
	unreadablePrefixes = []string{"keyword", "regex", "regexp", "ruleset", "geosite", "geoip"}
)

// The prefixes WritePresetLists puts before a value that would read as the other kind.
const (
	sitePrefix = "domain:"
	appPrefix  = "processName:"
)

type listLine struct {
	site  bool
	value string
	// key is what the entry matches, so that two spellings of one site are one entry.
	key string
}

// readListLine tells what a line of the lists window is: by its prefix or, without one, by its look.
func readListLine(text string) (listLine, bool) {
	value, site, known := text, false, false
	if prefix, rest, found := strings.Cut(text, ":"); found {
		switch prefix = strings.ToLower(strings.TrimSpace(prefix)); {
		case slices.Contains(sitePrefixes, prefix):
			value, site, known = strings.TrimSpace(rest), true, true
		case slices.Contains(appPrefixes, prefix):
			value, known = strings.TrimSpace(rest), true
		case slices.Contains(unreadablePrefixes, prefix):
			return listLine{}, false
		}
	}
	if !known {
		site = looksLikeSite(value)
	}
	entry := PresetEntry{Value: value, Action: ActionProxy}
	if site {
		rule, err := siteRule(entry)
		if err != nil {
			return listLine{}, false
		}
		return listLine{site: true, value: value, key: "site " + strings.Join(slices.Concat(rule.DomainSuffix, rule.IPCIDR), "")}, true
	}
	rule, err := appRule(entry)
	if err != nil {
		return listLine{}, false
	}
	return listLine{value: value, key: "app " + strings.ToLower(strings.Join(slices.Concat(rule.ProcessName, rule.ProcessPath), ""))}, true
}

// looksLikeSite tells a site, address or subnet from a program, which is a file name ending in .exe or a path.
func looksLikeSite(value string) bool {
	if strings.Contains(value, "://") {
		return true
	}
	if _, err := netip.ParsePrefix(value); err == nil {
		return true
	}
	if _, err := netip.ParseAddr(strings.Trim(value, "[]")); err == nil {
		return true
	}
	return !strings.HasSuffix(strings.ToLower(value), ".exe") && !strings.Contains(value, `\`) && !strings.HasPrefix(value, "/")
}

// ReadPresetLists turns the text of the lists window into the entries of a preset, and names the lines that became
// none. Empty lines and lines starting with # are skipped; a line repeated on its own tab counts once.
func ReadPresetLists(p PresetLists) (PresetListsRead, error) {
	for action := range p.Lists {
		if !slices.Contains(listActions, action) {
			return PresetListsRead{}, jsonrpc.Failed(ReasonInvalidValue, "there is no list %q", action)
		}
	}
	read := PresetListsRead{Sites: []PresetEntry{}, Apps: []PresetEntry{}, Problems: []PresetListProblem{}}
	tabOf := map[string]string{}
	for _, action := range listActions {
		for i, line := range strings.Split(p.Lists[action], "\n") {
			text := strings.TrimSpace(line)
			if text == "" || strings.HasPrefix(text, "#") {
				continue
			}
			entry, ok := readListLine(text)
			if !ok {
				read.Problems = append(read.Problems, PresetListProblem{Action: action, Line: i + 1, Text: text, Reason: ReasonInvalidEntry})
				continue
			}
			if other, seen := tabOf[entry.key]; seen {
				if other != action {
					read.Problems = append(read.Problems, PresetListProblem{
						Action: action, Line: i + 1, Text: text, Reason: ReasonDuplicate, Other: other,
					})
				}
				continue
			}
			tabOf[entry.key] = action
			if entry.site {
				read.Sites = append(read.Sites, PresetEntry{Value: entry.value, Action: action})
			} else {
				read.Apps = append(read.Apps, PresetEntry{Value: entry.value, Action: action})
			}
		}
	}
	return read, nil
}

// WritePresetLists gives the entries of a preset as the text of the lists window, so that ReadPresetLists brings them
// back: a value that would read as the other kind, such as a program without .exe, gets its prefix.
func WritePresetLists(p PresetEntries) (PresetLists, error) {
	lines := map[string][]string{}
	write := func(entries []PresetEntry, site bool, prefix string) error {
		for _, entry := range entries {
			if !slices.Contains(listActions, entry.Action) {
				return jsonrpc.Failed(ReasonInvalidEntry, "%q: the action must be proxy, direct or block, not %q", entry.Value, entry.Action)
			}
			value := strings.TrimSpace(entry.Value)
			bare, ok := readListLine(value)
			if !ok || bare.site != site || bare.value != value {
				// An entry neither way reads stays as the user typed it, for the window to name.
				if _, prefixedOK := readListLine(prefix + value); prefixedOK {
					value = prefix + value
				}
			}
			lines[entry.Action] = append(lines[entry.Action], value)
		}
		return nil
	}
	if err := write(p.Sites, true, sitePrefix); err != nil {
		return PresetLists{}, err
	}
	if err := write(p.Apps, false, appPrefix); err != nil {
		return PresetLists{}, err
	}
	lists := PresetLists{Lists: map[string]string{}}
	for _, action := range listActions {
		lists.Lists[action] = strings.Join(lines[action], "\n")
	}
	return lists, nil
}
