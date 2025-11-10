import dayjs from 'dayjs'
import { Card } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { useTopRunsQuery } from '@/app/features/runs/api'
import { useTranslation } from '@/i18n/use-translation'
import { getStatusBadgeVariant, getStatusLabel } from '@/app/features/runs/utils'
import { TopRunsLeaderboard } from '@/app/components/dashboard/top-runs-leaderboard'

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

      <TopRunsLeaderboard runs={runs} />

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
