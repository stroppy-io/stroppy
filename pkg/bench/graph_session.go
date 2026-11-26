package bench

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

type graphSession struct {
	name      string
	driver    DriverRef
	mu        sync.Mutex
	backend   driver.Session
	remaining int
	invalid   bool
}

func (b *Bench) graphSession() *graphSession {
	if b.execution == nil || b.execution.session == nil {
		return nil
	}

	owner := b.execution.session

	name := b.databaseName
	if name == "default" {
		name = ""
	}

	owned := owner.driver.name
	if owned == "default" {
		owned = ""
	}

	if name != owned {
		return nil
	}

	return owner
}

func (s *graphSession) open(ctx context.Context, b *Bench) (driver.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.invalid {
		return nil, ErrSessionInvalidated
	}

	if s.backend != nil {
		return s.backend, nil
	}

	if err := b.ensureDriver(ctx); err != nil {
		return nil, err
	}

	provider, ok := b.drv.(driver.SessionProvider)
	if !ok {
		return nil, ErrSessionUnsupported
	}

	acquire, cancel := context.WithTimeout(ctx, defaultCleanupTimeout)
	defer cancel()

	backend, err := provider.OpenSession(b.operationContext(acquire))
	if err != nil {
		return nil, err
	}

	s.backend = backend

	return backend, nil
}

func (s *graphSession) close(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.closeLocked(ctx)
}

func (s *graphSession) closeLocked(ctx context.Context) error {
	if s.backend == nil {
		return nil
	}

	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), defaultCleanupTimeout)
	defer cancel()

	backend := s.backend
	s.backend = nil

	return backend.Close(cleanup)
}

func (s *graphSession) fail(err error) {
	if !errors.Is(err, ErrSessionInvalidated) {
		return
	}

	s.mu.Lock()
	s.invalid = true
	s.mu.Unlock()
}

func (s *graphSession) release(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.remaining--
	if s.remaining == 0 {
		return s.closeLocked(ctx)
	}

	return nil
}

func validateGraphSessions(g *Graph, selected map[*graphNode]bool) error {
	names := make(map[string]bool)
	for _, session := range g.sessions {
		if names[session.name] {
			return inputError("duplicate graph session %q", session.name)
		}

		names[session.name] = true
	}

	for index, node := range g.nodes {
		if node.session == nil || !selected[node] {
			continue
		}

		options, err := graphStepOptions(g.execution, node)
		if err != nil {
			return err
		}

		if options.policy.workers != 1 {
			return inputError("graph session requires one worker per node")
		}

		for _, other := range g.nodes[:index] {
			shared := selected[other] && node.session == other.session
			if shared && !graphCompletionPath(node, other, make(map[*graphNode]bool)) &&
				!graphCompletionPath(other, node, make(map[*graphNode]bool)) {
				return fmt.Errorf("%w: session %q shared by concurrent nodes %q and %q", ErrSessionCapacity,
					node.session.name, other.name, node.name)
			}
		}
	}

	return nil
}

func graphCompletionPath(node, prerequisite *graphNode, visited map[*graphNode]bool) bool {
	return graphResourcePath(node, prerequisite, visited, false)
}

func graphResourcePath(node, prerequisite *graphNode, visited map[*graphNode]bool, terminal bool) bool {
	if node == prerequisite {
		return true
	}

	if visited[node] {
		return false
	}

	visited[node] = true
	for _, dependency := range node.dependencies {
		if (dependency.beforeAction && !terminal) || dependency.event == "started" || dependency.event == "ready" {
			continue
		}

		if graphResourcePath(dependency.node, prerequisite, visited, true) {
			return true
		}
	}

	return false
}
