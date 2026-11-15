package engine

import (
	"slices"
	"strings"

	"PorosCore/internal/jsonrpc"
)

// listActions are the tabs of the lists window in the order it shows them; a line on two tabs is a problem of the
// later one. An advanced preset has «Through WARP» too, an endpoint one tab of its own.
func listActions(set ruleSet) []string {
	switch set {
	case endpointRules:
		return []string{ActionEndpoint}
	case advancedRules:
		return []string{ActionProxy, ActionDirect, ActionBlock, ActionWARP}
	}
	return []string{ActionProxy, ActionDirect, ActionBlock}
}

// listKind gives the kind of a prefix of the lists window, whatever its case. The private networks have no line.
func listKind(prefix string) (string, bool) {
	for _, kind := range ruleKinds {
		if kind != CondPrivate && strings.EqualFold(kind, prefix) {
			return kind, true
		}
	}
	return "", false
}

// inList tells whether the lists window has a line for a rule: one condition with one value, as kind:value.
func inList(rule PresetRule) bool {
	return single(rule) && rule.Conditions[0].Kind != CondPrivate
}

// WritePresetLists gives the rules of a preset or an endpoint as the text of the lists window: each line has the
// prefix of its kind. Rules of several conditions or values, inverted ones and the private networks have no line.
func WritePresetLists(p PresetRules) (PresetLists, error) {
	set := ruleSetOf(p.Advanced, p.Endpoint)
	lines := map[string][]string{}
	for _, rule := range p.Rules {
		if !inList(rule) {
			continue
		}
		if !slices.Contains(listActions(set), rule.Action) {
			return PresetLists{}, jsonrpc.Failed(ReasonInvalidValue, "there is no list for the action %q", rule.Action)
		}
		condition := rule.Conditions[0]
		lines[rule.Action] = append(lines[rule.Action], condition.Kind+":"+condition.Values[0])
	}
	lists := PresetLists{Lists: map[string]string{}}
	for _, action := range listActions(set) {
		lists.Lists[action] = strings.Join(lines[action], "\n")
	}
	return lists, nil
}

// ReadPresetLists puts the text of the lists window into the rules it was written from and names the lines that
// became no rule. Empty lines and lines starting with # are skipped; a line repeated on its own tab counts once.
func ReadPresetLists(p PresetListsInput) (PresetListsRead, error) {
	set := ruleSetOf(p.Advanced, p.Endpoint)
	for action := range p.Lists {
		if !slices.Contains(listActions(set), action) {
			return PresetListsRead{}, jsonrpc.Failed(ReasonInvalidValue, "there is no list %q", action)
		}
	}
	read := PresetListsRead{Rules: []PresetRule{}, Problems: []PresetListProblem{}}
	type line struct{ kind, value string }
	tabOf := map[string]string{}
	var keys []string
	lines := map[string]line{}
	for _, action := range listActions(set) {
		for i, raw := range strings.Split(p.Lists[action], "\n") {
			text := strings.TrimSpace(raw)
			if text == "" || strings.HasPrefix(text, "#") {
				continue
			}
			problem := PresetListProblem{Action: action, Line: i + 1, Text: text}
			prefix, rest, found := strings.Cut(text, ":")
			kind, known := listKind(strings.TrimSpace(prefix))
			value, valid := ruleValue(kind, rest)
			switch {
			case !found || !known:
				problem.Reason = ReasonNoPrefix
			case set == simpleRules && !slices.Contains(simpleKinds, kind):
				problem.Reason = ReasonAdvancedOnly
			case !valid:
				problem.Reason = ReasonInvalidEntry
			}
			if problem.Reason != "" {
				read.Problems = append(read.Problems, problem)
				continue
			}
			key := valueKey(kind, value)
			if other, seen := tabOf[key]; seen {
				if other != action {
					problem.Reason, problem.Other = ReasonDuplicate, other
					read.Problems = append(read.Problems, problem)
				}
				continue
			}
			tabOf[key] = action
			keys = append(keys, key)
			lines[key] = line{kind, value}
		}
	}
	// A rule with a line keeps its place, its name and its switch, and takes the action of its tab; one whose line
	// is gone goes too. New lines come last.
	kept := map[string]bool{}
	for _, rule := range p.Rules {
		if !inList(rule) {
			read.Rules = append(read.Rules, rule)
			continue
		}
		condition := rule.Conditions[0]
		value, ok := ruleValue(condition.Kind, condition.Values[0])
		if !ok {
			value = condition.Values[0]
		}
		key := valueKey(condition.Kind, value)
		action, listed := tabOf[key]
		if !listed || kept[key] {
			continue
		}
		kept[key] = true
		rule.Action = action
		read.Rules = append(read.Rules, rule)
	}
	for _, key := range keys {
		if kept[key] {
			continue
		}
		read.Rules = append(read.Rules, PresetRule{
			Conditions: []RuleCondition{{Kind: lines[key].kind, Values: []string{lines[key].value}}}, Action: tabOf[key],
		})
	}
	read.Rules = narrowFirst(read.Rules)
	return read, nil
}
