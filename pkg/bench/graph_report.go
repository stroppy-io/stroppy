package bench

import (
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func (g *Graph) report(runs map[*graphNode]*graphNodeRun, started time.Time) report.Graph {
	value := report.Graph{Name: g.name}
	if len(g.execution.graphs) > 0 {
		value.Mode = g.execution.graphs[len(g.execution.graphs)-1].Mode
	}

	for _, node := range g.nodes {
		run := runs[node]
		options, _ := graphStepOptions(g.execution, node)

		entry := report.GraphNode{
			Name: node.name, Status: string(run.result.Status), Workers: options.policy.workers,
			Executor: options.policy.mode, Cleanup: node.cleanup > 0, Join: node.join,
		}
		if node.auxiliary != nil {
			entry.AuxiliaryFor = node.auxiliary.name
		}

		if run.err != nil {
			entry.Reason = boundReportError(run.err, g.execution.root.reportRedactor)
		}

		for _, dependency := range node.dependencies {
			entry.Dependencies = append(entry.Dependencies, report.GraphDependency{
				Node: dependency.node.name, Event: dependency.event, BeforeAction: dependency.beforeAction,
			})
		}

		if !run.startedAt.IsZero() {
			seconds := run.startedAt.Sub(started).Seconds()
			entry.StartedSeconds = &seconds
		}

		if !run.finishedAt.IsZero() {
			seconds := run.finishedAt.Sub(started).Seconds()
			entry.FinishedSeconds = &seconds
		}

		value.Nodes = append(value.Nodes, entry)
	}

	for _, window := range g.measurements {
		entry := report.GraphMeasurement{Name: window.name}
		for _, node := range window.nodes {
			entry.Nodes = append(entry.Nodes, node.name)
		}

		if seconds, ok := g.execution.measurements[window.name]; ok {
			entry.Seconds = &seconds
		}

		value.Measurements = append(value.Measurements, entry)
	}

	return value
}
