package bench

import (
	"fmt"
	"io/fs"
	"slices"
	"sync"
)

// Test is an ordinary definition value. Mutable run state belongs inside Define.
type Test struct {
	Name   string
	Define func(*Def) error
	// Source publishes package-relative files for ejection. Nil means unpublished.
	Source fs.FS
}

var (
	regMu                   sync.RWMutex
	regTests                = map[string]Test{}
	errNoWorkloadRegistered = inputError("bench: no test registered")
	errEmptyCatalog         = inputError("bench: test catalog is empty")
	errDuplicateWorkload    = inputError("bench: duplicate test name")
)

func validateTest(test Test) error {
	if test.Name == "" || test.Define == nil {
		return inputError("test requires name and definition")
	}

	return nil
}

// Register stores a value copy and returns it. Importable packages normally call
// Register from init; declaration-time registration is also supported.
func Register(test Test) Test {
	if err := validateTest(test); err != nil {
		invalid("register test", err)
	}

	regMu.Lock()
	defer regMu.Unlock()

	if _, ok := regTests[test.Name]; ok {
		invalid("register test", fmt.Errorf("%w %q", errDuplicateWorkload, test.Name))
	}

	regTests[test.Name] = test

	return test
}

func Lookup(name string) (Test, bool) {
	regMu.RLock()
	defer regMu.RUnlock()

	test, ok := regTests[name]

	return test, ok
}

// Catalog is an explicit set of Test values, or a live registered view.
type Catalog struct {
	tests      map[string]Test
	registered bool
}

func NewCatalog(tests ...Test) (*Catalog, error) {
	if len(tests) == 0 {
		return nil, errEmptyCatalog
	}

	c := &Catalog{tests: map[string]Test{}}
	for _, test := range tests {
		if err := validateTest(test); err != nil {
			return nil, err
		}

		if _, ok := c.tests[test.Name]; ok {
			return nil, fmt.Errorf("%w %q", errDuplicateWorkload, test.Name)
		}

		c.tests[test.Name] = test
	}

	return c, nil
}
func RegisteredCatalog() *Catalog { return &Catalog{registered: true} }
func (c *Catalog) Test(name string) (Test, bool) {
	if c == nil {
		return Test{}, false
	}

	if c.registered {
		return Lookup(name)
	}

	test, ok := c.tests[name]

	return test, ok
}

func (c *Catalog) Describe(name string) (Description, error) {
	test, ok := c.Test(name)
	if !ok {
		return Description{}, fmt.Errorf("%w %q", errNoWorkloadRegistered, name)
	}

	return DescribeTest(test)
}

func (c *Catalog) Resolve(name string, inputs ParamInputs, drivers map[string]DriverConfig) (Description, error) {
	return c.ResolveRun(name, RunOptions{Params: inputs, Drivers: drivers})
}

// ResolveRun observes effective run inputs, including step selection, without actions.
// Logger, metrics export and report options do not affect discovery.
//
//nolint:gocritic // run inputs are copied operation values.
func (c *Catalog) ResolveRun(name string, options RunOptions) (description Description, err error) {
	defer recoverValidation(&err)

	test, ok := c.Test(name)
	if !ok {
		return Description{}, fmt.Errorf("%w %q", errNoWorkloadRegistered, name)
	}

	d, err := observeSelected(test, options.Params, copyDriverConfigs(options.Drivers), false,
		options.Steps, options.NoSteps)
	if err != nil {
		return Description{}, err
	}

	return d.description(test.Name), nil
}

func (c *Catalog) DescribeAll() ([]Description, error) {
	out := []Description{}

	for _, name := range c.names() {
		d, err := c.Describe(name)
		if err != nil {
			return nil, err
		}

		out = append(out, d)
	}

	return out, nil
}

func (c *Catalog) names() []string {
	if c == nil {
		return nil
	}

	var names []string

	if c.registered {
		regMu.RLock()

		for name := range regTests {
			names = append(names, name)
		}

		regMu.RUnlock()
	} else {
		for name := range c.tests {
			names = append(names, name)
		}
	}

	slices.Sort(names)

	return names
}
func Describe(name string) (Description, error) { return RegisteredCatalog().Describe(name) }
func DescribeAll() ([]Description, error)       { return RegisteredCatalog().DescribeAll() }
func DescribeTest(test Test) (Description, error) {
	d, err := observe(test, ParamInputs{}, nil, true)
	if err != nil {
		return Description{}, err
	}

	return d.description(test.Name), nil
}

func (d *Def) description(name string) Description {
	values := map[string]ResolvedParameter{}

	if !d.defaultsOnly {
		for _, p := range d.resolved {
			values[p.name] = ResolvedParameter{
				Value: p.value,
				Info:  ParamInfo{Name: p.name, Source: p.source, Spelling: p.spelling},
			}
		}
	}

	return Description{
		Name:    name,
		Params:  d.schema(),
		Steps:   slices.Clone(d.Execution.observed),
		Drivers: d.Drivers.description(),
		Values:  values,
		Metrics: slices.Clone(d.Metrics.schema),
	}
}

func observe(
	test Test,
	inputs ParamInputs,
	drivers map[string]DriverConfig,
	defaults bool,
) (*Def, error) {
	return observeSelected(test, inputs, drivers, defaults, nil, nil)
}

func observeSelected(
	test Test,
	inputs ParamInputs,
	drivers map[string]DriverConfig,
	defaults bool,
	steps, noSteps []string,
) (d *Def, err error) {
	defer recoverValidation(&err)

	if validationErr := validateTest(test); validationErr != nil {
		return nil, validationErr
	}

	d = newDef(inputs, defaults)
	d.Execution.filter = newStepFilter(steps, noSteps)

	d.Drivers.configs = copyDriverConfigs(drivers)
	if defineErr := test.Define(d); defineErr != nil {
		return nil, defineErr
	}

	if !defaults {
		err = d.finish()
	}

	return d, err
}
