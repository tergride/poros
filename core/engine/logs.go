package engine

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	// The log screen shows the last lines of the log, as many as the settings say.
	defaultLogLines = 500
	minLogLines     = 100
	maxLogLines     = 20000
	// logLineBytes is how much of the end of the file is read for each of them.
	logLineBytes = 512
)

// Logs gives the last lines of the core's log, which goes on across runs.
func (e *Engine) Logs() ([]string, error) {
	settings, err := e.Settings()
	if err != nil {
		settings = DefaultSettings()
	}
	logLines := settings.LogLines
	logTail := int64(max(logLines*logLineBytes, 256<<10))
	file, err := os.Open(filepath.Join(e.dataDir, "logs", "core.log"))
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	start := max(0, info.Size()-logTail)
	data, err := io.ReadAll(io.NewSectionReader(file, start, info.Size()-start))
	if err != nil {
		return nil, err
	}
	if start > 0 {
		// The first line read is cut.
		if newline := bytes.IndexByte(data, '\n'); newline >= 0 {
			data = data[newline+1:]
		}
	}
	lines := strings.Split(strings.TrimRight(string(data), "\r\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return []string{}, nil
	}
	if len(lines) > logLines {
		lines = lines[len(lines)-logLines:]
	}
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, "\r")
	}
	return lines, nil
}
