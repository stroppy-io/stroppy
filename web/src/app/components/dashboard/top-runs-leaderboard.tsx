import { useMemo } from 'react'
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
import { ChartContainer, ChartTooltipContent } from '@/components/ui/chart'
import { useTranslation } from '@/i18n/use-translation'
import { cn } from '@/lib/utils'
import type { RunSummary } from '@/app/features/runs/types'

const DASHBOARD_TOP_RUNS_FALLBACK: Array<{ label: string; average: number }> = [
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
const TOP_RUN_COLORS = ['var(--chart-1)', 'var(--chart-1)', 'var(--chart-1)']
const DEFAULT_RUN_COLOR = 'var(--chart-2)'

type TopRunsLeaderboardProps = {
  runs: RunSummary[]
  className?: string
}

export const TopRunsLeaderboard = ({ runs, className }: TopRunsLeaderboardProps) => {
  const { t } = useTranslation('dashboard')

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
    <Card
      className={cn(
        'rounded-[32px] border border-border/20 bg-gradient-to-b from-background via-background/90 to-background/70 p-6 text-card-foreground shadow-2xl',
        className,
      )}
    >
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
            <Tooltip content={<ChartTooltipContent />} cursor={{ fill: 'color-mix(in oklab, var(--foreground) 8%, transparent)' }} />
            <Bar dataKey="average" name="average" radius={[10, 10, 8, 8]} barSize={32}>
              {topRunsChartData.map((item) => (
                <Cell key={item.label} fill={item.rank < TOP_RUN_COLORS.length ? TOP_RUN_COLORS[item.rank] : DEFAULT_RUN_COLOR} />
              ))}
            </Bar>
          </BarChart>
        </ResponsiveContainer>
      </ChartContainer>
    </Card>
  )
}
