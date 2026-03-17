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

const executeSQLPreset = "execute_sql"

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
	script, err := resolveFile(executeSQLPreset, "execute_sql.ts", true)
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
	script, err := resolveFile(executeSQLPreset, "execute_sql.ts", true)
	if err != nil {
		return nil, fmt.Errorf("built-in execute_sql script not found: %w", err)
	}

	// Resolve the SQL file through search path.
	sqlFileName := filepath.Base(sqlArg)
	presetName := strings.TrimSuffix(sqlFileName, ".sql")

	var sql *ResolvedFile

	sql, err = resolveFile(presetName, sqlFileName, true)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrSQLNotFound, sqlArg)
	}

	return &ResolvedInput{Script: *script, SQL: sql}, nil
}

// resolveScriptMode handles the standard script resolution path.
func resolveScriptMode(scriptArg, sqlArg string) (*ResolvedInput, error) {
	presetName, scriptFileName := deriveNames(scriptArg, ".ts")

	var (
		script *ResolvedFile
		sql    *ResolvedFile
		err    error
	)

	if script, err = resolveFile(presetName, scriptFileName, true); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrScriptNotFound, scriptArg)
	}

	// Resolve SQL.
	if sqlArg == "" { // Perhaps the test does not need the .sql file
		// Auto-derive SQL from script name.
		candidateSQL := presetName + ".sql"
		sql, _ = resolveFile(presetName, candidateSQL, false)

		return &ResolvedInput{Script: *script, SQL: sql}, nil
	}

	// Explicit SQL arg — resolve through search path.
	_, sqlFileName := deriveNames(sqlArg, ".sql")

	if sql, err = resolveFile(presetName, sqlFileName, true); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrSQLNotFound, sqlArg)
	}

	return &ResolvedInput{Script: *script, SQL: sql}, nil
}

// resolveFile searches for a file through the default search path:
// cwd → ~/.stroppy/ → embedded workloads.
func resolveFile(presetName, fileName string, required bool) (*ResolvedFile, error) {
	return resolveFileInDirs(searchPathDirs(), presetName, fileName, required)
}

// resolveFileInDirs searches for a file in the given directories, then embedded workloads.
func resolveFileInDirs(
	dirs []string,
	presetName, fileName string,
	required bool,
) (*ResolvedFile, error) {
	// Search filesystem directories.
	for i, dir := range dirs {
		candidate := filepath.Join(dir, fileName)
		if _, err := os.Stat(candidate); err == nil {
			abs, _ := filepath.Abs(candidate)

			source := SourceCwd
			if i > 0 {
				source = SourceUserDir
			}

			return &ResolvedFile{
				Name:   fileName,
				Path:   abs,
				Source: source,
			}, nil
		}
	}

	// Search embedded workloads.
	if data, err := workloads.ReadPresetFile(presetName, fileName); err == nil {
		return &ResolvedFile{
			Name:    fileName,
			Content: data,
			Source:  SourceEmbedded,
		}, nil
	}

	if required {
		return nil, fmt.Errorf("%w: %s", ErrFileNotFound, fileName)
	}

	return nil, nil //nolint:nilnil // nil,nil signals "not found, not required"
}

// searchPathDirs returns filesystem directories to search, in order.
func searchPathDirs() []string {
	dirs := []string{"."}

	if home, err := os.UserHomeDir(); err == nil {
		stroppyDir := filepath.Join(home, ".stroppy")
		if info, err := os.Stat(stroppyDir); err == nil && info.IsDir() {
			dirs = append(dirs, stroppyDir)
		}
	}

	return dirs
}

// deriveNames extracts the preset name and script filename from an argument.
// "tpcc" → ("tpcc", "tpcc.ts")
// "tpcc.ts" → ("tpcc", "tpcc.ts")
// "./foo/tpcc.ts" → ("tpcc", "tpcc.ts").
func deriveNames(arg, suffix string) (presetName, scriptFileName string) {
	base := filepath.Base(arg)

	presetName = strings.TrimSuffix(base, suffix)
	scriptFileName = presetName + suffix

	return presetName, scriptFileName
}
