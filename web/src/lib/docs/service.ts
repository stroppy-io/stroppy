export interface DocSection {
  id: string
  title: string
  description: string
  path: string
  files: DocFile[]
  subsections?: DocSection[]
  parentId?: string
}

export interface DocFile {
  id: string
  title: string
  path: string
  content?: string
  lastModified?: string
  language: SupportedLanguage
  sectionId: string
  subsectionId?: string
}

export const SUPPORTED_LANGUAGES = ['ru', 'en'] as const
export type SupportedLanguage = (typeof SUPPORTED_LANGUAGES)[number]

type DocStructure = {
  sections: DocSection[]
}

const markdownModules = import.meta.glob('/src/docs/**/*.md', {
  query: '?raw',
  import: 'default',
})

const docsCache = new Map<string, DocStructure>()
const fileCache = new Map<string, string>()

const buildCacheKey = (language: SupportedLanguage) => `docs-${language}`

const extractTitleFromContent = (content: string) => {
  const match = content.match(/^#\s+(.+)$/m)
  return match ? match[1].trim() : undefined
}

const generateSectionTitle = (sectionId: string, language: SupportedLanguage) => {
  const titles: Record<SupportedLanguage, Record<string, string>> = {
    ru: {
      'getting-started': 'Быстрый старт',
      configuration: 'Конфигурация',
      api: 'API Reference',
      'proto-config': 'Protocol Buffers',
    },
    en: {
      'getting-started': 'Getting Started',
      configuration: 'Configuration',
      api: 'API Reference',
      'proto-config': 'Protocol Buffers',
    },
  }

  return (
    titles[language][sectionId] ??
    sectionId
      .split('-')
      .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
      .join(' ')
  )
}

const generateSectionDescription = (sectionId: string, language: SupportedLanguage) => {
  const descriptions: Record<SupportedLanguage, Record<string, string>> = {
    ru: {
      'getting-started': 'Как запустить Stroppy Cloud Panel за несколько минут',
      configuration: 'Настройка параметров тестирования и окружения',
      api: 'Документация по API и интеграциям',
      'proto-config': 'Protocol Buffers описание',
    },
    en: {
      'getting-started': 'Launch Stroppy Cloud Panel within minutes',
      configuration: 'Configure environments and workload parameters',
      api: 'API documentation and integrations',
      'proto-config': 'Protocol Buffers description',
    },
  }

  return descriptions[language][sectionId] ?? ''
}

const scanDocsStructure = async (language: SupportedLanguage): Promise<DocStructure> => {
  const sectionMap = new Map<string, { files: DocFile[]; subsections: Map<string, DocFile[]> }>()

  const fileEntries = await Promise.all(
    Object.entries(markdownModules).map(async ([modulePath, loader]) => {
      try {
        const content = (await loader()) as string
        return { modulePath, content }
      } catch (error) {
        console.warn('Failed to load markdown', modulePath, error)
        return null
      }
    }),
  )

  for (const entry of fileEntries) {
    if (!entry) continue
    const { modulePath, content } = entry

    const normalizedPath = modulePath.replace('/src', '')
    const segments = normalizedPath.split('/')
    const fileName = segments.at(-1) ?? ''
    const baseName = fileName.replace('.md', '')
    const langMatch = baseName.match(/_([a-z]{2})$/)
    const fileLanguage = (langMatch ? langMatch[1] : 'ru') as SupportedLanguage

    if (fileLanguage !== language) continue

    const plainName = langMatch ? baseName.replace(`_${fileLanguage}`, '') : baseName

    if (segments.length < 3 || segments[1] !== 'docs') continue
    const sectionId = segments[2]

    if (!sectionMap.has(sectionId)) {
      sectionMap.set(sectionId, { files: [], subsections: new Map() })
    }

    const docFile: DocFile = {
      id: `${sectionId}-${plainName}`,
      title: extractTitleFromContent(content) ?? plainName,
      path: normalizedPath,
      language: fileLanguage,
      sectionId,
    }

    if (segments.length >= 5) {
      const subsectionId = segments[3]
      docFile.subsectionId = subsectionId
      const bucket = sectionMap.get(sectionId)!
      if (!bucket.subsections.has(subsectionId)) {
        bucket.subsections.set(subsectionId, [])
      }
      bucket.subsections.get(subsectionId)!.push(docFile)
    } else {
      sectionMap.get(sectionId)!.files.push(docFile)
    }
  }

  const sections: DocSection[] = []

  for (const [sectionId, data] of sectionMap) {
    const subsections: DocSection[] = []
    for (const [subId, files] of data.subsections) {
      subsections.push({
        id: `${sectionId}-${subId}`,
        title: generateSectionTitle(subId, language),
        description: generateSectionDescription(subId, language),
        path: `/docs/${sectionId}/${subId}`,
        files,
        parentId: sectionId,
      })
    }
    subsections.sort((a, b) => a.id.localeCompare(b.id))

    sections.push({
      id: sectionId,
      title: generateSectionTitle(sectionId, language),
      description: generateSectionDescription(sectionId, language),
      path: `/docs/${sectionId}`,
      files: data.files,
      subsections: subsections.length ? subsections : undefined,
    })
  }

  sections.sort((a, b) => a.id.localeCompare(b.id))

  return { sections }
}

const resolveModulePath = (docPath: string) => `/src${docPath}`

const findFileMeta = (sections: DocSection[], path: string) =>
  sections
    .flatMap((section) => [
      ...section.files,
      ...(section.subsections?.flatMap((sub) => sub.files) ?? []),
    ])
    .find((file) => file.path === path)

const docsService = {
  async getSections(language: SupportedLanguage): Promise<DocSection[]> {
    const key = buildCacheKey(language)
    if (!docsCache.has(key)) {
      const structure = await scanDocsStructure(language)
      docsCache.set(key, structure)
    }
    return docsCache.get(key)!.sections
  },

  async getSection(sectionId: string, language: SupportedLanguage): Promise<DocSection | null> {
    const sections = await this.getSections(language)
    return (
      sections.find((section) => section.id === sectionId) ??
      sections
        .flatMap((section) => section.subsections ?? [])
        .find((sub) => sub?.id === sectionId) ??
      null
    )
  },

  async getFile(path: string, language: SupportedLanguage): Promise<DocFile> {
    const modulePath = resolveModulePath(path)
    const loader = markdownModules[modulePath]
    if (!loader) {
      throw new Error(`Documentation file not found: ${path}`)
    }
    if (!fileCache.has(path)) {
      const content = (await loader()) as string
      fileCache.set(path, content)
    }
    const sections = await this.getSections(language)
    const meta = findFileMeta(sections, path) ?? {
      id: path,
      title: extractTitleFromContent(fileCache.get(path) ?? '') ?? path,
      path,
      language,
      sectionId: '',
    }

    return {
      ...meta,
      content: fileCache.get(path),
    }
  },

  async searchDocs(query: string, language: SupportedLanguage): Promise<DocFile[]> {
    const normalizedQuery = query.toLowerCase()
    const sections = await this.getSections(language)
    const allFiles = sections.flatMap((section) => [
      ...section.files,
      ...(section.subsections?.flatMap((sub) => sub.files) ?? []),
    ])

    const results: DocFile[] = []
    for (const file of allFiles) {
      let content = fileCache.get(file.path)
      if (!content) {
        const modulePath = resolveModulePath(file.path)
        const loader = markdownModules[modulePath]
        if (!loader) continue
        content = (await loader()) as string
        fileCache.set(file.path, content)
      }

      if (
        file.title.toLowerCase().includes(normalizedQuery) ||
        content.toLowerCase().includes(normalizedQuery)
      ) {
        results.push({
          ...file,
          content,
        })
      }
    }

    return results
  },

  clearCache() {
    docsCache.clear()
    fileCache.clear()
  },
}

export { docsService }
