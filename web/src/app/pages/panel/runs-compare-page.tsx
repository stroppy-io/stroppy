import { useMemo, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import dayjs from 'dayjs'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card } from '@/components/ui/card'
import { useTranslation } from '@/i18n/use-translation'
import type { RunSummary } from '@/app/features/runs/types'
import { buildComparisonSections, countDifferences } from '@/app/features/runs/comparison-sections'
import { getStatusLabel } from '@/app/features/runs/utils'

interface CompareLocationState {
  runs?: RunSummary[]
}

const formatDefaultValue = (value?: string | number) => {
  if (value === undefined || value === null || value === '' || value === -1) {
    return '—'
  }
  return String(value)
}

export const RunsComparePage = () => {
  const navigate = useNavigate()
  const { t } = useTranslation('runs')
  const location = useLocation()
  const state = location.state as CompareLocationState | undefined
  const runs = state?.runs ?? []
  const canCompare = runs.length === 2

  const [collapsedFields, setCollapsedFields] = useState<Record<string, boolean>>({})

  const sections = useMemo(() => buildComparisonSections(t), [t])

  if (!canCompare) {
    return (
      <div className="space-y-6">
        <Card className="space-y-4 p-6">
          <p className="text-base text-muted-foreground">{t('messages.selectRunsToCompare')}</p>
          <Button onClick={() => navigate('/app/runs')}>{t('actions.backToList')}</Button>
        </Card>
      </div>
    )
  }

  const [runA, runB] = runs
  const differenceCount = countDifferences(sections, runA, runB)

  const toggleField = (key: string) =>
    setCollapsedFields((prev) => ({
      ...prev,
      [key]: !prev[key],
    }))

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <p className="text-xs uppercase tracking-[0.5em] text-muted-foreground">{t('comparison.title')}</p>
          <h1 className="text-2xl font-semibold text-foreground">{t('comparison.differencesHighlighted')}</h1>
        </div>
        <Button variant="outline" onClick={() => navigate('/app/runs')}>
          {t('actions.backToList')}
        </Button>
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        {[runA, runB].map((run, index) => (
          <Card key={run.id} className="space-y-2 p-4">
            <div className="flex items-center justify-between gap-2">
              <p className="text-xs uppercase tracking-[0.3em] text-muted-foreground">
                {index === 0 ? t('comparison.run1') : t('comparison.run2')}
              </p>
              <Badge variant="secondary">{getStatusLabel(run.status, t)}</Badge>
            </div>
            <p className="text-lg font-semibold text-foreground">{run.workloadName ?? '—'}</p>
            <p className="text-xs font-mono text-muted-foreground">{run.id}</p>
            <p className="text-xs text-muted-foreground">
              {run.createdAt ? dayjs(run.createdAt).format('DD MMM YYYY, HH:mm') : t('meta.unknownDate')}
            </p>
          </Card>
        ))}
      </div>

      <Card className="flex flex-wrap items-center gap-3 p-4">
        <Badge variant="outline" className="text-xs">
          {t('comparison.totalDifferences')}: {differenceCount}
        </Badge>
        <p className="text-sm text-muted-foreground">
          {differenceCount > 0 ? t('comparison.differencesFound') : t('comparison.identical')}
        </p>
      </Card>

      <div className="space-y-6">
        {sections.map((section) => (
          <div key={section.key} className="space-y-3">
            <div className="flex items-center gap-2">
              <span className="text-xs font-semibold uppercase tracking-[0.3em] text-muted-foreground">{section.label}</span>
              <div className="h-px flex-1 bg-border/60" />
            </div>
            {section.fields.map((field) => {
              const rawA = field.getRaw(runA)
              const rawB = field.getRaw(runB)
              const valueA = field.format ? field.format(runA) : formatDefaultValue(rawA)
              const valueB = field.format ? field.format(runB) : formatDefaultValue(rawB)
              const isDifferent = rawA !== rawB
              const isCollapsed = collapsedFields[field.key] ?? false
              return (
                <Card
                  key={field.key}
                  className={`space-y-3 border border-border/70 bg-card/60 p-4 text-sm shadow-sm ${isDifferent ? 'border-primary/70 bg-primary/5 shadow-primary/10' : ''}`}
                >
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <span className="text-xs font-semibold uppercase tracking-[0.3em] text-muted-foreground">{field.label}</span>
                    <div className="flex items-center gap-2">
                      <Badge variant={isDifferent ? 'default' : 'secondary'} className="text-[11px]">
                        {isDifferent ? t('comparison.different') : t('comparison.identical')}
                      </Badge>
                      <Button variant="ghost" size="sm" onClick={() => toggleField(field.key)}>
                        {isCollapsed ? t('comparison.expand') : t('comparison.collapse')}
                      </Button>
                    </div>
                  </div>
                  {!isCollapsed && (
                    <div className="grid gap-3 sm:grid-cols-2">
                      <div className="rounded-xl border border-border/70 bg-background/80 p-3">
                        <p className="text-xs uppercase tracking-[0.2em] text-muted-foreground">{t('comparison.run1')}</p>
                        <p className="mt-1 text-sm font-medium text-foreground">{valueA}</p>
                      </div>
                      <div className="rounded-xl border border-border/70 bg-background/80 p-3">
                        <p className="text-xs uppercase tracking-[0.2em] text-muted-foreground">{t('comparison.run2')}</p>
                        <p className="mt-1 text-sm font-medium text-foreground">{valueB}</p>
                      </div>
                    </div>
                  )}
                </Card>
              )
            })}
          </div>
        ))}
      </div>
    </div>
  )
}
