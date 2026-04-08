package help

func init() {
	Register(Topic{
		Name:  "probe",
		Short: "Probe output sections, flags, and output formats",
		Long: `PROBE

  stroppy probe runs a TypeScript benchmark script inside a mocked k6
  environment without connecting to any real database. It extracts metadata
  declared by the script — configuration, k6 options, SQL structure, steps,
  environment variables, and driver defaults — and prints them for inspection.

  Use probe to:
    - Verify a script is syntactically and structurally valid before a run
    - Understand what SQL sections and queries the script requires
    - Check which environment variables the script reads
    - Review the driver setup defaults declared in the script

SECTIONS

  Config (--config)

    Shows the Stroppy global configuration passed to DriverX.fromConfig(...)
    in the script. Output is in protojson format. The schema is defined in
    proto/stroppy/config.proto.

  K6 Options (--options)

    Shows the k6 options object exported from the script:
      export const options: Options = { ... };
    Follows the structure from k6/lib/options.go and
    @types/k6/options/index.d.ts. Null fields are omitted.

  SQL File Structure (--sql)

    Lists the SQL sections and named queries the script expects to find in
    its SQL file. Without these, the benchmark will fail at runtime.

    Sections begin with '--+ <SectionName>'.
    Queries within sections are named '--= <QueryName>'.

    The output can be copy-pasted into an SQL file as a skeleton to fill in.

  Steps (--steps)

    Shows the logical steps registered in the script via:
      Step("step name", () => { ... })
    or the begin/end pair:
      StepBegin("step name"); StepEnd("step name");

  Environment Variables (--envs)

    Lists env vars read by the script. Two forms are shown:

    Declared via ENV() — includes optional default values and descriptions:
      MY_VAR=""              (no default, not set in current env)
      MY_VAR="" (default: x) (has default, not set in current env)
      MY_VAR=value           (set in current environment)

    Legacy plain access via __ENV.<VAR_NAME> — shown with current value if set.

    Only script-specific env vars are listed. k6 built-ins and STROPPY_*
    variables are not included here.

  Drivers (--drivers)

    Shows the driver default configuration declared in the script via
    declareDriverSetup(index, defaults). This is the script's starting point
    before any CLI overrides (-d, -D) are applied.

    See 'stroppy help drivers' for how presets and CLI overrides work.

SECTION FILTER FLAGS

  By default all sections are shown. Pass one or more flags to show only
  those sections:

    --config     Show only the Stroppy Config section
    --options    Show only the K6 Options section
    --sql        Show only the SQL File Structure section
    --steps      Show only the Steps section
    --envs       Show only the Environment Variables section
    --drivers    Show only the Drivers section

  Multiple flags can be combined:
    stroppy probe tpcc/procs.ts --sql --steps

OUTPUT FORMATS (-o)

  -o human  (default) Human-readable labeled sections, as described above.

  -o json   Machine-readable JSON. All sections are included regardless of
            section filter flags. Config and driver fields use protojson
            encoding. Useful for tooling and CI inspection.

            Example keys: global_config, options, sql_sections, steps,
            envs, env_declarations, driver_setups, drivers.

CONFIG FILE (-f)

  Pass -f to load a config file when probing. The Config section then shows
  the file's global settings (logger, OTEL exporter) instead of the empty
  default. The effective script and SQL are also resolved from the file when
  not provided as positional arguments:

    stroppy probe -f stroppy-config.json          # script from file
    stroppy probe -f stroppy-config.json tpcc/procs     # script overrides file

  See 'stroppy help config-file' for the config file format.

--local FLAG

  By default, probe copies the script into a temporary directory and resolves
  dependencies there. Pass --local (-l) to skip tmp-dir creation and resolve
  imports relative to the script's own working directory instead.

  Use --local when the script imports local modules that are not bundled and
  must be resolved from the source tree.

EXAMPLES

  # Probe all sections (default); .ts extension is optional
  stroppy probe workloads/tpcc/procs
  stroppy probe workloads/tpcc/procs.ts

  # Probe with an explicit SQL file
  stroppy probe workloads/tpcc/procs workloads/tpcc/pg.sql

  # Show only SQL structure
  stroppy probe workloads/tpcc/procs --sql

  # Show SQL structure and steps together
  stroppy probe workloads/tpcc/procs.ts --sql --steps

  # Show driver defaults
  stroppy probe workloads/tpcc/procs --drivers

  # Machine-readable JSON output
  stroppy probe workloads/tpcc/procs -o json

  # Probe using local imports (no tmp dir)
  stroppy probe workloads/tpcc/procs.ts --local

SEE ALSO

  stroppy help drivers
  stroppy run --help
`,
	})
}
