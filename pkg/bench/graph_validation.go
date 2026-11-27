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

type graphLifecycleEvent struct {
	node *graphNode
	kind string
}

func validateGraphLifecycles(g *Graph) error {
	visiting, visited := make(map[graphLifecycleEvent]bool), make(map[graphLifecycleEvent]bool)

	var visit func(graphLifecycleEvent) error

	visit = func(event graphLifecycleEvent) error {
		if visiting[event] {
			return inputError("graph lifecycle cycle at %q (%s)", event.node.name, event.kind)
		}

		if visited[event] {
			return nil
		}

		visiting[event] = true
		for _, prerequisite := range graphLifecycleWaits(g, event) {
			if err := visit(prerequisite); err != nil {
				return err
			}
		}

		delete(visiting, event)
		visited[event] = true

		return nil
	}
	for _, node := range g.nodes {
		if err := visit(graphLifecycleEvent{node, "done"}); err != nil {
			return err
		}
	}

	return nil
}

func graphLifecycleWaits(g *Graph, event graphLifecycleEvent) []graphLifecycleEvent {
	node := event.node

	var waits []graphLifecycleEvent

	switch event.kind {
	case "entry":
		waits = append(waits, graphDependencyWaits(node, false)...)
	case "ready":
		kind := "entry"
		if node.ready {
			kind = "started"
		}

		waits = append(waits, graphLifecycleEvent{node, kind})
	case "started":
		waits = append(waits, graphLifecycleEvent{node, "entry"})
		waits = append(waits, graphDependencyWaits(node, true)...)
	case "action_done":
		waits = append(waits, graphLifecycleEvent{node, "started"})
		if node.auxiliary != nil {
			waits = append(waits, graphLifecycleEvent{node.auxiliary, "action_done"})
		}
	case "done":
		waits = append(waits, graphLifecycleEvent{node, "action_done"})
		for _, auxiliary := range g.nodes {
			if auxiliary.auxiliary == node {
				waits = append(waits, graphLifecycleEvent{auxiliary, "done"})
			}
		}
	}

	return waits
}

func graphDependencyWaits(node *graphNode, beforeAction bool) []graphLifecycleEvent {
	var waits []graphLifecycleEvent

	for _, dependency := range node.dependencies {
		if dependency.beforeAction != beforeAction {
			continue
		}

		kind := "done"
		if node.cleanup == 0 && (dependency.event == "ready" || dependency.event == "started") {
			kind = dependency.event
		}

		waits = append(waits, graphLifecycleEvent{dependency.node, kind})
	}

	return waits
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

	return validateGraphLifecycles(g)
}
