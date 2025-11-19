package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/orm"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
)

func (p *PanelService) ListRuns(
	ctx context.Context,
	request *panel.ListRunsRequest,
) (*panel.RunRecord_List, error) {
	user, err := p.getUserFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	q := orm.RunRecord.SelectAll()
	if limit := request.GetLimit(); limit != 0 {
		q = q.Limit(int(limit))
	}
	if offset := request.GetOffset(); offset != 0 {
		q = q.Offset(int(offset))
	}
	if request.Status != nil {
		q = q.Where(orm.RunRecord.Status.Eq(int32(request.GetStatus())))
	}
	if request.GetId() != "" {
		q = q.Where(orm.RunRecord.Id.Eq(request.GetId()))
	}
	if request.GetOnlyMine() {
		q = q.Where(orm.RunRecord.AuthorId.Eq(user.GetId().GetId()))
	}
	if request.GetTpsOrder() != nil {
		postfix := "ASC"
		if request.GetTpsOrder().GetDescending() {
			postfix = "DESC"
		}
		q = q.OrderByRaw(fmt.Sprintf(
			"(tps->>'%s') %s",
			strings.ToLower(
				strings.ReplaceAll(
					request.GetTpsOrder().GetParameterType().String(),
					"Tps_Order_TYPE_",
					"",
				),
			),
			postfix))
	}
	return p.runRecordRepo.ListBy(ctx, q)
}

func (p *PanelService) RunStroppyInCloud(
	ctx context.Context,
	request *panel.CloudRunParams,
) (*panel.RunRecord, error) {

}
