import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import dayjs from 'dayjs'
import { useTranslation } from '@/i18n/use-translation'
import { useAutomationsQuery } from '@/app/features/automations/api'
import type { SortDirection } from '@/app/features/automations/types'
import { Card } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { getStatusBadgeVariant, getStatusLabel } from '@/app/features/runs/utils'
import { ArrowUpDown, ChevronLeft, ChevronRight, RotateCcw } from 'lucide-react'

type SortableColumn = 'status' | 'createdAt'
type SortState = { column: SortableColumn; direction: SortDirection } | null

export const AutomationsPage = () => {
  const { t } = useTranslation('automations')
  const { t: tRuns } = useTranslation('runs')
  const navigate = useNavigate()
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [onlyMine, setOnlyMine] = useState(false)
  const [sort, setSort] = useState<SortState>({ column: 'createdAt', direction: 'desc' })

  const filters = useMemo(
    () => ({
      page,
      pageSize,
      onlyMine: onlyMine || undefined,
      orderByStatus: sort?.column === 'status' ? sort.direction : undefined,
      orderByCreatedAt: sort?.column === 'createdAt' ? sort.direction : undefined,
    }),
    [page, pageSize, onlyMine, sort],
  )

  const { data: automations = [], isLoading, refetch, isFetching } = useAutomationsQuery(filters)

  useEffect(() => {
    setPage(1)
  }, [onlyMine, pageSize, sort])

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

  const handleViewRun = (runId?: string) => {
    if (!runId) return
    navigate(`/app/runs/${runId}`)
  }

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <p className="text-xs uppercase tracking-[0.5em] text-muted-foreground">{t('page.title')}</p>
          <h1 className="text-3xl font-semibold text-foreground">{t('page.subtitle')}</h1>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button size="sm" onClick={() => navigate('/app/automations/new')} disabled={isFetching}>
            {t('actions.runAutomation')}
          </Button>
          <Button variant="outline" size="sm" onClick={() => refetch()} disabled={isFetching}>
            <RotateCcw className="mr-2 h-4 w-4" />
            {t('actions.refresh')}
          </Button>
        </div>
      </header>

      <Card className="flex flex-col gap-4 rounded-2xl p-4 shadow-sm">
        <div className="flex items-center gap-3 rounded-lg border border-border/60 bg-muted/20 p-4">
          <Checkbox id="onlyMineAutomations" checked={onlyMine} onCheckedChange={(checked) => setOnlyMine(checked === true)} />
          <div>
            <label htmlFor="onlyMineAutomations" className="text-sm font-medium text-foreground">
              {t('filters.onlyMine')}
            </label>
            <p className="text-xs text-muted-foreground">{t('filters.onlyMineHint')}</p>
          </div>
        </div>
      </Card>

      <Card className="overflow-hidden rounded-2xl">
        <div className="flex items-center justify-between gap-3 border-b border-border/70 px-4 py-3">
          <p className="text-sm text-muted-foreground">
            {isLoading ? t('list.loading') : t('list.count', { count: automations.length })}
          </p>
        </div>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="text-xs uppercase tracking-[0.3em] text-muted-foreground">{t('table.columns.id')}</TableHead>
              <TableHead
                className="text-xs uppercase tracking-[0.3em] text-muted-foreground"
                aria-sort={getAriaSort('status')}
              >
                <button type="button" className="flex w-full items-center gap-1" onClick={() => handleSort('status')}>
                  {t('table.columns.status')}
                  <ArrowUpDown
                    className={`h-4 w-4 ${sort?.column === 'status' ? 'text-foreground' : 'text-muted-foreground'}`}
                  />
                </button>
              </TableHead>
              <TableHead className="text-xs uppercase tracking-[0.3em] text-muted-foreground">{t('table.columns.run')}</TableHead>
              <TableHead
                className="text-xs uppercase tracking-[0.3em] text-muted-foreground"
                aria-sort={getAriaSort('createdAt')}
              >
                <button type="button" className="flex w-full items-center gap-1" onClick={() => handleSort('createdAt')}>
                  {t('table.columns.createdAt')}
                  <ArrowUpDown
                    className={`h-4 w-4 ${sort?.column === 'createdAt' ? 'text-foreground' : 'text-muted-foreground'}`}
                  />
                </button>
              </TableHead>
              <TableHead className="text-xs uppercase tracking-[0.3em] text-muted-foreground">{t('table.columns.updatedAt')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {automations.map((automation) => {
              const runId = automation.stroppyRun?.id
              const grafanaUrl = automation.stroppyRun?.grafanaDashboardUrl

              return (
                <TableRow key={`${automation.id}-${automation.createdAt?.toISOString() ?? ''}`}>
                  <TableCell className="font-mono text-xs text-muted-foreground">{automation.id || '—'}</TableCell>
                  <TableCell>
                    <Badge variant={getStatusBadgeVariant(automation.status)}>{getStatusLabel(automation.status, tRuns)}</Badge>
                  </TableCell>
                  <TableCell>
                    {runId ? (
                      <div className="space-y-1">
                        <Button variant="link" className="px-0" onClick={() => handleViewRun(runId)}>
                          {runId}
                        </Button>
                        {grafanaUrl && (
                          <Button
                            asChild
                            variant="link"
                            className="h-auto px-0 text-xs text-muted-foreground hover:text-foreground"
                          >
                            <a href={grafanaUrl} target="_blank" rel="noreferrer">
                              {t('actions.openGrafana')}
                            </a>
                          </Button>
                        )}
                      </div>
                    ) : (
                      <span className="text-sm text-muted-foreground">{t('messages.noRun')}</span>
                    )}
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {automation.createdAt ? dayjs(automation.createdAt).format('DD MMM YYYY, HH:mm') : t('messages.unknown')}
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {automation.updatedAt ? dayjs(automation.updatedAt).format('DD MMM YYYY, HH:mm') : '—'}
                  </TableCell>
                </TableRow>
              )
            })}
            {isLoading && (
              <TableRow>
                <TableCell colSpan={5} className="py-10 text-center text-sm text-muted-foreground">
                  {t('list.loading')}
                </TableCell>
              </TableRow>
            )}
            {!isLoading && automations.length === 0 && (
              <TableRow>
                <TableCell colSpan={5} className="py-10 text-center text-sm text-muted-foreground">
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
              <Button
                type="button"
                variant="outline"
                size="icon"
                className="h-8 w-8"
                onClick={() => setPage((prev) => Math.max(1, prev - 1))}
                disabled={page === 1 || isLoading}
              >
                <span className="sr-only">{t('list.previous')}</span>
                <ChevronLeft className="h-4 w-4" />
              </Button>
              <Button
                type="button"
                variant="outline"
                size="icon"
                className="h-8 w-8"
                onClick={() => setPage((prev) => prev + 1)}
                disabled={isLoading || automations.length < pageSize}
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
