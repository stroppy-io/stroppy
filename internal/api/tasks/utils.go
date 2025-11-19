package tasks

import (
	"slices"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/dag"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
)

func allDagNodesStatus(dagData *crossplane.ResourceDag, status crossplane.Resource_Status) bool {
	if dagData == nil {
		return false
	}
	return dag.AllBy[string, *crossplane.ResourceDag_Node, *crossplane.ResourceDag_Edge](dagData,
		func(node *crossplane.ResourceDag_Node) bool {
			return node.GetResource().GetStatus() == status
		})
}

func anyDagNodeInStatuses(dagData *crossplane.ResourceDag, statuses []crossplane.Resource_Status) bool {
	if dagData == nil {
		return false
	}
	return dag.AnyBy[string, *crossplane.ResourceDag_Node, *crossplane.ResourceDag_Edge](dagData,
		func(node *crossplane.ResourceDag_Node) bool {
			return slices.Contains(statuses, node.GetResource().GetStatus())
		})
}
