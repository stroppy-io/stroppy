import { useMemo } from 'react'
import { create } from '@bufbuild/protobuf'
import { timestampDate } from '@bufbuild/protobuf/wkt'
import { useTransport, useQuery } from '@connectrpc/connect-query'
import { listAutomations } from '@/proto/panel/automate-AutomateService_connectquery.ts'
import { ListAutomationsRequestSchema } from '@/proto/panel/automate_pb.ts'
import { OrderByTimestampSchema, Status } from '@/proto/panel/types_pb.ts'
import type { CloudAutomation } from '@/proto/panel/automate_pb.ts'
import type { AutomationSummary, AutomationsFilters, AutomationRunSummary } from './types'

const mapStroppyRun = (stroppyRun?: CloudAutomation['stroppyRun']): AutomationRunSummary | undefined => {
  if (!stroppyRun) {
    return undefined
  }

  const createdAt = stroppyRun.timing?.createdAt ? timestampDate(stroppyRun.timing.createdAt) : undefined
  const updatedAt = stroppyRun.timing?.updatedAt ? timestampDate(stroppyRun.timing.updatedAt) : undefined

  return {
    id: stroppyRun.id?.id,
    // panel.Status and stroppy.Status share the same enum values, so we can safely reuse helpers.
    status: stroppyRun.status as Status,
    createdAt,
    updatedAt,
    grafanaDashboardUrl: stroppyRun.grafanaDashboardUrl || undefined,
  }
}

const mapAutomation = (automation: CloudAutomation): AutomationSummary => ({
  id: automation.id?.id ?? '',
  status: automation.status,
  createdAt: automation.timing?.createdAt ? timestampDate(automation.timing.createdAt) : undefined,
  updatedAt: automation.timing?.updatedAt ? timestampDate(automation.timing.updatedAt) : undefined,
  authorId: automation.authorId?.id,
  databaseRootResourceId: automation.databaseRootResourceId?.id,
  workloadRootResourceId: automation.workloadRootResourceId?.id,
  stroppyRun: mapStroppyRun(automation.stroppyRun),
})

const buildListRequest = (filters: AutomationsFilters) => {
  const offset = Math.max(0, (filters.page - 1) * filters.pageSize)
  const orderByStatusAsc = filters.orderByStatus === 'asc'
  const orderByStatusDesc = filters.orderByStatus === 'desc'
  const orderByCreatedAtDirection = filters.orderByCreatedAt

  return create(ListAutomationsRequestSchema, {
    limit: filters.pageSize,
    offset,
    onlyMine: filters.onlyMine,
    orderByStatus: orderByStatusAsc ? Status.IDLE : undefined,
    orderByStatusDescending: orderByStatusDesc ? true : undefined,
    orderByCreatedAt: orderByCreatedAtDirection
      ? create(OrderByTimestampSchema, {
          descending: orderByCreatedAtDirection === 'desc',
        })
      : undefined,
  })
}

export const useAutomationsQuery = (filters: AutomationsFilters) => {
  const transport = useTransport()
  const request = useMemo(() => buildListRequest(filters), [filters])

  return useQuery(listAutomations, request, {
    transport,
    select: (payload) => payload.automations.map(mapAutomation),
  })
}
