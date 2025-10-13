// Сервис для работы с документацией
export interface DocSection {
  id: string
  title: string
  description: string
  path: string
  files: DocFile[]
  subsections?: DocSection[] // Подразделы
  parentId?: string // ID родительского раздела
}

export interface DocFile {
  id: string
  title: string
  path: string
  content?: string
  lastModified?: string
  language: string
  sectionId: string // ID раздела, к которому принадлежит файл
  subsectionId?: string // ID подраздела, если файл находится в подразделе
}

export interface DocStructure {
  sections: DocSection[]
}

// Поддерживаемые языки
export const SUPPORTED_LANGUAGES = ['ru', 'en'] as const
export type SupportedLanguage = typeof SUPPORTED_LANGUAGES[number]

// Динамический импорт всех markdown файлов из папки docs/
const markdownModules = import.meta.glob('/src/docs/**/*.md', { query: '?raw', import: 'default' })

// Автоматическое сканирование структуры документации из доступных файлов
const scanDocsStructure = async (language: SupportedLanguage = 'ru'): Promise<DocStructure> => {
  const sections: DocSection[] = []
  
  // Структура для хранения разделов и подразделов
  const sectionMap = new Map<string, { files: DocFile[], subsections: Map<string, DocFile[]> }>()
  
  // Загружаем все markdown файлы динамически
  const filePromises = Object.entries(markdownModules).map(async ([filePath, importFn]) => {
    try {
      const content = await importFn() as string
      return { filePath, content }
    } catch (error) {
      console.warn(`Failed to load markdown file: ${filePath}`, error)
      return null
    }
  })
  
  const files = (await Promise.all(filePromises)).filter(Boolean) as Array<{ filePath: string; content: string }>
  
  // Проходим по всем файлам и группируем их по разделам и подразделам
  for (const { filePath, content } of files) {
    // Преобразуем путь из /src/docs/... в /docs/...
    const normalizedPath = filePath.replace('/src', '')
    
    // Извлекаем язык из имени файла (например: installation_ru.md -> ru)
    const pathParts = normalizedPath.split('/')
    const fileName = pathParts[pathParts.length - 1]
    const fileNameWithoutExt = fileName.replace('.md', '')
    
    // Проверяем, есть ли суффикс языка в имени файла
    const languageMatch = fileNameWithoutExt.match(/_([a-z]{2})$/)
    const fileLanguage = languageMatch ? languageMatch[1] as SupportedLanguage : 'ru'
    
    // Пропускаем файлы, которые не соответствуют выбранному языку
    if (fileLanguage !== language) {
      continue
    }
    
    // Извлекаем базовое имя файла без суффикса языка
    const baseFileName = languageMatch ? fileNameWithoutExt.replace(`_${fileLanguage}`, '') : fileNameWithoutExt
    
    if (pathParts.length >= 3 && pathParts[1] === 'docs') {
      const sectionId = pathParts[2]
      
      // Инициализируем раздел, если его еще нет
      if (!sectionMap.has(sectionId)) {
        sectionMap.set(sectionId, { files: [], subsections: new Map() })
      }
      
      const title = extractTitleFromContent(content) || baseFileName
      const docFile: DocFile = {
        id: `${sectionId}-${baseFileName}`,
        title: title,
        path: normalizedPath,
        language: fileLanguage,
        sectionId: sectionId
      }
      
      // Проверяем, есть ли подраздел (глубина 5: /docs/section/subsection/file.md)
      // Пример: /docs/api/authentication_ru.md -> length 4 (файл в корне раздела)
      // Пример: /docs/api/test/first-test_ru.md -> length 5 (файл в подразделе)
      if (pathParts.length >= 5) {
        const subsectionId = pathParts[3]
        docFile.subsectionId = subsectionId
        
        // Инициализируем подраздел, если его еще нет
        if (!sectionMap.get(sectionId)!.subsections.has(subsectionId)) {
          sectionMap.get(sectionId)!.subsections.set(subsectionId, [])
        }
        
        sectionMap.get(sectionId)!.subsections.get(subsectionId)!.push(docFile)
      } else {
        // Файл находится в корне раздела
        sectionMap.get(sectionId)!.files.push(docFile)
      }
    }
  }
  
  // Создаем разделы на основе найденных папок
  for (const [sectionId, sectionData] of sectionMap) {
    // Автоматически генерируем название и описание на основе ID раздела
    const title = generateSectionTitle(sectionId, language)
    const description = generateSectionDescription(sectionId, language)
    
    // Создаем подразделы
    const subsections: DocSection[] = []
    for (const [subsectionId, subsectionFiles] of sectionData.subsections) {
      const subsectionTitle = generateSectionTitle(subsectionId, language)
      const subsectionDescription = generateSectionDescription(subsectionId, language)
      
      subsections.push({
        id: `${sectionId}-${subsectionId}`,
        title: subsectionTitle,
        description: subsectionDescription,
        path: `/docs/${sectionId}/${subsectionId}`,
        files: subsectionFiles,
        parentId: sectionId
      })
    }
    
    // Сортируем подразделы по алфавиту
    subsections.sort((a, b) => a.id.localeCompare(b.id))
    
    sections.push({
      id: sectionId,
      title: title,
      description: description,
      path: `/docs/${sectionId}`,
      files: sectionData.files,
      subsections: subsections.length > 0 ? subsections : undefined
    })
  }
  
  // Сортируем разделы по алфавиту
  sections.sort((a, b) => a.id.localeCompare(b.id))

  return { sections }
}

// Автоматическая генерация названия раздела на основе ID
const generateSectionTitle = (sectionId: string, language: SupportedLanguage = 'ru'): string => {
  const titles: Record<SupportedLanguage, Record<string, string>> = {
    ru: {
      'getting-started': 'Быстрый старт',
      'configuration': 'Конфигурация',
      'api': 'API Reference',
      'databases': 'Базы данных',
      'cloud': 'Облачные платформы',
      'kubernetes': 'Kubernetes',
      'deployment': 'Развертывание',
      'troubleshooting': 'Решение проблем',
      'examples': 'Примеры',
      'reference': 'Справочник',
      'proto': 'Protocol Buffers',
      'readme': 'Документация'
    },
    en: {
      'getting-started': 'Getting Started',
      'configuration': 'Configuration',
      'api': 'API Reference',
      'databases': 'Databases',
      'cloud': 'Cloud Platforms',
      'kubernetes': 'Kubernetes',
      'deployment': 'Deployment',
      'troubleshooting': 'Troubleshooting',
      'examples': 'Examples',
      'reference': 'Reference',
      'proto': 'Protocol Buffers',
      'readme': 'Documentation'
    }
  }
  
  return titles[language][sectionId] || sectionId
    .split('-')
    .map(word => word.charAt(0).toUpperCase() + word.slice(1))
    .join(' ')
}

// Автоматическая генерация описания раздела на основе ID
const generateSectionDescription = (sectionId: string, language: SupportedLanguage = 'ru'): string => {
  const descriptions: Record<SupportedLanguage, Record<string, string>> = {
    ru: {
      'getting-started': 'Начните работу с Stroppy Cloud Panel за несколько минут',
      'configuration': 'Настройка параметров тестирования и окружения',
      'api': 'Документация по REST API для интеграции',
      'databases': 'Поддерживаемые базы данных и их особенности',
      'cloud': 'Развертывание и тестирование в облачных средах',
      'kubernetes': 'Развертывание в Kubernetes кластерах',
      'deployment': 'Инструкции по развертыванию системы',
      'troubleshooting': 'Решение проблем и отладка',
      'examples': 'Примеры использования и конфигурации',
      'reference': 'Справочная документация',
      'proto': 'Документация по Protocol Buffers и схемам данных',
      'readme': 'Общая информация о документации'
    },
    en: {
      'getting-started': 'Get started with Stroppy Cloud Panel in minutes',
      'configuration': 'Configuration of testing parameters and environment',
      'api': 'REST API documentation for integration',
      'databases': 'Supported databases and their features',
      'cloud': 'Deployment and testing in cloud environments',
      'kubernetes': 'Deployment in Kubernetes clusters',
      'deployment': 'System deployment instructions',
      'troubleshooting': 'Problem solving and debugging',
      'examples': 'Usage examples and configurations',
      'reference': 'Reference documentation',
      'proto': 'Protocol Buffers and data schemas documentation',
      'readme': 'General documentation information'
    }
  }
  
  return descriptions[language][sectionId] || `Documentation for ${generateSectionTitle(sectionId, language)} section`
}


// Загрузка markdown файла
const loadMarkdownFile = async (filePath: string): Promise<string | null> => {
  try {
    // Преобразуем путь из /docs/... в /src/docs/...
    const srcPath = `/src${filePath}`
    const importFn = markdownModules[srcPath]
    
    if (importFn) {
      return await importFn() as string
    }
    
    return null
  } catch (error) {
    console.warn(`Failed to load markdown file: ${filePath}`, error)
    return null
  }
}

// Извлечение заголовка из содержимого markdown
const extractTitleFromContent = (content: string): string | null => {
  const lines = content.split('\n')
  for (const line of lines) {
    const trimmed = line.trim()
    if (trimmed.startsWith('# ')) {
      return trimmed.substring(2).trim()
    }
  }
  return null
}

// Кэш для структуры документации по языкам
const docsStructureCache = new Map<SupportedLanguage, DocStructure>()

// API функции
export const docsService = {
  // Получить структуру документации
  async getSections(language: SupportedLanguage = 'ru'): Promise<DocSection[]> {
    if (!docsStructureCache.has(language)) {
      const structure = await scanDocsStructure(language)
      docsStructureCache.set(language, structure)
    }
    return docsStructureCache.get(language)!.sections
  },

  // Получить конкретный раздел
  async getSection(id: string, language: SupportedLanguage = 'ru'): Promise<DocSection | null> {
    const sections = await this.getSections(language)
    return sections.find(s => s.id === id) || null
  },

  // Получить содержимое файла
  async getFile(path: string, language: SupportedLanguage = 'ru'): Promise<DocFile | null> {
    try {
      // Если путь не содержит суффикс языка, добавляем его
      let filePath = path
      if (!path.includes(`_${language}.md`)) {
        const pathParts = path.split('/')
        const fileName = pathParts[pathParts.length - 1]
        const fileNameWithoutExt = fileName.replace('.md', '')
        pathParts[pathParts.length - 1] = `${fileNameWithoutExt}_${language}.md`
        filePath = pathParts.join('/')
      }
      
      const content = await loadMarkdownFile(filePath)
      if (content) {
        const pathParts = filePath.split('/')
        const fileName = pathParts[pathParts.length - 1]
        const fileNameWithoutExt = fileName.replace('.md', '')
        
        // Убираем суффикс языка из ID
        const baseFileName = fileNameWithoutExt.replace(`_${language}`, '')
        const title = extractTitleFromContent(content) || baseFileName
        
        // Определяем раздел и подраздел из пути
        const sectionId = pathParts[pathParts.length - 2] // папка раздела или подраздела
        let subsectionId: string | undefined
        
        // Если глубина пути больше 3, значит есть подраздел
        if (pathParts.length > 4) {
          subsectionId = pathParts[pathParts.length - 2] // подраздел
          const mainSectionId = pathParts[pathParts.length - 3] // основной раздел
          return {
            id: `${mainSectionId}-${subsectionId}-${baseFileName}`,
            title: title,
            path: filePath,
            content: content,
            language: language,
            sectionId: mainSectionId,
            subsectionId: subsectionId
          }
        } else {
          return {
            id: `${sectionId}-${baseFileName}`,
            title: title,
            path: filePath,
            content: content,
            language: language,
            sectionId: sectionId
          }
        }
      }
      return null
    } catch (error) {
      console.error(`Failed to load file: ${path}`, error)
      return null
    }
  },

  // Поиск по документации
  async searchDocs(query: string, language: SupportedLanguage = 'ru'): Promise<DocFile[]> {
    const sections = await this.getSections(language)
    const results: DocFile[] = []
    const lowerCaseQuery = query.toLowerCase()

    for (const section of sections) {
      // Поиск в файлах основного раздела
      for (const file of section.files) {
        try {
          const fileContent = await this.getFile(file.path, language)
          if (fileContent && fileContent.content) {
            // Поиск по заголовку и содержимому
            const searchText = `${fileContent.title} ${fileContent.content}`.toLowerCase()
            if (searchText.includes(lowerCaseQuery)) {
              results.push(fileContent)
            }
          }
        } catch (error) {
          console.debug(`Error searching in file: ${file.path}`)
        }
      }
      
      // Поиск в подразделах
      if (section.subsections) {
        for (const subsection of section.subsections) {
          for (const file of subsection.files) {
            try {
              const fileContent = await this.getFile(file.path, language)
              if (fileContent && fileContent.content) {
                // Поиск по заголовку и содержимому
                const searchText = `${fileContent.title} ${fileContent.content}`.toLowerCase()
                if (searchText.includes(lowerCaseQuery)) {
                  results.push(fileContent)
                }
              }
            } catch (error) {
              console.debug(`Error searching in file: ${file.path}`)
            }
          }
        }
      }
    }

    return results
  },

  // Очистить кэш (для обновления структуры)
  clearCache(language?: SupportedLanguage): void {
    if (language) {
      docsStructureCache.delete(language)
    } else {
      docsStructureCache.clear()
    }
  },

  // Получить поддерживаемые языки
  getSupportedLanguages(): SupportedLanguage[] {
    return [...SUPPORTED_LANGUAGES]
  }
}