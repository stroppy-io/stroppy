import { useMemo } from 'react'
import { create } from '@bufbuild/protobuf'
import { timestampDate } from '@bufbuild/protobuf/wkt'
import { useQuery, useTransport } from '@connectrpc/connect-query'
import { searchTemplates, listTemplatesTags } from '@/proto/panel/template-TemplateService_connectquery.ts'
import { SearchTemplatesRequestSchema } from '@/proto/panel/template_pb.ts'
import { Tag_ListSchema, TagSchema, type Tag_List } from '@/proto/panel/types_pb.ts'
import type { Template, SearchTemplatesRequest } from '@/proto/panel/template_pb.ts'
import type { Tag } from '@/proto/panel/types_pb.ts'
import type { TemplateKind, TemplateSummary, TemplatesFilters, TemplateTag } from './types'

const resolveTemplateKind = (template: Template): TemplateKind => {
  switch (template.templateData.case) {
    case 'machineDeployment':
      return 'machine'
    case 'databaseDeployment':
      return 'database'
    case 'stroppyDeployment':
      return 'stroppy'
    default:
      return 'unknown'
  }
}

const mapTemplate = (template: Template): TemplateSummary => {
  const kind = resolveTemplateKind(template)
  const machineInfo = template.templateData.case === 'machineDeployment' ? template.templateData.value.machineInfo : undefined
  const databaseInfo = template.templateData.case === 'databaseDeployment' ? template.templateData.value : undefined
  const stroppyInfo = template.templateData.case === 'stroppyDeployment' ? template.templateData.value : undefined

  return {
    id: template.id?.id ?? '',
    name: template.name,
    authorId: template.authorId?.id,
    isDefault: template.isDefault ?? false,
    tags: template.tags.map((tag) => ({ key: tag.key, value: tag.value })),
    kind,
    machineInfo: machineInfo
      ? {
          cores: machineInfo.cores,
          memory: machineInfo.memory,
          disk: machineInfo.disk,
        }
      : undefined,
    prebuiltImageId: databaseInfo?.prebuiltImageId,
    stroppyFilePaths: stroppyInfo?.files?.map((file) => file.path).filter(Boolean),
    stroppyCmd: stroppyInfo?.cmd,
    createdAt: template.timing?.createdAt ? timestampDate(template.timing.createdAt) : undefined,
    updatedAt: template.timing?.updatedAt ? timestampDate(template.timing.updatedAt) : undefined,
  }
}

const buildTagsPayload = (tags: TemplateTag[]): Tag_List =>
  create(Tag_ListSchema, {
    tags: tags.map((tag) => create(TagSchema, { key: tag.key, value: tag.value })),
  })

const buildRequest = (filters: TemplatesFilters) =>
  create(SearchTemplatesRequestSchema, {
    name: filters.name?.trim() ?? '',
    tagsList: filters.tags?.length ? buildTagsPayload(filters.tags) : undefined,
  })

export const useTemplatesQuery = (filters: TemplatesFilters) => {
  const transport = useTransport()
  const request = useMemo(() => buildRequest(filters), [filters])

  return useQuery(searchTemplates, request, {
    transport,
    select: (response) => response.templates.map(mapTemplate),
  })
}

export const useTemplateTagsQuery = () => {
  const transport = useTransport()
  return useQuery(listTemplatesTags, undefined, {
    transport,
    select: (payload) => payload.tags.map((tag) => ({ key: tag.key, value: tag.value })),
  })
}
