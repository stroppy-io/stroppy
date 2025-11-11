import type { Status } from '@/proto/panel/types_pb.ts'

export type SortDirection = 'asc' | 'desc'

export interface AutomationsFilters {
  page: number
  pageSize: number
  onlyMine?: boolean
  orderByStatus?: SortDirection
  orderByCreatedAt?: SortDirection
}

export interface AutomationRunSummary {
  id?: string
  status?: Status
  createdAt?: Date
  updatedAt?: Date
  grafanaDashboardUrl?: string
}

export interface AutomationSummary {
  id: string
  status: Status
  createdAt?: Date
  updatedAt?: Date
  authorId?: string
  databaseRootResourceId?: string
  workloadRootResourceId?: string
  stroppyRun?: AutomationRunSummary
}
