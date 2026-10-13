package report

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

const (
	historyDirPerm    = 0o700
	historyFilePerm   = 0o600
	maxNameAttempts   = 1000
	historyTimeFormat = "2006-01-02T15-04-05Z"
)

var (
	errHistoryNameExhausted = errors.New("report history filename suffixes exhausted")
	errNilRunReport         = errors.New("nil run report")
)

// History manages run reports in one directory. Filenames identify stored reports
// independently of the report's potentially user-supplied ID.
type History struct {
	Directory string
}

// DefaultHistory resolves the run history without creating it.
func DefaultHistory() (*History, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}

	return &History{Directory: filepath.Join(home, ".stroppy", "reports")}, nil
}

// Save stores one run report under ~/.stroppy/reports and returns its path.
func Save(run *Run) (string, error) {
	if run == nil {
		return "", errNilRunReport
	}

	history, err := DefaultHistory()
	if err != nil {
		return "", err
	}

	return history.Save(run)
}

// Save publishes a complete report without replacing an existing entry.
func (h *History) Save(run *Run) (string, error) {
	if run == nil {
		return "", errNilRunReport
	}

	if err := os.MkdirAll(h.Directory, historyDirPerm); err != nil {
		return "", fmt.Errorf("create report history: %w", err)
	}

	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal report history: %w", err)
	}

	name := run.StartedAt.UTC().Format(historyTimeFormat) + "-" + safeName(run.Workload)

	return writeHistoryFile(h.Directory, name, append(data, '\n'))
}

func writeHistoryFile(dir, name string, data []byte) (path string, err error) {
	root, err := openHistory(dir)
	if err != nil {
		return "", err
	}
	defer root.Close()

	temporary := ".report-" + uuid.NewString() + ".tmp"

	file, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, historyFilePerm)
	if err != nil {
		return "", fmt.Errorf("create report history file: %w", err)
	}
	defer func() { err = errors.Join(err, root.Remove(temporary)) }()

	_, writeErr := file.Write(data)
	if writeErr == nil {
		writeErr = file.Sync()
	}

	if err := errors.Join(writeErr, file.Close()); err != nil {
		return "", fmt.Errorf("write report history: %w", err)
	}

	for suffix := 1; suffix <= maxNameAttempts; suffix++ {
		fileName := name + ".json"
		if suffix > 1 {
			fileName = fmt.Sprintf("%s-%d.json", name, suffix)
		}

		if err := root.Link(temporary, fileName); errors.Is(err, fs.ErrExist) {
			continue
		} else if err != nil {
			return "", fmt.Errorf("publish report history: %w", err)
		}

		return filepath.Join(dir, fileName), nil
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
