package runner

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/stroppy-io/stroppy/workloads"
)

var (
	// ErrScriptNotFound is returned when a script cannot be resolved.
	ErrScriptNotFound = errors.New("script not found in cwd, ~/.stroppy/, or embedded workloads")
	// ErrSQLNotFound is returned when an explicitly requested SQL file cannot be resolved.
	ErrSQLNotFound = errors.New("SQL file not found in cwd, ~/.stroppy/, or embedded workloads")
	// ErrFileNotFound is returned when a file cannot be found in any search path.
	ErrFileNotFound = errors.New("file not found")
)

// FileSource indicates where a resolved file was found.
type FileSource int

const (
	SourceCwd      FileSource = iota // Found in current working directory
	SourceUserDir                    // Found in ~/.stroppy/
	SourceEmbedded                   // Found in embedded workloads
)

func (s FileSource) String() string {
	switch s {
	case SourceCwd:
		return "cwd"
	case SourceUserDir:
		return "~/.stroppy"
	case SourceEmbedded:
		return "embedded"
	default:
		return "unknown"
	}
}

// ResolvedFile represents a file located through the search path.
type ResolvedFile struct {
	Name    string     // basename, e.g. "tpcc.ts"
	Path    string     // filesystem path (empty if embedded)
	Content []byte     // embedded content (nil if filesystem)
	Source  FileSource // where it was found
}

// ResolvedInput holds the fully resolved script and optional SQL file.
type ResolvedInput struct {
	Script ResolvedFile
	SQL    *ResolvedFile // nil if no SQL file needed
}

const executeSQLFile = "execute_sql/execute_sql.ts"

// ResolveInput resolves the first positional argument into a script + optional SQL.
// The extension determines the mode:
//
//	stroppy run tpcc           — no extension → preset (searches cwd → ~/.stroppy/ → embedded)
//	stroppy run bench.ts       — .ts          → test script
//	stroppy run queries.sql    — .sql         → SQL file (wraps with execute_sql)
//	stroppy run "select 1"    — spaces        → inline SQL (wraps with execute_sql)
func ResolveInput(scriptArg, sqlArg string) (*ResolvedInput, error) {
	// Inline SQL: contains spaces → "select 1", "create table foo (...)", etc.
	if strings.Contains(scriptArg, " ") {
		return resolveInlineSQL(scriptArg)
	}

	// SQL file: .sql extension → wrap with execute_sql preset
	if strings.HasSuffix(scriptArg, ".sql") {
		return resolveSQLFileMode(scriptArg)
	}

	// Preset or test script: no extension = preset, .ts = script
	return resolveScriptMode(scriptArg, sqlArg)
}

// resolveInlineSQL handles inline SQL passed directly as the argument.
func resolveInlineSQL(sql string) (*ResolvedInput, error) {
	script, err := resolveFile(executeSQLFile, "", true)
	if err != nil {
		return nil, fmt.Errorf("built-in execute_sql script not found: %w", err)
	}

	// Wrap inline SQL in the parse_sql format.
	content := fmt.Sprintf("--= query\n%s;\n", strings.TrimSuffix(strings.TrimSpace(sql), ";"))

	return &ResolvedInput{
		Script: *script,
		SQL: &ResolvedFile{
			Name:    "inline.sql",
			Content: []byte(content),
			Source:  SourceEmbedded,
		},
	}, nil
}

// resolveSQLFileMode handles when the first arg is a .sql file.
func resolveSQLFileMode(sqlArg string) (*ResolvedInput, error) {
	script, err := resolveFile(executeSQLFile, "", true)
	if err != nil {
		return nil, fmt.Errorf("built-in execute_sql script not found: %w", err)
	}

	preset := inferPreset(sqlArg)

	sql, err := resolveFile(sqlArg, preset, true)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrSQLNotFound, sqlArg)
	}

	return &ResolvedInput{Script: *script, SQL: sql}, nil
}

// resolveScriptMode handles the standard script resolution path.
func resolveScriptMode(scriptArg, sqlArg string) (*ResolvedInput, error) {
	// Preset from script arg first, fall back to sql arg.
	preset := inferPreset(scriptArg)
	if preset == "" && sqlArg != "" {
		preset = inferPreset(sqlArg)
	}

	script, err := resolveFile(ensureSuffix(scriptArg, ".ts"), preset, true)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrScriptNotFound, scriptArg)
	}

	// SQL is optional — some presets don't need it, users may bake SQL into the test.
	sql, err := resolveSQL(sqlArg, preset)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrSQLNotFound, sqlArg)
	}

	return &ResolvedInput{Script: *script, SQL: sql}, nil
}

// resolveSQL resolves the SQL file:
//   - explicit arg → required
//   - no arg + preset → auto-derive (optional)
//   - no arg + no preset → no SQL
func resolveSQL(sqlArg, preset string) (*ResolvedFile, error) {
	switch {
	case sqlArg != "":
		return resolveFile(ensureSuffix(sqlArg, ".sql"), preset, true)
	case preset != "":
		sql, _ := resolveFile(preset+".sql", preset, false)

		return sql, nil
	default:
		return nil, nil //nolint:nilnil // it's ok to not find the sql
	}
}

func ensureSuffix(s, suffix string) string {
	if strings.HasSuffix(s, suffix) {
		return s
	}

	return s + suffix
}

// resolveFile searches for a file through the default search path:
// cwd → ~/.stroppy/ → embedded (direct) → embedded (preset/).
// The preset parameter adds an extra search stage: if non-empty,
// tries preset/fileName in embedded workloads.
func resolveFile(filePath, preset string, required bool) (*ResolvedFile, error) {
	fileName := filepath.Base(filePath)

	// 1. Current working directory.
	if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
		abs, _ := filepath.Abs(filePath)

		return &ResolvedFile{
			Name:   fileName,
			Path:   abs,
			Source: SourceCwd,
		}, nil
	}

	// 2. User config directory: ~/.stroppy/
	if home, err := os.UserHomeDir(); err == nil {
		fromStroppyDir := filepath.Join(home, ".stroppy", filePath)
		if info, err := os.Stat(fromStroppyDir); err == nil && !info.IsDir() {
			abs, _ := filepath.Abs(fromStroppyDir)

			return &ResolvedFile{
				Name:   fileName,
				Path:   abs,
				Source: SourceUserDir,
			}, nil
		}
	}

	// 3. Embedded workloads: direct path match.
	if file, err := workloads.Content.ReadFile(filePath); err == nil {
		return &ResolvedFile{
			Name:    fileName,
			Content: file,
			Source:  SourceEmbedded,
		}, nil
	}

	// 4. Embedded workloads: preset/fileName.
	if preset != "" {
		candidate := filepath.Join(preset, fileName)
		if file, err := workloads.Content.ReadFile(candidate); err == nil {
			return &ResolvedFile{
				Name:    fileName,
				Content: file,
				Source:  SourceEmbedded,
			}, nil
		}
	}

	if required {
		return nil, fmt.Errorf("%w: %s", ErrFileNotFound, filePath)
	}

	return nil, nil //nolint:nilnil // nil,nil signals "not found, not required"
}

// inferPreset determines the preset name from a user argument.
// Bare names and preset-relative paths yield a preset; explicit
// relative (./) or absolute (/) paths yield no preset.
func inferPreset(arg string) string {
	// Explicit relative or absolute path → no preset context.
	if strings.HasPrefix(arg, "./") || strings.HasPrefix(arg, "/") {
		return ""
	}

	// Has directory component → use it: "tpcc/pick.ts" → "tpcc".
	if dir := filepath.Dir(arg); dir != "." {
		return dir
	}

	// Bare name: "tpcc" → "tpcc", "tpcc.ts" → "tpcc".
	base := filepath.Base(arg)

	return strings.TrimSuffix(strings.TrimSuffix(base, ".ts"), ".sql")
}
