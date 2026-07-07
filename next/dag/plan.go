package dag

import (
	"fmt"
	"strings"
)

// PlanText renders b as an indented list in topological order. Since b
// may be a genuine DAG (a node can have several parents), indentation
// reflects each node's depth — the longest dependency chain beneath it —
// rather than a literal, duplicated tree. Each line annotates the node's
// edges, If predicate, failure policy and retry bound.
func PlanText(b *Built) string {
	depth := nodeDepths(b)

	var sb strings.Builder
	for _, id := range b.topo {
		n := b.nodes[id]
		sb.WriteString(strings.Repeat("  ", depth[id]))
		sb.WriteString(planLine(n))
		sb.WriteByte('\n')
	}

	return sb.String()
}

func planLine(n *Node) string {
	var sb strings.Builder
	sb.WriteString(n.ID)

	writeEdgeList(&sb, "after", n.After)
	writeEdgeList(&sb, "afterAny", n.AfterAny)
	writeEdgeList(&sb, "onFailure", n.OnFailure)

	if n.If != nil {
		sb.WriteString(" if")
	}

	sb.WriteString(" policy=")
	sb.WriteString(n.Failure.String())

	return sb.String()
}

func writeEdgeList(sb *strings.Builder, label string, ids []string) {
	if len(ids) == 0 {
		return
	}

	fmt.Fprintf(sb, " %s=[%s]", label, strings.Join(ids, ","))
}

// nodeDepths returns, for every node, the length of its longest
// dependency chain (a root has depth 0).
func nodeDepths(b *Built) map[string]int {
	depth := make(map[string]int, len(b.topo))

	for _, id := range b.topo {
		deps := allDeps(b.nodes[id])
		if len(deps) == 0 {
			depth[id] = 0
			continue
		}

		max := 0
		for _, d := range deps {
			if depth[d]+1 > max {
				max = depth[d] + 1
			}
		}

		depth[id] = max
	}

	return depth
}

// PlanDOT renders b as Graphviz DOT: one edge per dependency, directed
// from dependency to dependent (execution order), labeled by edge kind.
func PlanDOT(b *Built) string {
	var sb strings.Builder
	sb.WriteString("digraph dag {\n")

	for _, id := range b.topo {
		n := b.nodes[id]
		fmt.Fprintf(&sb, "  %q [policy=%q];\n", id, n.Failure.String())
		writeDotEdges(&sb, n.ID, n.After, "after", false)
		writeDotEdges(&sb, n.ID, n.AfterAny, "afterAny", false)
		writeDotEdges(&sb, n.ID, n.OnFailure, "onFailure", true)
	}

	sb.WriteString("}\n")

	return sb.String()
}

func writeDotEdges(sb *strings.Builder, to string, from []string, label string, dashed bool) {
	for _, d := range from {
		if dashed {
			fmt.Fprintf(sb, "  %q -> %q [label=%q, style=dashed];\n", d, to, label)
		} else {
			fmt.Fprintf(sb, "  %q -> %q [label=%q];\n", d, to, label)
		}
	}
}
