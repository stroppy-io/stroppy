package cli

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

func TestInvalidRunDoesNotActivateProfiling(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	catalog, err := bench.NewCatalog(bench.Test{Name: "profile-validation", Define: func(*bench.Def) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"script":"profile-validation","steps":["work"]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"run", "--file", path, "--no-steps", "work"}, "--steps and --no-steps are mutually exclusive"},
		{[]string{"run", "profile-validation", "--", "unsupported"}, "arguments after '--' are not supported"},
	} {
		t.Run(test.want, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			args := append([]string{"--pprof", listener.Addr().String()}, test.args...)

			err := Execute(t.Context(), &Options{Catalog: catalog}, args, &stdout, &stderr)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validation error = %v, want %q", err, test.want)
			}

			if strings.Contains(stderr.String(), "pprof listening") {
				t.Fatalf("listener announced before validation: %s", &stderr)
			}
		})
	}
}
