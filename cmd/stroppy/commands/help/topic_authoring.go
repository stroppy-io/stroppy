package help

func init() {
	Register(Topic{
		Name:  "authoring",
		Short: "Create, test, record and eject Go workloads",
		Long: `AUTHORING

  stroppy init my-workload
  cd my-workload
  go run .
  go test ./...
  stroppy build .

  Init creates root main/module files and an importable workload/ package with
  a small Test definition, noop default, explicit source embedding and recording
  test. Go 1.27 or newer is required. --module sets module identity; --sdk-version
  supplies a release or pseudo-version for development binaries.

  Build checks the provided directory first, then its workload/ child. No manifest
  or recursive discovery is needed. Custom Go workloads are trusted native code.

BUILT WORKLOADS

  Build registers into the catalog under ~/.stroppy/workloads/; these manage it.

    stroppy list                          built-in and custom workloads
    stroppy run my-workload -d pg -D url=... runs from its snapshot, no source tree
    stroppy cache inspect DIGEST          build provenance and the SDK origin used
    stroppy cache clean                   drop reusable artifacts and private caches
    stroppy export my-workload -o ./out   portable binary for another machine
    stroppy remove my-workload            unregister and rebuild the runtime

  Published source is only needed to eject. See docs/author-tooling.md.

TESTS AND RECORDING

  bench.DescribeTest validates without actions. pkg/bench/testkit.Run executes
  against noop; Record executes against a caller-owned pkg/record.Recorder.
  Both use the real runtime, caller context and returned report without history.

  Recorder.Reply supplies explicit query rows/errors. Unconfigured reads have no
  rows, not fabricated database data. Operation snapshots preserve local order
  across database facades and serialize as schema-1 JSON without timestamps.

  stroppy run "SELECT 1" -d recording -D url=queries.json --iterations 2

  Recording output must not already exist. SQL and arguments may contain authored
  data: never record secrets. Concurrent global execution order is not guaranteed.

SOURCE PUBLICATION AND EJECTION

  bench.Test.Source is an optional fs.FS, normally populated with go:embed patterns.
  Leave it nil to keep source unpublished. Rebuild refreshes the embedded snapshot.
  Stroppy never copies unpublished source from build-machine paths or caches.

  stroppy eject example/query ./query-copy
  stroppy eject tpcc/tx ./tpcc-copy

  Eject combines shared scaffolding with the published package under workload/.
  Builtins include owned sources/assets/licenses; exported collections retain source
  publications. Non-empty or symlink destinations are refused, with no overwrite
  option. Missing publication returns an explicit source-unavailable error.

  Init/eject resolve project dependencies before success. --offline requires cached
  modules; -y consents to a verified private compiler under ~/.stroppy if needed.
  A dependency failure retains the created project for repair and reports a retry
  command. Direct go commands require system Go; no PATH settings are changed.

  Build compiles against the SDK module pinned in each project. --source-root PATH
  (or STROPPY_SOURCE_ROOT) compiles against a Stroppy source tree instead, for work
  on the SDK itself; a workload project is refused. The SDK tree is never inferred
  from the working directory, and every build reports which one the runtime uses.
`,
	})
}
