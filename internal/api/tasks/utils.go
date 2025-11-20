package tasks

import (
	"context"
	"errors"
	"slices"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/dag"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/quota"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/workflow"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgres/sqlerr"
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

func checkQuotaExceeded(
	ctx context.Context,
	quotaRepository QuotaRepository,
	needed *crossplane.Quota,
) error {
	available, err := quotaRepository.GetQuota(ctx, needed.GetCloud(), needed.GetKind())
	if err != nil {
		if sqlerr.IsNotFound(err) {
			return nil
		}
		return err
	}
	return quota.CheckQuotasExceeded([]*crossplane.Quota{needed}, []*crossplane.Quota{available})
}

func checkQuotasExceededOrDecrement(
	ctx context.Context,
	quotaRepository QuotaRepository,
	cloud crossplane.SupportedCloud,
	needed []*crossplane.Quota,
) error {
	quotas, err := quotaRepository.FindQuotas(ctx, cloud, quota.QuotasToKinds(needed))
	if err != nil {
		return err
	}
	exceededErr := quota.CheckQuotasExceeded(needed, quotas)
	if quota.IsExceededError(err) {
		return errors.Join(exceededErr, workflow.ErrStatusTemproraryFailed)
	}
	for _, q := range needed {
		err = quotaRepository.DecrementQuota(ctx, cloud, q.GetKind(), q.GetCurrent())
		if err != nil {
			return err
		}
	}
	return nil
}

func incrementQuotas(
	ctx context.Context,
	quotaRepository QuotaRepository,
	cloud crossplane.SupportedCloud,
	needed []*crossplane.Quota,
) error {
	for _, q := range needed {
		err := quotaRepository.IncrementQuota(ctx, cloud, q.GetKind(), q.GetCurrent())
		if err != nil {
			return err
		}
	}
	return nil
}
