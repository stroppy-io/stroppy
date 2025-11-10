import { useEffect, useMemo, useRef, useState } from 'react'
import dayjs from 'dayjs'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { useRunsQuery } from '@/app/features/runs/api'
import { useTranslation } from '@/i18n/use-translation'
import { getStatusBadgeVariant, getStatusLabel } from '@/app/features/runs/utils'
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

export const RunsPage = () => {
  const { t } = useTranslation('runs')
  const [page, setPage] = useState(1)
  const [pageSize] = useState(20)
  const [statusFilter, setStatusFilter] = useState<Status | undefined>()
  const [search, setSearch] = useState('')
  const [debouncedSearch, setDebouncedSearch] = useState('')
  const [databaseSearch, setDatabaseSearch] = useState('')
  const [debouncedDatabaseSearch, setDebouncedDatabaseSearch] = useState('')
  const [workloadTypeFilter, setWorkloadTypeFilter] = useState<Workload_Type | undefined>()
  const [databaseTypeFilter, setDatabaseTypeFilter] = useState<Database_Type | undefined>()
  const [tpsFilterState, setTpsFilterState] = useState<NumericFilterState<Tps_Filter_Type>>(() => createDefaultTpsFilterState())
  const [machineFilterState, setMachineFilterState] = useState<NumericFilterState<MachineInfo_Filter_Type>>(() => createDefaultMachineFilterState())
  const [renderedRuns, setRenderedRuns] = useState<RunSummary[]>([])

  const parsedTpsFilter = useMemo(() => parseNumericFilter(tpsFilterState), [tpsFilterState])
  const parsedMachineFilter = useMemo(() => parseNumericFilter(machineFilterState), [machineFilterState])

  const filters: RunsFilters = useMemo(
    () => ({
      page,
      pageSize,
      status: statusFilter,
      workloadType: workloadTypeFilter,
      databaseType: databaseTypeFilter,
      workloadName: debouncedSearch || undefined,
      databaseName: debouncedDatabaseSearch || undefined,
      tpsFilter: parsedTpsFilter,
      machineFilter: parsedMachineFilter,
    }),
    [
      page,
      pageSize,
      statusFilter,
      workloadTypeFilter,
      databaseTypeFilter,
      debouncedSearch,
      debouncedDatabaseSearch,
      parsedTpsFilter,
      parsedMachineFilter,
    ],
  )

  const { data: runsData, isLoading, dataUpdatedAt } = useRunsQuery(filters)
  const runs = runsData ?? EMPTY_RUNS
  const lastUpdateRef = useRef<number>(0)
  const lastPageRef = useRef<number>(1)

  const resetTpsFilter = () => setTpsFilterState(createDefaultTpsFilterState())
  const resetMachineFilter = () => setMachineFilterState(createDefaultMachineFilterState())
  const hasTpsFilter = tpsFilterState.value !== '' || Number(tpsFilterState.parameterType) !== 0
  const hasMachineFilter = machineFilterState.value !== '' || Number(machineFilterState.parameterType) !== 0

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
  }, [statusFilter, debouncedSearch, debouncedDatabaseSearch, workloadTypeFilter, databaseTypeFilter, parsedTpsFilter, parsedMachineFilter])

  useEffect(() => {
    if (!runsData) {
      return
    }
    if (lastUpdateRef.current === dataUpdatedAt && lastPageRef.current === page) {
      return
    }
    lastUpdateRef.current = dataUpdatedAt ?? 0
    lastPageRef.current = page
    if (page === 1) {
      setRenderedRuns(runsData)
    } else if (runsData.length) {
      setRenderedRuns((prev) => [...prev, ...runsData])
    }
  }, [runsData, page, dataUpdatedAt])

  const handleLoadMore = () => {
    if (!isLoading && runs.length === pageSize) {
      setPage((prev) => prev + 1)
    }
  }

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
            <div className="grid gap-2 sm:grid-cols-3">
              <FilterSelect
                value={tpsFilterState.parameterType !== Tps_Filter_Type.UNSPECIFIED ? tpsFilterState.parameterType.toString() : ANY_SELECT_VALUE}
                onChange={(value) =>
                  setTpsFilterState((prev) => ({
                    ...prev,
                    parameterType: value === ANY_SELECT_VALUE ? Tps_Filter_Type.UNSPECIFIED : (Number(value) as Tps_Filter_Type),
                  }))
                }
                options={tpsMetricSelectOptions}
                placeholder={t('filters.tps.metric')}
                anyLabel={t('filters.any')}
              />
              <FilterSelect
                value={tpsFilterState.operator.toString()}
                onChange={(value) =>
                  setTpsFilterState((prev) => ({
                    ...prev,
                    operator: Number(value) as NumberFilterOperator,
                  }))
                }
                includeAny={false}
                options={operatorSelectOptions}
                placeholder={t('filters.tps.operator')}
              />
              <Input
                type="number"
                inputMode="numeric"
                value={tpsFilterState.value}
                onChange={(event) =>
                  setTpsFilterState((prev) => ({
                    ...prev,
                    value: event.target.value,
                  }))
                }
                placeholder={t('filters.tps.value')}
              />
            </div>
          </div>

          <div className="space-y-2 rounded-2xl border border-dashed border-border/70 p-4">
            <div className="flex items-center justify-between">
              <p className="text-xs uppercase tracking-[0.3em] text-muted-foreground">{t('filters.machine.title')}</p>
              <Button type="button" size="sm" variant="ghost" onClick={resetMachineFilter} disabled={!hasMachineFilter}>
                {t('filters.machine.reset')}
              </Button>
            </div>
            <div className="grid gap-2 sm:grid-cols-3">
              <FilterSelect
                value={
                  machineFilterState.parameterType !== MachineInfo_Filter_Type.UNSPECIFIED
                    ? machineFilterState.parameterType.toString()
                    : ANY_SELECT_VALUE
                }
                onChange={(value) =>
                  setMachineFilterState((prev) => ({
                    ...prev,
                    parameterType: value === ANY_SELECT_VALUE ? MachineInfo_Filter_Type.UNSPECIFIED : (Number(value) as MachineInfo_Filter_Type),
                  }))
                }
                options={machineMetricSelectOptions}
                placeholder={t('filters.machine.metric')}
                anyLabel={t('filters.any')}
              />
              <FilterSelect
                value={machineFilterState.operator.toString()}
                onChange={(value) =>
                  setMachineFilterState((prev) => ({
                    ...prev,
                    operator: Number(value) as NumberFilterOperator,
                  }))
                }
                includeAny={false}
                options={operatorSelectOptions}
                placeholder={t('filters.machine.operator')}
              />
              <Input
                type="number"
                inputMode="numeric"
                value={machineFilterState.value}
                onChange={(event) =>
                  setMachineFilterState((prev) => ({
                    ...prev,
                    value: event.target.value,
                  }))
                }
                placeholder={t('filters.machine.value')}
              />
            </div>
          </div>
        </div>
      </Card>

      <Card className="overflow-hidden rounded-2xl">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-32 text-xs uppercase tracking-[0.3em] text-muted-foreground">{t('table.columns.id')}</TableHead>
              <TableHead className="text-xs uppercase tracking-[0.3em] text-muted-foreground">{t('table.columns.status')}</TableHead>
              <TableHead className="text-xs uppercase tracking-[0.3em] text-muted-foreground">{t('table.columns.name')}</TableHead>
              <TableHead className="text-xs uppercase tracking-[0.3em] text-muted-foreground">{t('table.columns.tps')}</TableHead>
              <TableHead className="text-xs uppercase tracking-[0.3em] text-muted-foreground">{t('table.columns.createdAt')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {renderedRuns.map((run) => (
              <TableRow key={`${run.id}-${run.createdAt?.toISOString() ?? ''}`}>
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
            {!isLoading && renderedRuns.length === 0 && (
              <TableRow>
                <TableCell colSpan={5} className="py-10 text-center text-sm text-muted-foreground">
                  {t('list.empty')}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
        <div className="flex items-center justify-between border-t border-border/70 px-4 py-3 text-xs text-muted-foreground">
          <span>
            {t('list.count', {
              count: renderedRuns.length,
            })}
          </span>
          <Button variant="outline" onClick={handleLoadMore} disabled={isLoading || runs.length < pageSize}>
            {isLoading ? t('list.loading') : t('list.loadMore')}
          </Button>
        </div>
      </Card>
    </div>
  )
}
