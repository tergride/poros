//go:build !windows

package logontask

func Remove(core, dataDir string) error {
	clearRemoval(dataDir)
	return nil
}

func Exists(core string) bool {
	return false
}
