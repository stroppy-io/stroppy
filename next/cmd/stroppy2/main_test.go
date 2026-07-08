package main

import (
	"reflect"
	"testing"

	"github.com/stroppy-io/stroppy/next/internal/runner"
)

func TestParseRun(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    runParams
		wantErr bool
	}{
		{
			name: "target only",
			args: []string{"simple"},
			want: runParams{target: "simple"},
		},
		{
			name: "env plus forwarded param and control flags",
			args: []string{"tpcc", "-e", "WAREHOUSES=1", "-e", "DURATION=5s", "--warehouses=4", "-skip", "validate_population"},
			want: runParams{target: "tpcc", env: []string{"WAREHOUSES=1", "DURATION=5s"}, flags: []string{"--warehouses=4", "-skip", "validate_population"}},
		},
		{
			name: "equals forms forwarded verbatim",
			args: []string{"./t", "-e=A=1", "--skip=check", "--seed=7"},
			want: runParams{target: "./t", env: []string{"A=1"}, flags: []string{"--skip=check", "--seed=7"}},
		},
		{
			name: "passthrough after dashdash",
			args: []string{"simple", "-e", "X=1", "--", "-probe", "-cpuprofile", "p"},
			want: runParams{target: "simple", env: []string{"X=1"}, flags: []string{"--", "-probe", "-cpuprofile", "p"}},
		},
		{
			name: "target after flags",
			args: []string{"-e", "X=1", "dir"},
			want: runParams{target: "dir", env: []string{"X=1"}},
		},
		{name: "missing target", args: []string{"-e", "X=1"}, wantErr: true},
		{name: "bad env", args: []string{"t", "-e", "NOEQUALS"}, wantErr: true},
		{name: "dangling -e", args: []string{"t", "-e"}, wantErr: true},
		// The shim is deliberately thin: an unknown flag forwards verbatim and
		// surfaces from the test binary's own param registry / flag parser, not
		// here. -skip is forwarded the same way and interpreted by the test
		// binary (which validates names against the author's Skippable set).
		{
			name: "unknown flag forwards",
			args: []string{"t", "--wat=1"},
			want: runParams{target: "t", flags: []string{"--wat=1"}},
		},
		{
			name: "skip forwards",
			args: []string{"t", "-skip=a,b"},
			want: runParams{target: "t", flags: []string{"-skip=a,b"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRun(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseRun(%v) = %+v, want error", tt.args, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseRun(%v): %v", tt.args, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseRun(%v) = %+v, want %+v", tt.args, got, tt.want)
			}
		})
	}
}

func TestTestArgs(t *testing.T) {
	p := runParams{flags: []string{"-skip=a,b", "-seed=9", "-plan"}}
	want := []string{"-skip=a,b", "-seed=9", "-plan"}
	if got := p.testArgs(); !reflect.DeepEqual(got, want) {
		t.Errorf("testArgs = %v, want %v", got, want)
	}
	// env never reaches the test argv; it is passed through the process env.
	if len(runParams{env: []string{"A=1"}}.testArgs()) != 0 {
		t.Error("env leaked into test argv")
	}
}

func TestResolveSourceBuiltin(t *testing.T) {
	src, err := resolveSource("tpcc")
	if err != nil {
		t.Fatal(err)
	}
	if !src.Builtin || src.Name != "tpcc" {
		t.Errorf("resolveSource(tpcc) = %+v", src)
	}
	// A non-builtin, non-existent path is an error.
	if _, err := resolveSource("definitely-not-a-path-or-builtin"); err == nil {
		t.Error("resolveSource of bogus target should error")
	}
	// Sanity: the builtin set is what the CLI advertises.
	if !runner.IsBuiltin("simple") || runner.IsBuiltin("nope") {
		t.Error("IsBuiltin disagrees with catalog")
	}
}
