import { useMemo } from 'react'
import { useLocation, useNavigate, useParams } from 'react-router-dom'
import dayjs from 'dayjs'
import { create } from '@bufbuild/protobuf'
import { timestampDate } from '@bufbuild/protobuf/wkt'
import { Card } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useTranslation } from '@/i18n/use-translation'
import type { RunSummary } from '@/app/features/runs/types'
import { getStatusLabel } from '@/app/features/runs/utils'
import { useQuery, useTransport } from '@connectrpc/connect-query'
import { UlidSchema } from '@/proto/panel/types_pb.ts'
import { getAutomation } from '@/proto/panel/automate-AutomateService_connectquery.ts'
import { getResource } from '@/proto/panel/automate-ResourcesService_connectquery.ts'
import type { CloudResource_TreeNode } from '@/proto/panel/automate_pb.ts'
import { CloudResource_Status } from '@/proto/panel/automate_pb.ts'

interface RunDetailLocationState {
  run?: RunSummary
}

type TranslateFn = (key: string, options?: Record<string, unknown>) => string

const infoRow = (label: string, value?: string | number) => (
  <div className="flex flex-col rounded-xl border border-border/60 bg-muted/10 p-4">
    <span className="text-xs uppercase tracking-[0.3em] text-muted-foreground">{label}</span>
    <span className="text-base font-semibold text-foreground">{value ?? '—'}</span>
  </div>
)

const getResourceStatusLabel = (status: CloudResource_Status, t: TranslateFn) => {
  switch (status) {
    case CloudResource_Status.CREATING:
      return t('automation.resourceStatus.creating')
    case CloudResource_Status.WORKING:
      return t('automation.resourceStatus.working')
    case CloudResource_Status.DESTROYING:
      return t('automation.resourceStatus.destroying')
    case CloudResource_Status.DESTROYED:
      return t('automation.resourceStatus.destroyed')
    case CloudResource_Status.DEGRADED:
      return t('automation.resourceStatus.degraded')
    default:
      return t('automation.resourceStatus.unknown')
  }
}

const ResourceNode = ({ node, t }: { node: CloudResource_TreeNode; t: TranslateFn }) => {
  const cloudResource = node.resource
  const resourceDef = cloudResource?.resource?.resourceDef
  const metadataName = resourceDef?.metadata?.name ?? cloudResource?.resource?.ref?.name ?? node.id?.id ?? '—'
  const kind = resourceDef?.kind ?? t('automation.resourceKindUnknown')
  const statusLabel = getResourceStatusLabel(cloudResource?.status ?? CloudResource_Status.UNSPECIFIED, t)
  const ready = cloudResource?.resource?.ready
  const synced = cloudResource?.resource?.synced

  return (
    <li className="space-y-2">
      <Card className="space-y-3 border border-border/70 bg-card/60 p-4 text-sm shadow-sm">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div>
            <p className="text-xs uppercase tracking-[0.3em] text-muted-foreground">{kind}</p>
            <p className="text-base font-semibold text-foreground">{metadataName}</p>
          </div>
          <Badge variant="outline" className="text-[11px]">
            {statusLabel}
          </Badge>
        </div>
        <div className="flex flex-wrap gap-2 text-xs text-muted-foreground">
          {ready !== undefined && <Badge variant="secondary">{ready ? t('automation.ready') : t('automation.notReady')}</Badge>}
          {synced !== undefined && <Badge variant="secondary">{synced ? t('automation.synced') : t('automation.notSynced')}</Badge>}
          <span>{t('automation.resourceId', { id: node.id?.id ?? '—' })}</span>
        </div>
      </Card>
      {node.children.length > 0 ? (
        <ul className="space-y-2 border-l border-border/40 pl-4">
          {node.children.map((child, index) => (
            <ResourceNode key={child.id?.id ?? child.resource?.resource?.ref?.name ?? `resource-${index}`} node={child} t={t} />
          ))}
        </ul>
      ) : null}
    </li>
  )
}

const ResourceTree = ({ title, tree, isLoading, t }: { title: string; tree?: CloudResource_TreeNode; isLoading: boolean; t: TranslateFn }) => (
  <div className="space-y-3">
    <p className="text-xs uppercase tracking-[0.3em] text-muted-foreground">{title}</p>
    {isLoading && <p className="text-sm text-muted-foreground">{t('automation.loading')}</p>}
    {!isLoading && tree && <ul className="space-y-3"><ResourceNode node={tree} t={t} /></ul>}
    {!isLoading && !tree && <p className="text-sm text-muted-foreground">{t('automation.noResources')}</p>}
  </div>
)

export const RunsDetailPage = () => {
  const { t } = useTranslation('runs')
  const navigate = useNavigate()
  const params = useParams()
  const location = useLocation()
  const transport = useTransport()
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

  const automationId = run.cloudAutomationId
  const automationRequest = useMemo(() => create(UlidSchema, { id: automationId ?? '' }), [automationId])
  const { data: automation, isLoading: isAutomationLoading } = useQuery(getAutomation, automationRequest, {
    transport,
    enabled: Boolean(automationId),
  })

  const databaseRootId = automation?.databaseRootResourceId?.id
  const workloadRootId = automation?.workloadRootResourceId?.id

  const databaseRequest = useMemo(() => create(UlidSchema, { id: databaseRootId ?? '' }), [databaseRootId])
  const workloadRequest = useMemo(() => create(UlidSchema, { id: workloadRootId ?? '' }), [workloadRootId])

  const { data: databaseTree, isLoading: isDatabaseLoading } = useQuery(getResource, databaseRequest, {
    transport,
    enabled: Boolean(databaseRootId),
  })
  const { data: workloadTree, isLoading: isWorkloadLoading } = useQuery(getResource, workloadRequest, {
    transport,
    enabled: Boolean(workloadRootId),
  })

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

      <Card className="space-y-4 p-6">
        <div className="flex items-center justify-between">
          <div>
            <p className="text-xs uppercase tracking-[0.4em] text-muted-foreground">{t('automation.title')}</p>
            <p className="text-sm text-muted-foreground">
              {automation?.id?.id ?? automationId ?? t('automation.notLinked')}
            </p>
          </div>
          {automation && (
            <Badge variant="secondary">{getStatusLabel(automation.status, t)}</Badge>
          )}
        </div>

        {!automationId && <p className="text-sm text-muted-foreground">{t('automation.notLinked')}</p>}
        {automationId && isAutomationLoading && <p className="text-sm text-muted-foreground">{t('automation.loading')}</p>}
        {automationId && !isAutomationLoading && !automation && (
          <p className="text-sm text-destructive">{t('automation.failed')}</p>
        )}

        {automation && (
          <div className="grid gap-4 sm:grid-cols-2">
            {infoRow(
              t('automation.startedAt'),
              automation.timing?.createdAt ? dayjs(timestampDate(automation.timing.createdAt)).format('DD MMM YYYY, HH:mm') : '—',
            )}
            {infoRow(
              t('automation.updatedAt'),
              automation.timing?.updatedAt ? dayjs(timestampDate(automation.timing.updatedAt)).format('DD MMM YYYY, HH:mm') : '—',
            )}
          </div>
        )}

        {automation && (
          <div className="space-y-6">
            <ResourceTree title={t('automation.databaseTree')} tree={databaseTree} isLoading={isDatabaseLoading} t={t} />
            <ResourceTree title={t('automation.workloadTree')} tree={workloadTree} isLoading={isWorkloadLoading} t={t} />
          </div>
        )}
      </Card>
    </div>
  )
}
