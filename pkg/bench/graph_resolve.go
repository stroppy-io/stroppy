package bench

func (g *Graph) resolve(mode string) (GraphDescription, map[*graphNode]bool, error) {
	g = g.root()
	g.frozen = true
	description := GraphDescription{Name: g.name, Mode: mode}

	byName, err := g.validateNodes()
	if err != nil {
		return description, nil, err
	}

	selected, modes, err := g.selectMode(mode)
	if err != nil {
		return description, nil, err
	}

	description.Modes = modes
	for _, node := range g.nodes {
		description.Nodes = append(description.Nodes, g.describeNode(node, selected[node]))
	}

	description.Measurements, err = g.describeMeasurements(byName)
	if err == nil {
		err = validateGraphSessions(g, selected)
	}

	if err == nil {
		err = validateGraphSelection(g, selected)
	}

	if err == nil {
		err = validateGraphCapacity(g, selected)
	}

	return description, selected, err
}

func (g *Graph) validateNodes() (map[string]*graphNode, error) {
	byName := make(map[string]*graphNode)
	for _, node := range g.nodes {
		if byName[node.name] != nil {
			return nil, inputError("duplicate graph node %q", node.name)
		}

		byName[node.name] = node
		if _, err := graphStepOptions(g.execution, node); err != nil {
			return nil, err
		}
	}

	visiting, visited := make(map[*graphNode]bool), make(map[*graphNode]bool)
	for _, node := range g.nodes {
		if err := visitGraphNode(node, byName, visiting, visited); err != nil {
			return nil, err
		}
	}

	return byName, nil
}

func visitGraphNode(node *graphNode, byName map[string]*graphNode, visiting, visited map[*graphNode]bool) error {
	if visiting[node] {
		return inputError("graph cycle at %q", node.name)
	}

	if visited[node] {
		return nil
	}

	visiting[node] = true
	for _, dependency := range node.dependencies {
		if byName[dependency.node.name] != dependency.node {
			return inputError("missing dependency")
		}

		if !dependency.beforeAction {
			if err := visitGraphNode(dependency.node, byName, visiting, visited); err != nil {
				return err
			}
		}
	}

	delete(visiting, node)
	visited[node] = true

	return nil
}

func includeGraphNode(node *graphNode, selected map[*graphNode]bool) {
	if selected[node] {
		return
	}

	selected[node] = true
	for _, dependency := range node.dependencies {
		if dependency.event != "ordered" {
			includeGraphNode(dependency.node, selected)
		}
	}

	if node.auxiliary != nil {
		includeGraphNode(node.auxiliary, selected)
	}
}

func (g *Graph) selectMode(name string) (map[*graphNode]bool, []ModeDescription, error) {
	selected := make(map[*graphNode]bool)
	seen := make(map[string]bool)

	var modes []ModeDescription
	for _, mode := range g.modes {
		if seen[mode.name] {
			return nil, nil, inputError("duplicate mode %q", mode.name)
		}

		seen[mode.name] = true

		entry := ModeDescription{Name: mode.name, Description: mode.description, Available: mode.err == nil}
		if mode.err != nil {
			entry.Reason = mode.err.Error()
		}

		if mode.name == name && mode.err != nil {
			return nil, nil, mode.err
		}

		for _, node := range mode.members {
			entry.Nodes = append(entry.Nodes, node.name)
			if mode.name == name {
				includeGraphNode(node, selected)
			}
		}

		modes = append(modes, entry)
	}

	if len(g.modes) == 0 && name == "" {
		for _, node := range g.nodes {
			includeGraphNode(node, selected)
		}
	} else if !seen[name] {
		return nil, nil, inputError("unknown graph mode %q", name)
	}

	g.includeCleanup(selected)

	return selected, modes, nil
}

func (g *Graph) includeCleanup(selected map[*graphNode]bool) {
	for changed := true; changed; {
		changed = false

		for _, node := range g.nodes {
			if node.cleanup == 0 || selected[node] {
				continue
			}

			for _, dependency := range node.dependencies {
				if selected[dependency.node] {
					selected[node], changed = true, true

					break
				}
			}
		}
	}
}

func (g *Graph) describeNode(node *graphNode, selected bool) NodeDescription {
	options, _ := graphStepOptions(g.execution, node)

	entry := NodeDescription{
		Name: node.name, Selected: selected, Cleanup: node.cleanup > 0, Join: node.join, RetainedSessions: options.retained,
		Driver: options.driver.name, SignalsReady: node.ready, CleanupTimeout: node.cleanup,
		OptionalCleanup: node.optionalCleanup, Iterations: options.policy.iterations, Duration: options.policy.duration,
		DrainTimeout: options.policy.drain.duration, DrainUnlimited: options.policy.drain.unlimited,
		Policy: StepDescription{
			Name: node.name, Executor: options.policy.mode, Workers: options.policy.workers,
			Measured: options.measured || options.policy.mode != "once",
			Progress: policyProgressDescription(options.policy.mode),
		},
	}
	if node.auxiliary != nil {
		entry.AuxiliaryFor = node.auxiliary.name
	}

	if node.session != nil {
		entry.Session = node.session.name
	}

	for _, dependency := range node.dependencies {
		entry.Dependencies = append(entry.Dependencies, DependencyDescription{
			Node: dependency.node.name, Event: dependency.event, BeforeAction: dependency.beforeAction,
		})
	}

	return entry
}

func (g *Graph) describeMeasurements(byName map[string]*graphNode) ([]MeasurementDescription, error) {
	names := make(map[string]bool)

	var descriptions []MeasurementDescription

	for _, window := range g.measurements {
		if names[window.name] || byName[window.name] != nil {
			return nil, inputError("duplicate measurement %q", window.name)
		}

		names[window.name] = true
		entry := MeasurementDescription{Name: window.name}

		seen := make(map[*graphNode]bool)
		for _, node := range window.nodes {
			if seen[node] {
				return nil, inputError("duplicate measurement participant")
			}

			seen[node] = true
			entry.Nodes = append(entry.Nodes, node.name)
		}

		descriptions = append(descriptions, entry)
	}

	return descriptions, nil
}
