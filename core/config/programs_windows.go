package config

// Windows takes a file name in any case and tells the path of a program in the case of its file, while sing-box
// compares process_name and process_path exactly: game.exe would not find Game.exe. So on Windows both are
// patterns of process_path_regex, and RuleText shows them as programs again.

func programsByName(names []string) programs {
	return programs{patterns: []string{nameStart + anyOf(names) + patternEnd}}
}

func programsByPath(paths []string) programs {
	return programs{patterns: []string{pathStart + anyOf(paths) + patternEnd}}
}
