package bench

import (
	"errors"
	"testing"
	"time"
)

type allOpts struct {
	Str string        `env:"STR" default:"hello"`
	I   int           `env:"I" default:"7"`
	I64 int64         `env:"I64" default:"9"`
	U64 uint64        `env:"U64" default:"11"`
	F   float64       `env:"F" default:"1.5"`
	B   bool          `env:"B" default:"true"`
	D   time.Duration `env:"D" default:"3s"`

	Ignored  string // no env tag: not an option
	Excluded string `env:"-"`
}

// envMap builds a getenv from a map.
func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// runParse runs the struct-tag bridge over opts with the given env and returns
// the populated paramSet (schema available via set.Schema()).
func runParse(opts any, env map[string]string) (*paramSet, error) {
	set := newParamSet(nil, envMap(env), nil)
	return set, parseOptions(opts, set)
}

func TestParseOptionsDefaults(t *testing.T) {
	o := &allOpts{}
	set, err := runParse(o, nil)
	if err != nil {
		t.Fatalf("parseOptions: %v", err)
	}
	if o.Str != "hello" || o.I != 7 || o.I64 != 9 || o.U64 != 11 || o.F != 1.5 || !o.B || o.D != 3*time.Second {
		t.Fatalf("defaults not applied: %+v", o)
	}
	schema := set.Schema()
	if len(schema) != 7 {
		t.Fatalf("schema has %d entries, want 7 (Ignored/Excluded skipped)", len(schema))
	}
	s0 := schema[0]
	if s0.Name != "str" || s0.Env != "STR" || s0.Flag != "--str" || s0.Config != "str" ||
		s0.Type != "string" || s0.Default != "hello" || s0.Current != "hello" || s0.Source != "default" {
		t.Fatalf("first schema entry wrong: %+v", s0)
	}
	if schema[6].Type != "duration" || schema[6].Current != "3s" || schema[6].Flag != "--d" {
		t.Fatalf("duration schema entry wrong: %+v", schema[6])
	}
}

func TestParseOptionsFromEnv(t *testing.T) {
	o := &allOpts{}
	if _, err := runParse(o, map[string]string{
		"STR": "world", "I": "42", "I64": "-5", "U64": "1000",
		"F": "2.75", "B": "false", "D": "1m30s",
	}); err != nil {
		t.Fatalf("parseOptions: %v", err)
	}
	if o.Str != "world" || o.I != 42 || o.I64 != -5 || o.U64 != 1000 ||
		o.F != 2.75 || o.B || o.D != 90*time.Second {
		t.Fatalf("env not applied: %+v", o)
	}
}

func TestParseOptionsBadValue(t *testing.T) {
	o := &allOpts{}
	set, err := runParse(o, map[string]string{"I": "notanint"})
	if err != nil {
		t.Fatalf("parseOptions struct-level error unexpected: %v", err)
	}
	if set.Err() == nil {
		t.Fatal("expected a parse error recorded on the set for a non-integer int option")
	}
}

func TestParseOptionsNotPointer(t *testing.T) {
	if _, err := runParse(allOpts{}, nil); err == nil {
		t.Fatal("expected an error for a non-pointer Opts")
	}
	if _, err := runParse(nil, nil); err != nil {
		t.Fatalf("nil Opts must be a no-op, got %v", err)
	}
}

type validatedOpts struct {
	N int `env:"N" default:"0"`
}

func (o *validatedOpts) Validate() error {
	if o.N <= 0 {
		return errors.New("N must be positive")
	}
	return nil
}

func TestValidateOptionsHook(t *testing.T) {
	o := &validatedOpts{}
	if _, err := runParse(o, nil); err != nil {
		t.Fatalf("parseOptions: %v", err)
	}
	if err := validateOptions(o); err == nil {
		t.Fatal("Validate should reject N=0")
	}
	o.N = 5
	if err := validateOptions(o); err != nil {
		t.Fatalf("Validate should accept N=5, got %v", err)
	}
	// A struct without Validate is a no-op.
	if err := validateOptions(&allOpts{}); err != nil {
		t.Fatalf("no-Validate struct must pass, got %v", err)
	}
}
