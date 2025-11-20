export type TemplateKind = 'machine' | 'database' | 'stroppy' | 'unknown'

export interface TemplateTag {
  key: string
  value: string
}

export interface TemplateSummary {
  id: string
  name: string
  kind: TemplateKind
  isDefault: boolean
  tags: TemplateTag[]
  createdAt?: Date
  updatedAt?: Date
  authorId?: string
  machineInfo?: {
    cores?: number
    memory?: number
    disk?: number
  }
  prebuiltImageId?: string
  stroppyFilePaths?: string[]
  stroppyCmd?: string
}

export interface TemplatesFilters {
  name?: string
  tags?: TemplateTag[]
}
