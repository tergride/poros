//go:build !debug && !noparentcheck

package parentcheck

import (
	"log"
	"os"
	"path/filepath"
	"strings"
)

func CheckParentProcess() {
	parentPath, err := getParentExePath(ParentPID)
	if err != nil {
		log.Fatalf("parent check: cannot read parent executable: %v", err)
	}
	parentPath = resolveFinalPath(parentPath)

	selfPath, err := os.Executable()
	if err != nil {
		log.Fatalf("parent check: cannot read own executable: %v", err)
	}
	selfPath = resolveFinalPath(selfPath)

	selfDir := filepath.Dir(selfPath)
	parentDir := filepath.Dir(parentPath)
	parentBase := filepath.Base(parentPath)

	if !strings.EqualFold(parentDir, selfDir) || !strings.EqualFold(parentBase, "Poros.exe") {
		log.Fatalf("parent check failed: unexpected parent %q, selfPath is %q", parentPath, selfPath)
	}
}
