import { useLocation, useNavigate, useParams } from 'react-router-dom'
import dayjs from 'dayjs'
import { Card } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useTranslation } from '@/i18n/use-translation'
import type { RunSummary } from '@/app/features/runs/types'
import { getStatusLabel } from '@/app/features/runs/utils'

interface RunDetailLocationState {
  run?: RunSummary
}

const infoRow = (label: string, value?: string | number) => (
  <div className="flex flex-col rounded-xl border border-border/60 bg-muted/10 p-4">
    <span className="text-xs uppercase tracking-[0.3em] text-muted-foreground">{label}</span>
    <span className="text-base font-semibold text-foreground">{value ?? '—'}</span>
  </div>
)

export const RunsDetailPage = () => {
  const { t } = useTranslation('runs')
  const navigate = useNavigate()
  const params = useParams()
  const location = useLocation()
  const run = (location.state as RunDetailLocationState | undefined)?.run

  if (!run) {
    return (
      <div className="space-y-6">
        <Card className="space-y-4 p-6">
          <p className="text-base text-muted-foreground">{t('messages.noRunsFound')}</p>
          <Button onClick={() => navigate('/app/runs')}>{t('actions.backToList')}</Button>
        </Card>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <p className="text-xs uppercase tracking-[0.4em] text-muted-foreground">{t('page.title')}</p>
          <h1 className="text-3xl font-semibold text-foreground">{run.workloadName}</h1>
          <p className="text-sm text-muted-foreground">{params.runId}</p>
        </div>
        <div className="flex items-center gap-3">
          <Badge variant="secondary">{getStatusLabel(run.status, t)}</Badge>
          <Button variant="outline" onClick={() => navigate('/app/runs')}>
            {t('actions.backToList')}
          </Button>
        </div>
      </div>

      <Card className="p-6">
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {infoRow('TPS AVG', run.tpsAverage ? `${run.tpsAverage.toLocaleString()} TPS` : '—')}
          {infoRow('TPS P95', run.tpsP95 ? `${run.tpsP95.toLocaleString()} TPS` : '—')}
          {infoRow(t('filters.workloadType'), run.workloadType ? t('filters.workloadTypes.tpcc') : t('filters.workloadTypes.unspecified'))}
          {infoRow(t('filters.databaseType'), run.databaseType ? t('filters.databaseTypes.postgresOriole') : t('filters.databaseTypes.unspecified'))}
        </div>
      </Card>

      <div className="grid gap-6 md:grid-cols-2">
        <Card className="space-y-3 p-6">
          <p className="text-xs uppercase tracking-[0.4em] text-muted-foreground">{t('comparison.machine.runnerTitle')}</p>
          <div className="space-y-3">
            {infoRow(t('comparison.machine.runnerSignature'), run.runnerMachineSignature)}
            {infoRow(t('comparison.machine.runnerCores'), run.runnerMachineCores ? `${run.runnerMachineCores} vCPU` : '—')}
            {infoRow(t('comparison.machine.runnerMemory'), run.runnerMachineMemory ? `${run.runnerMachineMemory} GB` : '—')}
            {infoRow(t('comparison.machine.runnerDisk'), run.runnerMachineDisk ? `${run.runnerMachineDisk} GB` : '—')}
          </div>
        </Card>
        <Card className="space-y-3 p-6">
          <p className="text-xs uppercase tracking-[0.4em] text-muted-foreground">{t('comparison.machine.databaseTitle')}</p>
          <div className="space-y-3">
            {infoRow(t('comparison.machine.databaseSignature'), run.databaseMachineSignature)}
            {infoRow(t('comparison.machine.databaseCores'), run.databaseMachineCores ? `${run.databaseMachineCores} vCPU` : '—')}
            {infoRow(t('comparison.machine.databaseMemory'), run.databaseMachineMemory ? `${run.databaseMachineMemory} GB` : '—')}
            {infoRow(t('comparison.machine.databaseDisk'), run.databaseMachineDisk ? `${run.databaseMachineDisk} GB` : '—')}
          </div>
        </Card>
      </div>

      <Card className="p-6">
        <p className="text-xs uppercase tracking-[0.4em] text-muted-foreground">{t('comparison.cluster.title')}</p>
        <div className="mt-4 grid gap-4 sm:grid-cols-2">
          {infoRow(t('comparison.cluster.runnerNodes'), run.runnerClusterNodes ?? '—')}
          {infoRow(t('comparison.cluster.databaseNodes'), run.databaseClusterNodes ?? '—')}
        </div>
      </Card>

      <Card className="p-6">
        <p className="text-xs uppercase tracking-[0.4em] text-muted-foreground">{t('table.columns.createdAt')}</p>
        <div className="mt-3 grid gap-4 sm:grid-cols-2">
          {infoRow(t('table.columns.createdAt'), run.createdAt ? dayjs(run.createdAt).format('DD MMM YYYY, HH:mm') : t('meta.unknownDate'))}
          {infoRow(t('table.columns.updatedAt'), run.updatedAt ? dayjs(run.updatedAt).format('DD MMM YYYY, HH:mm') : '—')}
        </div>
      </Card>

    </div>
  )
}
