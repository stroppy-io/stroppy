package author

import (
	"fmt"
	"strings"

	_ "embed"
)

//go:embed LICENSE
var starterLicense []byte

func Starter(name string, sourcePackage ...string) map[string][]byte {
	origin := ""
	if len(sourcePackage) != 0 {
		origin = sourcePackage[0]
	}

	source := fmt.Sprintf(`// Package workload defines a standalone database stress test.
package workload

import (
 "context"
 "embed"

 "github.com/stroppy-io/stroppy/v6/pkg/bench"
)

//go:embed *.go LICENSE README.md
var source embed.FS

var Test = bench.Test{Name: %q, Define: define, Source: source, SourcePackage: %q}

func init() { bench.Register(Test) }

func define(d *bench.Def) error {
 run := bench.RunParameters(&d.Param, bench.RunDefaults{Iterations: 10})
 d.Drivers.Declare("default", bench.DriverConfig{Kind: bench.DriverNoop})
 d.Execution.Step("query", query, run.Policy())
 return d.Execution.Err()
}

func query(ctx context.Context, b *bench.Bench) error {
 return b.Exec(ctx, "SELECT :value", map[string]any{"value": b.Iteration()})
}
`, name, origin)
	test := `package workload

import (
 "testing"

 "github.com/stroppy-io/stroppy/v6/pkg/bench"
 "github.com/stroppy-io/stroppy/v6/pkg/bench/testkit"
 "github.com/stroppy-io/stroppy/v6/pkg/record"
)

func TestQuery(t *testing.T) {
 recorder := &record.Recorder{}
 _, err := testkit.Record(t.Context(), Test, recorder, bench.RunOptions{
  Params: bench.ParamInputs{CLI: map[string]string{"iterations": "2"}},
 })
 if err != nil { t.Fatal(err) }
 if len(recorder.Operations()) != 2 { t.Fatal("expected two queries") }
}
`

	return map[string][]byte{
		"workload.go": []byte(source), "workload_test.go": []byte(test), "LICENSE": starterLicense,
		"README.md": []byte("# Workload\n\nEdit the Test definition and named query action, " +
			"then run go test ./... from project root.\n"),
	}
}

func ModuleName(name string) string {
	value := strings.ToLower(strings.ReplaceAll(name, "_", "-"))
	value = strings.Trim(value, "/")

	return "example.com/" + value
}
