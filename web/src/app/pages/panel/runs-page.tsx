import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import dayjs from 'dayjs'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Checkbox } from '@/components/ui/checkbox'
import { useRunsQuery } from '@/app/features/runs/api'
import { useTranslation } from '@/i18n/use-translation'
import { getStatusBadgeVariant, getStatusLabel } from '@/app/features/runs/utils'
import { ArrowUpDown, ChevronLeft, ChevronRight } from 'lucide-react'
import { getDatabaseTypeLabel, getMachineParameterLabel, getNumberOperatorLabel, getTpsMetricLabel, getWorkloadTypeLabel } from '@/app/features/runs/labels'
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

export const RunsPage = () => {
  const navigate = useNavigate()
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
  const [onlyMine, setOnlyMine] = useState(false)
  const [selectedRuns, setSelectedRuns] = useState<RunSummary[]>([])
  const [compareMessage, setCompareMessage] = useState<string | null>(null)
  const [sort, setSort] = useState<SortState>(null)

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
      onlyMine: onlyMine || undefined,
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
      onlyMine,
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
    navigate('/app/runs/compare', { state: { runs: selectedRuns } })
  }

  const handleClearSelection = () => {
    setCompareMessage(null)
    setSelectedRuns([])
  }

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
    onlyMine,
    parsedMachineFilters,
    sort,
  ])

  useEffect(() => {
    setPage(1)
  }, [pageSize])

  return (
    <div className="space-y-6">
      <header className="flex flex-col gap-2 md:flex-row md:items-end md:justify-between">
        <div>
          <p className="text-xs uppercase tracking-[0.5em] text-muted-foreground">{t('page.title')}</p>
          <h1 className="text-3xl font-semibold text-foreground">{t('page.subtitle')}</h1>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button onClick={() => navigate('/app/runs/new')}>{t('actions.createRun')}</Button>
        </div>
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
          <div className="flex items-center gap-2 rounded-lg border border-border/60 bg-muted/20 p-3">
            <Checkbox
              id="onlyMine"
              checked={onlyMine}
              onCheckedChange={(checked) => setOnlyMine(checked === true)}
            />
            <label htmlFor="onlyMine" className="text-sm text-foreground">
              {t('filters.onlyMine')}
            </label>
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
                  onCheckedChange={(checked) => handleSelectAllOnPage(checked === true)}
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
                    onCheckedChange={(checked) => handleRowSelectionChange(run, checked === true)}
                  />
                </TableCell>
                <TableCell className="font-mono text-xs text-muted-foreground">{run.id || '—'}</TableCell>
                <TableCell>
                  <Badge variant={getStatusBadgeVariant(run.status)}>{getStatusLabel(run.status, t)}</Badge>
                </TableCell>
                <TableCell>
                  <button
                    type="button"
                    onClick={() => navigate(`/app/runs/${run.id}`, { state: { run } })}
                    className="font-semibold text-primary transition hover:underline"
                  >
                    {run.workloadName}
                  </button>
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

    </div>
  )
}
