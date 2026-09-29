package workloadcatalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

var ErrProbeContract = errors.New("workload probe contract failed")

// Process describes one managed workload process invocation.
type Process struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Env    []string
	Dir    string
}

// WorkloadProbe is minimal machine-readable identity from a standalone workload.
type WorkloadProbe struct {
	Name string `json:"name"`
}

// Run executes an active catalog artifact with direct argument forwarding.
func (store *Store) Run(ctx context.Context, name string, args []string, process *Process) error {
	if process == nil {
		process = &Process{}
	}

	entry, err := store.Get(name)
	if err != nil {
		return err
	}

	command := exec.CommandContext( //nolint:gosec // catalog validates Stroppy-owned executable path
		context.WithoutCancel(ctx), entry.ArtifactPath, args...,
	)
	command.Stdin = process.Stdin
	command.Stdout = process.Stdout
	command.Stderr = process.Stderr
	command.Env = process.Env

	if len(command.Env) == 0 {
		command.Env = os.Environ()
	}

	command.Dir = process.Dir
	if command.Dir == "" {
		command.Dir = filepath.Dir(entry.ArtifactPath)
	}

	configureProcessGroup(command)

	if err := command.Start(); err != nil {
		return fmt.Errorf("start workload %q: %w", name, err)
	}

	wait := make(chan error, 1)
	go func() { wait <- command.Wait() }()

	select {
	case err := <-wait:
		if err != nil {
			return fmt.Errorf("workload %q: %w", name, err)
		}
	case <-ctx.Done():
		terminateProcessGroup(command)
		<-wait

		return ctx.Err()
	}

	return nil
}

// Probe runs the standalone workload's machine-readable probe command.
func Probe(ctx context.Context, artifact string) (WorkloadProbe, error) {
	command := exec.CommandContext(ctx, artifact, "probe", "-o", "json")
	command.Env = os.Environ()
	command.Dir = filepath.Dir(artifact)

	var stdout, stderr bytes.Buffer

	command.Stdout = &stdout
	command.Stderr = &stderr

	if err := command.Run(); err != nil {
		return WorkloadProbe{}, errors.Join(
			ErrProbeContract,
			fmt.Errorf("run probe: %w: %s", err, stderr.String()),
		)
	}

	var envelope struct {
		Workloads []WorkloadProbe `json:"workloads"`
	}

	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		return WorkloadProbe{}, errors.Join(ErrProbeContract, fmt.Errorf("decode JSON: %w", err))
	}

	if len(envelope.Workloads) != 1 || envelope.Workloads[0].Name == "" {
		return WorkloadProbe{}, fmt.Errorf("%w: expected exactly one named workload", ErrProbeContract)
	}

	return envelope.Workloads[0], nil
}
