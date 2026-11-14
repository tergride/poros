//go:build !windows

package engine

import "runtime"

var osName = runtime.GOOS

func osVersion() string { return "" }
