package help

func init() {
	Register(Topic{
		Name:  "steps",
		Short: "Logical benchmark phases: defining, filtering, and discovering steps",
		Long: `STEPS

  Steps are named logical phases of a benchmark script — for example,
  drop_schema, create_schema, load_data, run. They let you run only part of a
  benchmark without changing the script.

DEFINING STEPS IN TYPESCRIPT

  Wrap a phase in Step() inside the setup() or default() function:

    Step("create_schema", () => {
      sql("create_schema").forEach((q) => driver.exec(q, {}));
    });

    Step("load_data", () => {
      driver.insert("orders", COUNT, { params: { ... } });
    });

  When a step is filtered out (via --steps or --no-steps), Step() logs
  "Skipping step '<name>'" and returns immediately without executing the body.

  Step() also exposes begin/end helpers for splitting a step across multiple
  code blocks:

    Step.begin("my_step");
    // ... work ...
    Step.end("my_step");

FILTERING FROM THE CLI

  --steps step1,step2      Run only the listed steps; skip all others.
  --no-steps step1,step2   Skip the listed steps; run everything else.

  The two flags are mutually exclusive. Stroppy validates the names against
  the script's declared steps before launching k6 — unknown names are
  rejected immediately.

  Comma-separated list or space-separated repeated flag forms both work:

    --steps create_schema,load_data
    --steps=create_schema,load_data

DISCOVERING STEPS

  To see which steps a script declares before running it:

    stroppy probe <script> --steps

  This runs the script in a mocked environment and prints the registered
  step names in order.

EXAMPLES

  # Only create the schema — skip data load and benchmark run
  stroppy run tpcc --steps create_schema

  # Create schema and load data, then stop
  stroppy run tpcc --steps create_schema,load_data

  # Run everything except the schema drop
  stroppy run tpcc --no-steps drop_schema

  # See what steps tpcc defines
  stroppy probe tpcc --steps

SEE ALSO

  stroppy run --help
  stroppy probe --help
  stroppy help probe
`,
	})
}
