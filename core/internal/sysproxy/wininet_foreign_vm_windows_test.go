package sysproxy

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	perConnProxyBypass   = 3
	perConnAutoconfigURL = 4
	flagAutoProxyURL     = 4
)

// foreignProxy is all a user may have set before Poros: a proxy, the list of what goes past it and a PAC script.
type foreignProxy struct {
	flags               uint32
	server, bypass, pac string
}

func readForeign(t *testing.T) foreignProxy {
	t.Helper()
	b := winINet{}
	text := func(which uint32) string {
		option := perConnOption{option: which}
		if !query("", &option) || option.string() == nil {
			return ""
		}
		value := option.string()
		defer globalFree.Call(uintptr(unsafe.Pointer(value)))
		return windows.UTF16PtrToString(value)
	}
	return foreignProxy{flags: b.flags(""), server: b.server(""), bypass: text(perConnProxyBypass), pac: text(perConnAutoconfigURL)}
}

func writeForeign(t *testing.T, p foreignProxy) {
	t.Helper()
	texts := make([]*uint16, 3)
	for i, value := range []string{p.server, p.bypass, p.pac} {
		if value != "" {
			texts[i], _ = windows.UTF16PtrFromString(value)
		}
	}
	options := make([]perConnOption, 4)
	options[0].option = perConnFlags
	options[0].setDWORD(p.flags)
	for i, which := range []uint32{perConnProxyServer, perConnProxyBypass, perConnAutoconfigURL} {
		options[i+1].option = which
		options[i+1].setString(texts[i])
	}
	list, err := optionList("", options)
	if err != nil {
		t.Fatal(err)
	}
	ret, _, callErr := internetSetOption.Call(0, optionPerConnectionOption, uintptr(unsafe.Pointer(&list)), uintptr(list.size))
	runtime.KeepAlive(texts)
	if ret == 0 {
		t.Fatalf("InternetSetOption: %v", callErr)
	}
	winINet{}.announce()
}

// A user's own proxy, its exceptions and its PAC script come back exactly as they were after Poros: after a stop and
// after a crash that the next core recovers from.
func TestWinINetKeepsForeignSettingsInVM(t *testing.T) {
	if os.Getenv("POROS_VM_TESTS") != "1" {
		t.Skip("changes the system proxy; runs only in the virtual machine")
	}
	original := readForeign(t)
	t.Cleanup(func() { writeForeign(t, original) })
	foreign := foreignProxy{
		flags:  flagDirect | flagProxy | flagAutoProxyURL,
		server: "http=corp.example:3128;https=corp.example:3129",
		bypass: "*.corp.example;10.*;<local>",
		pac:    "http://wpad.corp.example/proxy.pac",
	}
	writeForeign(t, foreign)
	if got := readForeign(t); got != foreign {
		t.Fatalf("the foreign settings did not take: %+v", got)
	}

	path := filepath.Join(t.TempDir(), "system-proxy.json")
	p := New(path)
	if err := p.Apply("127.0.0.1", 2090); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got := readForeign(t); got.server != "127.0.0.1:2090" || got.flags&flagProxy == 0 {
		t.Fatalf("Apply: %+v", got)
	}
	if err := p.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if got := readForeign(t); got != foreign {
		t.Errorf("after Clear: %+v, before Poros: %+v", got, foreign)
	}

	if err := New(path).Apply("127.0.0.1", 2090); err != nil {
		t.Fatal(err)
	}
	if err := New(path).Recover(); err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if got := readForeign(t); got != foreign {
		t.Errorf("after Recover: %+v, before Poros: %+v", got, foreign)
	}
}
