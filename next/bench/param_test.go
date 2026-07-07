package bench

import (
	"encoding/json"
	"testing"
	"time"
)

// TestParamTypedHandle exercises the typed Param[T] surface directly: each
// accessor resolves its value at registration from the input bags (cli > env >
// config > default) and reports its source. This is the canonical D1 handle the
// struct-tag adapter and the SDK's standard params are built on, and that D7's
// Define callback will hand to authors.
func TestParamTypedHandle(t *testing.T) {
	cli := map[string]string{"int": "42", "from_cli": "cli-val"}
	cfg := map[string]json.RawMessage{
		"from_cfg": json.RawMessage(`"cfg-val"`),
		"f64":      json.RawMessage(`2.5`),
		"dur":      json.RawMessage(`"90s"`),
	}
	env := map[string]string{"FROM_ENV": "env-val", "U64": "1000"}
	set := newParamSet(cli, envMap(env), cfg)

	i := set.Int("int", 1, "i")
	if i.Value() != 42 || i.Source() != SourceCLI {
		t.Fatalf("int: got %d/%s, want 42/cli", i.Value(), i.Source())
	}
	iDef := set.Int("other_int", 7, "def")
	if iDef.Value() != 7 || iDef.Source() != SourceDefault {
		t.Fatalf("default int: got %d/%s, want 7/default", iDef.Value(), iDef.Source())
	}
	u := set.Uint64("u64", 1, "u")
	if u.Value() != 1000 || u.Source() != SourceEnv {
		t.Fatalf("uint64: got %d/%s, want 1000/env", u.Value(), u.Source())
	}
	f := set.Float64("f64", 1.0, "f")
	if f.Value() != 2.5 || f.Source() != SourceConfig {
		t.Fatalf("float64 from cfg number: got %v/%s, want 2.5/config", f.Value(), f.Source())
	}
	d := set.Duration("dur", time.Second, "d")
	if d.Value() != 90*time.Second || d.Source() != SourceConfig {
		t.Fatalf("duration from cfg string: got %v/%s, want 90s/config", d.Value(), d.Source())
	}
	b := set.Bool("flag", false, "b")
	if b.Value() || b.Source() != SourceDefault {
		t.Fatalf("bool default: got %v/%s, want false/default", b.Value(), b.Source())
	}
	if got := set.String("from_cli", "x", "c").Value(); got != "cli-val" {
		t.Fatalf("cli wins over default: %q", got)
	}
	fromEnv := set.String("from_env", "x", "e")
	if got, src := fromEnv.Value(), fromEnv.Source(); got != "env-val" || src != SourceEnv {
		t.Fatalf("env beats config/default: %q/%s", got, src)
	}
	if got := set.String("from_cfg", "x", "g").Value(); got != "cfg-val" {
		t.Fatalf("cfg beats default: %q", got)
	}
	if err := set.Err(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

// TestParamDuplicateRejected: declaring one name twice is an error.
func TestParamDuplicateRejected(t *testing.T) {
	set := newParamSet(nil, envMap(nil), nil)
	set.Int("x", 1, "first")
	set.Int("x", 2, "second")
	if set.Err() == nil {
		t.Fatal("expected a duplicate-declaration error")
	}
}

// TestParamBadValueRecorded: a parse failure is recorded (not thrown), so a
// later probe can still report the full catalog; the handle falls back to the
// declared default.
func TestParamBadValueRecorded(t *testing.T) {
	set := newParamSet(map[string]string{"n": "nope"}, envMap(nil), nil)
	h := set.Int("n", 5, "n")
	if set.Err() == nil {
		t.Fatal("expected a parse error recorded on the set")
	}
	if h.Value() != 5 {
		t.Fatalf("bad value should fall back to default 5, got %d", h.Value())
	}
}

// TestParamCheckUnknown: a --flag no param consumed is a typo error.
func TestParamCheckUnknown(t *testing.T) {
	set := newParamSet(map[string]string{"known": "1", "typo": "2"}, envMap(nil), nil)
	set.Int("known", 0, "k")
	if err := set.checkUnknown(); err == nil {
		t.Fatal("expected an unknown-flag error for \"typo\"")
	}
}

// TestParamSchemaProjection: every declared param appears in the schema with its
// flag/env/config projections and resolved current value + source.
func TestParamSchemaProjection(t *testing.T) {
	set := newParamSet(map[string]string{"warehouses": "4"}, envMap(nil), nil)
	set.Int("warehouses", 1, "warehouse count (scale)", optEnv("WAREHOUSES"))
	set.String("driver.url", "postgres://", "slot-0 URL", optEnv("STROPPY_DRIVER_URL"), optStandard())
	schema := set.Schema()
	if len(schema) != 2 {
		t.Fatalf("schema len = %d, want 2", len(schema))
	}
	w := schema[0]
	if w.Name != "warehouses" || w.Env != "WAREHOUSES" || w.Flag != "--warehouses" ||
		w.Config != "warehouses" || w.Type != "int" || w.Default != "1" ||
		w.Current != "4" || w.Source != "cli" || w.Standard {
		t.Fatalf("warehouses schema wrong: %+v", w)
	}
	d := schema[1]
	if d.Name != "driver.url" || d.Env != "STROPPY_DRIVER_URL" || !d.Standard || d.Source != "default" {
		t.Fatalf("driver.url schema wrong: %+v", d)
	}
}
