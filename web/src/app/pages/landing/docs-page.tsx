import { useCallback, useEffect, useState } from 'react'
import { Input } from '@/components/ui/input'
import { Card } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { MarkdownRenderer } from '@/components/docs/markdown-renderer'
import { docsService, type DocFile, type DocSection, type SupportedLanguage } from '@/lib/docs/service'
import { useTranslation } from '@/i18n/use-translation'

export const LandingDocsPage = () => {
  const { t, getCurrentLanguage } = useTranslation('landing')
  const [sections, setSections] = useState<DocSection[]>([])
  const [currentFile, setCurrentFile] = useState<DocFile | null>(null)
  const [searchQuery, setSearchQuery] = useState('')
  const [searchResults, setSearchResults] = useState<DocFile[]>([])
  const [language, setLanguage] = useState<SupportedLanguage>((getCurrentLanguage() as SupportedLanguage) ?? 'ru')
  const [loading, setLoading] = useState(true)

  const selectFile = useCallback(async (file: DocFile) => {
    const fullFile = await docsService.getFile(file.path, language)
    setCurrentFile(fullFile)
    const hashSection = file.subsectionId ? `${file.sectionId}-${file.subsectionId}` : file.sectionId
    window.location.hash = `#${hashSection}/${file.id}`
  }, [language])

  const loadSections = useCallback(async () => {
    setLoading(true)
    try {
      const result = await docsService.getSections(language)
      setSections(result)

      const hash = window.location.hash.replace('#', '')
      if (hash) {
        const [sectionId, fileId] = hash.split('/')
        const sectionCandidates = result.flatMap((section) => [section, ...(section.subsections ?? [])])
        const section = sectionCandidates.find((item) => item.id === sectionId)
        const file =
          section?.files.find((doc) => doc.id === fileId) ??
          section?.subsections?.flatMap((sub) => sub.files).find((doc) => doc.id === fileId) ??
          result
            .flatMap((sec) => [...sec.files, ...(sec.subsections?.flatMap((sub) => sub.files) ?? [])])
            .find((doc) => doc.id === fileId)
        if (file) {
          await selectFile(file)
          return
        }
      }

      const firstFile = result[0]?.files[0] ?? result[0]?.subsections?.[0]?.files[0]
      if (firstFile) {
        await selectFile(firstFile)
      } else {
        setCurrentFile(null)
      }
    } finally {
      setLoading(false)
    }
  }, [language, selectFile])

  useEffect(() => {
    loadSections().catch((error) => console.error('docs load failed', error))
  }, [loadSections])

  useEffect(() => {
    const handleHashChange = () => {
      const hash = window.location.hash.replace('#', '')
      if (!hash) return
      const [sectionId, fileId] = hash.split('/')
      const section = sections.find((sec) => sec.id === sectionId) ?? sections.flatMap((sec) => sec.subsections ?? []).find((sub) => sub.id === sectionId)
      const file =
        section?.files.find((doc) => doc.id === fileId) ??
        section?.subsections?.flatMap((sub) => sub.files).find((doc) => doc.id === fileId)
      if (file) {
        selectFile(file)
      }
    }
    window.addEventListener('hashchange', handleHashChange)
    return () => window.removeEventListener('hashchange', handleHashChange)
  }, [sections, selectFile])

  useEffect(() => {
    const newLanguage = (getCurrentLanguage() as SupportedLanguage) ?? 'ru'
    if (newLanguage !== language) {
      setLanguage(newLanguage)
      docsService.clearCache()
    }
  }, [getCurrentLanguage, language])

  const handleSearch = async (query: string) => {
    setSearchQuery(query)
    if (!query.trim()) {
      setSearchResults([])
      return
    }
    const results = await docsService.searchDocs(query, language)
    setSearchResults(results.slice(0, 10))
  }

  return (
    <div className="mx-auto flex w-full max-w-6xl flex-col gap-6 px-4 py-12 sm:px-6">
      <header className="space-y-2">
        <p className="text-xs uppercase tracking-[0.5em] text-muted-foreground">{t('documentation.title')}</p>
        <h1 className="text-3xl font-semibold text-foreground">{t('documentation.mainTitle')}</h1>
        <p className="text-sm text-muted-foreground">{t('documentation.mainDescription')}</p>
      </header>

      <div className="flex flex-row gap-6 lg:grid-cols-[280px,1fr]">
        <Card className="rounded-2xl p-4 shadow-sm">
          <div className="space-y-4">
            <div>
              <label className="text-xs uppercase tracking-[0.3em] text-muted-foreground">{t('documentation.search.title')}</label>
              <Input
                value={searchQuery}
                onChange={(event) => handleSearch(event.target.value)}
                placeholder={t('documentation.search.placeholder')}
              />
            </div>

            {searchResults.length > 0 && (
              <div className="rounded-xl border border-border/70 bg-muted/60 p-2">
                <p className="px-2 text-xs uppercase tracking-[0.3em] text-muted-foreground">
                  {t('documentation.search.foundResults', { count: searchResults.length })}
                </p>
                <ul className="mt-2 divide-y divide-border/70 text-sm">
                  {searchResults.map((file) => (
                    <li key={file.id}>
                      <button
                        onClick={() => selectFile(file)}
                        className="w-full px-2 py-2 text-left text-foreground/80 transition hover:bg-card/80"
                      >
                        {file.title}
                      </button>
                    </li>
                  ))}
                </ul>
              </div>
            )}

            <ScrollArea className="max-h-[70vh] pr-2">
              <div className="space-y-4">
                {sections.map((section) => (
                  <div key={section.id} className="space-y-2">
                    <p className="text-xs uppercase tracking-[0.3em] text-muted-foreground">{section.title}</p>
                    <div className="space-y-1">
                      {section.files.map((file) => (
                        <Button
                          key={file.id}
                          variant={currentFile?.id === file.id ? 'default' : 'ghost'}
                          className="w-full justify-start"
                          onClick={() => selectFile(file)}
                        >
                          {file.title}
                        </Button>
                      ))}
                      {section.subsections?.map((subsection) => (
                        <div key={subsection.id} className="pl-2">
                          <p className="text-[11px] uppercase tracking-[0.3em] text-muted-foreground/80">{subsection.title}</p>
                          {subsection.files.map((file) => (
                            <Button
                              key={file.id}
                              variant={currentFile?.id === file.id ? 'default' : 'ghost'}
                              className="w-full justify-start"
                              onClick={() => selectFile(file)}
                            >
                              {file.title}
                            </Button>
                          ))}
                        </div>
                      ))}
                    </div>
                  </div>
                ))}
              </div>
            </ScrollArea>
          </div>
        </Card>

        <Card className="min-h-[60vh] rounded-2xl p-6 shadow-sm">
          {loading && <p className="text-sm text-muted-foreground">{t('documentation.loading')}</p>}
          {!loading && currentFile && currentFile.content && <MarkdownRenderer content={currentFile.content} />}
          {!loading && !currentFile && (
            <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
              {t('documentation.loadingError')}
            </div>
          )}
        </Card>
      </div>
    </div>
  )
}
