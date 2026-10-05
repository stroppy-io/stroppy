package help

func init() {
	Register(Topic{
		Name:  "probe",
		Short: "Describe workload schemas, resolved inputs, and driver capabilities",
		Long: `PROBE

  Probe observes definitions without running step actions or opening databases.
  Default discovery uses declared defaults, independent of ambient input.

    stroppy probe                         # full compiled catalog
    stroppy probe -o json                 # machine-readable catalog
    stroppy probe tpcc/tx -o json          # selected default schema
    stroppy probe tpcc/tx --resolved --scale-factor 2 -d noop -o json

  --resolved accepts the same typed parameter, config-file, SQL override, and
  driver inputs as run. It adds effective values, sources, and winning spellings.
  Supplied run inputs without --resolved are rejected. Driver facts contain no
  credentials. Do not put secrets in workload parameters.

OUTPUT

  PRESETS lists embedded workload assets. WORKLOADS lists typed parameters,
  encountered steps, finite metric declarations, and named driver facts.
  DRIVERS lists supported insert methods.

  JSON contains presets, drivers, and a sorted workloads array. Each workload
  contains name, params, steps, driver_refs, metrics, and optional values.
  Parameters include name, flag, scope, type, description, default,
  default_description, env, aliases, config, and constraints.

  Observation describes the normal input-resolved path, not every possible
  runtime-result-dependent Go branch. Use dynamic help for full parameter detail:

    stroppy run tpcc/tx --help

FLAGS

  -o, --output human|json   Output format; default human
      --resolved           Resolve inputs for a selected workload
`,
	})
}
