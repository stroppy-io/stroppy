package report

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

const maxHistoryMetadataBytes = 32 << 20

var (
	ErrInvalidHistoryID = errors.New("invalid report history ID")
	errHistorySymlink   = errors.New("report history must not be a symlink")
	errHistoryFile      = errors.New("report history entry must be a regular file")
	errHistoryEnvelope  = errors.New("invalid run report envelope")
	errHistoryQuery     = errors.New("invalid report history query")
	errHistoryTooLarge  = errors.New("report exceeds 32 MiB metadata inspection limit; use get for full retrieval")
	errHistoryChanged   = errors.New("report changed after selection")
)

// HistoryEntry is common metadata, not a workload-specific result projection.
type HistoryEntry struct {
	ID                string    `json:"id"`
	ReportID          string    `json:"report_id"`
	RunID             string    `json:"run_id,omitempty"`
	Schema            int       `json:"schema"`
	StartedAt         time.Time `json:"started_at"`
	FinishedAt        time.Time `json:"finished_at"`
	Workload          string    `json:"workload"`
	Driver            string    `json:"driver"`
	Drivers           []Driver  `json:"drivers"`
	Status            Status    `json:"status"`
	TPS               *float64  `json:"tps,omitempty"`
	SizeBytes         int64     `json:"size_bytes"`
	identity          fs.FileInfo
	directoryIdentity fs.FileInfo
}

// HistoryProblem identifies an unreadable entry without hiding healthy entries.
type HistoryProblem struct {
	ID    string `json:"id"`
	Error string `json:"error"`
}

// HistoryStatus describes local storage, including entries that need attention.
type HistoryStatus struct {
	Directory  string           `json:"directory"`
	Exists     bool             `json:"exists"`
	Reports    int              `json:"reports"`
	Unreadable int              `json:"unreadable"`
	SizeBytes  int64            `json:"size_bytes"`
	Problems   []HistoryProblem `json:"problems"`
}

// HistoryQuery applies OR within each field and AND across fields. Since is
// inclusive, Until is exclusive, and Limit zero returns all matching entries.
type HistoryQuery struct {
	Workloads []string
	Drivers   []string
	Statuses  []Status
	Since     time.Time
	Until     time.Time
	OlderThan time.Time
	Reverse   bool
	Limit     int
}

// List orders matching entries by start time, then history ID.
//
//nolint:gocritic // query is a caller-owned value, including slices and time boundaries.
func (h *History) List(query HistoryQuery) ([]HistoryEntry, []HistoryProblem, error) {
	if query.Limit < 0 || (!query.Since.IsZero() && !query.Until.IsZero() && !query.Since.Before(query.Until)) {
		return nil, nil, errHistoryQuery
	}

	entries, status, err := h.scan()
	if err != nil {
		return nil, nil, err
	}

	entries = slices.DeleteFunc(entries, func(entry HistoryEntry) bool { return !query.matches(&entry) })
	slices.SortFunc(entries, func(a, b HistoryEntry) int {
		order := a.StartedAt.Compare(b.StartedAt)
		if order == 0 {
			order = strings.Compare(a.ID, b.ID)
		}

		if !query.Reverse {
			order = -order
		}

		return order
	})

	if query.Limit > 0 && len(entries) > query.Limit {
		entries = entries[:query.Limit]
	}

	return entries, status.Problems, nil
}

func (q *HistoryQuery) matches(entry *HistoryEntry) bool {
	if len(q.Workloads) > 0 && !slices.Contains(q.Workloads, entry.Workload) {
		return false
	}

	if len(q.Statuses) > 0 && !slices.Contains(q.Statuses, entry.Status) {
		return false
	}

	if !q.Since.IsZero() && entry.StartedAt.Before(q.Since) {
		return false
	}

	if !q.Until.IsZero() && !entry.StartedAt.Before(q.Until) {
		return false
	}

	if !q.OlderThan.IsZero() && !entry.StartedAt.Before(q.OlderThan) {
		return false
	}

	if len(q.Drivers) == 0 || slices.Contains(q.Drivers, entry.Driver) {
		return true
	}

	for _, driver := range entry.Drivers {
		if slices.Contains(q.Drivers, driver.Type) {
			return true
		}
	}

	return false
}

// Status inspects storage without creating the directory.
func (h *History) Status() (HistoryStatus, error) {
	_, status, err := h.scan()

	return status, err
}

func (h *History) scan() ([]HistoryEntry, HistoryStatus, error) {
	status := HistoryStatus{Directory: h.Directory, Problems: []HistoryProblem{}}
	entries := []HistoryEntry{}

	root, err := openHistory(h.Directory)
	if errors.Is(err, fs.ErrNotExist) {
		return entries, status, nil
	}

	if err != nil {
		return nil, status, err
	}

	defer root.Close()

	status.Exists = true

	directoryInfo, err := root.Stat(".")
	if err != nil {
		return nil, status, err
	}

	files, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return nil, status, fmt.Errorf("list report history: %w", err)
	}

	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		id := strings.TrimSuffix(file.Name(), ".json")

		data, info, readErr := readHistoryFile(root, id, maxHistoryMetadataBytes)
		if info != nil && info.Mode().IsRegular() {
			status.SizeBytes += info.Size()
		}

		var entry HistoryEntry
		if readErr == nil {
			entry, readErr = historyMetadata(id, data)
		}

		if readErr != nil {
			status.Problems = append(status.Problems, HistoryProblem{ID: id, Error: readErr.Error()})

			continue
		}

		entry.SizeBytes = info.Size()
		entry.identity = info
		entry.directoryIdentity = directoryInfo
		entries = append(entries, entry)
	}

	status.Reports = len(entries)
	status.Unreadable = len(status.Problems)

	return entries, status, nil
}

// Get returns the original JSON, including unknown fields and workload payloads.
func (h *History) Get(id string) ([]byte, error) {
	if err := validateHistoryID(id); err != nil {
		return nil, err
	}

	root, err := openHistory(h.Directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	data, _, err := readHistoryFile(root, id, 0)
	if err != nil {
		return nil, err
	}

	if _, err := historyMetadata(id, data); err != nil {
		return nil, err
	}

	return data, nil
}

// CheckRemoval validates every explicit target before a batch removal starts.
func (h *History) CheckRemoval(ids []string) error {
	_, err := h.SelectRemoval(ids)

	return err
}

// Remove deletes only regular files with managed history names. Corrupt reports
// can be removed explicitly without decoding their payload.
func (h *History) Remove(id string) error {
	if err := validateHistoryID(id); err != nil {
		return err
	}

	root, err := openHistory(h.Directory)
	if err != nil {
		return err
	}
	defer root.Close()

	info, err := root.Lstat(id + ".json")
	if err != nil {
		return err
	}

	if !info.Mode().IsRegular() {
		return errHistoryFile
	}

	return removeHistoryEntry(root, &HistoryEntry{ID: id, identity: info})
}

// SelectRemoval snapshots explicit targets, including corrupt regular reports.
func (h *History) SelectRemoval(ids []string) ([]HistoryEntry, error) {
	entries := make([]HistoryEntry, 0, len(ids))
	if len(ids) == 0 {
		return entries, nil
	}

	for _, id := range ids {
		if err := validateHistoryID(id); err != nil {
			return nil, err
		}
	}

	root, err := openHistory(h.Directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	directory, err := root.Stat(".")
	if err != nil {
		return nil, err
	}

	for _, id := range ids {
		info, err := root.Lstat(id + ".json")
		if err != nil || !info.Mode().IsRegular() {
			return nil, errors.Join(errHistoryFile, err)
		}

		entries = append(entries, HistoryEntry{ID: id, identity: info, directoryIdentity: directory})
	}

	return entries, nil
}

// RemoveSelected removes only unchanged files from the selected history root.
func (h *History) RemoveSelected(entries []HistoryEntry) ([]string, error) {
	removed := []string{}
	if len(entries) == 0 {
		return removed, nil
	}

	root, err := openHistory(h.Directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	directory, err := root.Stat(".")
	if err != nil {
		return nil, err
	}

	for i := range entries {
		entry := &entries[i]
		if entry.directoryIdentity == nil || entry.identity == nil ||
			!os.SameFile(entry.directoryIdentity, directory) {
			return nil, errHistoryChanged
		}

		current, err := root.Lstat(entry.ID + ".json")
		if err != nil || !sameHistoryFile(entry.identity, current) {
			return nil, errors.Join(errHistoryChanged, err)
		}
	}

	var removalErr error

	for i := range entries {
		if err := removeHistoryEntry(root, &entries[i]); err != nil {
			removalErr = errors.Join(removalErr, fmt.Errorf("remove report %q: %w", entries[i].ID, err))
		} else {
			removed = append(removed, entries[i].ID)
		}
	}

	return removed, removalErr
}

func sameHistoryFile(expected, current fs.FileInfo) bool {
	return current != nil && current.Mode().IsRegular() && os.SameFile(expected, current) &&
		expected.Size() == current.Size() && expected.ModTime().Equal(current.ModTime())
}

func removeHistoryEntry(root *os.Root, entry *HistoryEntry) error {
	if err := validateHistoryID(entry.ID); err != nil {
		return err
	}

	name := entry.ID + ".json"

	quarantine := ".remove-" + uuid.NewString()
	if err := root.Mkdir(quarantine, historyDirPerm); err != nil {
		return err
	}

	staged := quarantine + "/report.json"
	if err := root.Rename(name, staged); err != nil {
		return errors.Join(err, root.Remove(quarantine))
	}

	info, err := root.Lstat(staged)
	if err != nil || !sameHistoryFile(entry.identity, info) {
		return fmt.Errorf("%w; replacement retained at %s", errors.Join(errHistoryChanged, err), staged)
	}

	return errors.Join(root.Remove(staged), root.Remove(quarantine))
}

func openHistory(directory string) (*os.Root, error) {
	info, err := os.Lstat(directory)
	if err != nil {
		return nil, err
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return nil, errHistorySymlink
	}

	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}

	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		root.Close()

		return nil, errors.Join(errHistorySymlink, err)
	}

	return root, nil
}

func readHistoryFile(root *os.Root, id string, limit int64) ([]byte, fs.FileInfo, error) {
	if err := validateHistoryID(id); err != nil {
		return nil, nil, err
	}

	info, err := root.Lstat(id + ".json")
	if err != nil {
		return nil, nil, err
	}

	if !info.Mode().IsRegular() {
		return nil, info, errHistoryFile
	}

	if limit > 0 && info.Size() > limit {
		return nil, info, errHistoryTooLarge
	}

	file, err := openHistoryReport(root, id+".json")
	if err != nil {
		return nil, info, err
	}
	defer file.Close()

	opened, err := file.Stat()
	if err != nil {
		return nil, info, err
	}

	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, info, errHistoryFile
	}

	var reader io.Reader = file
	if limit > 0 {
		reader = io.LimitReader(file, limit+1)
	}

	data, err := io.ReadAll(reader)
	if limit > 0 && int64(len(data)) > limit {
		return nil, info, errHistoryTooLarge
	}

	return data, info, err
}

func validateHistoryID(id string) error {
	prefixLength := len(historyTimeFormat)
	if len(id) <= prefixLength+1 || id[prefixLength] != '-' {
		return fmt.Errorf("%w: %q", ErrInvalidHistoryID, id)
	}

	if _, err := time.Parse(historyTimeFormat, id[:prefixLength]); err != nil {
		return fmt.Errorf("%w: %q", ErrInvalidHistoryID, id)
	}

	name := id[prefixLength+1:]
	if safeName(name) != name {
		return fmt.Errorf("%w: %q", ErrInvalidHistoryID, id)
	}

	return nil
}

func historyMetadata(id string, data []byte) (HistoryEntry, error) {
	var envelope struct {
		HistoryEntry
		Kind    string `json:"kind"`
		Metrics struct {
			TPS Metric `json:"tps"`
		} `json:"metrics"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return HistoryEntry{}, fmt.Errorf("decode report metadata: %w", err)
	}

	if envelope.Kind != Kind || envelope.Schema < 1 || envelope.StartedAt.IsZero() ||
		envelope.Workload == "" || envelope.Status == "" {
		return HistoryEntry{}, errHistoryEnvelope
	}

	entry := envelope.HistoryEntry
	entry.ReportID = entry.ID
	entry.ID = id

	entry.TPS = nil
	for _, series := range envelope.Metrics.TPS.Series {
		if step, ok := series.Attributes["step"].(string); ok && step == "workload" {
			entry.TPS = series.Total

			break
		}
	}

	if len(envelope.Metrics.TPS.Series) == 0 {
		entry.TPS = envelope.Metrics.TPS.Total
	}

	return entry, nil
}
