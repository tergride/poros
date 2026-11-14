package config

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestMain lets sing-box load libcronet.dll for NaiveProxy from the module cache: the build copies it next to
// PorosCore.exe, a test binary has none beside it.
func TestMain(m *testing.M) {
	if dir := cronetDir(); dir != "" {
		os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	os.Exit(m.Run())
}

func cronetDir() string {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		return ""
	}
	cache := os.Getenv("GOMODCACHE")
	if cache == "" {
		if gopath := os.Getenv("GOPATH"); gopath != "" {
			cache = filepath.Join(strings.Split(gopath, string(os.PathListSeparator))[0], "pkg", "mod")
		}
	}
	modFile, err := os.Open(filepath.Join("..", "go.mod"))
	if cache == "" || err != nil {
		return ""
	}
	defer modFile.Close()
	const module = "github.com/sagernet/cronet-go/lib/windows_amd64"
	for scanner := bufio.NewScanner(modFile); scanner.Scan(); {
		if fields := strings.Fields(scanner.Text()); len(fields) >= 2 && fields[0] == module {
			dir := filepath.Join(cache, filepath.FromSlash(module)+"@"+fields[1])
			if _, err := os.Stat(filepath.Join(dir, "libcronet.dll")); err == nil {
				return dir
			}
		}
	}
	return ""
}
