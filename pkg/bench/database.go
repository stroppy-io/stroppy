package bench

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

// DriverConfig supplies soft defaults or explicit host configuration.
// Secrets are consumed by backends, never emitted by driver discovery.
type DriverConfig struct {
	Kind                  DriverTypeName  `json:"kind,omitempty"`
	URL                   string          `json:"url,omitempty"`
	DefaultInsertMethod   InsertStrategy  `json:"defaultInsertMethod,omitempty"`
	BulkSize              *int32          `json:"bulkSize,omitempty"`
	Postgres              *PostgresConfig `json:"postgres,omitempty"`
	SQL                   *SQLConfig      `json:"sql,omitempty"`
	CaCertFile            *string         `json:"caCertFile,omitempty"`
	AuthToken             *string         `json:"authToken,omitempty"`
	ServiceAccountKeyFile *string         `json:"serviceAccountKeyFile,omitempty"`
	AuthUser              *string         `json:"authUser,omitempty"`
	AuthPassword          *string         `json:"authPassword,omitempty"`
	TLSInsecureSkipVerify *bool           `json:"tlsInsecureSkipVerify,omitempty"`
}
type (
	PostgresConfig = config.PostgresConfig
	SQLConfig      = config.SQLConfig
)

// DriverRef identifies a named declaration in one definition replay.
type DriverRef struct {
	name  string
	owner *Def
	kind  DriverTypeName
}

func (r DriverRef) Name() string         { return r.name }
func (r DriverRef) Kind() DriverTypeName { return r.kind }
func (r DriverRef) SupportsInsert(method InsertStrategy) bool {
	kind, err := ParseDriverType(string(r.kind))

	return err == nil && driver.SupportsInsertMethod(kind, driver.InsertMethod(method))
}

// DriverDescription is secret-free authored driver metadata.
type DriverDescription struct {
	Name string         `json:"name"`
	Kind DriverTypeName `json:"kind"`
}
type DriverDeclarations struct {
	def          *Def
	configs      map[string]DriverConfig
	declared     map[string]DriverConfig
	queryTimeout time.Duration
}

//nolint:gocritic // immutable declaration defaults are copied.
func (d *DriverDeclarations) Declare(name string, defaults DriverConfig) DriverRef {
	if !paramNamePattern.MatchString(name) {
		invalid("driver", inputError("invalid name %q", name))
	}

	if d.declared == nil {
		d.declared = map[string]DriverConfig{}
	}

	if _, ok := d.declared[name]; ok {
		invalid("driver", inputError("duplicate name %q", name))
	}

	if name == "default" {
		defaults = mergeDriverDefaults(defaultDriverConfig(), defaults)
	}

	effective := mergeDriverDefaults(defaults, d.configs[name])
	if name == "default" {
		effective = mergeDriverDefaults(effective, d.configs[""])
	}

	if effective.Kind == "" {
		effective.Kind = DriverPostgres
	}

	if _, err := ParseDriverType(string(effective.Kind)); err != nil {
		invalid("driver "+name, err)
	}

	if effective.DefaultInsertMethod == 0 {
		effective.DefaultInsertMethod = InsertNative
	}

	d.declared[name] = effective

	return DriverRef{name: name, owner: d.def, kind: effective.Kind}
}

func (d *DriverDeclarations) description() []DriverDescription {
	out := make([]DriverDescription, 0, len(d.declared))
	for name, cfg := range d.declared {
		out = append(out, DriverDescription{name, cfg.Kind})
	}

	slices.SortFunc(out, func(a, b DriverDescription) int { return strings.Compare(a.Name, b.Name) })

	return out
}

//nolint:gocritic // configuration values are copied rather than retaining author pointers.
func mergeDriverDefaults(base, override DriverConfig) DriverConfig {
	// Serialization is confined to in-memory field merging; it is never provenance.
	var left, right map[string]json.RawMessage

	a, err := json.Marshal(base) //nolint:gosec // private in-memory copy, not serialized provenance.
	if err != nil {
		invalid("driver configuration", err)
	}

	b, err := json.Marshal(override) //nolint:gosec // private in-memory copy, not serialized provenance.
	if err != nil {
		invalid("driver configuration", err)
	}

	if err = json.Unmarshal(a, &left); err != nil {
		invalid("driver configuration", err)
	}

	if err = json.Unmarshal(b, &right); err != nil {
		invalid("driver configuration", err)
	}

	data, err := json.Marshal(mergeRawFields(left, right))
	if err != nil {
		invalid("driver configuration", err)
	}

	var out DriverConfig
	if err = json.Unmarshal(data, &out); err != nil {
		invalid("driver configuration", err)
	}

	return out
}

func mergeRawFields(base, override map[string]json.RawMessage) map[string]json.RawMessage {
	for key, value := range override {
		var left, right map[string]json.RawMessage
		if json.Unmarshal(base[key], &left) == nil && json.Unmarshal(value, &right) == nil && left != nil && right != nil {
			encoded, err := json.Marshal(mergeRawFields(left, right))
			if err != nil {
				invalid("driver configuration", err)
			}

			base[key] = encoded
		} else {
			base[key] = value
		}
	}

	return base
}

func (d *DriverDeclarations) QueryTimeout(timeout time.Duration) {
	if timeout < 0 {
		invalid("query-timeout", inputError("must not be negative"))
	}

	d.queryTimeout = timeout
}

//nolint:gocritic // runtime configuration is an isolated value copy.
func (c DriverConfig) runtime() (*config.DriverConfig, error) {
	kind, err := ParseDriverType(string(c.Kind))
	if err != nil {
		return nil, err
	}

	method := ""
	if c.DefaultInsertMethod != 0 {
		method = c.DefaultInsertMethod.String()
	}

	return &config.DriverConfig{
		DriverType:            kind,
		URL:                   c.URL,
		DefaultInsertMethod:   method,
		BulkSize:              c.BulkSize,
		Postgres:              c.Postgres,
		SQL:                   c.SQL,
		CaCertFile:            c.CaCertFile,
		AuthToken:             c.AuthToken,
		ServiceAccountKeyFile: c.ServiceAccountKeyFile,
		AuthUser:              c.AuthUser,
		AuthPassword:          c.AuthPassword,
		TLSInsecureSkipVerify: c.TLSInsecureSkipVerify,
	}, nil
}

// DriverConfiguration converts implementation-facing configuration into a copied
// neutral host input. Backend implementations need not be imported by authors.
func DriverConfiguration(c *config.DriverConfig) DriverConfig {
	if c == nil {
		return DriverConfig{}
	}

	method, _ := driver.ParseInsertMethod(c.DefaultInsertMethod)
	if c.DefaultInsertMethod == "" {
		method = 0
	}

	return mergeDriverDefaults(DriverConfig{}, DriverConfig{
		Kind:                  DriverTypeNameOf(c.DriverType),
		URL:                   c.URL,
		DefaultInsertMethod:   InsertStrategy(method),
		BulkSize:              c.BulkSize,
		Postgres:              c.Postgres,
		SQL:                   c.SQL,
		CaCertFile:            c.CaCertFile,
		AuthToken:             c.AuthToken,
		ServiceAccountKeyFile: c.ServiceAccountKeyFile,
		AuthUser:              c.AuthUser,
		AuthPassword:          c.AuthPassword,
		TLSInsecureSkipVerify: c.TLSInsecureSkipVerify,
	})
}

// Database is a step-worker-scoped neutral facade for a named driver.
type Database struct{ *Bench }

func (b *Bench) Database(ref DriverRef) *Database {
	if ref.owner != b.execution.def {
		invalid("database", inputError("reference belongs to another definition"))
	}

	return &Database{Bench: b.execution.bench(b.vu, ref.name)}
}

func (e *Execution) database(ctx context.Context, name string) (driver.Driver, *config.DriverConfig, error) {
	e.databaseMu.Lock()
	defer e.databaseMu.Unlock()

	if name == "default" {
		name = ""
	}

	if slot := e.databases[name]; slot != nil {
		return slot.drv, slot.cfg, nil
	}

	cfg, err := e.configuration(name).runtime()
	if err != nil {
		return nil, nil, err
	}

	drv, err := driver.Dispatch(ctx, driver.Options{
		Config:       cfg,
		Logger:       e.root.lg,
		DialFunc:     e.root.dialer.DialContext,
		QueryTimeout: e.def.Drivers.queryTimeout,
	})
	if err != nil {
		return nil, nil, err
	}

	e.databases[name] = &databaseSlot{drv: drv, cfg: cfg}

	return drv, cfg, nil
}

func (e *Execution) configuration(name string) DriverConfig {
	if name == "default" {
		name = ""
	}

	resolved := e.def.Drivers.declared[name]
	if name == "" {
		resolved = e.def.Drivers.declared["default"]
	}

	if resolved.Kind == "" {
		resolved = mergeDriverDefaults(defaultDriverConfig(), e.def.Drivers.configs[name])
	}

	return resolved
}

func (b *Bench) ensureDriver(ctx context.Context) error {
	if b.drv != nil {
		return nil
	}

	drv, cfg, err := b.execution.database(ctx, b.databaseName)
	if err != nil {
		return err
	}

	b.drv, b.cfg = drv, cfg

	return nil
}

type databaseSlot struct {
	drv driver.Driver
	cfg *config.DriverConfig
}

func defaultDriverConfig() DriverConfig {
	//nolint:gosec // local development default only.
	return DriverConfig{
		Kind:                DriverPostgres,
		URL:                 "postgres://postgres:postgres@localhost:5432",
		DefaultInsertMethod: InsertNative,
	}
}

func copyDriverConfigs(values map[string]DriverConfig) map[string]DriverConfig {
	out := make(map[string]DriverConfig, len(values))
	for name, value := range values {
		if name != "" && !paramNamePattern.MatchString(name) {
			invalid("driver configuration", inputError("invalid name %q", name))
		}

		if name == "default" {
			name = ""
		}

		if _, exists := out[name]; exists {
			invalid("driver configuration", inputError("duplicate default database"))
		}

		out[name] = mergeDriverDefaults(DriverConfig{}, value)
	}

	return out
}

// ErrorKind and ErrorFacts expose backend-neutral classification, not concrete errors.
type (
	ErrorKind  = driver.ErrorKind
	ErrorFacts = driver.ErrorFacts
)

const (
	ErrorUnknown       = driver.ErrorKindUnknown
	ErrorSerialization = driver.ErrorKindSerialization
	ErrorDeadlock      = driver.ErrorKindDeadlock
	ErrorLockTimeout   = driver.ErrorKindLockTimeout
	ErrorTransient     = driver.ErrorKindTransient
	ErrorUnsupported   = driver.ErrorKindUnsupported
	ErrorCanceled      = driver.ErrorKindCanceled
	ErrorTimeout       = driver.ErrorKindTimeout
)
