// Package workloadcatalog stores compiled custom workloads owned by Stroppy.
package workloadcatalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	SchemaVersion   = 1
	catalogSubDir   = "workloads"
	entriesSubDir   = "entries"
	artifactsSubDir = "artifacts"
	dirPerm         = 0o700
	filePerm        = 0o600
	executablePerm  = 0o700
)

var (
	ErrNotFound      = errors.New("workload catalog entry not found")
	ErrAlreadyExists = errors.New("workload catalog entry already exists")
	ErrInvalidEntry  = errors.New("invalid workload catalog entry")
)

// Entry describes one active custom workload artifact.
type Entry struct {
	Schema       int       `json:"schema"`
	Name         string    `json:"name"`
	Source       string    `json:"source"`
	BuiltAt      time.Time `json:"built_at"`
	ArtifactPath string    `json:"artifact_path"`
}

// Store owns one local custom-workload catalog.
type Store struct {
	root string
}

// Open resolves the current user's Stroppy workload catalog.
func Open() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}

	return OpenAt(filepath.Join(home, ".stroppy", catalogSubDir))
}

// OpenAt creates a Store rooted at path without creating files yet.
func OpenAt(path string) (*Store, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve catalog path: %w", err)
	}

	return &Store{root: absolute}, nil
}

// Root returns Stroppy-owned catalog root.
func (store *Store) Root() string {
	if store == nil {
		return ""
	}

	return store.root
}

// List returns valid catalog entries ordered by workload name.
func (store *Store) List() ([]Entry, error) {
	entriesDir := store.entriesDir()
	files, err := os.ReadDir(entriesDir)
	if errors.Is(err, fs.ErrNotExist) {
		return []Entry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read workload catalog: %w", err)
	}

	entries := make([]Entry, 0, len(files))
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		entry, err := store.readManifest(filepath.Join(entriesDir, file.Name()))
		if err != nil {
			return nil, err
		}

		entries = append(entries, entry)
	}

	sort.Slice(entries, func(left, right int) bool { return entries[left].Name < entries[right].Name })

	return entries, nil
}

// Get returns one catalog entry by workload name.
func (store *Store) Get(name string) (Entry, error) {
	path, err := store.manifestPath(name)
	if err != nil {
		return Entry{}, err
	}

	entry, err := store.readManifest(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Entry{}, fmt.Errorf("%w: %s", ErrNotFound, name)
	}

	return entry, err
}

// Publish atomically activates artifact for entry.Name. Source artifact remains caller-owned.
func (store *Store) Publish(entry Entry, sourceArtifact string, replace bool) (Entry, error) {
	if err := validateName(entry.Name); err != nil {
		return Entry{}, err
	}

	if err := store.ensureDirs(); err != nil {
		return Entry{}, err
	}

	manifestPath, _ := store.manifestPath(entry.Name)

	var previous *Entry
	if _, err := os.Stat(manifestPath); err == nil {
		if !replace {
			return Entry{}, fmt.Errorf("%w: %s", ErrAlreadyExists, entry.Name)
		}

		existing, readErr := store.readManifest(manifestPath)
		if readErr != nil {
			return Entry{}, readErr
		}

		previous = &existing
	} else if !errors.Is(err, fs.ErrNotExist) {
		return Entry{}, fmt.Errorf("inspect catalog entry %q: %w", entry.Name, err)
	}

	artifactPath, err := copyStaged(sourceArtifact, store.artifactsDir(), entryID(entry.Name)+"-", executablePerm)
	if err != nil {
		return Entry{}, fmt.Errorf("publish workload artifact: %w", err)
	}

	published := false
	defer func() {
		if !published {
			_ = os.Remove(artifactPath)
		}
	}()

	entry.Schema = SchemaVersion
	entry.BuiltAt = entry.BuiltAt.UTC()
	if entry.BuiltAt.IsZero() {
		entry.BuiltAt = time.Now().UTC()
	}
	entry.ArtifactPath = artifactPath

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return Entry{}, fmt.Errorf("marshal catalog entry: %w", err)
	}

	if err := writeAtomic(manifestPath, append(data, '\n'), filePerm); err != nil {
		return Entry{}, fmt.Errorf("publish catalog entry: %w", err)
	}

	published = true
	if previous != nil && previous.ArtifactPath != artifactPath {
		_ = os.Remove(previous.ArtifactPath)
	}

	return entry, nil
}

// Remove deletes only Stroppy-owned catalog data for name.
func (store *Store) Remove(name string) error {
	entry, err := store.Get(name)
	if err != nil {
		return err
	}

	manifestPath, _ := store.manifestPath(name)
	if err := os.Remove(manifestPath); err != nil {
		return fmt.Errorf("remove catalog entry: %w", err)
	}

	if err := os.Remove(entry.ArtifactPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove workload artifact: %w", err)
	}

	return nil
}

func (store *Store) readManifest(path string) (Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Entry{}, err
	}

	var entry Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		return Entry{}, fmt.Errorf("%w %q: %w", ErrInvalidEntry, path, err)
	}
	if entry.Schema != SchemaVersion {
		return Entry{}, fmt.Errorf("%w %q: schema %d", ErrInvalidEntry, path, entry.Schema)
	}
	if err := validateName(entry.Name); err != nil {
		return Entry{}, fmt.Errorf("%w %q: %w", ErrInvalidEntry, path, err)
	}

	artifactDir := store.artifactsDir() + string(os.PathSeparator)
	if !strings.HasPrefix(entry.ArtifactPath, artifactDir) ||
		!strings.HasPrefix(filepath.Base(entry.ArtifactPath), entryID(entry.Name)+"-") {
		return Entry{}, fmt.Errorf("%w %q: unexpected artifact path", ErrInvalidEntry, path)
	}

	info, err := os.Stat(entry.ArtifactPath)
	if err != nil {
		return Entry{}, fmt.Errorf("%w %q: artifact: %v", ErrInvalidEntry, path, err)
	}
	if !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return Entry{}, fmt.Errorf("%w %q: artifact is not executable", ErrInvalidEntry, path)
	}

	return entry, nil
}

func (store *Store) ensureDirs() error {
	for _, path := range []string{store.root, store.entriesDir(), store.artifactsDir()} {
		if err := os.MkdirAll(path, dirPerm); err != nil {
			return fmt.Errorf("create workload catalog: %w", err)
		}
	}

	return nil
}

func (store *Store) manifestPath(name string) (string, error) {
	if err := validateName(name); err != nil {
		return "", err
	}

	return filepath.Join(store.entriesDir(), entryID(name)+".json"), nil
}

func (store *Store) entriesDir() string   { return filepath.Join(store.root, entriesSubDir) }
func (store *Store) artifactsDir() string { return filepath.Join(store.root, artifactsSubDir) }
