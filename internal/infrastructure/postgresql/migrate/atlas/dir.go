package atlas

import (
	"errors"
	"fmt"
	"io/fs"
	"sort"

	"ariga.io/atlas/sql/migrate"
)

type EmbedDir struct {
	fs.FS
}

func NewEmbedDir(fsys fs.FS) EmbedDir {
	return EmbedDir{fsys}
}

func (e EmbedDir) WriteFile(_ string, _ []byte) error {
	return errors.New("not implemented")
}

func (e EmbedDir) Files() ([]migrate.File, error) {
	names, err := fs.Glob(e, "*.sql")
	if err != nil {
		return nil, err
	}
	// Sort files lexicographically.
	sort.Slice(names, func(i, j int) bool {
		return names[i] < names[j]
	})
	files := make([]migrate.File, 0, len(names))
	for _, n := range names {
		b, err := fs.ReadFile(e, n)
		if err != nil {
			return nil, fmt.Errorf("sql/migrate: read file %q: %w", n, err)
		}
		files = append(files, migrate.NewLocalFile(n, b))
	}
	return files, nil
}

func (e EmbedDir) Checksum() (migrate.HashFile, error) {
	files, err := e.Files()
	if err != nil {
		return nil, err
	}
	return migrate.NewHashFile(files)
}
