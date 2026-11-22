package config

import (
	"regexp"
	"strings"
)

// The patterns of programsByName and programsByPath on Windows. The group is named for the condition of the preset,
// so that RuleText tells them from the user's own patterns and reads the values back.
const (
	nameStart  = `(?i)(?:^|[\\/])(?P<process_name>`
	pathStart  = `(?i)^(?P<process_path>`
	patternEnd = `)$`

	patternItem = "process_path_regex="
)

// The values are quoted, so the first bracket without a backslash ends them.
var (
	writtenNames = writtenPattern(nameStart)
	writtenPaths = writtenPattern(pathStart)
)

func writtenPattern(start string) *regexp.Regexp {
	return regexp.MustCompile(regexp.QuoteMeta(patternItem+start) + `((?:\\.|[^\\)])*)` + regexp.QuoteMeta(patternEnd))
}

func anyOf(values []string) string {
	quoted := make([]string, len(values))
	for i, value := range values {
		quoted[i] = regexp.QuoteMeta(value)
	}
	return strings.Join(quoted, "|")
}

// valuesOf reads back what anyOf wrote.
func valuesOf(text string) []string {
	var values []string
	var value strings.Builder
	escaped := false
	for _, r := range text {
		switch {
		case escaped:
			value.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case r == '|':
			values = append(values, value.String())
			value.Reset()
		default:
			value.WriteRune(r)
		}
	}
	return append(values, value.String())
}

// RuleText is the text sing-box gives of a matched rule, for people to read, with programs in place of the patterns
// Build writes. paths are those of the connection's program, the one the rule matched by first: the rule shows the
// program as its file is named, for a pattern of the user's too. An inverted rule matched a program that it does not
// name, so it stays as written.
func RuleText(text string, paths []string) string {
	if strings.HasPrefix(text, "!(") {
		paths = nil
	}
	text = showPrograms(text, writtenNames, "process_name", paths, fileName)
	text = showPrograms(text, writtenPaths, "process_path", paths, func(path string) string { return path })
	// What is left is a pattern of the user's, always the last condition of its rule.
	if at := strings.Index(text, patternItem); at >= 0 && len(paths) > 0 {
		text = text[:at] + "process_name=" + fileName(paths[0])
	}
	return text
}

func showPrograms(text string, written *regexp.Regexp, key string, paths []string, part func(path string) string) string {
	return written.ReplaceAllStringFunc(text, func(item string) string {
		values := valuesOf(written.FindStringSubmatch(item)[1])
		for _, path := range paths {
			for _, value := range values {
				if program := part(path); strings.EqualFold(program, value) {
					return key + "=" + program
				}
			}
		}
		if len(values) == 1 {
			return key + "=" + values[0]
		}
		return key + "=[" + strings.Join(values, " ") + "]"
	})
}

func fileName(path string) string {
	return path[strings.LastIndexAny(path, `\/`)+1:]
}
