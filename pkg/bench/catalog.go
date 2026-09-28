package bench

import (
	"errors"
	"fmt"
	"slices"
)

var (
	errEmptyCatalog       = errors.New("bench: workload catalog is empty")
	errDuplicateWorkload  = errors.New("bench: duplicate workload name")
	errFactoryNameChanged = errors.New("bench: workload factory returned a different name")
)

// Catalog is an explicit set of workload factories.
type Catalog struct {
	factories  map[string]Factory
	registered bool
}

// NewCatalog validates factories and creates an isolated workload catalog.
func NewCatalog(factories ...Factory) (*Catalog, error) {
	if len(factories) == 0 {
		return nil, errEmptyCatalog
	}

	catalog := &Catalog{factories: make(map[string]Factory, len(factories))}
	for _, factory := range factories {
		workload := workloadFromFactory(factory)
		name := workload.Name()

		if _, exists := catalog.factories[name]; exists {
			return nil, fmt.Errorf("%w %q", errDuplicateWorkload, name)
		}

		catalog.factories[name] = factory
	}

	return catalog, nil
}

// RegisteredCatalog returns a live view of factories added through Register.
func RegisteredCatalog() *Catalog {
	return &Catalog{registered: true}
}

// Factory returns a workload factory by name.
func (c *Catalog) Factory(name string) (Factory, bool) {
	if c == nil {
		return nil, false
	}

	if c.registered {
		return registeredFactory(name)
	}

	factory, ok := c.factories[name]

	return factory, ok
}

// Describe returns one workload's deterministic parameter schema.
func (c *Catalog) Describe(name string) (Description, error) {
	factory, ok := c.Factory(name)
	if !ok {
		return Description{}, fmt.Errorf("%w as %q", errNoWorkloadRegistered, name)
	}

	description, err := DescribeFactory(factory)
	if err != nil {
		return Description{}, err
	}

	if description.Name != name {
		return Description{}, fmt.Errorf("%w: got %q, want %q", errFactoryNameChanged, description.Name, name)
	}

	return description, nil
}

// DescribeAll returns every workload schema ordered by workload name.
func (c *Catalog) DescribeAll() ([]Description, error) {
	names := c.names()
	descriptions := make([]Description, 0, len(names))

	for _, name := range names {
		description, err := c.Describe(name)
		if err != nil {
			return nil, err
		}

		descriptions = append(descriptions, description)
	}

	return descriptions, nil
}

func (c *Catalog) names() []string {
	if c == nil {
		return nil
	}

	if c.registered {
		regMu.RLock()

		names := make([]string, 0, len(regWorkloads))
		for name := range regWorkloads {
			names = append(names, name)
		}

		regMu.RUnlock()
		slices.Sort(names)

		return names
	}

	names := make([]string, 0, len(c.factories))
	for name := range c.factories {
		names = append(names, name)
	}

	slices.Sort(names)

	return names
}
