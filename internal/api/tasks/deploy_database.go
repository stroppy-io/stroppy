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
	quotaRepository   QuotaRepository
	deploymentActor   DeploymentActor
	deploymentBuilder DeploymentBuilder
}

func NewDeployDatabaseTaskHandler(
	quotaRepository QuotaRepository,
	deploymentActor DeploymentActor,
	deploymentBuilder DeploymentBuilder,
) *DeployDatabaseTaskHandler {
	return &DeployDatabaseTaskHandler{
		quotaRepository:   quotaRepository,
		deploymentActor:   deploymentActor,
		deploymentBuilder: deploymentBuilder,
	}
}
func (d *DeployDatabaseTaskHandler) Start(
	ctx context.Context,
	input *panel.WorkflowTask_DeployDatabase_Input,
) (*panel.WorkflowTask_DeployDatabase_Output, error) {
	if input.GetDatabaseInstanceParams().GetMachineInstance().GetPublicIp() {
		err := checkQuotaExceeded(ctx, d.quotaRepository, &crossplane.Quota{
			Cloud:   input.GetDatabaseInstanceParams().GetSupportedCloud(),
			Kind:    crossplane.Quota_KIND_PUBLIC_IP_ADDRESS,
			Current: 1,
		})
		if err != nil {
			return nil, err
		}
	}

	instanceParams := input.GetDatabaseInstanceParams()
	instanceTemplate := instanceParams.GetDatabaseDeploymentTemplate()
	machineTemplate := instanceTemplate.GetMachineDeployment()
	deploymentTemplate := instanceTemplate.GetDatabaseDeployment()
	vmDeploymentData := &crossplane.Deployment_Vm{
		PublicIp:    false,                     // NOTE: may be set outside in input
		InternalIp:  &crossplane.Ip{Value: ""}, // NOTE: this value will be set by DeploymentBuilder randomly for vm
		MachineInfo: machineTemplate.GetMachineInfo(),
		SshUser:     instanceParams.GetMachineInstance().GetSshUser(),
		Strategy: &crossplane.Deployment_Strategy{
			Strategy: &crossplane.Deployment_Strategy_PrebuiltImage_{
				PrebuiltImage: &crossplane.Deployment_Strategy_PrebuiltImage{
					ImageId: deploymentTemplate.GetPrebuiltImageId(),
				},
			},
		},
	}
	vmDeployment, err := d.deploymentBuilder.BuildVmDeployment(ctx,
		instanceParams.GetSupportedCloud(),
		input.GetStroppyRunId(),
		vmDeploymentData,
	)
	if err != nil {
		return nil, err
	}
	err = checkQuotasExceededOrDecrement(ctx, d.quotaRepository,
		instanceParams.GetSupportedCloud(),
		vmDeployment.GetUsingQuotas().GetQuotas(),
	)
	if err != nil {
		return nil, err
	}
	vmDeployment, err = d.deploymentActor.CreateDeployment(ctx, vmDeployment)
	if err != nil {
		return nil, err
	}
	return &panel.WorkflowTask_DeployDatabase_Output{
		DatabaseDeployment:         vmDeployment,
		DatabaseAssignedInternalIp: vmDeploymentData.GetInternalIp(),
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
	err = d.deploymentActor.DestroyDeployment(ctx, state.GetOutput().GetDatabaseDeployment())
	if err != nil {
		return err
	}
	return incrementQuotas(ctx, d.quotaRepository,
		state.GetInput().GetDatabaseInstanceParams().GetSupportedCloud(),
		state.GetOutput().GetDatabaseDeployment().GetUsingQuotas().GetQuotas(),
	)
}
