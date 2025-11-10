import { Link } from 'react-router-dom'
import { useMemo } from 'react'
import dayjs from 'dayjs'
import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import { Badge } from '@/components/ui/badge'
import { Card } from '@/components/ui/card'
import { ChartContainer, ChartTooltipContent } from '@/components/ui/chart'
import { useTopRunsQuery } from '@/app/features/runs/api'
import { useTranslation } from '@/i18n/use-translation'
import { getStatusBadgeVariant, getStatusLabel } from '@/app/features/runs/utils'

const FALLBACK_TPS_TREND = [
  { label: 'TPCC ∙ 08:00', average: 1200, p95: 950 },
  { label: 'TPCC ∙ 10:00', average: 1850, p95: 1400 },
  { label: 'TPCC ∙ 12:00', average: 2100, p95: 1600 },
  { label: 'TPCC ∙ 14:00', average: 2600, p95: 1900 },
  { label: 'TPCC ∙ 16:00', average: 2400, p95: 1800 },
  { label: 'TPCC ∙ 18:00', average: 2800, p95: 2100 },
]

export const LandingHomePage = () => {
  const { t } = useTranslation('landing')
  const { data: topRuns = [], isLoading } = useTopRunsQuery()

  const heroRuns = useMemo(() => topRuns.slice(0, 3), [topRuns])
  const tpsChartConfig = useMemo(
    () => ({
      average: { label: t('charts.tps.series.average'), color: 'var(--chart-2)' },
      p95: { label: t('charts.tps.series.p95'), color: 'var(--chart-4)' },
    }),
    [t],
  )
  const tpsTrendData = useMemo(() => {
    if (!topRuns.length) {
      return FALLBACK_TPS_TREND
    }
    return topRuns.slice(0, 8).map((run) => ({
      label: run.createdAt ? dayjs(run.createdAt).format('DD MMM HH:mm') : run.workloadName,
      average: run.tpsAverage ?? 0,
      p95: run.tpsP95 ?? run.tpsAverage ?? 0,
    }))
  }, [topRuns])

  return (
    <section className="mx-auto flex w-full max-w-5xl flex-col gap-12 px-4 py-16 sm:px-6">
      <div className="rounded-3xl bg-gradient-to-b from-background via-background/90 to-background/70 px-6 py-12 shadow-xl">
        <p className="text-xs uppercase tracking-[0.7em] text-muted-foreground">{t('hero.kicker')}</p>
        <h1 className="mt-4 text-4xl font-bold leading-tight text-foreground sm:text-5xl">{t('hero.title')}</h1>
        <p className="mt-4 max-w-2xl text-lg text-muted-foreground">{t('hero.description')}</p>
        <div className="mt-8 flex flex-wrap gap-3">
          <Link
            to="/register"
            className="rounded-full bg-primary px-6 py-3 text-sm font-semibold uppercase tracking-wide text-primary-foreground transition hover:opacity-90"
          >
            {t('hero.actions.register')}
          </Link>
          <Link
            to="/app/dashboard"
            className="rounded-full border border-border px-6 py-3 text-sm font-semibold uppercase tracking-wide text-muted-foreground transition hover:border-foreground hover:text-foreground"
          >
            {t('hero.actions.openConsole')}
          </Link>
        </div>
      </div>

      <Card className="rounded-3xl p-6 shadow-lg">
        <div className="flex flex-col gap-1 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <p className="text-xs uppercase tracking-[0.5em] text-muted-foreground">{t('charts.tps.title')}</p>
            <p className="text-sm text-muted-foreground">{t('charts.tps.subtitle')}</p>
          </div>
          <div className="text-right text-3xl font-semibold text-foreground">
            {tpsTrendData.at(-1)?.average?.toLocaleString('ru-RU') ?? '—'}
            <span className="ml-1 text-xs text-muted-foreground">TPS</span>
          </div>
        </div>
        <ChartContainer config={tpsChartConfig} className="mt-6 h-[260px]">
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={tpsTrendData}>
              <defs>
                <linearGradient id="averageGradient" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor="var(--chart-average)" stopOpacity={0.35} />
                  <stop offset="100%" stopColor="var(--chart-average)" stopOpacity={0.05} />
                </linearGradient>
                <linearGradient id="p95Gradient" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor="var(--chart-p95)" stopOpacity={0.25} />
                  <stop offset="100%" stopColor="var(--chart-p95)" stopOpacity={0.02} />
                </linearGradient>
              </defs>
              <CartesianGrid strokeDasharray="4 4" stroke="var(--border)" strokeOpacity={0.3} />
              <XAxis dataKey="label" tickLine={false} axisLine={false} />
              <YAxis tickLine={false} axisLine={false} allowDecimals={false} />
              <Tooltip content={<ChartTooltipContent />} />
              <Area type="monotone" dataKey="average" stroke="var(--chart-average)" strokeWidth={2.4} fill="url(#averageGradient)" name="average" />
              <Area type="basis" dataKey="p95" stroke="var(--chart-p95)" strokeWidth={2} fill="url(#p95Gradient)" name="p95" />
            </AreaChart>
          </ResponsiveContainer>
        </ChartContainer>
      </Card>

      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-lg font-semibold text-foreground">{t('topRuns.title')}</h2>
          <Link to="/app/runs" className="text-sm font-semibold uppercase tracking-wide text-muted-foreground hover:text-foreground">
            {t('topRuns.cta')}
          </Link>
        </div>

        <div className="grid gap-4 md:grid-cols-3">
          {isLoading &&
            Array.from({ length: 3 }).map((_, index) => (
              <div key={index} className="h-36 animate-pulse rounded-2xl bg-muted" />
            ))}
          {!isLoading &&
            heroRuns.map((run) => (
              <Card key={run.id} className="flex flex-col gap-3 rounded-2xl p-4">
                <div className="flex items-center justify-between">
                  <p className="text-xs font-semibold uppercase tracking-[0.3em] text-muted-foreground">{run.workloadName}</p>
                  <Badge variant={getStatusBadgeVariant(run.status)}>{getStatusLabel(run.status, t)}</Badge>
                </div>
                <div>
                  <p className="text-2xl font-semibold text-foreground">
                    {run.tpsAverage ? run.tpsAverage.toLocaleString('ru-RU') : '—'} TPS
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {run.createdAt ? dayjs(run.createdAt).format('DD MMM, HH:mm') : t('topRuns.unknownDate')}
                  </p>
                </div>
                <div className="text-xs text-muted-foreground">
                  {run.databaseName ?? t('topRuns.unknownDatabase')}
                </div>
              </Card>
            ))}
        </div>
      </div>
    </section>
  )
}
