package modulecompat_test

// The two sample workloads that stroppy-contrib publishes as the reference
// examples are assembled here from the same starter a user gets, plus the edits
// the guide walks through. They are compiled and tested as an external module
// against the release-shaped archive without a replace directive, so a change
// that breaks the documented authoring surface fails this suite rather than a
// reader's project. Keep the testdata copies in step with stroppy-contrib.

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stroppy-io/stroppy/v6/internal/author"
)

type sampleWorkload struct {
	name   string
	module string
	// files are the guide's edits, layered over the generated starter.
	files []string
	// schema entries that must appear in the workload's probe output.
	schema []string
}

func sampleWorkloads() []sampleWorkload {
	return []sampleWorkload{
		{
			name:   "select1",
			module: "example.com/select1",
			files:  []string{"workload.go", "workload_test.go"},
			schema: []string{`"iterations"`, `"executor"`},
		},
		{
			name:   "selectfile",
			module: "example.com/selectfile",
			files:  []string{"workload.go", "workload_test.go", "queries.sql"},
			schema: []string{`"expected"`, `"label"`, `"sql-file"`},
		},
	}
}

// TestSampleWorkloadsServeTheDocumentedContract builds each published sample as
// its own module, runs its own tests, and inspects the workload it registers.
func TestSampleWorkloadsServeTheDocumentedContract(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	proxyDir := t.TempDir()
	writeModuleProxy(t, proxyDir, repoRoot)

	proxyURL := (&url.URL{Scheme: "file", Path: proxyDir}).String()
	moduleCache := filepath.Join(t.TempDir(), "modcache")
	t.Cleanup(func() { makeWritable(t, moduleCache) })

	env := append(os.Environ(),
		"GOPROXY="+proxyURL+",https://proxy.golang.org,direct",
		"GONOSUMDB="+modulePath,
		"GOWORK=off",
		"GOMODCACHE="+moduleCache,
		"GOCACHE="+filepath.Join(t.TempDir(), "gocache"),
		"HOME="+t.TempDir(),
	)

	for _, sample := range sampleWorkloads() {
		t.Run(sample.name, func(t *testing.T) {
			directory := writeSampleProject(t, sample)

			runCommand(t, directory, env, "go", "test", "-mod=mod", "-race", "./...")

			binary := filepath.Join(directory, "sample")
			runCommand(t, directory, env, "go", "build", "-mod=mod", "-o", binary, ".")

			output := runCommand(t, directory, env, binary, "probe", "-o", "json")

			if !strings.Contains(output, `"name":"`+sample.name+`"`) {
				t.Fatalf("probe output does not report the workload name:\n%s", output)
			}

			for _, entry := range sample.schema {
				if !strings.Contains(output, entry) {
					t.Fatalf("probe output does not report %s:\n%s", entry, output)
				}
			}
		})
	}
}

// writeSampleProject assembles one published sample: the starter every user gets,
// with the guide's edits layered on top.
func writeSampleProject(t *testing.T, sample sampleWorkload) string {
	t.Helper()

	published := author.Starter(sample.name, sample.module+"/workload")

	for _, name := range sample.files {
		data, err := os.ReadFile(filepath.Join("testdata", "samples", sample.name, name))
		if err != nil {
			t.Fatal(err)
		}

		published[name] = data
	}

	files, err := author.Project(sample.name, sample.module, version, published, sample.module+"/workload")
	if err != nil {
		t.Fatal(err)
	}

	directory := filepath.Join(t.TempDir(), sample.name)
	if err := author.Write(directory, files); err != nil {
		t.Fatal(err)
	}

	return directory
}
