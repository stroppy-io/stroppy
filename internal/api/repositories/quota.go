package repositories

import (
	"context"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/quota"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
)

type QuotaRepository struct {
}

func (q QuotaRepository) FindQuotas(
	ctx context.Context,
	cloud crossplane.SupportedCloud,
	kinds []quota.QKind,
) ([]*crossplane.Quota, error) {
	//TODO implement me
	panic("implement me")
}

func (q QuotaRepository) IncrementQuota(
	ctx context.Context,
	cloud crossplane.SupportedCloud,
	kind quota.QKind,
	added uint32,
) error {
	//TODO implement me
	panic("implement me")
}

func (q QuotaRepository) DecrementQuota(
	ctx context.Context,
	cloud crossplane.SupportedCloud,
	kind quota.QKind,
	subtracted uint32,
) error {
	//TODO implement me
	panic("implement me")
}
