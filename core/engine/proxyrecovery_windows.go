package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

const runOnceKey = `Software\Microsoft\Windows\CurrentVersion\RunOnce`

// ProxyRecovery keeps a RunOnce entry while the proxy of Windows points at Poros. A crash of the whole Poros, a power
// cut or a shutdown of Windows that ends the core leaves the proxy at a port no one serves, and browsers without a
// connection until Poros starts again; at the next sign-in Windows runs the core once to give the settings back.
func ProxyRecovery(dataDir string) func(bool) {
	program, err := os.Executable()
	if err != nil {
		return nil
	}
	sum := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(dataDir))))
	name := "Poros proxy " + hex.EncodeToString(sum[:4])
	command := `"` + program + `" --restore-proxy --data-dir ` + syscall.EscapeArg(dataDir)
	var written *bool
	return func(pending bool) {
		if written != nil && *written == pending {
			return
		}
		key, _, err := registry.CreateKey(registry.CURRENT_USER, runOnceKey, registry.SET_VALUE)
		if err != nil {
			log.Printf("system proxy: the entry for the next sign-in: %v", err)
			return
		}
		defer key.Close()
		if pending {
			err = key.SetStringValue(name, command)
		} else if err = key.DeleteValue(name); err == registry.ErrNotExist {
			err = nil
		}
		if err != nil {
			log.Printf("system proxy: the entry for the next sign-in: %v", err)
			return
		}
		written = &pending
	}
}
