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
	ErrNotFound       = errors.New("workload catalog entry not found")
	ErrAlreadyExists  = errors.New("workload catalog entry already exists")
	ErrInvalidEntry   = errors.New("invalid workload catalog entry")
	errOutsideCatalog = errors.New("path resolves outside catalog root")
)

// Entry describes one active custom workload artifact.
type Entry struct {
	Schema       int       `json:"schema"`
	Name         string    `json:"name"`
	Source       string    `json:"source"`
	Package      string    `json:"package,omitempty"`
	ModulePath   string    `json:"module_path,omitempty"`
	ModuleRoot   string    `json:"module_root,omitempty"`
	BuiltAt      time.Time `json:"built_at"`
	ArtifactPath string    `json:"artifact_path"`
	Status       string    `json:"status,omitempty"`
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

// StroppyRoot returns ~/.stroppy for toolchains, caches, and generated builds.
func (store *Store) StroppyRoot() string {
	if store == nil {
		return ""
	}

	return filepath.Dir(store.root)
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

		if err := store.validateArtifactPath(entry.ArtifactPath); err != nil {
			entry.Status = "broken"
		} else if err := store.validateArtifact(&entry); err != nil {
			entry.Status = "broken"
		} else {
			entry.Status = "ready"
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

	if err != nil {
		return Entry{}, err
	}

	if err := store.validateArtifact(&entry); err != nil {
		return Entry{}, err
	}

	entry.Status = "ready"

	return entry, nil
}

// Publish atomically activates artifact for entry.Name. Source artifact remains caller-owned.
func (store *Store) Publish(entry *Entry, sourceArtifact string, replace bool) (publishedEntry Entry, returnErr error) {
	if entry == nil {
		return Entry{}, ErrInvalidEntry
	}

	if err := validateName(entry.Name); err != nil {
		return Entry{}, err
	}

	if err := store.ensureDirs(); err != nil {
		return Entry{}, err
	}

	unlock, err := store.lockExclusive()
	if err != nil {
		return Entry{}, err
	}
	defer func() { returnErr = errors.Join(returnErr, unlock()) }()

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

	publishedEntry = *entry
	publishedEntry.Schema = SchemaVersion
	publishedEntry.BuiltAt = publishedEntry.BuiltAt.UTC()
	publishedEntry.Status = "ready"

	if publishedEntry.BuiltAt.IsZero() {
		publishedEntry.BuiltAt = time.Now().UTC()
	}

	publishedEntry.ArtifactPath = artifactPath

	data, err := json.MarshalIndent(publishedEntry, "", "  ")
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

	return publishedEntry, nil
}

// Remove deletes only Stroppy-owned catalog data for name.
func (store *Store) Remove(name string) (returnErr error) {
	manifestPath, err := store.manifestPath(name)
	if err != nil {
		return err
	}

	if err := store.ensureDirs(); err != nil {
		return err
	}

	unlock, err := store.lockExclusive()
	if err != nil {
		return err
	}
	defer func() { returnErr = errors.Join(returnErr, unlock()) }()

	entry, err := store.readManifest(manifestPath)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}

	if err != nil {
		return err
	}

	if err := os.Remove(manifestPath); err != nil {
		return fmt.Errorf("remove catalog entry: %w", err)
	}

	if err := store.validateArtifactPath(entry.ArtifactPath); err != nil {
		if errors.Is(err, errOutsideCatalog) || errors.Is(err, fs.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("catalog entry removed; artifact left in place: %w", err)
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

	if err := store.validateArtifactPath(entry.ArtifactPath); err != nil {
		return Entry{}, fmt.Errorf("%w %q: unexpected artifact path: %w", ErrInvalidEntry, path, err)
	}

	return entry, nil
}

func (store *Store) validateArtifact(entry *Entry) error {
	if entry == nil {
		return ErrInvalidEntry
	}

	info, err := os.Stat(entry.ArtifactPath)
	if err != nil {
		return fmt.Errorf("%w: artifact: %w", ErrInvalidEntry, err)
	}

	if !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return fmt.Errorf("%w: artifact is not executable", ErrInvalidEntry)
	}

	return nil
}

func (store *Store) ensureDirs() error {
	for _, path := range []string{store.root, store.entriesDir(), store.artifactsDir()} {
		if err := os.MkdirAll(path, dirPerm); err != nil {
			return fmt.Errorf("create workload catalog: %w", err)
		}
	}

	if err := store.validateOwnedPath(store.entriesDir()); err != nil {
		return fmt.Errorf("validate workload entries directory: %w", err)
	}

	if err := store.validateOwnedPath(store.artifactsDir()); err != nil {
		return fmt.Errorf("validate workload artifacts directory: %w", err)
	}

	return nil
}

func (store *Store) validateArtifactPath(path string) error {
	if err := store.validateOwnedPath(path); err != nil {
		return err
	}

	artifactDir, err := filepath.EvalSymlinks(store.artifactsDir())
	if err != nil {
		return err
	}

	resolved, err := evalExistingPath(path)
	if err != nil {
		return err
	}

	relative, err := filepath.Rel(artifactDir, resolved)
	if err != nil || filepath.IsAbs(relative) || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return errOutsideCatalog
	}

	return nil
}

func (store *Store) validateOwnedPath(path string) error {
	root, err := filepath.EvalSymlinks(store.root)
	if err != nil {
		return err
	}

	resolved, err := evalExistingPath(path)
	if err != nil {
		return err
	}

	relative, err := filepath.Rel(root, resolved)
	if err != nil || filepath.IsAbs(relative) || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return errOutsideCatalog
	}

	return nil
}

func evalExistingPath(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}

	if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}

	parent, parentErr := filepath.EvalSymlinks(filepath.Dir(path))
	if parentErr != nil {
		return "", parentErr
	}

	return filepath.Join(parent, filepath.Base(path)), nil
}

func (store *Store) manifestPath(name string) (string, error) {
	if err := validateName(name); err != nil {
		return "", err
	}

	return filepath.Join(store.entriesDir(), entryID(name)+".json"), nil
}

func (store *Store) entriesDir() string   { return filepath.Join(store.root, entriesSubDir) }
func (store *Store) artifactsDir() string { return filepath.Join(store.root, artifactsSubDir) }
