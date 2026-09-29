package toolchain

import (
	"strings"
	"testing"
)

func TestCompatibleVersion(t *testing.T) {
	for version, want := range map[string]bool{
		"1.25.9": false,
		"1.26":   true,
		"1.26.8": true,
		"1.27.1": true,
		"2.0.0":  true,
		"bad":    false,
	} {
		if got := compatible(version); got != want {
			t.Errorf("compatible(%q) = %v, want %v", version, got, want)
		}
	}
}

func TestCompilerEnvIsIsolated(t *testing.T) {
	compiler := &Compiler{Root: t.TempDir()}
	env := compiler.Env("linux", "arm64", true)

	for key, want := range map[string]string{
		"GOENV": "off", "GOTOOLCHAIN": "local", "GOWORK": "off",
		"GOOS": "linux", "GOARCH": "arm64", "CGO_ENABLED": "0", "GOPROXY": "off",
	} {
		if got := envValue(env, key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	for _, key := range []string{"GOPATH", "GOMODCACHE", "GOCACHE", "GOTMPDIR"} {
		if got := envValue(env, key); !strings.HasPrefix(got, compiler.Root) {
			t.Errorf("%s = %q, want under %s", key, got, compiler.Root)
		}
	}
}

func TestResolveUsesCompatibleSystemGo(t *testing.T) {
	compiler, err := Resolve(t.Context(), Options{Root: t.TempDir(), Consent: ConsentNever})
	if err != nil {
		t.Fatal(err)
	}
	if compiler.Private || compiler.Path == "" || !compatible(compiler.Version) {
		t.Fatalf("compiler = %#v", compiler)
	}
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, value := range env {
		if strings.HasPrefix(value, prefix) {
			return strings.TrimPrefix(value, prefix)
		}
	}

	return ""
}
