import { useMemo, useState } from 'react'
import dayjs from 'dayjs'
import { LayoutTemplate, RefreshCcw, Sparkles, Tag as TagIcon } from 'lucide-react'
import { useTemplatesQuery, useTemplateTagsQuery } from '@/app/features/templates/api'
import type { TemplateSummary, TemplateTag } from '@/app/features/templates/types'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { useTranslation } from '@/i18n/use-translation'
import { cn } from '@/lib/utils'

const tagKey = (tag: TemplateTag) => `${tag.key}:${tag.value}`

const isTagSelected = (selected: TemplateTag[], candidate: TemplateTag) =>
  selected.some((tag) => tagKey(tag) === tagKey(candidate))

export const TemplatesPage = () => {
  const { t } = useTranslation('templates')
  const [search, setSearch] = useState('')
  const [selectedTags, setSelectedTags] = useState<TemplateTag[]>([])

  const formatDetails = (template: TemplateSummary) => {
    if (template.kind === 'machine' && template.machineInfo) {
      const parts: string[] = []
      if (template.machineInfo.cores) parts.push(`${template.machineInfo.cores} vCPU`)
      if (template.machineInfo.memory) parts.push(`${template.machineInfo.memory} GB RAM`)
      if (template.machineInfo.disk) parts.push(`${template.machineInfo.disk} GB SSD`)
      return parts.join(' · ')
    }

    if (template.kind === 'database' && template.prebuiltImageId) {
      return t('details.databaseImage', { value: template.prebuiltImageId })
    }

    if (template.kind === 'stroppy') {
      const parts: string[] = []
      if (template.stroppyFilePaths?.length) {
        parts.push(t('details.stroppyFiles', { count: template.stroppyFilePaths.length }))
      }
      if (template.stroppyCmd) {
        parts.push(t('details.stroppyCmd', { value: template.stroppyCmd }))
      }
      return parts.join(' · ')
    }

    return t('details.none')
  }

  const filters = useMemo(
    () => ({
      name: search,
      tags: selectedTags,
    }),
    [search, selectedTags],
  )

  const { data: templates = [], isLoading, isFetching, refetch } = useTemplatesQuery(filters)
  const { data: availableTags = [], isLoading: isTagsLoading } = useTemplateTagsQuery()

  const uniqueTags = useMemo(() => {
    const unique = new Map<string, TemplateTag>()
    availableTags.forEach((tag) => unique.set(tagKey(tag), tag))
    return Array.from(unique.values())
  }, [availableTags])

  const toggleTag = (tag: TemplateTag) => {
    setSelectedTags((current) => {
      if (isTagSelected(current, tag)) {
        return current.filter((item) => tagKey(item) !== tagKey(tag))
      }
      return [...current, tag]
    })
  }

  const clearTags = () => setSelectedTags([])

  const kindBadgeVariant = (kind: TemplateSummary['kind']): 'default' | 'secondary' | 'outline' => {
    if (kind === 'stroppy') return 'default'
    if (kind === 'machine') return 'secondary'
    if (kind === 'database') return 'outline'
    return 'outline'
  }

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <div className="space-y-1">
          <p className="flex items-center gap-2 text-xs uppercase tracking-[0.5em] text-muted-foreground">
            <LayoutTemplate className="h-4 w-4" />
            {t('page.title')}
          </p>
          <h1 className="text-3xl font-semibold text-foreground">{t('page.subtitle')}</h1>
          <p className="text-sm text-muted-foreground">{t('page.adminOnly')}</p>
        </div>
        <Button variant="outline" size="sm" onClick={() => refetch()} disabled={isFetching}>
          <RefreshCcw className="mr-2 h-4 w-4" />
          {t('filters.refresh')}
        </Button>
      </header>

      <Card className="space-y-4 rounded-2xl p-4 shadow-sm">
        <div className="grid gap-4 md:grid-cols-3">
          <div className="md:col-span-2">
            <label className="mb-2 block text-xs uppercase tracking-[0.3em] text-muted-foreground">
              {t('filters.searchLabel')}
            </label>
            <Input
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder={t('filters.searchPlaceholder')}
              className="h-11"
            />
          </div>
          <div className="flex flex-col gap-2 rounded-xl border border-border/60 bg-muted/30 p-3">
            <div className="flex items-center justify-between">
              <p className="text-xs uppercase tracking-[0.3em] text-muted-foreground">{t('filters.tagsLabel')}</p>
              <Button
                variant="ghost"
                size="sm"
                className="h-7 px-2 text-xs"
                onClick={clearTags}
                disabled={selectedTags.length === 0}
              >
                {t('filters.clearTags')}
              </Button>
            </div>
            <div className="flex flex-wrap gap-2">
              {isTagsLoading && <span className="text-xs text-muted-foreground">{t('filters.tagsLoading')}</span>}
              {!isTagsLoading && uniqueTags.length === 0 && (
                <span className="text-xs text-muted-foreground">{t('filters.noTags')}</span>
              )}
              {!isTagsLoading &&
                uniqueTags.map((tag) => {
                  const selected = isTagSelected(selectedTags, tag)
                  return (
                    <Badge
                      key={tagKey(tag)}
                      variant={selected ? 'default' : 'outline'}
                      className={cn(
                        'cursor-pointer border border-border/60',
                        selected && 'shadow-sm',
                        'hover:bg-primary/10 hover:text-foreground',
                      )}
                      onClick={() => toggleTag(tag)}
                    >
                      <TagIcon className="mr-1 h-3 w-3" />
                      {tag.key}:{tag.value}
                    </Badge>
                  )
                })}
            </div>
          </div>
        </div>
      </Card>

      <Card className="overflow-hidden rounded-2xl">
        <div className="flex items-center justify-between border-b border-border/70 px-4 py-3">
          <div>
            <p className="text-sm font-semibold text-foreground">{t('table.title')}</p>
            <p className="text-xs uppercase tracking-[0.3em] text-muted-foreground">{t('table.subtitle')}</p>
          </div>
          <Badge variant="outline" className="flex items-center gap-2">
            <Sparkles className="h-3 w-3" />
            {t('table.total', { count: templates.length })}
          </Badge>
        </div>

        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="text-xs uppercase tracking-[0.25em] text-muted-foreground">
                {t('table.columns.name')}
              </TableHead>
              <TableHead className="text-xs uppercase tracking-[0.25em] text-muted-foreground">
                {t('table.columns.type')}
              </TableHead>
              <TableHead className="text-xs uppercase tracking-[0.25em] text-muted-foreground">
                {t('table.columns.details')}
              </TableHead>
              <TableHead className="text-xs uppercase tracking-[0.25em] text-muted-foreground">
                {t('table.columns.tags')}
              </TableHead>
              <TableHead className="text-xs uppercase tracking-[0.25em] text-muted-foreground">
                {t('table.columns.updatedAt')}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {templates.map((template) => (
              <TableRow key={template.id || template.name}>
                <TableCell>
                  <div className="space-y-1">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-semibold text-foreground">{template.name}</span>
                      {template.isDefault && (
                        <Badge variant="secondary" className="h-5 px-2 text-[11px]">
                          {t('badges.default')}
                        </Badge>
                      )}
                    </div>
                    <p className="font-mono text-[11px] text-muted-foreground">{template.id}</p>
                  </div>
                </TableCell>
                <TableCell>
                  <Badge variant={kindBadgeVariant(template.kind)} className="capitalize">
                    {t(`kinds.${template.kind}`)}
                  </Badge>
                </TableCell>
                <TableCell>
                  <p className="text-sm text-muted-foreground">{formatDetails(template)}</p>
                </TableCell>
                <TableCell>
                  <div className="flex flex-wrap gap-1">
                    {template.tags.length === 0 && <span className="text-xs text-muted-foreground">—</span>}
                    {template.tags.map((tag) => (
                      <Badge key={tagKey(tag)} variant="outline" className="border-dashed text-[11px]">
                        {tag.key}:{tag.value}
                      </Badge>
                    ))}
                  </div>
                </TableCell>
                <TableCell>
                  <div className="text-sm text-muted-foreground">
                    {template.updatedAt
                      ? dayjs(template.updatedAt).format('DD MMM YYYY, HH:mm')
                      : template.createdAt
                        ? dayjs(template.createdAt).format('DD MMM YYYY, HH:mm')
                        : '—'}
                  </div>
                </TableCell>
              </TableRow>
            ))}
            {isLoading && (
              <TableRow>
                <TableCell colSpan={5} className="py-10 text-center text-sm text-muted-foreground">
                  {t('table.loading')}
                </TableCell>
              </TableRow>
            )}
            {!isLoading && templates.length === 0 && (
              <TableRow>
                <TableCell colSpan={5} className="py-10 text-center text-sm text-muted-foreground">
                  {t('table.empty')}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </Card>
    </div>
  )
}
