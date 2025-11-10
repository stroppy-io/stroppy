import { useEffect, useMemo, useState } from 'react'
import dayjs from 'dayjs'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Checkbox } from '@/components/ui/checkbox'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { useRunsQuery } from '@/app/features/runs/api'
import { useTranslation } from '@/i18n/use-translation'
import { getStatusBadgeVariant, getStatusLabel } from '@/app/features/runs/utils'
import { ArrowUpDown, ChevronDown, ChevronLeft, ChevronRight } from 'lucide-react'
import {cn, scrollbarCn} from '@/lib/utils'
import type { NumericFilter, RunsFilters, RunSummary } from '@/app/features/runs/types'
import {
  Status,
  Workload_Type,
  Database_Type,
  Tps_Filter_Type,
  MachineInfo_Filter_Type,
  NumberFilterOperator,
} from '@/proto/panel/types_pb.ts'

type NumericFilterState<TParameter> = {
  parameterType: TParameter
  operator: NumberFilterOperator
  value: string
}

const createDefaultTpsFilterState = (): NumericFilterState<Tps_Filter_Type> => ({
  parameterType: Tps_Filter_Type.UNSPECIFIED,
  operator: NumberFilterOperator.TYPE_GREATER_THAN,
  value: '',
})

const createDefaultMachineFilterState = (): NumericFilterState<MachineInfo_Filter_Type> => ({
  parameterType: MachineInfo_Filter_Type.UNSPECIFIED,
  operator: NumberFilterOperator.TYPE_GREATER_THAN,
  value: '',
})

const ANY_SELECT_VALUE = '__any__'
const MAX_SELECTED_RUNS = 2
type SortableColumn = 'id' | 'status' | 'workloadName' | 'databaseName' | 'createdAt' | 'tpsAverage'
type SortState = { column: SortableColumn; direction: 'asc' | 'desc' } | null
type ComparisonField = {
  key: string
  label: string
  getRaw: (run: RunSummary) => string | number | undefined
  format?: (run: RunSummary) => string
}

interface FilterSelectProps {
  value: string
  onChange: (value: string) => void
  options: Array<{ value: string; label: string }>
  placeholder: string
  anyLabel?: string
  includeAny?: boolean
}

const FilterSelect = ({ value, onChange, options, placeholder, anyLabel, includeAny = true }: FilterSelectProps) => {
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger>
        <SelectValue placeholder={placeholder} />
      </SelectTrigger>
      <SelectContent>
        {includeAny && <SelectItem value={ANY_SELECT_VALUE}>{anyLabel ?? placeholder}</SelectItem>}
        {options.map((option) => (
          <SelectItem key={option.value} value={option.value}>
            {option.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

function parseNumericFilter<TParameter>(state: NumericFilterState<TParameter>): NumericFilter<TParameter> | undefined {
  if (!state.value) {
    return undefined
  }
  const numericValue = Number(state.value)
  if (!Number.isFinite(numericValue)) {
    return undefined
  }
  if (Number(state.parameterType) === 0 || Number(state.operator) === NumberFilterOperator.TYPE_UNSPECIFIED) {
    return undefined
  }
  return {
    parameterType: state.parameterType,
    operator: state.operator,
    value: numericValue,
  }
}

const isDefined = <T,>(value: T | undefined): value is T => value !== undefined

const statusOptions: Status[] = [Status.RUNNING, Status.COMPLETED, Status.FAILED, Status.CANCELED, Status.IDLE]
const workloadTypeValues: Workload_Type[] = [Workload_Type.TPCC]
const databaseTypeValues: Database_Type[] = [Database_Type.POSTGRES_ORIOLE]
const tpsMetricValues: Tps_Filter_Type[] = [
  Tps_Filter_Type.AVERAGE,
  Tps_Filter_Type.MAX,
  Tps_Filter_Type.MIN,
  Tps_Filter_Type.P95TH,
  Tps_Filter_Type.P99TH,
]
const machineParameterValues: MachineInfo_Filter_Type[] = [
  MachineInfo_Filter_Type.CORES,
  MachineInfo_Filter_Type.MEMORY,
  MachineInfo_Filter_Type.DISK,
]
const numberOperatorValues: NumberFilterOperator[] = [
  NumberFilterOperator.TYPE_GREATER_THAN,
  NumberFilterOperator.TYPE_GREATER_THAN_OR_EQUAL,
  NumberFilterOperator.TYPE_LESS_THAN,
  NumberFilterOperator.TYPE_LESS_THAN_OR_EQUAL,
  NumberFilterOperator.TYPE_EQUAL,
  NumberFilterOperator.TYPE_NOT_EQUAL,
]

const getWorkloadTypeLabel = (type: Workload_Type, t: (key: string) => string) => {
  switch (type) {
    case Workload_Type.TPCC:
      return t('filters.workloadTypes.tpcc')
    default:
      return t('filters.workloadTypes.unspecified')
  }
}

const getDatabaseTypeLabel = (type: Database_Type, t: (key: string) => string) => {
  switch (type) {
    case Database_Type.POSTGRES_ORIOLE:
      return t('filters.databaseTypes.postgresOriole')
    default:
      return t('filters.databaseTypes.unspecified')
  }
}

const getTpsMetricLabel = (metric: Tps_Filter_Type, t: (key: string) => string) => {
  switch (metric) {
    case Tps_Filter_Type.AVERAGE:
      return t('filters.tps.metrics.average')
    case Tps_Filter_Type.MAX:
      return t('filters.tps.metrics.max')
    case Tps_Filter_Type.MIN:
      return t('filters.tps.metrics.min')
    case Tps_Filter_Type.P95TH:
      return t('filters.tps.metrics.p95')
    case Tps_Filter_Type.P99TH:
      return t('filters.tps.metrics.p99')
    default:
      return t('filters.tps.metrics.unspecified')
  }
}

const getMachineParameterLabel = (param: MachineInfo_Filter_Type, t: (key: string) => string) => {
  switch (param) {
    case MachineInfo_Filter_Type.CORES:
      return t('filters.machine.parameters.cores')
    case MachineInfo_Filter_Type.MEMORY:
      return t('filters.machine.parameters.memory')
    case MachineInfo_Filter_Type.DISK:
      return t('filters.machine.parameters.disk')
    default:
      return t('filters.machine.parameters.unspecified')
  }
}

const getNumberOperatorLabel = (operator: NumberFilterOperator, t: (key: string) => string) => {
  switch (operator) {
    case NumberFilterOperator.TYPE_GREATER_THAN:
      return t('filters.operators.gt')
    case NumberFilterOperator.TYPE_GREATER_THAN_OR_EQUAL:
      return t('filters.operators.gte')
    case NumberFilterOperator.TYPE_LESS_THAN:
      return t('filters.operators.lt')
    case NumberFilterOperator.TYPE_LESS_THAN_OR_EQUAL:
      return t('filters.operators.lte')
    case NumberFilterOperator.TYPE_EQUAL:
      return t('filters.operators.eq')
    case NumberFilterOperator.TYPE_NOT_EQUAL:
      return t('filters.operators.neq')
    default:
      return t('filters.operators.default')
  }
}

const EMPTY_RUNS: RunSummary[] = []

const getSortValue = (run: RunSummary, column: SortableColumn): string | number | undefined => {
  switch (column) {
    case 'id':
      return run.id
    case 'status':
      return run.status
    case 'workloadName':
      return run.workloadName
    case 'databaseName':
      return run.databaseName
    case 'createdAt':
      return run.createdAt?.getTime()
    case 'tpsAverage':
      return run.tpsAverage
    default:
      return undefined
  }
}

const compareValues = (valueA?: string | number, valueB?: string | number) => {
  if (valueA === valueB) {
    return 0
  }
  if (valueA === undefined || valueA === null) {
    return 1
  }
  if (valueB === undefined || valueB === null) {
    return -1
  }
  if (typeof valueA === 'number' && typeof valueB === 'number') {
    return valueA - valueB
  }
  return valueA.toString().localeCompare(valueB.toString(), undefined, { sensitivity: 'base' })
}

const formatDefaultValue = (value?: string | number | null) => {
  if (value === undefined || value === null || value === '') {
    return '—'
  }
  return String(value)
}

export const RunsPage = () => {
  const { t } = useTranslation('runs')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [statusFilter, setStatusFilter] = useState<Status | undefined>()
  const [search, setSearch] = useState('')
  const [debouncedSearch, setDebouncedSearch] = useState('')
  const [databaseSearch, setDatabaseSearch] = useState('')
  const [debouncedDatabaseSearch, setDebouncedDatabaseSearch] = useState('')
  const [workloadTypeFilter, setWorkloadTypeFilter] = useState<Workload_Type | undefined>()
  const [databaseTypeFilter, setDatabaseTypeFilter] = useState<Database_Type | undefined>()
  const [tpsFiltersState, setTpsFiltersState] = useState<NumericFilterState<Tps_Filter_Type>[]>([createDefaultTpsFilterState()])
  const [machineFiltersState, setMachineFiltersState] = useState<NumericFilterState<MachineInfo_Filter_Type>[]>([
    createDefaultMachineFilterState(),
  ])
  const [selectedRuns, setSelectedRuns] = useState<RunSummary[]>([])
  const [compareMessage, setCompareMessage] = useState<string | null>(null)
  const [isCompareOpen, setCompareOpen] = useState(false)
  const [sort, setSort] = useState<SortState>(null)
  const [collapsedFields, setCollapsedFields] = useState<Record<string, boolean>>({})

  const parsedTpsFilters = useMemo(
    () => tpsFiltersState.map((state) => parseNumericFilter(state)).filter(isDefined),
    [tpsFiltersState],
  )
  const parsedMachineFilters = useMemo(
    () => machineFiltersState.map((state) => parseNumericFilter(state)).filter(isDefined),
    [machineFiltersState],
  )

  const orderByTps = useMemo(() => {
    if (!sort || sort.column !== 'tpsAverage') {
      return undefined
    }
    return {
      parameterType: Tps_Filter_Type.AVERAGE,
      descending: sort.direction === 'desc',
    }
  }, [sort])

  const filters: RunsFilters = useMemo(
    () => ({
      page,
      pageSize,
      status: statusFilter,
      workloadType: workloadTypeFilter,
      databaseType: databaseTypeFilter,
      workloadName: debouncedSearch || undefined,
      databaseName: debouncedDatabaseSearch || undefined,
      tpsFilters: parsedTpsFilters.length ? parsedTpsFilters : undefined,
      orderByTps,
      machineFilters: parsedMachineFilters.length ? parsedMachineFilters : undefined,
    }),
    [
      page,
      pageSize,
      statusFilter,
      workloadTypeFilter,
      databaseTypeFilter,
      debouncedSearch,
      debouncedDatabaseSearch,
      parsedTpsFilters,
      orderByTps,
      parsedMachineFilters,
    ],
  )

  const { data: runsData, isLoading } = useRunsQuery(filters)
  const runs = useMemo(() => {
    const base = runsData ?? EMPTY_RUNS
    if (!sort || sort.column === 'tpsAverage') {
      return base
    }
    const sorted = [...base].sort((a, b) => {
      const valueA = getSortValue(a, sort.column)
      const valueB = getSortValue(b, sort.column)
      const directionFactor = sort.direction === 'asc' ? 1 : -1
      return compareValues(valueA, valueB) * directionFactor
    })
    return sorted
  }, [runsData, sort])
  const selectedRunIds = useMemo(() => selectedRuns.map((run) => run.id), [selectedRuns])
  const selectedRunIdSet = useMemo(() => new Set(selectedRunIds), [selectedRunIds])
  const selectedCount = selectedRuns.length
  const canCompare = selectedCount === MAX_SELECTED_RUNS
  const pageRunIds = runs.map((run) => run.id)
  const selectedOnPageCount = runs.reduce((count, run) => (selectedRunIdSet.has(run.id) ? count + 1 : count), 0)
  const allSelectedOnPage = pageRunIds.length > 0 && selectedOnPageCount === pageRunIds.length
  const partiallySelectedOnPage = selectedOnPageCount > 0 && selectedOnPageCount < pageRunIds.length

  const resetTpsFilter = () => setTpsFiltersState([createDefaultTpsFilterState()])
  const resetMachineFilter = () => setMachineFiltersState([createDefaultMachineFilterState()])
  const hasTpsFilter = tpsFiltersState.some((filter) => filter.value !== '' || Number(filter.parameterType) !== 0)
  const hasMachineFilter = machineFiltersState.some((filter) => filter.value !== '' || Number(filter.parameterType) !== 0)

  const handleTpsFilterChange = (index: number, updates: Partial<NumericFilterState<Tps_Filter_Type>>) => {
    setTpsFiltersState((prev) => prev.map((filter, idx) => (idx === index ? { ...filter, ...updates } : filter)))
  }

  const handleAddTpsFilter = () => {
    setTpsFiltersState((prev) => [...prev, createDefaultTpsFilterState()])
  }

  const handleRemoveTpsFilter = (index: number) => {
    setTpsFiltersState((prev) => {
      if (prev.length === 1) {
        return prev
      }
      return prev.filter((_, idx) => idx !== index)
    })
  }

  const handleMachineFilterChange = (index: number, updates: Partial<NumericFilterState<MachineInfo_Filter_Type>>) => {
    setMachineFiltersState((prev) => prev.map((filter, idx) => (idx === index ? { ...filter, ...updates } : filter)))
  }

  const handleAddMachineFilter = () => {
    setMachineFiltersState((prev) => [...prev, createDefaultMachineFilterState()])
  }

  const handleRemoveMachineFilter = (index: number) => {
    setMachineFiltersState((prev) => {
      if (prev.length === 1) {
        return prev
      }
      return prev.filter((_, idx) => idx !== index)
    })
  }

  const handleRowSelectionChange = (run: RunSummary, checked: boolean) => {
    setSelectedRuns((prev) => {
      if (checked) {
        if (prev.find((item) => item.id === run.id)) {
          setCompareMessage(null)
          return prev
        }
        if (prev.length >= MAX_SELECTED_RUNS) {
          setCompareMessage(t('messages.selectExactlyTwoRuns'))
          return prev
        }
        setCompareMessage(null)
        return [...prev, run]
      }
      setCompareMessage(null)
      return prev.filter((item) => item.id !== run.id)
    })
  }

  const handleSelectAllOnPage = (checked: boolean) => {
    if (checked) {
      setSelectedRuns((prev) => {
        if (prev.length >= MAX_SELECTED_RUNS) {
          setCompareMessage(t('messages.selectExactlyTwoRuns'))
          return prev
        }
        const next = [...prev]
        for (const run of runs) {
          if (next.find((item) => item.id === run.id)) {
            continue
          }
          if (next.length >= MAX_SELECTED_RUNS) {
            break
          }
          next.push(run)
        }
        if (next.length === prev.length) {
          setCompareMessage(t('messages.selectExactlyTwoRuns'))
          return prev
        }
        setCompareMessage(null)
        return next
      })
      return
    }
    setCompareMessage(null)
    setSelectedRuns((prev) => prev.filter((run) => !pageRunIds.includes(run.id)))
  }

  const handleCompare = () => {
    if (selectedRuns.length === 0) {
      setCompareMessage(t('messages.selectRunsToCompare'))
      return
    }
    if (!canCompare) {
      setCompareMessage(t('messages.selectExactlyTwoRuns'))
      return
    }
    setCompareMessage(null)
    setCompareOpen(true)
  }

  const handleClearSelection = () => {
    setCompareMessage(null)
    setSelectedRuns([])
  }

  const toggleFieldCollapse = (key: string) =>
    setCollapsedFields((prev) => ({
      ...prev,
      [key]: !prev[key],
    }))

  const statusSelectOptions = useMemo(
    () =>
      statusOptions.map((status) => ({
        value: String(status),
        label: getStatusLabel(status, t),
      })),
    [t],
  )

  const workloadSelectOptions = useMemo(
    () =>
      workloadTypeValues.map((type) => ({
        value: String(type),
        label: getWorkloadTypeLabel(type, t),
      })),
    [t],
  )

  const databaseSelectOptions = useMemo(
    () =>
      databaseTypeValues.map((type) => ({
        value: String(type),
        label: getDatabaseTypeLabel(type, t),
      })),
    [t],
  )

  const tpsMetricSelectOptions = useMemo(
    () =>
      tpsMetricValues.map((metric) => ({
        value: String(metric),
        label: getTpsMetricLabel(metric, t),
      })),
    [t],
  )

  const machineMetricSelectOptions = useMemo(
    () =>
      machineParameterValues.map((parameter) => ({
        value: String(parameter),
        label: getMachineParameterLabel(parameter, t),
      })),
    [t],
  )

  const operatorSelectOptions = useMemo(
    () =>
      numberOperatorValues.map((operator) => ({
        value: String(operator),
        label: getNumberOperatorLabel(operator, t),
      })),
    [t],
  )

  const comparisonFieldSections = useMemo(
    () => [
      {
        key: 'status',
        label: t('comparison.statusSection'),
        fields: [
          {
            key: 'status',
            label: t('table.columns.status'),
            getRaw: (run: RunSummary) => run.status,
            format: (run: RunSummary) => getStatusLabel(run.status, t),
          },
          {
            key: 'workloadType',
            label: t('filters.workloadType'),
            getRaw: (run: RunSummary) => run.workloadType ?? -1,
            format: (run: RunSummary) =>
              run.workloadType !== undefined ? getWorkloadTypeLabel(run.workloadType, t) : t('filters.workloadTypes.unspecified'),
          },
          {
            key: 'databaseType',
            label: t('filters.databaseType'),
            getRaw: (run: RunSummary) => run.databaseType ?? -1,
            format: (run: RunSummary) =>
              run.databaseType !== undefined ? getDatabaseTypeLabel(run.databaseType, t) : t('filters.databaseTypes.unspecified'),
          },
        ],
      },
      {
        key: 'cluster',
        label: t('comparison.cluster.title'),
        fields: [
          {
            key: 'runnerClusterNodes',
            label: t('comparison.cluster.runnerNodes'),
            getRaw: (run: RunSummary) => run.runnerClusterNodes ?? -1,
            format: (run: RunSummary) => (run.runnerClusterNodes ? run.runnerClusterNodes.toString() : '—'),
          },
          {
            key: 'databaseClusterNodes',
            label: t('comparison.cluster.databaseNodes'),
            getRaw: (run: RunSummary) => run.databaseClusterNodes ?? -1,
            format: (run: RunSummary) => (run.databaseClusterNodes ? run.databaseClusterNodes.toString() : '—'),
          },
        ],
      },
      {
        key: 'runnerMachine',
        label: t('comparison.machine.runnerTitle'),
        fields: [
          {
            key: 'runnerMachineSignature',
            label: t('comparison.machine.runnerSignature'),
            getRaw: (run: RunSummary) => run.runnerMachineSignature ?? '',
            format: (run: RunSummary) => run.runnerMachineSignature ?? '—',
          },
          {
            key: 'runnerMachineCores',
            label: t('comparison.machine.runnerCores'),
            getRaw: (run: RunSummary) => run.runnerMachineCores ?? -1,
            format: (run: RunSummary) => (run.runnerMachineCores ? `${run.runnerMachineCores} vCPU` : '—'),
          },
          {
            key: 'runnerMachineMemory',
            label: t('comparison.machine.runnerMemory'),
            getRaw: (run: RunSummary) => run.runnerMachineMemory ?? -1,
            format: (run: RunSummary) => (run.runnerMachineMemory ? `${run.runnerMachineMemory} GB` : '—'),
          },
          {
            key: 'runnerMachineDisk',
            label: t('comparison.machine.runnerDisk'),
            getRaw: (run: RunSummary) => run.runnerMachineDisk ?? -1,
            format: (run: RunSummary) => (run.runnerMachineDisk ? `${run.runnerMachineDisk} GB` : '—'),
          },
        ],
      },
      {
        key: 'databaseMachine',
        label: t('comparison.machine.databaseTitle'),
        fields: [
          {
            key: 'databaseMachineSignature',
            label: t('comparison.machine.databaseSignature'),
            getRaw: (run: RunSummary) => run.databaseMachineSignature ?? '',
            format: (run: RunSummary) => run.databaseMachineSignature ?? '—',
          },
          {
            key: 'databaseMachineCores',
            label: t('comparison.machine.databaseCores'),
            getRaw: (run: RunSummary) => run.databaseMachineCores ?? -1,
            format: (run: RunSummary) => (run.databaseMachineCores ? `${run.databaseMachineCores} vCPU` : '—'),
          },
          {
            key: 'databaseMachineMemory',
            label: t('comparison.machine.databaseMemory'),
            getRaw: (run: RunSummary) => run.databaseMachineMemory ?? -1,
            format: (run: RunSummary) => (run.databaseMachineMemory ? `${run.databaseMachineMemory} GB` : '—'),
          },
          {
            key: 'databaseMachineDisk',
            label: t('comparison.machine.databaseDisk'),
            getRaw: (run: RunSummary) => run.databaseMachineDisk ?? -1,
            format: (run: RunSummary) => (run.databaseMachineDisk ? `${run.databaseMachineDisk} GB` : '—'),
          },
        ],
      },
      {
        key: 'metrics',
        label: t('comparison.metrics.title'),
        fields: [
          {
            key: 'tpsAverage',
            label: t('table.columns.tps'),
            getRaw: (run: RunSummary) => run.tpsAverage ?? -1,
            format: (run: RunSummary) => (run.tpsAverage ? `${run.tpsAverage.toLocaleString()} TPS` : '—'),
          },
          {
            key: 'tpsP95',
            label: t('filters.tps.metrics.p95'),
            getRaw: (run: RunSummary) => run.tpsP95 ?? -1,
            format: (run: RunSummary) => (run.tpsP95 ? `${run.tpsP95.toLocaleString()} TPS` : '—'),
          },
        ],
      },
    ],
    [t],
  )

  const comparisonDifferenceCount = useMemo(() => {
    if (!canCompare || selectedRuns.length < 2) {
      return 0
    }
    const [runA, runB] = selectedRuns
    if (!runA || !runB) {
      return 0
    }
    return comparisonFieldSections.reduce((sectionCount, section) => {
      const fieldDiffs = section.fields.reduce((count, field) => {
        const rawA = field.getRaw(runA)
        const rawB = field.getRaw(runB)
        return rawA === rawB ? count : count + 1
      }, 0)
      return sectionCount + fieldDiffs
    }, 0)
  }, [canCompare, comparisonFieldSections, selectedRuns])

  const handleSort = (column: SortableColumn) => {
    setSort((prev) => {
      if (!prev || prev.column !== column) {
        return { column, direction: 'asc' }
      }
      if (prev.direction === 'asc') {
        return { column, direction: 'desc' }
      }
      return null
    })
  }

  const getAriaSort = (column: SortableColumn): 'ascending' | 'descending' | 'none' => {
    if (sort?.column !== column) {
      return 'none'
    }
    return sort.direction === 'asc' ? 'ascending' : 'descending'
  }

  useEffect(() => {
    const timeout = setTimeout(() => setDebouncedSearch(search.trim()), 300)
    return () => clearTimeout(timeout)
  }, [search])

  useEffect(() => {
    const timeout = setTimeout(() => setDebouncedDatabaseSearch(databaseSearch.trim()), 300)
    return () => clearTimeout(timeout)
  }, [databaseSearch])

  useEffect(() => {
    setPage(1)
  }, [
    statusFilter,
    debouncedSearch,
    debouncedDatabaseSearch,
    workloadTypeFilter,
    databaseTypeFilter,
    parsedTpsFilters,
    parsedMachineFilters,
    sort,
  ])

  useEffect(() => {
    setPage(1)
  }, [pageSize])

  useEffect(() => {
    if (!canCompare) {
      setCompareOpen(false)
    }
  }, [canCompare])

  return (
    <div className="space-y-6">
      <header className="flex flex-col gap-2">
        <p className="text-xs uppercase tracking-[0.5em] text-muted-foreground">{t('page.title')}</p>
        <h1 className="text-3xl font-semibold text-foreground">{t('page.subtitle')}</h1>
      </header>

      <Card className="flex flex-col gap-4 rounded-2xl p-4 shadow-sm">
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
          <div className="space-y-2">
            <label className="text-xs uppercase tracking-[0.3em] text-muted-foreground">{t('filters.search')}</label>
            <Input value={search} onChange={(event) => setSearch(event.target.value)} placeholder={t('filters.search')} />
          </div>
          <div className="space-y-2">
            <label className="text-xs uppercase tracking-[0.3em] text-muted-foreground">{t('filters.databaseName')}</label>
            <Input value={databaseSearch} onChange={(event) => setDatabaseSearch(event.target.value)} placeholder={t('filters.databaseName')} />
          </div>
          <div className="space-y-2">
            <label className="text-xs uppercase tracking-[0.3em] text-muted-foreground">{t('filters.status')}</label>
            <FilterSelect
              value={statusFilter !== undefined ? statusFilter.toString() : ANY_SELECT_VALUE}
              onChange={(value) => {
                setStatusFilter(value === ANY_SELECT_VALUE ? undefined : (Number(value) as Status))
              }}
              options={statusSelectOptions}
              placeholder={t('filters.status')}
              anyLabel={t('filters.any')}
            />
          </div>
        </div>

        <div className="grid gap-4 md:grid-cols-2">
          <div className="space-y-2">
            <label className="text-xs uppercase tracking-[0.3em] text-muted-foreground">{t('filters.workloadType')}</label>
            <FilterSelect
              value={workloadTypeFilter !== undefined ? workloadTypeFilter.toString() : ANY_SELECT_VALUE}
              onChange={(value) => {
                setWorkloadTypeFilter(value === ANY_SELECT_VALUE ? undefined : (Number(value) as Workload_Type))
              }}
              options={workloadSelectOptions}
              placeholder={t('filters.workloadType')}
              anyLabel={t('filters.any')}
            />
          </div>
          <div className="space-y-2">
            <label className="text-xs uppercase tracking-[0.3em] text-muted-foreground">{t('filters.databaseType')}</label>
            <FilterSelect
              value={databaseTypeFilter !== undefined ? databaseTypeFilter.toString() : ANY_SELECT_VALUE}
              onChange={(value) => {
                setDatabaseTypeFilter(value === ANY_SELECT_VALUE ? undefined : (Number(value) as Database_Type))
              }}
              options={databaseSelectOptions}
              placeholder={t('filters.databaseType')}
              anyLabel={t('filters.any')}
            />
          </div>
        </div>

        <div className="grid gap-4 md:grid-cols-2">
          <div className="space-y-2 rounded-2xl border border-dashed border-border/70 p-4">
            <div className="flex items-center justify-between">
              <p className="text-xs uppercase tracking-[0.3em] text-muted-foreground">{t('filters.tps.title')}</p>
              <Button type="button" size="sm" variant="ghost" onClick={resetTpsFilter} disabled={!hasTpsFilter}>
                {t('filters.tps.reset')}
              </Button>
            </div>
            <div className="space-y-3">
              {tpsFiltersState.map((filterState, index) => (
                <div key={`tps-filter-${index}`} className="grid gap-2 sm:grid-cols-3">
                  <FilterSelect
                    value={
                      filterState.parameterType !== Tps_Filter_Type.UNSPECIFIED ? filterState.parameterType.toString() : ANY_SELECT_VALUE
                    }
                    onChange={(value) =>
                      handleTpsFilterChange(index, {
                        parameterType: value === ANY_SELECT_VALUE ? Tps_Filter_Type.UNSPECIFIED : (Number(value) as Tps_Filter_Type),
                      })
                    }
                    options={tpsMetricSelectOptions}
                    placeholder={t('filters.tps.metric')}
                    anyLabel={t('filters.any')}
                  />
                  <FilterSelect
                    value={filterState.operator.toString()}
                    onChange={(value) =>
                      handleTpsFilterChange(index, {
                        operator: Number(value) as NumberFilterOperator,
                      })
                    }
                    includeAny={false}
                    options={operatorSelectOptions}
                    placeholder={t('filters.tps.operator')}
                  />
                  <div className="flex gap-2">
                    <Input
                      type="number"
                      inputMode="numeric"
                      value={filterState.value}
                      onChange={(event) => handleTpsFilterChange(index, { value: event.target.value })}
                      placeholder={t('filters.tps.value')}
                      className="flex-1"
                    />
                    {tpsFiltersState.length > 1 && (
                      <Button type="button" variant="ghost" onClick={() => handleRemoveTpsFilter(index)}>
                        {t('filters.tps.remove')}
                      </Button>
                    )}
                  </div>
                </div>
              ))}
            </div>
            <Button type="button" variant="outline" size="sm" className="mt-2 w-full sm:w-auto" onClick={handleAddTpsFilter}>
              {t('filters.tps.add')}
            </Button>
          </div>

          <div className="space-y-2 rounded-2xl border border-dashed border-border/70 p-4">
            <div className="flex items-center justify-between">
              <p className="text-xs uppercase tracking-[0.3em] text-muted-foreground">{t('filters.machine.title')}</p>
              <Button type="button" size="sm" variant="ghost" onClick={resetMachineFilter} disabled={!hasMachineFilter}>
                {t('filters.machine.reset')}
              </Button>
            </div>
            <div className="space-y-3">
              {machineFiltersState.map((filterState, index) => (
                <div key={`machine-filter-${index}`} className="grid gap-2 sm:grid-cols-3">
                  <FilterSelect
                    value={
                      filterState.parameterType !== MachineInfo_Filter_Type.UNSPECIFIED
                        ? filterState.parameterType.toString()
                        : ANY_SELECT_VALUE
                    }
                    onChange={(value) =>
                      handleMachineFilterChange(index, {
                        parameterType:
                          value === ANY_SELECT_VALUE ? MachineInfo_Filter_Type.UNSPECIFIED : (Number(value) as MachineInfo_Filter_Type),
                      })
                    }
                    options={machineMetricSelectOptions}
                    placeholder={t('filters.machine.metric')}
                    anyLabel={t('filters.any')}
                  />
                  <FilterSelect
                    value={filterState.operator.toString()}
                    onChange={(value) =>
                      handleMachineFilterChange(index, {
                        operator: Number(value) as NumberFilterOperator,
                      })
                    }
                    includeAny={false}
                    options={operatorSelectOptions}
                    placeholder={t('filters.machine.operator')}
                  />
                  <div className="flex gap-2">
                    <Input
                      type="number"
                      inputMode="numeric"
                      value={filterState.value}
                      onChange={(event) => handleMachineFilterChange(index, { value: event.target.value })}
                      placeholder={t('filters.machine.value')}
                      className="flex-1"
                    />
                    {machineFiltersState.length > 1 && (
                      <Button type="button" variant="ghost" onClick={() => handleRemoveMachineFilter(index)}>
                        {t('filters.machine.remove')}
                      </Button>
                    )}
                  </div>
                </div>
              ))}
            </div>
            <Button type="button" variant="outline" size="sm" className="mt-2 w-full sm:w-auto" onClick={handleAddMachineFilter}>
              {t('filters.machine.add')}
            </Button>
          </div>
        </div>
      </Card>

      <Card className="overflow-hidden rounded-2xl">
        <div className="flex flex-col gap-2 border-b border-border/70 px-4 py-3">
          <div className="flex flex-wrap items-center gap-3">
            <p className="text-sm text-muted-foreground">
              {t('modals.compareRuns.selectRuns')}
              <span className="ml-2 font-semibold text-foreground">
                {selectedCount}/{MAX_SELECTED_RUNS}
              </span>
            </p>
            <Button type="button" size="sm" onClick={handleCompare} disabled={!canCompare}>
              {t('modals.compareRuns.compare')}
            </Button>
            <Button type="button" variant="ghost" size="sm" onClick={handleClearSelection} disabled={!selectedCount}>
              {t('actions.clearSelection')}
            </Button>
          </div>
          {compareMessage && <p className="text-xs text-destructive">{compareMessage}</p>}
        </div>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-10">
                <Checkbox
                  aria-label={t('comparison.title')}
                  checked={allSelectedOnPage}
                  indeterminate={partiallySelectedOnPage}
                  onCheckedChange={(checked) => handleSelectAllOnPage(Boolean(checked))}
                />
              </TableHead>
              <TableHead
                className="w-32 text-xs uppercase tracking-[0.3em] text-muted-foreground"
                aria-sort={getAriaSort('id')}
              >
                <button
                  type="button"
                  className="flex w-full items-center gap-1"
                  onClick={() => handleSort('id')}
                >
                  {t('table.columns.id')}
                  <ArrowUpDown
                    className={`h-4 w-4 ${sort?.column === 'id' ? 'text-foreground' : 'text-muted-foreground'}`}
                  />
                </button>
              </TableHead>
              <TableHead
                className="text-xs uppercase tracking-[0.3em] text-muted-foreground"
                aria-sort={getAriaSort('status')}
              >
                <button
                  type="button"
                  className="flex w-full items-center gap-1"
                  onClick={() => handleSort('status')}
                >
                  {t('table.columns.status')}
                  <ArrowUpDown
                    className={`h-4 w-4 ${sort?.column === 'status' ? 'text-foreground' : 'text-muted-foreground'}`}
                  />
                </button>
              </TableHead>
              <TableHead
                className="text-xs uppercase tracking-[0.3em] text-muted-foreground"
                aria-sort={getAriaSort('workloadName')}
              >
                <button
                  type="button"
                  className="flex w-full items-center gap-1"
                  onClick={() => handleSort('workloadName')}
                >
                  {t('table.columns.name')}
                  <ArrowUpDown
                    className={`h-4 w-4 ${sort?.column === 'workloadName' ? 'text-foreground' : 'text-muted-foreground'}`}
                  />
                </button>
              </TableHead>
              <TableHead
                className="text-xs uppercase tracking-[0.3em] text-muted-foreground"
                aria-sort={getAriaSort('tpsAverage')}
              >
                <button
                  type="button"
                  className="flex w-full items-center gap-1"
                  onClick={() => handleSort('tpsAverage')}
                >
                  {t('table.columns.tps')}
                  <ArrowUpDown
                    className={`h-4 w-4 ${sort?.column === 'tpsAverage' ? 'text-foreground' : 'text-muted-foreground'}`}
                  />
                </button>
              </TableHead>
              <TableHead
                className="text-xs uppercase tracking-[0.3em] text-muted-foreground"
                aria-sort={getAriaSort('createdAt')}
              >
                <button
                  type="button"
                  className="flex w-full items-center gap-1"
                  onClick={() => handleSort('createdAt')}
                >
                  {t('table.columns.createdAt')}
                  <ArrowUpDown
                    className={`h-4 w-4 ${sort?.column === 'createdAt' ? 'text-foreground' : 'text-muted-foreground'}`}
                  />
                </button>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {runs.map((run) => (
              <TableRow key={`${run.id}-${run.createdAt?.toISOString() ?? ''}`}>
                <TableCell className="w-10">
                  <Checkbox
                    aria-label={t('modals.compareRuns.selectRuns')}
                    checked={selectedRunIdSet.has(run.id)}
                    onCheckedChange={(checked) => handleRowSelectionChange(run, Boolean(checked))}
                  />
                </TableCell>
                <TableCell className="font-mono text-xs text-muted-foreground">{run.id || '—'}</TableCell>
                <TableCell>
                  <Badge variant={getStatusBadgeVariant(run.status)}>{getStatusLabel(run.status, t)}</Badge>
                </TableCell>
                <TableCell>
                  <div className="font-semibold text-foreground">{run.workloadName}</div>
                  <p className="text-xs text-muted-foreground">{run.databaseName ?? t('meta.unknownDatabase')}</p>
                </TableCell>
                <TableCell className="font-mono text-sm text-foreground/80">
                  {run.tpsAverage ? `${run.tpsAverage.toLocaleString('ru-RU')} TPS` : '—'}
                </TableCell>
                <TableCell className="text-sm text-muted-foreground">
                  {run.createdAt ? dayjs(run.createdAt).format('DD MMM, HH:mm') : t('meta.unknownDate')}
                </TableCell>
              </TableRow>
            ))}
            {!isLoading && runs.length === 0 && (
              <TableRow>
                <TableCell colSpan={6} className="py-10 text-center text-sm text-muted-foreground">
                  {t('list.empty')}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
        <div className="flex flex-col gap-3 border-t border-border/70 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-2">
            <p className="text-sm font-medium text-foreground">{t('list.rowsPerPage')}</p>
            <Select value={pageSize.toString()} onValueChange={(value) => setPageSize(Number(value))}>
              <SelectTrigger className="h-8 w-20">
                <SelectValue placeholder={pageSize.toString()} />
              </SelectTrigger>
              <SelectContent side="top">
                {[10, 20, 30, 40, 50].map((size) => (
                  <SelectItem key={size} value={size.toString()}>
                    {size}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="flex items-center gap-2">
            <p className="text-sm text-muted-foreground">{t('list.page', { page })}</p>
            <div className="flex items-center gap-1">
              <Button type="button" variant="outline" size="icon" className="h-8 w-8" onClick={() => setPage((prev) => Math.max(1, prev - 1))} disabled={page === 1}>
                <span className="sr-only">{t('list.previous')}</span>
                <ChevronLeft className="h-4 w-4" />
              </Button>
              <Button
                type="button"
                variant="outline"
                size="icon"
                className="h-8 w-8"
                onClick={() => setPage((prev) => prev + 1)}
                disabled={isLoading || runs.length < pageSize}
              >
                <span className="sr-only">{t('list.next')}</span>
                <ChevronRight className="h-4 w-4" />
              </Button>
            </div>
          </div>
        </div>
      </Card>

      <Dialog open={isCompareOpen && canCompare} onOpenChange={setCompareOpen}>
        <DialogContent className={scrollbarCn("max-h-[90vh] w-[96vw] max-w-5xl overflow-y-auto")}>
          <DialogHeader>
            <DialogTitle>{t('comparison.title')}</DialogTitle>
            <DialogDescription>{t('comparison.differencesHighlighted')}</DialogDescription>
          </DialogHeader>
          {canCompare &&
            selectedRuns.length === 2 &&
            (() => {
              const runA = selectedRuns[0]
              const runB = selectedRuns[1]
              return (
                <div className="space-y-6">
                  <div className="grid gap-4 sm:grid-cols-2">
                    <div className="rounded-2xl border border-border/80 bg-muted/20 p-4">
                      <div className="flex items-center justify-between gap-2">
                        <p className="text-xs uppercase tracking-[0.3em] text-muted-foreground">{t('comparison.run1')}</p>
                        <Badge variant="secondary">{getStatusLabel(runA.status, t)}</Badge>
                      </div>
                      <p className="mt-2 text-base font-semibold text-foreground">{runA.workloadName ?? '—'}</p>
                      <p className="text-xs text-muted-foreground">{runA.id}</p>
                    </div>
                    <div className="rounded-2xl border border-border/80 bg-muted/20 p-4">
                      <div className="flex items-center justify-between gap-2">
                        <p className="text-xs uppercase tracking-[0.3em] text-muted-foreground">{t('comparison.run2')}</p>
                        <Badge variant="secondary">{getStatusLabel(runB.status, t)}</Badge>
                      </div>
                      <p className="mt-2 text-base font-semibold text-foreground">{runB.workloadName ?? '—'}</p>
                      <p className="text-xs text-muted-foreground">{runB.id}</p>
                    </div>
                  </div>
                  <div className="flex flex-wrap items-center gap-3 rounded-2xl border border-border/80 bg-card/60 p-4">
                    <Badge variant="outline" className="text-xs">
                      {t('comparison.totalDifferences')}: {comparisonDifferenceCount}
                    </Badge>
                    <p className="text-sm text-muted-foreground">
                      {comparisonDifferenceCount > 0 ? t('comparison.differencesFound') : t('comparison.identical')}
                    </p>
                  </div>
                  <div className="space-y-6">
                    {comparisonFieldSections.map((section) => (
                      <div key={section.key} className="space-y-3">
                        <div className="flex items-center gap-2">
                          <span className="text-xs font-semibold uppercase tracking-[0.3em] text-muted-foreground">
                            {section.label}
                          </span>
                          <div className="h-px flex-1 bg-border/60" />
                        </div>
                        {section.fields.map((field) => {
                      const rawA = field.getRaw(runA)
                      const rawB = field.getRaw(runB)
                      const displayA = field.format ? field.format(runA) : formatDefaultValue(rawA)
                      const displayB = field.format ? field.format(runB) : formatDefaultValue(rawB)
                      const isDifferent = rawA !== rawB
                      const isCollapsed = collapsedFields[field.key] ?? false
                      return (
                        <div
                          key={field.key}
                          className={cn(
                            'rounded-2xl border border-border/80 bg-card/50 p-4 text-sm shadow-sm',
                            isDifferent && 'border-primary/70 bg-primary/5 shadow-primary/10',
                          )}
                        >
                          <div className="flex flex-wrap items-center justify-between gap-2">
                            <span className="text-xs font-semibold uppercase tracking-[0.3em] text-muted-foreground">{field.label}</span>
                            <div className="flex items-center gap-2">
                              <Badge variant={isDifferent ? 'default' : 'secondary'} className="text-[11px]">
                                {isDifferent ? t('comparison.different') : t('comparison.identical')}
                              </Badge>
                              <Button
                                type="button"
                                variant="ghost"
                                size="icon"
                                className="h-8 w-8"
                                onClick={() => toggleFieldCollapse(field.key)}
                                aria-label={isCollapsed ? t('comparison.expand') : t('comparison.collapse')}
                              >
                                <ChevronDown
                                  className={cn(
                                    'h-4 w-4 transition-transform',
                                    isCollapsed ? '-rotate-90' : 'rotate-0',
                                  )}
                                />
                              </Button>
                            </div>
                          </div>
                          <div className={cn('mt-3 grid gap-3 sm:grid-cols-2', isCollapsed && 'hidden')}>
                            <div className="rounded-xl border border-border/70 bg-background/80 p-3">
                              <p className="text-xs uppercase tracking-[0.2em] text-muted-foreground">{t('comparison.run1')}</p>
                              <p className="mt-1 text-sm font-medium text-foreground">{displayA}</p>
                            </div>
                            <div className="rounded-xl border border-border/70 bg-background/80 p-3">
                              <p className="text-xs uppercase tracking-[0.2em] text-muted-foreground">{t('comparison.run2')}</p>
                              <p className="mt-1 text-sm font-medium text-foreground">{displayB}</p>
                            </div>
                          </div>
                        </div>
                      )
                        })}
                      </div>
                    ))}
                  </div>
                </div>
              )
            })()}
        </DialogContent>
      </Dialog>
    </div>
  )
}
