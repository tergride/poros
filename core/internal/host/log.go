package host

import (
	"os"
	"path/filepath"
	"strings"
)

// maxLogSize is where a log moves aside: the runs before stay readable after a restart or a UAC prompt, and two
// files bound the space.
const maxLogSize = 2 << 20

// OpenLog appends to the log at path across runs. Past maxLogSize it moves to <name>.1<ext>, in place of the one
// before.
func OpenLog(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if info, err := os.Stat(path); err == nil && info.Size() > maxLogSize {
		_ = os.Rename(path, strings.TrimSuffix(path, filepath.Ext(path))+".1"+filepath.Ext(path))
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
}
