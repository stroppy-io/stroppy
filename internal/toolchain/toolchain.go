// Package toolchain resolves system or Stroppy-owned Go compilers.
package toolchain

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

const (
	PinnedVersion = "1.26.8"
	minimumMajor  = 1
	minimumMinor  = 26
)

var (
	ErrConsentRequired = errors.New("private Go toolchain download requires consent")
	ErrUnsupportedHost = errors.New("private Go toolchain is unavailable for this host")
	goVersionPattern   = regexp.MustCompile(`go([0-9]+)\.([0-9]+)(?:\.([0-9]+))?`)
)

// Consent controls private toolchain acquisition.
type Consent uint8

const (
	ConsentAsk Consent = iota
	ConsentAlways
	ConsentNever
)

// Options configures compiler resolution.
type Options struct {
	Root    string
	Consent Consent
	Input   io.Reader
	Output  io.Writer
	Offline bool
}

// Compiler is one resolved Go executable and isolated build environment.
type Compiler struct {
	Path    string
	Version string
	Private bool
	Root    string
}

// Resolve prefers a compatible system Go and falls back to a private pinned toolchain.
func Resolve(ctx context.Context, options Options) (*Compiler, error) {
	root, err := resolveRoot(options.Root)
	if err != nil {
		return nil, err
	}

	if system, ok := compatibleSystemGo(ctx, root); ok {
		return system, nil
	}

	private := privateCompiler(root)
	if privateAvailable(ctx, private) {
		return private, nil
	}
	if options.Offline || options.Consent == ConsentNever {
		return nil, ErrConsentRequired
	}

	if options.Consent == ConsentAsk {
		allowed, err := prompt(options.Input, options.Output)
		if err != nil {
			return nil, err
		}
		if !allowed {
			return nil, ErrConsentRequired
		}
	}

	if err := install(ctx, root, options.Output); err != nil {
		return nil, err
	}
	if !privateAvailable(ctx, private) {
		return nil, errors.New("installed Go toolchain failed validation")
	}

	return private, nil
}

// Env returns isolated Go environment while preserving process credentials and proxy policy.
func (compiler *Compiler) Env(targetOS, targetArch string, offline bool) []string {
	values := append([]string(nil), os.Environ()...)
	privateRoot := filepath.Join(compiler.Root, "go")
	values = setEnv(values, "GOENV", "off")
	values = setEnv(values, "GOTOOLCHAIN", "local")
	values = setEnv(values, "GOWORK", "off")
	values = setEnv(values, "GOPATH", filepath.Join(privateRoot, "gopath"))
	values = setEnv(values, "GOMODCACHE", filepath.Join(privateRoot, "modcache"))
	values = setEnv(values, "GOFLAGS", "-modcacherw")
	values = setEnv(values, "GOCACHE", filepath.Join(privateRoot, "buildcache"))
	values = setEnv(values, "GOTMPDIR", filepath.Join(privateRoot, "tmp"))
	values = setEnv(values, "CGO_ENABLED", "0")
	if targetOS != "" {
		values = setEnv(values, "GOOS", targetOS)
	}
	if targetArch != "" {
		values = setEnv(values, "GOARCH", targetArch)
	}
	if offline {
		values = setEnv(values, "GOPROXY", "off")
	}

	for _, path := range []string{
		filepath.Join(privateRoot, "gopath"),
		filepath.Join(privateRoot, "modcache"),
		filepath.Join(privateRoot, "buildcache"),
		filepath.Join(privateRoot, "tmp"),
	} {
		_ = os.MkdirAll(path, 0o700)
	}

	return values
}

func compatibleSystemGo(ctx context.Context, root string) (*Compiler, bool) {
	path, err := exec.LookPath("go")
	if err != nil {
		return nil, false
	}

	version, err := compilerVersion(ctx, path)
	if err != nil || !compatible(version) {
		return nil, false
	}

	return &Compiler{Path: path, Version: version, Root: root}, true
}

func compilerVersion(ctx context.Context, path string) (string, error) {
	command := exec.CommandContext(ctx, path, "version")
	command.Env = append(os.Environ(), "GOTOOLCHAIN=local", "GOENV=off")
	output, err := command.Output()
	if err != nil {
		return "", err
	}

	match := goVersionPattern.FindStringSubmatch(string(output))
	if len(match) == 0 {
		return "", fmt.Errorf("unrecognized Go version: %s", strings.TrimSpace(string(output)))
	}

	return strings.TrimPrefix(match[0], "go"), nil
}

func compatible(version string) bool {
	match := goVersionPattern.FindStringSubmatch("go" + version)
	if len(match) == 0 {
		return false
	}

	major, _ := strconv.Atoi(match[1])
	minor, _ := strconv.Atoi(match[2])

	return major > minimumMajor || major == minimumMajor && minor >= minimumMinor
}

func privateCompiler(root string) *Compiler {
	name := "go"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}

	return &Compiler{
		Path:    filepath.Join(root, "toolchains", "go"+PinnedVersion, "go", "bin", name),
		Version: PinnedVersion,
		Private: true,
		Root:    root,
	}
}

func privateAvailable(ctx context.Context, compiler *Compiler) bool {
	version, err := compilerVersion(ctx, compiler.Path)

	return err == nil && version == compiler.Version
}

func resolveRoot(root string) (string, error) {
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, ".stroppy")
	}

	return filepath.Abs(root)
}

func prompt(input io.Reader, output io.Writer) (bool, error) {
	if input == nil || output == nil {
		return false, ErrConsentRequired
	}

	if _, err := fmt.Fprintf(output, "Download verified Go %s into ~/.stroppy? [y/N] ", PinnedVersion); err != nil {
		return false, err
	}

	line, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}

	line = strings.TrimSpace(strings.ToLower(line))

	return line == "y" || line == "yes", nil
}

func setEnv(values []string, key, value string) []string {
	prefix := key + "="
	for index := range values {
		if strings.HasPrefix(values[index], prefix) {
			values[index] = prefix + value

			return values
		}
	}

	return append(values, prefix+value)
}
