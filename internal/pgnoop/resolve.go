package pgnoop

import (
	"errors"
	"fmt"
	"os"
)

// Consent controls runtime download of the server binary.
type Consent int

const (
	// ConsentAsk prompts on an interactive terminal and refuses elsewhere.
	ConsentAsk Consent = iota
	// ConsentAlways downloads without prompting (CI, scripts).
	ConsentAlways
	// ConsentNever refuses downloads; only embedded/cached/explicit binaries resolve.
	ConsentNever
)

// ErrNoServerBinary reports that no binary source is available and what to do.
var ErrNoServerBinary = errors.New("pgnoop server binary unavailable")

// Options steers Resolve.
type Options struct {
	// Path short-circuits resolution to an explicit binary (flag or env).
	Path string
	// Consent gates the download fallback.
	Consent Consent
	// Prompt receives the interactive download question and the user's answer
	// source. Defaults to stdin/stderr when nil.
	Prompt func(question string) bool
	// Log receives progress lines. Optional.
	Log func(format string, args ...any)
}

// Resolve locates a ready-to-run pg-noop binary: explicit path, embedded
// copy, local cache, or a verified release download.
func Resolve(opts Options) (string, error) {
	if opts.Path != "" {
		if err := checkBinary(opts.Path); err != nil {
			return "", fmt.Errorf("pgnoop: explicit server path: %w", err)
		}

		return opts.Path, nil
	}

	cachePath, err := CachePath()
	if err != nil {
		return "", err
	}

	if embedded := EmbeddedBinary(); len(embedded) > 0 {
		if err := materialize(cachePath, embedded); err != nil {
			return "", err
		}

		return cachePath, nil
	}

	if err := checkBinary(cachePath); err == nil {
		return cachePath, nil
	}

	return download(cachePath, opts)
}

func checkBinary(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s is not a regular file", ErrNoServerBinary, path)
	}

	if info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("%w: %s is not executable", ErrNoServerBinary, path)
	}

	return nil
}

// materialize writes data to path when missing, making it executable.
func materialize(path string, data []byte) error {
	if err := checkBinary(path); err == nil {
		return nil
	}

	return writeBinary(path, data)
}

func logf(opts Options, format string, args ...any) {
	if opts.Log != nil {
		opts.Log(format, args...)
	}
}

func prompt(opts Options, question string) bool {
	if opts.Prompt != nil {
		return opts.Prompt(question)
	}

	return askTerminal(question)
}
