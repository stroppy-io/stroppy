import React, { useState, useEffect, useMemo, useCallback } from 'react'
import { 
  Layout, 
  Typography, 
  Card, 
  Row, 
  Col, 
  List, 
  Button, 
  Breadcrumb, 
  Spin, 
  Alert, 
  Input
} from 'antd'
import { 
  BookOutlined,
  FileTextOutlined,
  ApiOutlined,
  SettingOutlined,
  RocketOutlined,
  DatabaseOutlined,
  CloudOutlined,
  GlobalOutlined,
  SearchOutlined,
  HomeOutlined
} from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { useTheme } from '../../common/contexts/ThemeContext'
import { useTranslation } from '../../common/hooks/useTranslation'
import MarkdownRenderer from '../../common/components/MarkdownRenderer'
import Header from '../components/Header'
import { docsService, type DocSection, type DocFile, type SupportedLanguage } from '../../common/services/docs'
import TreeView, { flattenTree } from 'react-accessible-treeview'
import type { INode } from 'react-accessible-treeview'

const { Content, Footer } = Layout
const { Title, Paragraph, Text } = Typography

const DocumentationPage: React.FC = () => {
  const navigate = useNavigate()
  const { darkReaderEnabled } = useTheme()
  const { getCurrentLanguage, t } = useTranslation('landing')
  
  const [sections, setSections] = useState<DocSection[]>([])
  const [currentSection, setCurrentSection] = useState<DocSection | null>(null)
  const [currentFile, setCurrentFile] = useState<DocFile | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [searchQuery, setSearchQuery] = useState('')
  const [searchResults, setSearchResults] = useState<DocFile[]>([])
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set())
  const [currentLanguage, setCurrentLanguage] = useState<SupportedLanguage>(getCurrentLanguage() as SupportedLanguage)
  const [searchOverlayVisible, setSearchOverlayVisible] = useState(false)

  // Обработчик изменения hash для загрузки контента
  const handleHashChange = useCallback(async (hash: string) => {
    if (!hash) return
    
    const [sectionId, fileId] = hash.replace('#', '').split('/')
    if (!sectionId) return

    try {
      const section = await docsService.getSection(sectionId, currentLanguage)
      setCurrentSection(section)
      
      if (fileId && section) {
        const file = section.files.find(f => f.id === fileId)
        if (file) {
          const fileContent = await docsService.getFile(file.path, currentLanguage)
          setCurrentFile(fileContent)
        }
      } else {
        // Если нет fileId, очищаем текущий файл
        setCurrentFile(null)
      }
    } catch (err) {
      console.error('Error loading content from hash:', err)
    }
  }, [currentLanguage])

  // Загрузка структуры документации
  useEffect(() => {
    const loadDocs = async () => {
      try {
        setLoading(true)
        const docSections = await docsService.getSections(currentLanguage)
        setSections(docSections)
        
        
        // Загружаем контент из hash если он есть
        const hash = window.location.hash
        if (hash) {
          await handleHashChange(hash)
        } else {
          // Если hash пустой, сбрасываем текущий раздел и файл
          setCurrentSection(null)
          setCurrentFile(null)
        }
      } catch (err) {
        setError(t('documentation.loadingError'))
        console.error('Error loading docs:', err)
      } finally {
        setLoading(false)
      }
    }
    
    loadDocs()
  }, [currentLanguage, handleHashChange, t])

  // Обработчик изменения языка
  useEffect(() => {
    const currentLang = getCurrentLanguage() as SupportedLanguage
    if (currentLang !== currentLanguage) {
      setCurrentLanguage(currentLang)
      // Очищаем кэш документации при смене языка
      docsService.clearCache()
    }
  }, [getCurrentLanguage, currentLanguage])

  // Обработчик изменения hash
  useEffect(() => {
    const handleHashChangeEvent = () => {
      const hash = window.location.hash
      if (hash) {
        handleHashChange(hash)
      } else {
        // Если hash пустой, сбрасываем текущий раздел и файл
        setCurrentSection(null)
        setCurrentFile(null)
      }
    }

    window.addEventListener('hashchange', handleHashChangeEvent)
    return () => window.removeEventListener('hashchange', handleHashChangeEvent)
  }, [handleHashChange])

  // Обработчик горячих клавиш для поиска
  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      // Ctrl+K или Cmd+K для открытия поиска
      if ((event.ctrlKey || event.metaKey) && event.key === 'k') {
        event.preventDefault()
        setSearchOverlayVisible(true)
      }
      // Escape для закрытия поиска
      if (event.key === 'Escape') {
        setSearchOverlayVisible(false)
        setSearchQuery('')
        setSearchResults([])
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [])

  // Поиск по документации
  const handleSearch = async (query: string) => {
    setSearchQuery(query)
    if (query.trim()) {
      try {
        const results = await docsService.searchDocs(query, currentLanguage)
        setSearchResults(results)
      } catch (err) {
        console.error('Search error:', err)
      }
    } else {
      setSearchResults([])
    }
  }


  // Навигация по файлам
  const handleFileClick = useCallback(async (file: DocFile) => {
    try {
      const fileContent = await docsService.getFile(file.path, currentLanguage)
      setCurrentFile(fileContent)
      
      // Найти раздел, к которому принадлежит файл
      let section = sections.find(s => s.files.some(f => f.id === file.id))
      
      // Если файл не найден в основных разделах, ищем в подразделах
      if (!section && file.subsectionId) {
        section = sections.find(s => 
          s.subsections?.some(sub => sub.files.some(f => f.id === file.id))
        )
      }
      
      if (section) {
        setCurrentSection(section)
        window.location.hash = `#${section.id}/${file.id}`
      }
    } catch (err) {
      console.error('Error loading file:', err)
    }
  }, [currentLanguage, sections])


  // Получение иконки для раздела
  const getSectionIcon = useCallback((sectionId: string) => {
    switch (sectionId) {
      case 'getting-started': return <RocketOutlined />
      case 'configuration': return <SettingOutlined />
      case 'api': return <ApiOutlined />
      case 'databases': return <DatabaseOutlined />
      case 'cloud': return <CloudOutlined />
      case 'kubernetes': return <GlobalOutlined />
      default: return <FileTextOutlined />
    }
  }, [])

  // Преобразование данных в формат TreeView
  const treeData = useMemo(() => {
    if (!sections.length) return { name: 'root', children: [] }
    
    const createNode = (id: string, name: string, children: any[] = [], metadata: any = {}) => ({
      id,
      name,
      children,
      metadata
    })

    const rootChildren = sections.map(section => {
      // Добавляем файлы основного раздела
      const sectionFiles = section.files.map(file => 
        createNode(`file-${section.id}-${file.id}`, file.title, [], { 
          type: 'file', 
          file,
          sectionId: section.id
        })
      )

      // Добавляем подразделы
      const subsections = section.subsections?.map(subsection => {
        const subsectionFiles = subsection.files.map(file => 
          createNode(`file-${section.id}-${subsection.id}-${file.id}`, file.title, [], { 
            type: 'file', 
            file,
            sectionId: section.id,
            subsectionId: subsection.id
          })
        )

        return createNode(`subsection-${section.id}-${subsection.id}`, subsection.title, subsectionFiles, { 
          type: 'subsection', 
          subsectionId: subsection.id,
          sectionId: section.id
        })
      }) || []

      return createNode(`section-${section.id}`, section.title, [...sectionFiles, ...subsections], { 
        type: 'section', 
        sectionId: section.id,
        icon: getSectionIcon(section.id)
      })
    })

    return createNode('root', 'root', rootChildren)
  }, [sections, getSectionIcon])

  // Инициализация expandedIds на основе treeData
  useEffect(() => {
    if (!sections.length) return

    const allExpandedIds = new Set<string>()
    
    // Функция для рекурсивного обхода дерева
    const traverseTree = (node: any) => {
      if (node.children && node.children.length > 0) {
        // Если узел имеет детей, добавляем его в expandedIds
        allExpandedIds.add(node.id)
        
        // Рекурсивно обходим детей
        node.children.forEach((child: any) => traverseTree(child))
      }
    }

    // Обходим корневой узел
    traverseTree(treeData)
    
    setExpandedIds(allExpandedIds)
  }, [sections.length]) // Убираем treeData из зависимостей, чтобы избежать циклических обновлений

  // Обработчик клика по узлу TreeView
  const handleNodeClick = useCallback(async (node: INode) => {
    const metadata = node.metadata
    if (!metadata) return

    if (metadata.type === 'file' && metadata.file) {
      await handleFileClick(metadata.file as unknown as DocFile)
    }
  }, [handleFileClick])

  // Компонент для отображения узла TreeView
  const NodeComponent = useCallback(({ element, isBranch, isExpanded, isSelected, getNodeProps, handleToggle }: any) => {
    const metadata = element.metadata
    const isFile = metadata?.type === 'file'
    const isSection = metadata?.type === 'section'
    const isSubsection = metadata?.type === 'subsection'
    
    const nodeProps = getNodeProps({
      onClick: handleToggle,
    })

    const handleClick = (e: React.MouseEvent) => {
      e.stopPropagation()
      if (isFile) {
        handleNodeClick(element)
      } else {
        // Для разделов и подразделов переключаем состояние расширения
        setExpandedIds(prev => {
          const newSet = new Set(prev)
          if (newSet.has(element.id)) {
            newSet.delete(element.id)
          } else {
            newSet.add(element.id)
          }
          return newSet
        })
      }
    }

    // Базовый отступ для всех элементов
    const leftPadding = 16

    return (
      <div
        {...nodeProps}
        onClick={handleClick}
        style={{
          padding: '6px 8px',
          margin: '1px 0',
          borderRadius: '4px',
          cursor: 'pointer',
          backgroundColor: isSelected 
            ? (darkReaderEnabled ? 'rgba(24, 144, 255, 0.15)' : 'rgba(24, 144, 255, 0.08)')
            : 'transparent',
          borderLeft: isSelected 
            ? '2px solid #1890ff' 
            : '2px solid transparent',
          paddingLeft: isSelected ? `${leftPadding - 2}px` : `${leftPadding}px`,
          transition: 'all 0.15s ease',
          color: isSelected 
            ? '#1890ff' 
            : (darkReaderEnabled ? '#e5e7eb' : '#6b7280'),
          fontWeight: isSelected ? 500 : 400,
          fontSize: isFile ? '13px' : (isSection ? '14px' : '13px'),
          lineHeight: '1.3',
          display: 'flex',
          alignItems: 'center',
          gap: '6px',
          minHeight: '28px'
        }}
        onMouseEnter={(e) => {
          if (!isSelected) {
            e.currentTarget.style.backgroundColor = darkReaderEnabled 
              ? 'rgba(24, 144, 255, 0.08)' 
              : 'rgba(24, 144, 255, 0.04)'
            e.currentTarget.style.color = darkReaderEnabled ? '#60a5fa' : '#1890ff'
            e.currentTarget.style.borderLeft = '2px solid #1890ff'
            e.currentTarget.style.paddingLeft = `${leftPadding - 2}px`
          }
        }}
        onMouseLeave={(e) => {
          if (!isSelected) {
            e.currentTarget.style.backgroundColor = 'transparent'
            e.currentTarget.style.color = darkReaderEnabled ? '#e5e7eb' : '#6b7280'
            e.currentTarget.style.borderLeft = '2px solid transparent'
            e.currentTarget.style.paddingLeft = `${leftPadding}px`
          }
        }}
      >
        {isBranch && (
          <span style={{ 
            fontSize: '10px',
            color: darkReaderEnabled ? '#9ca3af' : '#6b7280',
            marginRight: '2px',
            width: '12px',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center'
          }}>
            {isExpanded ? '▼' : '▶'}
          </span>
        )}
        
        {metadata?.icon && (
          <span style={{ 
            fontSize: isSection ? '14px' : '12px',
            color: isSelected ? '#1890ff' : (darkReaderEnabled ? '#9ca3af' : '#6b7280'),
            width: '16px',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center'
          }}>
            {metadata.icon}
          </span>
        )}
        
        {isFile && (
          <FileTextOutlined style={{ 
            fontSize: '12px',
            color: isSelected ? '#1890ff' : (darkReaderEnabled ? '#9ca3af' : '#6b7280'),
            width: '16px',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center'
          }} />
        )}
        
        <span style={{
          fontWeight: isSection ? 600 : (isSubsection ? 500 : 400),
          letterSpacing: isSection ? '-0.01em' : 'normal',
          flex: 1,
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap'
        }}>
          {element.name}
        </span>
      </div>
    )
  }, [darkReaderEnabled, handleNodeClick])

  // Меню разделов с TreeView
  const menuContent = useMemo(() => {
    if (!sections.length) return null

    return (
      <TreeView
        data={flattenTree(treeData)}
        expandedIds={Array.from(expandedIds)}
        nodeRenderer={NodeComponent}
        className="tree-view-menu"
      />
    )
  }, [sections, treeData, expandedIds, NodeComponent])

  if (loading) {
    return (
      <div style={{ 
        display: 'flex', 
        justifyContent: 'center', 
        alignItems: 'center', 
        minHeight: '100vh',
        background: darkReaderEnabled ? '#141414' : '#f5f5f5'
      }}>
        <Spin size="large" />
      </div>
    )
  }

  if (error) {
    return (
      <div style={{ padding: '24px' }}>
        <Alert
          message={t('documentation.loadingError')}
          description={error}
          type="error"
          showIcon
        />
      </div>
    )
  }

  return (
    <div className="documentation-page">
      <style>{`
        .documentation-page .tree-view-menu {
          background: transparent !important;
          border: none !important;
          font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif !important;
          padding: 0 !important;
          margin: 0 !important;
        }
        
        .documentation-page .tree-view-menu [role="tree"] {
          padding: 0 !important;
          margin: 0 !important;
          list-style: none !important;
        }
        
        .documentation-page .tree-view-menu [role="treeitem"] {
          outline: none !important;
          list-style: none !important;
          padding: 0 !important;
          margin: 0 !important;
        }
        
        .documentation-page .tree-view-menu [role="treeitem"]::before {
          display: none !important;
        }
        
        .documentation-page .tree-view-menu [role="treeitem"]::marker {
          display: none !important;
        }
        
        .documentation-page .tree-view-menu [role="group"] {
          padding: 0 !important;
          margin: 0 !important;
          list-style: none !important;
          margin-left: 0 !important;
        }
        
        .documentation-page .tree-view-menu [role="group"]::before {
          display: none !important;
        }
        
        .documentation-page .tree-view-menu [role="group"]::marker {
          display: none !important;
        }
        
        /* Специальные стили для дочерних элементов */
        .documentation-page .tree-view-menu [role="group"] {
          margin-left: 28px !important;
        }
        
        .documentation-page .tree-view-menu [role="group"] [role="group"] {
          margin-left: 28px !important;
        }
        
        .documentation-page .tree-view-menu [role="group"] [role="treeitem"] {
          margin-left: 0 !important;
        }
        
        .documentation-page .tree-view-menu [role="treeitem"]:focus {
          background-color: ${darkReaderEnabled ? 'rgba(24, 144, 255, 0.08)' : 'rgba(24, 144, 255, 0.04)'} !important;
          color: ${darkReaderEnabled ? '#60a5fa' : '#1890ff'} !important;
        }
        
        .documentation-page .tree-view-menu [role="treeitem"]:focus-visible {
          box-shadow: 0 0 0 1px ${darkReaderEnabled ? '#60a5fa' : '#1890ff'} !important;
        }
        
        .documentation-page .tree-view-menu [role="treeitem"][aria-selected="true"] {
          background-color: ${darkReaderEnabled ? 'rgba(24, 144, 255, 0.15)' : 'rgba(24, 144, 255, 0.08)'} !important;
          color: #1890ff !important;
          border-left: 2px solid #1890ff !important;
        }
        
        /* Убираем все возможные маркеры списков */
        .documentation-page .tree-view-menu ul,
        .documentation-page .tree-view-menu ol,
        .documentation-page .tree-view-menu li {
          list-style: none !important;
          padding: 0 !important;
          margin: 0 !important;
        }
        
        .documentation-page .tree-view-menu ul::before,
        .documentation-page .tree-view-menu ol::before,
        .documentation-page .tree-view-menu li::before {
          display: none !important;
        }
        
        .documentation-page .tree-view-menu ul::marker,
        .documentation-page .tree-view-menu ol::marker,
        .documentation-page .tree-view-menu li::marker {
          display: none !important;
        }
      `}</style>
      {/* Header */}
      <Header 
        showDocsButton={false}
        showSearchButton={true}
        onSearchClick={() => setSearchOverlayVisible(true)}
      />

      {/* Основной контент */}
      <Layout style={{ 
        minHeight: '100vh',
        marginTop: '60px',
        background: 'transparent'
      }}>
        {/* Sidebar */}
        <Layout.Sider
          width={280}
          style={{
            background: darkReaderEnabled ? '#1f1f1f' : '#ffffff',
            borderRight: darkReaderEnabled ? '1px solid #333' : '1px solid #e8e8e8',
            height: 'calc(100vh - 60px)',
            overflow: 'auto',
            position: 'relative',
            zIndex: 1
          }}
        >
          <div style={{ padding: '12px 8px' }}>
            <div style={{ 
              marginBottom: '12px',
              paddingLeft: '4px'
            }}>
              <Title level={5} style={{ 
                margin: 0, 
                color: darkReaderEnabled ? '#ffffff' : '#262626',
                fontSize: '16px',
                fontWeight: 600
              }}>
                {t('documentation.title')}
              </Title>
            </div>

            {/* Разделы */}
            <div style={{ paddingLeft: '4px' }}>
              {menuContent}
            </div>
          </div>
        </Layout.Sider>

        {/* Основной контент */}
        <Layout style={{ background: 'transparent' }}>
          <Content style={{ 
            padding: '24px',
            background: 'transparent',
            position: 'relative',
            zIndex: 1
          }}>
            <div style={{ maxWidth: '800px', margin: '0 auto' }}>
              {/* Breadcrumb */}
              <Breadcrumb style={{ marginBottom: '24px' }}>
                <Breadcrumb.Item>
                  <span 
                    onClick={() => navigate('/')} 
                    style={{ cursor: 'pointer', display: 'flex', alignItems: 'center', gap: '8px' }}
                  >
                    <HomeOutlined />
                    {t('documentation.breadcrumb.home')}
                  </span>
                </Breadcrumb.Item>
                <Breadcrumb.Item>
                  <span style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                    <BookOutlined />
                    {t('documentation.breadcrumb.documentation')}
                  </span>
                </Breadcrumb.Item>
                {currentSection && (
                  <Breadcrumb.Item>
                    <span style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                      {getSectionIcon(currentSection.id)}
                      {currentSection.title}
                    </span>
                  </Breadcrumb.Item>
                )}
                {currentFile && (
                  <Breadcrumb.Item>
                    <span style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                      <FileTextOutlined />
                      {currentFile.title}
                    </span>
                  </Breadcrumb.Item>
                )}
              </Breadcrumb>

              {/* Контент */}
              {currentFile && currentFile.content ? (
                <Card
                  style={{
                    background: darkReaderEnabled ? '#1f1f1f' : '#ffffff',
                    border: darkReaderEnabled ? '1px solid #333' : '1px solid #e8e8e8'
                  }}
                >
                  <MarkdownRenderer content={currentFile.content} />
                </Card>
              ) : (
                <Card
                  style={{
                    background: darkReaderEnabled ? '#1f1f1f' : '#ffffff',
                    border: darkReaderEnabled ? '1px solid #333' : '1px solid #e8e8e8'
                  }}
                >
                  <div style={{ textAlign: 'center', padding: '48px 24px' }}>
                    <div style={{
                      width: '80px',
                      height: '80px',
                      background: 'linear-gradient(135deg, #1890ff, #096dd9)',
                      borderRadius: '50%',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      margin: '0 auto 24px',
                      color: 'white',
                      fontSize: '32px'
                    }}>
                      <BookOutlined />
                    </div>
                    <Title level={2} style={{ 
                      color: darkReaderEnabled ? '#ffffff' : '#262626',
                      marginBottom: '16px'
                    }}>
                      {t('documentation.mainTitle')}
                    </Title>
                    <Paragraph style={{ 
                      color: darkReaderEnabled ? '#d9d9d9' : '#595959',
                      fontSize: '16px',
                      marginBottom: '32px'
                    }}>
                      {t('documentation.mainDescription')}
                    </Paragraph>
                    <Row gutter={[24, 24]}>
                      {sections.map((section) => (
                        <Col xs={24} sm={12} lg={8} key={section.id}>
                          <Card
                            hoverable
                              onClick={async () => {
                                setCurrentSection(section)
                                
                                // Если в разделе есть файлы, открываем первый
                                if (section.files.length > 0) {
                                  const firstFile = section.files[0]
                                  try {
                                    const fileContent = await docsService.getFile(firstFile.path, currentLanguage)
                                    setCurrentFile(fileContent)
                                    window.location.hash = `#${section.id}/${firstFile.id}`
                                  } catch (err) {
                                    console.error('Error loading first file:', err)
                                    window.location.hash = `#${section.id}`
                                  }
                                } else {
                                  window.location.hash = `#${section.id}`
                                }
                              }}
                            style={{ 
                              height: '100%',
                              background: currentSection?.id === section.id 
                                ? (darkReaderEnabled ? '#1a3a5c' : '#e6f7ff')
                                : (darkReaderEnabled ? '#262626' : '#fafafa'),
                              border: currentSection?.id === section.id
                                ? '2px solid #1890ff'
                                : (darkReaderEnabled ? '1px solid #404040' : '1px solid #e8e8e8'),
                              boxShadow: currentSection?.id === section.id
                                ? '0 4px 12px rgba(24, 144, 255, 0.15)'
                                : 'none'
                            }}
                          >
                            <div style={{ textAlign: 'center' }}>
                              <div style={{
                                width: '48px',
                                height: '48px',
                                background: 'linear-gradient(135deg, #1890ff, #096dd9)',
                                borderRadius: '8px',
                                display: 'flex',
                                alignItems: 'center',
                                justifyContent: 'center',
                                margin: '0 auto 16px',
                                color: 'white',
                                fontSize: '20px'
                              }}>
                                {getSectionIcon(section.id)}
                              </div>
                              <Title level={5} style={{ 
                                color: currentSection?.id === section.id 
                                  ? '#1890ff' 
                                  : (darkReaderEnabled ? '#ffffff' : '#262626'),
                                marginBottom: '8px',
                                fontWeight: currentSection?.id === section.id ? 600 : 400
                              }}>
                                {section.title}
                              </Title>
                              <Paragraph style={{ 
                                color: darkReaderEnabled ? '#d9d9d9' : '#595959',
                                fontSize: '14px',
                                margin: 0
                              }}>
                                {section.description}
                              </Paragraph>
                            </div>
                          </Card>
                        </Col>
                      ))}
                    </Row>
                  </div>
                </Card>
              )}
            </div>
          </Content>

          {/* Footer */}
          <Footer style={{ 
            textAlign: 'center', 
            background: darkReaderEnabled ? '#141414' : '#f5f5f5',
            borderTop: darkReaderEnabled ? '1px solid #333' : '1px solid #e8e8e8',
            color: darkReaderEnabled ? '#d9d9d9' : '#595959'
          }}>
            <Text>
              {t('documentation.footer.copyright')}
            </Text>
          </Footer>
        </Layout>
      </Layout>

      {/* Overlay поиска */}
      {searchOverlayVisible && (
        <div
          style={{
            position: 'fixed',
            top: 0,
            left: 0,
            right: 0,
            bottom: 0,
            background: 'rgba(0, 0, 0, 0.5)',
            zIndex: 2000,
            display: 'flex',
            alignItems: 'flex-start',
            justifyContent: 'center',
            paddingTop: '10vh'
          }}
          onClick={() => {
            setSearchOverlayVisible(false)
            setSearchQuery('')
            setSearchResults([])
          }}
        >
          <div
            style={{
              background: darkReaderEnabled ? '#1f1f1f' : '#ffffff',
              borderRadius: '12px',
              boxShadow: '0 20px 25px -5px rgba(0, 0, 0, 0.1), 0 10px 10px -5px rgba(0, 0, 0, 0.04)',
              width: '90%',
              maxWidth: '600px',
              maxHeight: '70vh',
              overflow: 'hidden',
              border: darkReaderEnabled ? '1px solid #333' : '1px solid #e8e8e8'
            }}
            onClick={(e) => e.stopPropagation()}
          >
            {/* Заголовок */}
            <div style={{
              padding: '20px 24px 16px',
              borderBottom: darkReaderEnabled ? '1px solid #333' : '1px solid #e8e8e8',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between'
            }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
                <SearchOutlined style={{ 
                  color: darkReaderEnabled ? '#ffffff' : '#595959',
                  fontSize: '18px'
                }} />
                <Title level={4} style={{ 
                  margin: 0, 
                  color: darkReaderEnabled ? '#ffffff' : '#262626' 
                }}>
                  {t('documentation.search.title')}
                </Title>
              </div>
              <Button
                type="text"
                onClick={() => {
                  setSearchOverlayVisible(false)
                  setSearchQuery('')
                  setSearchResults([])
                }}
                style={{
                  color: darkReaderEnabled ? '#ffffff' : '#595959',
                  padding: '4px 8px'
                }}
              >
                ✕
              </Button>
            </div>

            {/* Поле поиска */}
            <div style={{ padding: '20px 24px' }}>
              <Input
                placeholder={t('documentation.search.placeholder')}
                prefix={<SearchOutlined />}
                value={searchQuery}
                onChange={(e) => handleSearch(e.target.value)}
                style={{
                  fontSize: '16px',
                  padding: '12px 16px',
                  borderRadius: '8px'
                }}
                autoFocus
              />
            </div>

            {/* Результаты поиска */}
            {searchQuery.trim() && (
              <div style={{
                maxHeight: '400px',
                overflowY: 'auto',
                borderTop: darkReaderEnabled ? '1px solid #333' : '1px solid #e8e8e8'
              }}>
                {searchResults.length > 0 ? (
                  <div style={{ padding: '16px 24px' }}>
                    <Text strong style={{ 
                      color: darkReaderEnabled ? '#ffffff' : '#262626',
                      fontSize: '14px',
                      marginBottom: '12px',
                      display: 'block'
                    }}>
                      {t('documentation.search.foundResults', { count: searchResults.length })}
                    </Text>
                    <List
                      size="small"
                      dataSource={searchResults}
                      renderItem={(item) => (
                        <List.Item
                          style={{ 
                            padding: '12px 0',
                            cursor: 'pointer',
                            border: 'none',
                            borderRadius: '6px',
                            margin: '4px 0',
                            transition: 'background-color 0.2s ease'
                          }}
                          onClick={() => {
                            // Найти раздел для этого файла
                            const section = sections.find(s => 
                              s.files.some(f => f.path === item.path)
                            )
                            if (section) {
                              setCurrentSection(section)
                              handleFileClick(item)
                              setSearchOverlayVisible(false)
                              setSearchQuery('')
                              setSearchResults([])
                            }
                          }}
                          onMouseEnter={(e) => {
                            e.currentTarget.style.backgroundColor = darkReaderEnabled 
                              ? 'rgba(24, 144, 255, 0.1)' 
                              : 'rgba(24, 144, 255, 0.05)'
                          }}
                          onMouseLeave={(e) => {
                            e.currentTarget.style.backgroundColor = 'transparent'
                          }}
                        >
                          <div style={{ width: '100%' }}>
                            <Text style={{ 
                              color: darkReaderEnabled ? '#40a9ff' : '#1890ff',
                              fontSize: '14px',
                              fontWeight: '500',
                              display: 'block',
                              marginBottom: '4px'
                            }}>
                              {item.title}
                            </Text>
                            <Text style={{ 
                              color: darkReaderEnabled ? '#d9d9d9' : '#595959',
                              fontSize: '12px',
                              display: 'block'
                            }}>
                              {sections.find(s => s.files.some(f => f.path === item.path))?.title}
                            </Text>
                          </div>
                        </List.Item>
                      )}
                    />
                  </div>
                ) : (
                  <div style={{
                    padding: '40px 24px',
                    textAlign: 'center',
                    color: darkReaderEnabled ? '#d9d9d9' : '#595959'
                  }}>
                    <SearchOutlined style={{ fontSize: '32px', marginBottom: '16px', opacity: 0.5 }} />
                    <div>{t('documentation.search.noResults')}</div>
                    <div style={{ fontSize: '12px', marginTop: '8px', opacity: 0.7 }}>
                      {t('documentation.search.noResultsDescription')}
                    </div>
                  </div>
                )}
              </div>
            )}

            {/* Подсказка */}
            {!searchQuery.trim() && (
              <div style={{
                padding: '20px 24px',
                textAlign: 'center',
                color: darkReaderEnabled ? '#d9d9d9' : '#595959',
                fontSize: '14px'
              }}>
                <div style={{ marginBottom: '8px' }}>{t('documentation.search.startTyping')}</div>
                <div style={{ fontSize: '12px', opacity: 0.7 }}>
                  {t('documentation.search.shortcut')}
                </div>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

export default DocumentationPage
