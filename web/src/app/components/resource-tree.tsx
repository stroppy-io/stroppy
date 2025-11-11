import { Card } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import type { CloudResource_TreeNode } from '@/proto/panel/automate_pb.ts'
import { CloudResource_Status } from '@/proto/panel/automate_pb.ts'

export type TranslateFn = (key: string, options?: Record<string, unknown>) => string

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

interface ResourceTreeSectionProps {
  title: string
  tree?: CloudResource_TreeNode
  isLoading: boolean
  t: TranslateFn
}

export const ResourceTreeSection = ({ title, tree, isLoading, t }: ResourceTreeSectionProps) => (
  <div className="space-y-3">
    <p className="text-xs uppercase tracking-[0.3em] text-muted-foreground">{title}</p>
    {isLoading && <p className="text-sm text-muted-foreground">{t('automation.loading')}</p>}
    {!isLoading && tree && (
      <ul className="space-y-3">
        <ResourceNode node={tree} t={t} />
      </ul>
    )}
    {!isLoading && !tree && <p className="text-sm text-muted-foreground">{t('automation.noResources')}</p>}
  </div>
)
