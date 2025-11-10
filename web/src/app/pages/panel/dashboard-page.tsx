import { useMemo } from 'react'
import dayjs from 'dayjs'
import {
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import { Card } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { ChartContainer, ChartTooltipContent } from '@/components/ui/chart'
import { useTopRunsQuery } from '@/app/features/runs/api'
import { useTranslation } from '@/i18n/use-translation'
import { getStatusBadgeVariant, getStatusLabel } from '@/app/features/runs/utils'

const DASHBOARD_TOP_RUNS_FALLBACK = [
  { label: 'TPCC #129', average: 2300 },
  { label: 'TPCC #128', average: 2180 },
  { label: 'TPCC #127', average: 2050 },
  { label: 'TPCC #126', average: 1960 },
  { label: 'TPCC #125', average: 1880 },
  { label: 'TPCC #124', average: 1740 },
  { label: 'TPCC #123', average: 1650 },
  { label: 'TPCC #122', average: 1530 },
  { label: 'TPCC #121', average: 1410 },
  { label: 'TPCC #120', average: 1320 },
]
const TOP_RUN_COLORS = ['var(--chart-1)', 'var(--chart-2)', 'var(--chart-3)']
const DEFAULT_RUN_COLOR = 'var(--chart-2)'

export const DashboardPage = () => {
  const { data: runs = [], isLoading } = useTopRunsQuery()
  const { t } = useTranslation('dashboard')

  const runsToday = runs.filter((run) => run.createdAt && dayjs(run.createdAt).isSame(dayjs(), 'day')).length
  const averageTps =
    runs.length > 0
      ? Math.round(
          runs.reduce((sum, run) => sum + (run.tpsAverage ?? 0), 0) / runs.length,
        )
      : null

  const cards = [
    {
      label: t('metrics.active'),
      value: runs.length.toString(),
      trend: runs.length > 0 ? `+${runs.length}` : '0',
    },
    {
      label: t('metrics.today'),
      value: runsToday.toString(),
      trend: runsToday > 0 ? `+${runsToday}` : '0',
    },
    {
      label: t('metrics.avgTps'),
      value: averageTps ? averageTps.toLocaleString('ru-RU') : '—',
      trend: averageTps ? '~ TPS' : '',
    },
  ]

  const topRunsChartConfig = useMemo(
    () => ({
      average: { label: t('charts.topRuns.series.average'), color: 'var(--chart-2)' },
    }),
    [t],
  )

  const topRunsChartData = useMemo(() => {
    if (!runs.length) {
      return DASHBOARD_TOP_RUNS_FALLBACK.map((run, index) => ({ ...run, rank: index }))
    }
    return runs
      .slice(0, 10)
      .map((run, index) => ({
        label: run.workloadName || `Run #${index + 1}`,
        average: run.tpsAverage ?? 0,
      }))
      .sort((a, b) => b.average - a.average)
      .map((run, index) => ({ ...run, rank: index }))
  }, [runs])

  return (
    <div className="space-y-6">
      <div className="grid gap-4 md:grid-cols-3">
        {cards.map((card) => (
          <Card key={card.label} className="rounded-2xl p-4 shadow-sm">
            <p className="text-xs uppercase tracking-[0.4em] text-muted-foreground">{card.label}</p>
            <p className="mt-3 text-3xl font-semibold text-foreground">{card.value}</p>
            <p className="text-xs text-primary">{card.trend}</p>
          </Card>
        ))}
      </div>

      <div>
        <Card className="rounded-[32px] border border-border/20 bg-gradient-to-b from-background via-background/90 to-background/70 p-6 text-card-foreground shadow-2xl">
          <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
            <div>
              <p className="text-[11px] uppercase tracking-[0.6em] text-muted-foreground/80">{t('charts.topRuns.title')}</p>
              <p className="text-base text-card-foreground/80">{t('charts.topRuns.subtitle')}</p>
            </div>
            <div className="text-right">
              <p className="text-[11px] uppercase tracking-[0.5em] text-muted-foreground">
                {t('charts.topRuns.count', { count: topRunsChartData.length })}
              </p>
              <p className="text-3xl font-semibold text-card-foreground">
                {topRunsChartData[0]?.average?.toLocaleString('ru-RU') ?? '—'}
                <span className="ml-1 text-xs font-medium text-muted-foreground">TPS</span>
              </p>
            </div>
          </div>
          <ChartContainer config={topRunsChartConfig} className="mt-8 h-[360px] text-card-foreground/80">
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={topRunsChartData} margin={{ left: 0, right: 0, bottom: 20 }}>
                <CartesianGrid strokeDasharray="4 4" stroke="var(--border)" strokeOpacity={0.25} vertical={false} />
                <XAxis
                  dataKey="label"
                  tickLine={false}
                  axisLine={false}
                  tick={{ fontSize: 11, fill: 'var(--muted-foreground)', fillOpacity: 0.6 }}
                  interval={0}
                  height={70}
                  angle={-20}
                  textAnchor="end"
                />
                <YAxis
                  tickLine={false}
                  axisLine={false}
                  tick={{ fontSize: 11, fill: 'var(--muted-foreground)', fillOpacity: 0.75 }}
                  allowDecimals={false}
                  domain={[0, (dataMax: number | undefined) => Math.ceil(((dataMax ?? 1000) + 1) * 1.05)]}
                  width={48}
                />
                <Tooltip
                  content={<ChartTooltipContent />}
                  cursor={{ fill: 'color-mix(in oklab, var(--foreground) 8%, transparent)' }}
                />
                <Bar dataKey="average" name="average" radius={[10, 10, 8, 8]} barSize={32}>
                  {topRunsChartData.map((item) => (
                    <Cell
                      key={item.label}
                      fill={item.rank < TOP_RUN_COLORS.length ? TOP_RUN_COLORS[item.rank] : DEFAULT_RUN_COLOR}
                    />
                  ))}
                </Bar>
              </BarChart>
            </ResponsiveContainer>
          </ChartContainer>
        </Card>
      </div>

      <Card className="rounded-3xl p-6 shadow-lg">
        <p className="text-xs uppercase tracking-[0.5em] text-muted-foreground">{t('events.title')}</p>
        <ul className="mt-4 space-y-3 text-sm text-foreground/80">
          {t('events.items', { returnObjects: true }) instanceof Array
            ? (t('events.items', { returnObjects: true }) as string[]).map((item) => <li key={item}>• {item}</li>)
            : null}
        </ul>
      </Card>

      <Card className="rounded-3xl p-0 shadow-sm">
        <div className="border-b border-border/70 px-6 py-4">
          <p className="text-xs uppercase tracking-[0.5em] text-muted-foreground">{t('latest.title')}</p>
        </div>
        {isLoading && <div className="h-40 animate-pulse rounded-b-3xl bg-muted" />}
        {!isLoading && runs.length === 0 && (
          <div className="px-6 py-10 text-center text-sm text-muted-foreground">{t('latest.empty')}</div>
        )}
        {!isLoading && runs.length > 0 && (
          <ul className="divide-y divide-border/70">
            {runs.slice(0, 5).map((run) => (
              <li key={run.id} className="flex items-center justify-between gap-4 px-6 py-4">
                <div>
                  <p className="font-semibold text-foreground">{run.workloadName}</p>
                  <p className="text-xs text-muted-foreground">
                    {run.databaseName ?? t('latest.unknownDatabase')} ·{' '}
                    {run.createdAt ? dayjs(run.createdAt).format('DD MMM HH:mm') : t('latest.unknownDate')}
                  </p>
                </div>
                <div className="flex items-center gap-3">
                  <p className="text-sm font-mono text-foreground/80">
                    {run.tpsAverage ? `${run.tpsAverage.toLocaleString('ru-RU')} TPS` : '—'}
                  </p>
                  <Badge variant={getStatusBadgeVariant(run.status)}>{getStatusLabel(run.status, t)}</Badge>
                </div>
              </li>
            ))}
          </ul>
        )}
      </Card>
    </div>
  )
}
