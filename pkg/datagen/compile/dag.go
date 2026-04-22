package compile

import (
	"fmt"
	"sort"
	"strings"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// DAG is the compile-time dependency graph over a RelSource's attrs.
// Order lists the attrs topologically, with every producer preceding
// every consumer; attrs with no dependencies retain their declaration
// order. Index maps attr name to its position in Order.
type DAG struct {
	Order []*dgproto.Attr
	Index map[string]int
}

// Build validates attrs and returns a DAG with topologically sorted
// order. It reports ErrDuplicateAttr on repeated names, ErrUnknownRef
// when a ColRef targets an absent attr, and ErrCycle when the
// dependency graph is not acyclic. An empty attrs slice is valid and
// yields an empty DAG.
func Build(attrs []*dgproto.Attr) (*DAG, error) {
	nameToPos, err := indexByName(attrs)
	if err != nil {
		return nil, err
	}

	deps, err := collectDeps(attrs, nameToPos)
	if err != nil {
		return nil, err
	}

	order, err := topoSort(attrs, deps)
	if err != nil {
		return nil, err
	}

	index := make(map[string]int, len(order))
	for i, a := range order {
		index[a.GetName()] = i
	}

	// Assign StreamDraw / Choose IDs after topological ordering so that
	// numbering reflects evaluation order rather than raw declaration
	// order. Build mutates the input attrs; callers hand over ownership
	// at compile time.
	if err := AssignStreamIDs(order); err != nil {
		return nil, err
	}

	return &DAG{Order: order, Index: index}, nil
}

// indexByName returns a map from attr name to its declaration position,
// rejecting nil entries and duplicate names.
func indexByName(attrs []*dgproto.Attr) (map[string]int, error) {
	out := make(map[string]int, len(attrs))

	for i, a := range attrs {
		if a == nil {
			return nil, fmt.Errorf("%w: index %d", ErrNilAttr, i)
		}

		name := a.GetName()
		if _, dup := out[name]; dup {
			return nil, fmt.Errorf("%w: %q", ErrDuplicateAttr, name)
		}

		out[name] = i
	}

	return out, nil
}

// collectDeps returns, for each attr index, the set of indices of attrs
// it depends on. Dependency edges go from producer to consumer; a
// consumer's slice lists its producers. An unknown ColRef is reported
// as ErrUnknownRef naming both the referring attr and the missing name.
func collectDeps(attrs []*dgproto.Attr, nameToPos map[string]int) ([][]int, error) {
	deps := make([][]int, len(attrs))

	for i, attr := range attrs {
		refs := CollectColRefs(attr.GetExpr())
		if len(refs) == 0 {
			continue
		}

		seen := make(map[int]struct{}, len(refs))
		producers := make([]int, 0, len(refs))

		for _, ref := range refs {
			pos, ok := nameToPos[ref]
			if !ok {
				return nil, fmt.Errorf("%w: attr %q references %q", ErrUnknownRef, attr.GetName(), ref)
			}

			if _, dup := seen[pos]; dup {
				continue
			}

			seen[pos] = struct{}{}

			producers = append(producers, pos)
		}

		deps[i] = producers
	}

	return deps, nil
}

// topoSort runs Kahn's algorithm over attrs using deps. The ready queue
// is drained in ascending declaration index, so attrs with no
// dependencies emerge in their declared order. A non-empty remainder
// after the queue drains indicates a cycle.
func topoSort(attrs []*dgproto.Attr, deps [][]int) ([]*dgproto.Attr, error) {
	total := len(attrs)
	inDeg := make([]int, total)
	consumers := make([][]int, total)

	for i, producers := range deps {
		inDeg[i] = len(producers)
		for _, p := range producers {
			consumers[p] = append(consumers[p], i)
		}
	}

	ready := make([]int, 0, total)

	for i := range total {
		if inDeg[i] == 0 {
			ready = append(ready, i)
		}
	}

	order := make([]*dgproto.Attr, 0, total)

	for len(ready) > 0 {
		sort.Ints(ready)

		next := ready[0]
		ready = ready[1:]

		order = append(order, attrs[next])

		for _, c := range consumers[next] {
			inDeg[c]--
			if inDeg[c] == 0 {
				ready = append(ready, c)
			}
		}
	}

	if len(order) != total {
		return nil, cycleError(attrs, inDeg)
	}

	return order, nil
}

// cycleError builds an ErrCycle naming every attr that remained with a
// positive in-degree after Kahn's algorithm completed.
func cycleError(attrs []*dgproto.Attr, inDeg []int) error {
	names := make([]string, 0)

	for i, d := range inDeg {
		if d > 0 {
			names = append(names, attrs[i].GetName())
		}
	}

	sort.Strings(names)

	return fmt.Errorf("%w: involving %s", ErrCycle, strings.Join(names, ", "))
}
