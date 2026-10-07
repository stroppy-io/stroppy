package help

func init() {
	Register(Topic{
		Name:  "envs",
		Short: "Typed parameters, environment inputs, and precedence",
		Long: `ENVS AND TYPED PARAMETERS

  Workloads declare typed run and workload parameters. Inspect the selected
  schema and set direct flags:

    stroppy run tpcc/tx --help
    stroppy run tpcc/tx --scale-factor 10 --load-workers 8

  Built-in run parameters:

    --executor       shared-iterations or constant-vus
    --vus            Concurrent workers
    --iterations     Total shared iterations, not per-worker
    --duration       Run length as a Go duration
    --drain-timeout  Grace after duration; none waits without timeout
    --query-timeout  Per-statement deadline; 0 disables it

  Select the executor explicitly; DURATION alone does not infer constant-vus.
  Custom workloads can declare individual parameters or use RunParameters.

SOURCES AND PRECEDENCE

    1. Typed --name CLI flag
    2. Process environment
    3. Matching typed config object ("run" or "params")
    4. Declared default

  Lower-case kebab names project to flags, upper snake environment names, and
  lower-camel config keys. scale-factor becomes --scale-factor, SCALE_FACTOR,
  and scaleFactor. Aliases belong to the parameter and project to all channels.
  Canonical spelling wins within a channel, then aliases in declaration order.
  Malformed winning input fails; lower-priority sources do not rescue it.
  Unknown supplied flags/config keys fail; unrelated environment is ignored.

    SCALE_FACTOR=10 stroppy run tpcc/tx
    LOAD_WORKERS=8 stroppy run tpcc/tx

  -e/--env and the config "env" map are not supported. Boolean flags require
  explicit values, for example --validate=false. Arguments after "--" are
  not supported.

  Logging uses --log-level/--log-mode > LOG_LEVEL/LOG_MODE process variables >
  global.logger > debug/development defaults. It is separate from workload
  parameter declarations.

DISCOVERY

    stroppy probe
    stroppy probe tpcc/tx -o json
    stroppy probe tpcc/tx --resolved --scale-factor 2 -d noop -o json

  Default discovery ignores ambient workload inputs. Resolved discovery adds
  effective values, sources, and spellings without running actions or opening
  databases. Schema includes aliases, constraints, and derived defaults.

CONFIG FILE

    {
      "run": {"executor": "constant-vus", "vus": 10, "duration": "60s"},
      "params": {"scaleFactor": 10, "loadWorkers": 8}
    }

  See 'stroppy help config-file' for the strict file envelope and named drivers.
`,
	})
}
