package dag

import (
	"errors"
	"fmt"
	"strings"
)

// ErrEmptyID reports a Node with an empty ID.
var ErrEmptyID = errors.New("dag: empty node id")

// ErrDuplicateID reports two or more nodes sharing the same ID.
var ErrDuplicateID = errors.New("dag: duplicate node id")

// ErrUnknownDep reports an edge naming a node absent from the graph.
var ErrUnknownDep = errors.New("dag: unknown dependency")

// ErrCycle reports a cyclic dependency among nodes.
var ErrCycle = errors.New("dag: cyclic dependency")

// Graph is a mutable, unvalidated collection of [Node]s. Build validates
// it and returns an immutable [Built] graph the walker can run.
type Graph struct {
	nodes []*Node
}

// NewGraph returns an empty Graph.
func NewGraph() *Graph {
	return &Graph{}
}

// Add appends a node to the graph and returns g for chaining. Validation
// (duplicate IDs, unknown deps, cycles) happens in Build.
func (g *Graph) Add(n *Node) *Graph {
	g.nodes = append(g.nodes, n)
	return g
}

// Built is a validated, immutable graph ready to [Run] or render with
// [PlanText] / [PlanDOT].
type Built struct {
	nodes map[string]*Node
	// order is insertion order, used as a deterministic tie-break.
	order []string
	// topo is a valid topological order (deps before dependents).
	topo []string
	// dependents maps a node ID to the IDs that name it in an edge.
	dependents map[string][]string
	// watched is the number of unique dependency IDs a node names
	// across all its edges — the walker's in-degree counter.
	watched map[string]int
}

// Build validates the graph — empty/duplicate IDs, unknown dependencies,
// cycles — and returns an executable [Built] graph. The zero-value
// Graph builds to an empty, valid [Built] graph.
func (g *Graph) Build() (*Built, error) {
	nodes, order, err := indexNodes(g.nodes)
	if err != nil {
		return nil, err
	}

	if err := checkUnknownDeps(nodes); err != nil {
		return nil, err
	}

	if err := checkCycles(nodes, order); err != nil {
		return nil, err
	}

	dependents, watched := buildEdges(nodes, order)

	return &Built{
		nodes:      nodes,
		order:      order,
		topo:       topoOrder(order, watched, dependents),
		dependents: dependents,
		watched:    watched,
	}, nil
}

// allDeps returns every ID a node names across After, AfterAny and
// OnFailure, in declaration order (may contain duplicates).
func allDeps(n *Node) []string {
	out := make([]string, 0, len(n.After)+len(n.AfterAny)+len(n.OnFailure))
	out = append(out, n.After...)
	out = append(out, n.AfterAny...)
	out = append(out, n.OnFailure...)

	return out
}

func indexNodes(nodes []*Node) (map[string]*Node, []string, error) {
	byID := make(map[string]*Node, len(nodes))
	order := make([]string, 0, len(nodes))

	for _, n := range nodes {
		if n.ID == "" {
			return nil, nil, ErrEmptyID
		}

		if _, dup := byID[n.ID]; dup {
			return nil, nil, fmt.Errorf("%w: %q", ErrDuplicateID, n.ID)
		}

		byID[n.ID] = n
		order = append(order, n.ID)
	}

	return byID, order, nil
}

func checkUnknownDeps(nodes map[string]*Node) error {
	for id, n := range nodes {
		for _, d := range allDeps(n) {
			if _, ok := nodes[d]; !ok {
				return fmt.Errorf("%w: node %q references %q", ErrUnknownDep, id, d)
			}
		}
	}

	return nil
}

// checkCycles runs a depth-first, 3-color search over the depends-on
// relation. On a cycle it reports the path from the cycle's entry point
// back to itself.
func checkCycles(nodes map[string]*Node, order []string) error {
	const (
		white = 0
		gray  = 1
		black = 2
	)

	color := make(map[string]int, len(nodes))
	path := make([]string, 0, len(nodes))

	var visit func(id string) error
	visit = func(id string) error {
		color[id] = gray
		path = append(path, id)

		for _, d := range allDeps(nodes[id]) {
			switch color[d] {
			case gray:
				start := 0
				for i, p := range path {
					if p == d {
						start = i
						break
					}
				}

				cycle := append(append([]string{}, path[start:]...), d)

				return fmt.Errorf("%w: %s", ErrCycle, strings.Join(cycle, " -> "))
			case white:
				if err := visit(d); err != nil {
					return err
				}
			}
		}

		path = path[:len(path)-1]
		color[id] = black

		return nil
	}

	for _, id := range order {
		if color[id] == white {
			if err := visit(id); err != nil {
				return err
			}
		}
	}

	return nil
}

// buildEdges returns the reverse-edge map (dependency -> dependents) and,
// per node, the count of unique dependencies it names.
func buildEdges(nodes map[string]*Node, order []string) (map[string][]string, map[string]int) {
	dependents := make(map[string][]string, len(nodes))
	watched := make(map[string]int, len(nodes))

	for _, id := range order {
		seen := make(map[string]bool)
		count := 0

		for _, d := range allDeps(nodes[id]) {
			if seen[d] {
				continue
			}

			seen[d] = true
			count++
			dependents[d] = append(dependents[d], id)
		}

		watched[id] = count
	}

	return dependents, watched
}

// topoOrder runs Kahn's algorithm, draining the ready queue in the order
// nodes become ready and breaking initial ties by insertion order, so the
// result is deterministic for a fixed graph.
func topoOrder(order []string, watched map[string]int, dependents map[string][]string) []string {
	indeg := make(map[string]int, len(order))
	for id, c := range watched {
		indeg[id] = c
	}

	queue := make([]string, 0, len(order))
	for _, id := range order {
		if indeg[id] == 0 {
			queue = append(queue, id)
		}
	}

	result := make([]string, 0, len(order))
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		result = append(result, id)

		for _, dep := range dependents[id] {
			indeg[dep]--
			if indeg[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	return result
}
