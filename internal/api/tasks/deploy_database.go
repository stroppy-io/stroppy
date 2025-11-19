package tasks

import (
	"context"
	"errors"
	"fmt"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/workflow"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
)

type deployDatabaseTaskState = workflow.TaskState[*panel.WorkflowTask_DeployDatabase_Input, *panel.WorkflowTask_DeployDatabase_Output]
type DeployDatabaseTaskHandler struct {
	deploymentActor   DeploymentActor
	deploymentBuilder DeploymentBuilder
}

func NewDeployDatabaseTaskHandler(
	deploymentActor DeploymentActor,
	deploymentBuilder DeploymentBuilder,
) *DeployDatabaseTaskHandler {
	return &DeployDatabaseTaskHandler{
		deploymentActor:   deploymentActor,
		deploymentBuilder: deploymentBuilder,
	}
}
func (d *DeployDatabaseTaskHandler) Start(
	ctx context.Context,
	input *panel.WorkflowTask_DeployDatabase_Input,
) (*panel.WorkflowTask_DeployDatabase_Output, error) {
	instanceParams := input.GetDatabaseInstanceParams()
	instanceTemplate := instanceParams.GetDatabaseDeploymentTemplate()
	machineTemplate := instanceTemplate.GetMachineDeployment()
	deploymentTemplate := instanceTemplate.GetDatabaseDeployment()
	deployment, err := d.deploymentBuilder.BuildVmDeployment(ctx,
		instanceParams.GetSupportedCloud(),
		&crossplane.Deployment_Vm{
			PublicIp:    false, // NOTE: may be set outside in input
			InternalIp:  "",    // NOTE: this value will be set by DeploymentBuilder
			MachineInfo: machineTemplate.GetMachineInfo(),
			Strategy: &crossplane.Deployment_Strategy{
				Strategy: &crossplane.Deployment_Strategy_PrebuiltImage_{
					PrebuiltImage: &crossplane.Deployment_Strategy_PrebuiltImage{
						ImageId: deploymentTemplate.GetPrebuiltImageId(),
					},
				},
			},
		},
	)
	deployment, err = d.deploymentActor.CreateDeployment(ctx, deployment)
	if err != nil {
		return nil, err
	}
	return &panel.WorkflowTask_DeployDatabase_Output{
		DatabaseDeployment: deployment,
	}, nil
}

func (d *DeployDatabaseTaskHandler) Status(
	ctx context.Context,
	state deployDatabaseTaskState,
) (panel.WorkflowTask_Status, error) {
	deployment, err := d.deploymentActor.ProcessDeploymentStatus(ctx, state.GetOutput().GetDatabaseDeployment())
	if err != nil {
		return panel.WorkflowTask_STATUS_UNSPECIFIED,
			errors.Join(
				err,
				workflow.ErrStatusTemproraryFailed,
				fmt.Errorf("failed to process deployment status"),
			)
	}
	state.GetOutput().DatabaseDeployment = deployment
	allResourcesReady := allDagNodesStatus(
		deployment.GetResourceDag(),
		crossplane.Resource_STATUS_READY,
	)
	if allResourcesReady {
		return panel.WorkflowTask_STATUS_COMPLETED, nil
	}
	anyDagNodeFailed := anyDagNodeInStatuses(
		deployment.GetResourceDag(),
		[]crossplane.Resource_Status{
			crossplane.Resource_STATUS_DESTROYING,
			crossplane.Resource_STATUS_DESTROYED,
			crossplane.Resource_STATUS_DEGRADED,
		},
	)
	if anyDagNodeFailed {
		return panel.WorkflowTask_STATUS_FAILED, nil
	}
	return panel.WorkflowTask_STATUS_RUNNING, nil
}

func (d *DeployDatabaseTaskHandler) Cleanup(
	ctx context.Context,
	state deployDatabaseTaskState,
) (err error) {
	if state.GetOutput().GetDatabaseDeployment() != nil {
		return d.deploymentActor.DestroyDeployment(ctx, state.GetOutput().GetDatabaseDeployment())
	}
	return nil
}
