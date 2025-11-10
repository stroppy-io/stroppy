import { useMemo } from 'react'
import { useTransport, useQuery } from '@connectrpc/connect-query'
import { create } from '@bufbuild/protobuf'
import { timestampDate } from '@bufbuild/protobuf/wkt'
import type { ListRunsRequest, RunRecord } from '@/proto/panel/run_pb.ts'
import { ListRunsRequestSchema } from '@/proto/panel/run_pb.ts'
import {
  Tps_FilterSchema,
  MachineInfo_FilterSchema,
} from '@/proto/panel/types_pb.ts'
import { listRuns, listTopRuns } from '@/proto/panel/run-RunService_connectquery.ts'
import type { RunSummary, RunsFilters } from './types'

const toNumber = (value?: bigint) => (value !== undefined ? Number(value) : undefined)

const mapRunRecord = (record: RunRecord): RunSummary => ({
  id: record.id?.id ?? '',
  status: record.status,
  workloadName: record.workload?.name ?? '—',
  workloadType: record.workload?.workloadType,
  databaseName: record.database?.name ?? record.database?.databaseType?.toString(),
  databaseType: record.database?.databaseType,
  tpsAverage: toNumber(record.tps?.average),
  tpsP95: toNumber(record.tps?.p95th),
  createdAt: record.timing?.createdAt ? timestampDate(record.timing.createdAt) : undefined,
  updatedAt: record.timing?.updatedAt ? timestampDate(record.timing.updatedAt) : undefined,
})

const buildListRunsRequest = (filters: RunsFilters): ListRunsRequest => {
  const offset = Math.max(0, (filters.page - 1) * filters.pageSize)
  const request: ListRunsRequest = create(ListRunsRequestSchema, {
    limit: filters.pageSize,
    offset,
  })

  if (filters.status !== undefined) {
    request.status = filters.status
  }
  if (filters.workloadType !== undefined) {
    request.workloadType = filters.workloadType
  }
  if (filters.databaseType !== undefined) {
    request.databaseType = filters.databaseType
  }
  if (filters.workloadName) {
    request.workloadName = filters.workloadName
  }
  if (filters.databaseName) {
    request.databaseName = filters.databaseName
  }
  if (filters.tpsFilter) {
    const { value, parameterType, operator } = filters.tpsFilter
    request.tpsFilter = create(Tps_FilterSchema, {
      parameterType,
      operator,
      value: BigInt(Math.trunc(value)),
    })
  }
  if (filters.machineFilter) {
    const { value, parameterType, operator } = filters.machineFilter
    request.machineFilter = create(MachineInfo_FilterSchema, {
      parameterType,
      operator,
      value: BigInt(Math.trunc(value)),
    })
  }

  return request
}

export const useRunsQuery = (filters: RunsFilters) => {
  const transport = useTransport()
  const request = useMemo(() => buildListRunsRequest(filters), [filters])

  return useQuery(listRuns, request, {
    transport,
    select: (payload) => payload.records.map(mapRunRecord),
  })
}

export const useTopRunsQuery = (options?: { enabled?: boolean }) => {
  const transport = useTransport()
  return useQuery(listTopRuns, undefined, {
    transport,
    enabled: options?.enabled ?? true,
    select: (payload) => payload.records.map(mapRunRecord),
  })
}
