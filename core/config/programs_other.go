//go:build !windows

package config

// Where files differ by case, so do programs: sing-box compares them as they are written.

func programsByName(names []string) programs {
	return programs{names: names}
}

func programsByPath(paths []string) programs {
	return programs{paths: paths}
}
