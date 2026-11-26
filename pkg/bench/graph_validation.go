package bench

func validateGraphSelection(g *Graph, selected map[*graphNode]bool) error {
	for _, node := range g.nodes {
		if !selected[node] || !g.execution.Enabled(node.name) {
			continue
		}

		for _, dependency := range node.dependencies {
			if dependency.event == "ordered" || dependency.event == "completed" || node.cleanup > 0 {
				continue
			}

			if !g.execution.Enabled(dependency.node.name) {
				return inputError("node %q requires excluded prerequisite %q", node.name, dependency.node.name)
			}
		}

		if node.auxiliary != nil && (!selected[node.auxiliary] || !g.execution.Enabled(node.auxiliary.name)) {
			return inputError("auxiliary node %q has no selected owner", node.name)
		}
	}

	return validateAuxiliaryLifetimes(g)
}

func validateAuxiliaryLifetimes(g *Graph) error {
	for _, auxiliary := range g.nodes {
		if auxiliary.auxiliary == nil {
			continue
		}

		seen := map[*graphNode]bool{auxiliary: true}
		for owner := auxiliary.auxiliary; owner != nil; owner = owner.auxiliary {
			if seen[owner] {
				return inputError("auxiliary ownership cycle at %q", auxiliary.name)
			}

			seen[owner] = true
		}

		if graphCompletionPath(auxiliary, auxiliary.auxiliary, make(map[*graphNode]bool)) {
			return inputError("auxiliary node %q cannot start after its owner completes", auxiliary.name)
		}

		if graphCompletionPath(auxiliary.auxiliary, auxiliary, make(map[*graphNode]bool)) {
			return inputError("auxiliary lifetime cycle between %q and %q", auxiliary.name, auxiliary.auxiliary.name)
		}
	}

	return nil
}
