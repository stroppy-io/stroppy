import type { ReactNode } from 'react'
import { useMemo } from 'react'
import { useLocation, useNavigate, useParams } from 'react-router-dom'
import dayjs from 'dayjs'
import { create } from '@bufbuild/protobuf'
import { timestampDate } from '@bufbuild/protobuf/wkt'
import { Card } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ResourceTreeSection } from '@/app/components/resource-tree'
import { useTranslation } from '@/i18n/use-translation'
import type { AutomationSummary } from '@/app/features/automations/types'
import { getStatusBadgeVariant, getStatusLabel } from '@/app/features/runs/utils'
import type { Status } from '@/proto/panel/types_pb.ts'
import { UlidSchema } from '@/proto/panel/types_pb.ts'
import { getAutomation } from '@/proto/panel/automate-AutomateService_connectquery.ts'
import { getResource } from '@/proto/panel/automate-ResourcesService_connectquery.ts'
import { useQuery, useTransport } from '@connectrpc/connect-query'

interface AutomationDetailLocationState {
  automation?: AutomationSummary
}

const infoRow = (label: string, value?: ReactNode) => (
  <div className="flex flex-col rounded-xl border border-border/60 bg-muted/10 p-4">
    <span className="text-xs uppercase tracking-[0.3em] text-muted-foreground">{label}</span>
    <span className="text-base font-semibold text-foreground">{value ?? '—'}</span>
  </div>
)

const formatDateTime = (date?: Date, fallback = '—') => (date ? dayjs(date).format('DD MMM YYYY, HH:mm') : fallback)

export const AutomationsDetailPage = () => {
  const { t } = useTranslation('automations')
  const { t: tRuns } = useTranslation('runs')
  const navigate = useNavigate()
  const params = useParams()
  const transport = useTransport()
  const location = useLocation()
  const automationFromState = (location.state as AutomationDetailLocationState | undefined)?.automation

  const automationId = params.automationId ?? automationFromState?.id
  const automationRequest = useMemo(() => create(UlidSchema, { id: automationId ?? '' }), [automationId])

  const {
    data: automation,
    isLoading: isAutomationLoading,
    error: automationError,
  } = useQuery(getAutomation, automationRequest, {
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

  const displayId = automation?.id?.id ?? automationFromState?.id ?? automationId ?? '—'
  const automationStatus = automation?.status ?? automationFromState?.status
  const automationCreatedAt = automation?.timing?.createdAt
    ? timestampDate(automation.timing.createdAt)
    : automationFromState?.createdAt
  const automationUpdatedAt = automation?.timing?.updatedAt
    ? timestampDate(automation.timing.updatedAt)
    : automationFromState?.updatedAt

  const stroppyRun = automation?.stroppyRun
  const runSummary = automationFromState?.stroppyRun
  const linkedRunId = stroppyRun?.id?.id ?? runSummary?.id
  const grafanaUrl = stroppyRun?.grafanaDashboardUrl ?? runSummary?.grafanaDashboardUrl
  const runStatus = (stroppyRun?.status as Status | undefined) ?? runSummary?.status
  const runCreatedAt = stroppyRun?.timing?.createdAt ? timestampDate(stroppyRun.timing.createdAt) : runSummary?.createdAt
  const runUpdatedAt = stroppyRun?.timing?.updatedAt ? timestampDate(stroppyRun.timing.updatedAt) : runSummary?.updatedAt

  const showAutomationContent = Boolean(automation)

  const handleViewRun = () => {
    if (!linkedRunId) return
    navigate(`/app/runs/${linkedRunId}`)
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <p className="text-xs uppercase tracking-[0.4em] text-muted-foreground">{t('detail.pageTitle')}</p>
          <h1 className="text-3xl font-semibold text-foreground">{t('detail.subtitle', { id: displayId })}</h1>
          <p className="text-sm text-muted-foreground">{automationId}</p>
        </div>
        <div className="flex items-center gap-3">
          {automationStatus !== undefined && (
            <Badge variant={getStatusBadgeVariant(automationStatus)}>{getStatusLabel(automationStatus, tRuns)}</Badge>
          )}
          <Button variant="outline" onClick={() => navigate('/app/automations')}>
            {t('actions.backToList')}
          </Button>
        </div>
      </div>

      {isAutomationLoading && !automation && (
        <Card className="p-6">
          <p className="text-sm text-muted-foreground">{tRuns('automation.loading')}</p>
        </Card>
      )}

      {!isAutomationLoading && automationError && !automation && (
        <Card className="space-y-4 p-6">
          <p className="text-base text-destructive">{t('detail.messages.failedToLoad')}</p>
          <Button variant="outline" onClick={() => navigate('/app/automations')}>
            {t('actions.backToList')}
          </Button>
        </Card>
      )}

      {!isAutomationLoading && !automation && !automationError && (
        <Card className="space-y-4 p-6">
          <p className="text-base text-muted-foreground">{t('detail.messages.noAutomation')}</p>
          <Button variant="outline" onClick={() => navigate('/app/automations')}>
            {t('actions.backToList')}
          </Button>
        </Card>
      )}

      {showAutomationContent && (
        <>
          <Card className="space-y-4 p-6">
            <p className="text-xs uppercase tracking-[0.4em] text-muted-foreground">{t('detail.sections.overview')}</p>
            <div className="mt-2 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {infoRow(t('detail.fields.automationId'), displayId)}
              {infoRow(
                t('detail.fields.status'),
                automationStatus !== undefined ? getStatusLabel(automationStatus, tRuns) : t('messages.unknown'),
              )}
              {infoRow(t('detail.fields.author'), automation?.authorId?.id ?? t('detail.messages.unknownUser'))}
              {infoRow(t('detail.fields.createdAt'), formatDateTime(automationCreatedAt, t('messages.unknown')))}
              {infoRow(t('detail.fields.updatedAt'), formatDateTime(automationUpdatedAt))}
              {infoRow(t('detail.fields.databaseRoot'), automation?.databaseRootResourceId?.id ?? '—')}
              {infoRow(t('detail.fields.workloadRoot'), automation?.workloadRootResourceId?.id ?? '—')}
            </div>
          </Card>

          <Card className="space-y-4 p-6">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div>
                <p className="text-xs uppercase tracking-[0.4em] text-muted-foreground">{t('detail.run.title')}</p>
                <p className="text-sm text-muted-foreground">{linkedRunId ?? t('detail.run.notLinked')}</p>
              </div>
              {linkedRunId && (
                <div className="flex flex-wrap gap-2">
                  <Button size="sm" variant="outline" onClick={handleViewRun}>
                    {t('detail.run.viewRun')}
                  </Button>
                  {grafanaUrl && (
                    <Button size="sm" variant="secondary" asChild>
                      <a href={grafanaUrl} target="_blank" rel="noreferrer">
                        {t('actions.openGrafana')}
                      </a>
                    </Button>
                  )}
                </div>
              )}
            </div>
            {linkedRunId ? (
              <div className="grid gap-4 sm:grid-cols-2">
                {infoRow(
                  t('detail.run.status'),
                  runStatus !== undefined ? getStatusLabel(runStatus, tRuns) : t('messages.unknown'),
                )}
                {infoRow(t('detail.run.createdAt'), formatDateTime(runCreatedAt, t('messages.unknown')))}
                {infoRow(t('detail.run.updatedAt'), formatDateTime(runUpdatedAt))}
              </div>
            ) : (
              <p className="text-sm text-muted-foreground">{t('detail.run.notLinkedHint')}</p>
            )}
          </Card>

          <Card className="space-y-6 p-6">
            <ResourceTreeSection title={tRuns('automation.databaseTree')} tree={databaseTree} isLoading={isDatabaseLoading} t={tRuns} />
            <ResourceTreeSection title={tRuns('automation.workloadTree')} tree={workloadTree} isLoading={isWorkloadLoading} t={tRuns} />
          </Card>
        </>
      )}
    </div>
  )
}
