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

// Logs gives the lines of the core's log, which goes on across runs: those written after the byte q.After, or with
// 0 the last ones. Only whole lines come; one still being written comes with the next query.
func (e *Engine) Logs(q LogsQuery) (LogsChunk, error) {
	settings, err := e.Settings()
	if err != nil {
		settings = DefaultSettings()
	}
	maxLines := settings.LogLines
	tail := int64(max(maxLines*logLineBytes, 256<<10))
	file, err := os.Open(filepath.Join(e.dataDir, "logs", "core.log"))
	if errors.Is(err, os.ErrNotExist) {
		return LogsChunk{Lines: []string{}, Reset: q.After != 0}, nil
	}
	if err != nil {
		return LogsChunk{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return LogsChunk{}, err
	}
	size := info.Size()
	// A log shorter than the byte asked for began anew; one that grew by more than the screen holds is read from
	// its end, as at the first query.
	chunk := LogsChunk{Reset: q.After > size}
	start := q.After
	if q.After <= 0 || q.After > size || size-q.After > tail {
		chunk.Reset = q.After != 0
		start = max(0, size-tail)
	}
	data, err := io.ReadAll(io.NewSectionReader(file, start, size-start))
	if err != nil {
		return LogsChunk{}, err
	}
	if start > 0 && start != q.After {
		// The first line read is cut.
		if newline := bytes.IndexByte(data, '\n'); newline >= 0 {
			data, start = data[newline+1:], start+int64(newline+1)
		} else {
			data = nil
		}
	}
	whole := bytes.LastIndexByte(data, '\n') + 1
	chunk.Next = start + int64(whole)
	chunk.Lines = []string{}
	if whole == 0 {
		return chunk, nil
	}
	lines := strings.Split(string(data[:whole-1]), "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, "\r")
	}
	chunk.Lines = lines
	return chunk, nil
}
