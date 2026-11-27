package report

import (
	"encoding/json"
	"errors"
	"fmt"
)

const ExecutionGraphsKind = "stroppy.execution_graphs"

var errExecutionGraphSchema = errors.New("unsupported execution graph schema")

// ExecutionGraphs decodes runtime topology without extending the Run struct.
func (run *Run) ExecutionGraphs() ([]Graph, error) {
	if run == nil {
		return nil, nil
	}

	for _, contribution := range run.WorkloadReports {
		if contribution.Kind != ExecutionGraphsKind {
			continue
		}

		if contribution.Schema != 1 {
			return nil, fmt.Errorf("%w %d", errExecutionGraphSchema, contribution.Schema)
		}

		var graphs []Graph
		if err := json.Unmarshal(contribution.Data, &graphs); err != nil {
			return nil, err
		}

		return graphs, nil
	}

	return nil, nil
}

// Graph records the selected execution topology and its realized node outcomes.
// It is separate from flat step totals so overlapping work remains inspectable.
type Graph struct {
	Name         string             `json:"name"`
	Mode         string             `json:"mode"`
	Nodes        []GraphNode        `json:"nodes"`
	Measurements []GraphMeasurement `json:"measurements,omitempty"`
}

type GraphNode struct {
	Name            string            `json:"name"`
	Status          string            `json:"status"`
	Dependencies    []GraphDependency `json:"dependencies,omitempty"`
	Workers         int               `json:"workers"`
	Executor        string            `json:"executor"`
	StartedSeconds  *float64          `json:"started_seconds,omitempty"`
	FinishedSeconds *float64          `json:"finished_seconds,omitempty"`
	Join            bool              `json:"join,omitempty"`
	Cleanup         bool              `json:"cleanup,omitempty"`
	AuxiliaryFor    string            `json:"auxiliary_for,omitempty"`
	Reason          string            `json:"reason,omitempty"`
}

type GraphDependency struct {
	Node         string `json:"node"`
	Event        string `json:"event"`
	BeforeAction bool   `json:"before_action,omitempty"`
}

type GraphMeasurement struct {
	Name    string   `json:"name"`
	Nodes   []string `json:"nodes"`
	Seconds *float64 `json:"seconds,omitempty"`
}
