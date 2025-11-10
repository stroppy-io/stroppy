import { useMemo } from 'react'
import { useTransport, useQuery } from '@connectrpc/connect-query'
import { create } from '@bufbuild/protobuf'
import { timestampDate } from '@bufbuild/protobuf/wkt'
import type { ListRunsRequest, RunRecord } from '@/proto/panel/run_pb.ts'
import { ListRunsRequestSchema } from '@/proto/panel/run_pb.ts'
import {
  Tps_FilterSchema,
  Tps_OrderBySchema,
  MachineInfo_FilterSchema,
} from '@/proto/panel/types_pb.ts'
import { listRuns, listTopRuns } from '@/proto/panel/run-RunService_connectquery.ts'
import type { RunSummary, RunsFilters } from './types'

const toNumber = (value?: bigint) => (value !== undefined ? Number(value) : undefined)

type ClusterLike = {
  machines?: Array<{
    cores?: number
    memory?: number
    disk?: number
  }>
}

const summarizeCluster = (cluster?: ClusterLike) => {
  const machines = cluster?.machines ?? []
  const primary = machines[0]
  const buildSignature = () => {
    if (!primary) return undefined
    const parts = [
      primary.cores ? `${primary.cores} vCPU` : null,
      primary.memory ? `${primary.memory} GB RAM` : null,
      primary.disk ? `${primary.disk} GB SSD` : null,
    ].filter(Boolean)
    return parts.length ? parts.join(' · ') : undefined
  }
  return {
    nodes: machines.length || undefined,
    machineSignature: buildSignature(),
    machineCores: primary?.cores,
    machineMemory: primary?.memory,
    machineDisk: primary?.disk,
  }
}

const mapRunRecord = (record: RunRecord): RunSummary => {
  const runnerClusterMetrics = summarizeCluster(record.workload?.runnerCluster)
  const databaseClusterMetrics = summarizeCluster(record.database?.runnerCluster)
  return {
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
    cloudAutomationId: record.cloudAutomationId?.id,
    runnerClusterNodes: runnerClusterMetrics.nodes,
    runnerMachineSignature: runnerClusterMetrics.machineSignature,
    runnerMachineCores: runnerClusterMetrics.machineCores,
    runnerMachineMemory: runnerClusterMetrics.machineMemory,
    runnerMachineDisk: runnerClusterMetrics.machineDisk,
    databaseClusterNodes: databaseClusterMetrics.nodes,
    databaseMachineSignature: databaseClusterMetrics.machineSignature,
    databaseMachineCores: databaseClusterMetrics.machineCores,
    databaseMachineMemory: databaseClusterMetrics.machineMemory,
    databaseMachineDisk: databaseClusterMetrics.machineDisk,
  }
}

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
  if (filters.onlyMine) {
    request.onlyMine = filters.onlyMine
  }
  if (filters.tpsFilters?.length) {
    request.tpsFilter = filters.tpsFilters.map(({ value, parameterType, operator }) =>
      create(Tps_FilterSchema, {
        parameterType,
        operator,
        value: BigInt(Math.trunc(value)),
      }),
    )
  }
  if (filters.orderByTps) {
    request.orderByTps = create(Tps_OrderBySchema, {
      parameterType: filters.orderByTps.parameterType,
      descending: filters.orderByTps.descending,
    })
  }
  if (filters.machineFilters?.length) {
    request.machineFilter = filters.machineFilters.map(({ value, parameterType, operator }) =>
      create(MachineInfo_FilterSchema, {
        parameterType,
        operator,
        value: BigInt(Math.trunc(value)),
      }),
    )
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
