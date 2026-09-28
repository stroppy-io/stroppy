package report

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	historyDirPerm  = 0o700
	historyFilePerm = 0o600
	maxNameAttempts = 1000
)

var errHistoryNameExhausted = errors.New("report history filename suffixes exhausted")

// Save stores one run report under ~/.stroppy/reports and returns its path.
func Save(run *Run) (string, error) {
	if run == nil {
		return "", errors.New("save nil run report")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}

	dir := filepath.Join(home, ".stroppy", "reports")
	if err := os.MkdirAll(dir, historyDirPerm); err != nil {
		return "", fmt.Errorf("create report history: %w", err)
	}

	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal report history: %w", err)
	}

	name := run.StartedAt.UTC().Format("2006-01-02T15-04-05Z") + "-" + safeName(run.Workload)

	return writeHistoryFile(dir, name, append(data, '\n'))
}

func writeHistoryFile(dir, name string, data []byte) (string, error) {
	for suffix := 1; suffix <= maxNameAttempts; suffix++ {
		fileName := name + ".json"
		if suffix > 1 {
			fileName = fmt.Sprintf("%s-%d.json", name, suffix)
		}

		path := filepath.Join(dir, fileName)
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, historyFilePerm)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("create report history file: %w", err)
		}

		_, writeErr := file.Write(data)
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			removeErr := os.Remove(path)

			return "", fmt.Errorf("write report history: %w", errors.Join(writeErr, closeErr, removeErr))
		}

		return path, nil
	}

	return "", fmt.Errorf("%w: %s", errHistoryNameExhausted, name)
}

func safeName(name string) string {
	var builder strings.Builder
	for _, value := range strings.ToLower(name) {
		switch {
		case value >= 'a' && value <= 'z', value >= '0' && value <= '9', value == '-', value == '_':
			builder.WriteRune(value)
		default:
			builder.WriteByte('-')
		}
	}

	name = strings.Trim(builder.String(), "-")
	if name == "" {
		return "workload"
	}

	return name
}
