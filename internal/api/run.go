package api

import (
	"connectrpc.com/connect"
	"context"
	"errors"
	"fmt"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/ids"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/orm"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql/sqlerr"
	"strings"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
	"google.golang.org/protobuf/types/known/emptypb"
)

var (
	ErrRunsNotFound         = connect.NewError(connect.CodeNotFound, errors.New("no runs found"))
	ErrTopRunsNotFound      = connect.NewError(connect.CodeNotFound, errors.New("no top runs found"))
	ErrInvalidTpsFilter     = connect.NewError(connect.CodeInvalidArgument, errors.New("invalid tps filter"))
	ErrInvalidMachineFilter = connect.NewError(connect.CodeInvalidArgument, errors.New("invalid machine filter"))
)

func (p *PanelService) numericOperator(op panel.NumberFilterOperator) string {
	switch op {
	case panel.NumberFilterOperator_TYPE_UNSPECIFIED:
		panic("invalid numericOperator")
	case panel.NumberFilterOperator_TYPE_EQUAL:
		return "="
	case panel.NumberFilterOperator_TYPE_NOT_EQUAL:
		return "!="
	case panel.NumberFilterOperator_TYPE_GREATER_THAN:
		return ">"
	case panel.NumberFilterOperator_TYPE_LESS_THAN:
		return "<"
	case panel.NumberFilterOperator_TYPE_GREATER_THAN_OR_EQUAL:
		return ">="
	case panel.NumberFilterOperator_TYPE_LESS_THAN_OR_EQUAL:
		return "<="
	}
	panic("invalid numericOperator")
}

func (p *PanelService) ListRuns(ctx context.Context, request *panel.ListRunsRequest) (*panel.RunRecord_List, error) {
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
	if request.GetWorkloadName() != "" {
		q = q.Where(orm.RunRecord.Raw("(workload->>'name')::text LIKE ?", request.GetWorkloadName()))
	}
	if request.GetWorkloadType() != panel.Workload_TYPE_UNSPECIFIED {
		q = q.Where(orm.RunRecord.Raw("(workload->>'workloadType')::text = ?", request.GetWorkloadType().String()))
	}
	if request.GetDatabaseName() != "" {
		q = q.Where(orm.RunRecord.Raw("(database->>'name')::text LIKE ?", request.GetDatabaseName()))
	}
	if request.GetDatabaseType() != panel.Database_TYPE_UNSPECIFIED {
		q = q.Where(orm.RunRecord.Raw("(database->>'databaseType')::text = ?", request.GetDatabaseType().String()))
	}
	if request.GetOnlyMine() {
		q = q.Where(orm.RunRecord.AuthorId.Eq(user.GetId().GetId()))
	}
	for _, filterData := range request.GetTpsFilter() {
		switch filterData.GetParameterType() {
		case panel.Tps_Filter_TYPE_UNSPECIFIED:
			return nil, ErrInvalidTpsFilter
		case panel.Tps_Filter_TYPE_AVERAGE:
			q = q.Where(orm.RunRecord.Raw(
				"(tps->>'average')::numeric "+p.numericOperator(filterData.GetOperator()),
				filterData.GetValue(),
			))
		case panel.Tps_Filter_TYPE_MAX:
			q = q.Where(orm.RunRecord.Raw(
				"(tps->>'max')::numeric "+p.numericOperator(filterData.GetOperator()),
				filterData.GetValue(),
			))
		case panel.Tps_Filter_TYPE_MIN:
			q = q.Where(orm.RunRecord.Raw(
				"(tps->>'min')::numeric "+p.numericOperator(filterData.GetOperator()),
				filterData.GetValue(),
			))
		case panel.Tps_Filter_TYPE_P95TH:
			q = q.Where(orm.RunRecord.Raw(
				"(tps->>'p95th')::numeric "+p.numericOperator(filterData.GetOperator()),
				filterData.GetValue(),
				filterData.GetValue(),
			))
		case panel.Tps_Filter_TYPE_P99TH:
			q = q.Where(orm.RunRecord.Raw(
				"(tps->>'p99th')::numeric "+p.numericOperator(filterData.GetOperator()),
				filterData.GetValue(),
			))
		}
	}
	for _, filterData := range request.GetMachineFilter() {
		filter := func(filterType string, operator panel.NumberFilterOperator) orm.Clause[orm.RunRecordField] {
			operatorStr := p.numericOperator(filterData.GetOperator())
			underWorkload := fmt.Sprintf(
				"(workload->'runnerCluster'->'%s') ?| array_agg(m.%s) AND (m.%s)::numeric %s ?",
				filterType, filterType, filterType, operatorStr,
			)
			underDatabase := fmt.Sprintf(
				"(database->'runnerCluster'->'%s') ?| array_agg(m.%s) AND (m.%s)::numeric %s ?",
				filterType, filterType, filterType, operatorStr,
			)
			return orm.RunRecord.Or(
				orm.RunRecord.ExistsRaw(
					fmt.Sprintf("SELECT 1 FROM jsonb_array_elements(%s) AS m WHERE %s", "workload->'runnerCluster'", underWorkload),
					filterData.GetValue(),
				),
				orm.RunRecord.ExistsRaw(
					fmt.Sprintf("SELECT 1 FROM jsonb_array_elements(%s) AS m WHERE %s", "database->'runnerCluster'", underDatabase),
					filterData.GetValue(),
				),
			)
		}
		switch filterData.GetParameterType() {
		case panel.MachineInfo_Filter_TYPE_UNSPECIFIED:
			return nil, ErrInvalidMachineFilter
		case panel.MachineInfo_Filter_TYPE_CORES:
			q = q.Where(filter("cores", filterData.GetOperator()))
		case panel.MachineInfo_Filter_TYPE_MEMORY:
			q = q.Where(filter("memory", filterData.GetOperator()))
		case panel.MachineInfo_Filter_TYPE_DISK:
			q = q.Where(filter("disk", filterData.GetOperator()))
		}
	}
	if request.GetOrderByTps() != nil {
		postfix := "ASC"
		if request.GetOrderByTps().GetDescending() {
			postfix = "DESC"
		}
		q = q.OrderByRaw(fmt.Sprintf(
			"(tps->>'%s') %s",
			strings.ToLower(
				strings.ReplaceAll(
					request.GetOrderByTps().GetParameterType().String(),
					"TYPE_",
					"",
				),
			),
			postfix))
	}
	records, err := p.runRecordRepo.ListBy(ctx, q)
	if err != nil {
		if sqlerr.IsNotFound(err) {
			return nil, ErrRunsNotFound
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if len(records) == 0 {
		return nil, ErrRunsNotFound
	}
	return &panel.RunRecord_List{
		Records: records,
	}, nil
}

func (p *PanelService) ListTopRuns(ctx context.Context, e *emptypb.Empty) (*panel.RunRecord_List, error) {
	records, err := p.runRecordRepo.ListBy(ctx, orm.RunRecord.SelectAll().
		Where(orm.RunRecord.Raw("(tps->>'average') IS NOT NULL AND (tps->>'average') <> ''")).
		OrderByRaw("(tps->'average') DESC").Limit(20),
	)
	if err != nil {
		if sqlerr.IsNotFound(err) {
			return nil, ErrTopRunsNotFound
		}
	}
	if len(records) == 0 {
		return nil, ErrTopRunsNotFound
	}
	return &panel.RunRecord_List{
		Records: records,
	}, nil
}

func (p *PanelService) AddRun(ctx context.Context, record *panel.RunRecord) (*panel.RunRecord, error) {
	user, err := p.getUserFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	record.Id = ids.NewUlid()
	record.AuthorId = user.Id
	insertedRecord, err := p.runRecordRepo.InsertRet(ctx, record)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return insertedRecord, nil
}
