import React, { useState, useEffect } from 'react'
import { 
  Layout, 
  Typography, 
  Card, 
  Row, 
  Col, 
  Space, 
  List,
  Button,
  Breadcrumb,
  Spin,
  Alert,
  Input,
  Menu
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
  HomeOutlined,
  SearchOutlined
} from '@ant-design/icons'
import { useNavigate, useParams } from 'react-router-dom'
import { useTheme } from '../../common/contexts/ThemeContext'
import MarkdownRenderer from '../../common/components/MarkdownRenderer'
import { docsService, type DocSection, type DocFile, type SupportedLanguage, SUPPORTED_LANGUAGES } from '../../common/services/docs'

const { Content, Footer } = Layout
const { Title, Paragraph, Text } = Typography

const DocumentationPage: React.FC = () => {
  const navigate = useNavigate()
  const { sectionId, fileId } = useParams<{ sectionId?: string; fileId?: string }>()
  const { darkReaderEnabled } = useTheme()
  
  const [sections, setSections] = useState<DocSection[]>([])
  const [currentSection, setCurrentSection] = useState<DocSection | null>(null)
  const [currentFile, setCurrentFile] = useState<DocFile | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [searchQuery, setSearchQuery] = useState('')
  const [searchResults, setSearchResults] = useState<DocFile[]>([])
  const [openKeys, setOpenKeys] = useState<string[]>([])
  const [currentLanguage, setCurrentLanguage] = useState<SupportedLanguage>('ru')

  // Загрузка структуры документации
  useEffect(() => {
    const loadDocs = async () => {
      try {
        setLoading(true)
        const docSections = await docsService.getSections(currentLanguage)
        setSections(docSections)
        
        // Если есть sectionId, загружаем раздел
        if (sectionId) {
          const section = await docsService.getSection(sectionId, currentLanguage)
          setCurrentSection(section)
          
          // Автоматически открываем раздел в меню
          setOpenKeys([sectionId])
          
          // Если есть fileId, загружаем файл
          if (fileId && section) {
            const file = section.files.find(f => f.id === fileId)
            if (file) {
              const fileContent = await docsService.getFile(file.path, currentLanguage)
              setCurrentFile(fileContent)
            }
          }
        }
      } catch (err) {
        setError('Ошибка загрузки документации')
        console.error('Error loading docs:', err)
      } finally {
        setLoading(false)
      }
    }
    
    loadDocs()
  }, [sectionId, fileId, currentLanguage])

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

  // Навигация по разделам
  const handleSectionClick = (section: DocSection) => {
    setCurrentSection(section)
    setCurrentFile(null)
    setOpenKeys([section.id])
    navigate(`/docs/${section.id}`)
  }

  // Навигация по файлам
  const handleFileClick = async (file: DocFile) => {
    try {
      const fileContent = await docsService.getFile(file.path, currentLanguage)
      setCurrentFile(fileContent)
      navigate(`/docs/${currentSection?.id}/${file.id}`)
    } catch (err) {
      console.error('Error loading file:', err)
    }
  }

  // Переключение языка
  const handleLanguageChange = (language: SupportedLanguage) => {
    setCurrentLanguage(language)
    setCurrentFile(null)
    setSearchResults([])
    setSearchQuery('')
  }

  // Получение иконки для раздела
  const getSectionIcon = (sectionId: string) => {
    switch (sectionId) {
      case 'getting-started': return <RocketOutlined />
      case 'configuration': return <SettingOutlined />
      case 'api': return <ApiOutlined />
      case 'databases': return <DatabaseOutlined />
      case 'cloud': return <CloudOutlined />
      case 'kubernetes': return <GlobalOutlined />
      default: return <FileTextOutlined />
    }
  }

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
          message="Ошибка загрузки документации"
          description={error}
          type="error"
          showIcon
        />
      </div>
    )
  }

  return (
    <div className="documentation-page">
      {/* Header */}
      <div 
        style={{ 
          background: darkReaderEnabled ? '#1f1f1f' : '#fafafa', 
          padding: '8px 32px',
          boxShadow: darkReaderEnabled ? '0 2px 8px rgba(0,0,0,0.2)' : '0 2px 8px rgba(0,0,0,0.05)',
          position: 'fixed',
          top: 0,
          left: 0,
          right: 0,
          zIndex: 1000,
          width: '100%',
          height: '60px',
          display: 'flex',
          alignItems: 'center',
          borderBottom: darkReaderEnabled ? '1px solid #333' : '1px solid #e8e8e8'
        }}>
        <div style={{ 
          display: 'flex', 
          alignItems: 'center', 
          justifyContent: 'space-between',
          maxWidth: '1200px',
          margin: '0 auto',
          width: '100%'
        }}>
          {/* Логотип */}
          <div 
            style={{ 
              display: 'flex', 
              alignItems: 'center', 
              gap: '16px', 
              flex: '0 0 auto', 
              padding: '8px 0',
              cursor: 'pointer',
              transition: 'opacity 0.3s ease'
            }}
            onClick={() => navigate('/')}
            onMouseEnter={(e) => e.currentTarget.style.opacity = '0.8'}
            onMouseLeave={(e) => e.currentTarget.style.opacity = '1'}
          >
            <div style={{
              width: '40px',
              height: '40px',
              background: 'linear-gradient(135deg, #1890ff, #096dd9)',
              borderRadius: '8px',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center'
            }}>
              <GlobalOutlined style={{ color: 'white', fontSize: '20px' }} />
            </div>
            <Title level={3} style={{ margin: 0, color: darkReaderEnabled ? '#ffffff' : '#595959' }}>
              Stroppy Cloud Panel
            </Title>
          </div>
          
          {/* Навигация */}
          <div style={{ flex: '0 0 auto', padding: '8px 0' }}>
            <Space size="middle">
              <Button 
                type="text" 
                onClick={() => navigate('/')}
                icon={<HomeOutlined />}
                style={{ 
                  color: darkReaderEnabled ? '#ffffff' : '#595959',
                  fontWeight: '500',
                  borderRadius: '6px',
                  padding: '6px 12px',
                  background: darkReaderEnabled ? 'rgba(255, 255, 255, 0.08)' : 'rgba(0, 0, 0, 0.03)',
                  border: '1px solid transparent',
                  transition: 'all 0.3s ease'
                }}
              >
                {currentLanguage === 'ru' ? 'Главная' : 'Home'}
              </Button>
              
              {/* Переключатель языка */}
              <Button.Group>
                {SUPPORTED_LANGUAGES.map((lang) => (
                  <Button
                    key={lang}
                    type={currentLanguage === lang ? 'primary' : 'text'}
                    onClick={() => handleLanguageChange(lang)}
                    style={{
                      color: currentLanguage === lang 
                        ? 'white' 
                        : darkReaderEnabled ? '#ffffff' : '#595959',
                      fontWeight: '500',
                      borderRadius: '6px',
                      padding: '6px 12px',
                      background: currentLanguage === lang
                        ? '#1890ff'
                        : darkReaderEnabled ? 'rgba(255, 255, 255, 0.08)' : 'rgba(0, 0, 0, 0.03)',
                      border: '1px solid transparent',
                      transition: 'all 0.3s ease'
                    }}
                  >
                    {lang.toUpperCase()}
                  </Button>
                ))}
              </Button.Group>
            </Space>
          </div>
        </div>
      </div>

      {/* Основной контент */}
      <Layout style={{ 
        minHeight: '100vh',
        marginTop: '60px',
        background: 'transparent'
      }}>
        {/* Sidebar */}
        <Layout.Sider
          width={300}
          style={{
            background: darkReaderEnabled ? '#1f1f1f' : '#ffffff',
            borderRight: darkReaderEnabled ? '1px solid #333' : '1px solid #e8e8e8',
            height: 'calc(100vh - 60px)',
            overflow: 'auto',
            position: 'relative',
            zIndex: 1
          }}
        >
          <div style={{ padding: '16px' }}>
            <div style={{ 
              marginBottom: '16px'
            }}>
              <Title level={4} style={{ 
                margin: 0, 
                color: darkReaderEnabled ? '#ffffff' : '#262626' 
              }}>
                Документация
              </Title>
            </div>

            {/* Поиск */}
            <Input
              placeholder="Поиск по документации..."
              prefix={<SearchOutlined />}
              value={searchQuery}
              onChange={(e) => handleSearch(e.target.value)}
              style={{ marginBottom: '16px' }}
            />

            {/* Результаты поиска */}
            {searchResults.length > 0 && (
              <div style={{ marginBottom: '16px' }}>
                <Text strong style={{ color: darkReaderEnabled ? '#ffffff' : '#262626' }}>
                  Результаты поиска:
                </Text>
                <List
                  size="small"
                  dataSource={searchResults}
                  renderItem={(item) => (
                    <List.Item
                      style={{ 
                        padding: '8px 0',
                        cursor: 'pointer',
                        border: 'none'
                      }}
                      onClick={() => {
                        // Найти раздел для этого файла
                        const section = sections.find(s => 
                          s.files.some(f => f.path === item.path)
                        )
                        if (section) {
                          handleSectionClick(section)
                          handleFileClick(item)
                        }
                      }}
                    >
                      <Text style={{ 
                        color: darkReaderEnabled ? '#40a9ff' : '#1890ff',
                        fontSize: '12px'
                      }}>
                        {item.title}
                      </Text>
                    </List.Item>
                  )}
                />
              </div>
            )}

            {/* Разделы */}
            <Menu
              mode="inline"
              selectedKeys={currentSection ? [currentSection.id] : []}
              openKeys={openKeys}
              onOpenChange={setOpenKeys}
              style={{ 
                background: 'transparent',
                border: 'none'
              }}
            >
              {sections.map((section) => (
                <Menu.SubMenu
                  key={section.id}
                  title={
                    <Space>
                      {getSectionIcon(section.id)}
                      <span style={{ color: darkReaderEnabled ? '#ffffff' : '#262626' }}>
                        {section.title}
                      </span>
                    </Space>
                  }
                  onTitleClick={() => handleSectionClick(section)}
                >
                  {section.files.map((file) => (
                    <Menu.Item
                      key={file.id}
                      onClick={() => handleFileClick(file)}
                      style={{
                        color: darkReaderEnabled ? '#d9d9d9' : '#595959',
                        fontSize: '12px'
                      }}
                    >
                      {file.title}
                    </Menu.Item>
                  ))}
                </Menu.SubMenu>
              ))}
            </Menu>
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
                    Главная
                  </span>
                </Breadcrumb.Item>
                <Breadcrumb.Item>
                  <span style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                    <BookOutlined />
                    Документация
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
              ) : currentSection ? (
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
                      {getSectionIcon(currentSection.id)}
                    </div>
                    <Title level={2} style={{ 
                      color: darkReaderEnabled ? '#ffffff' : '#262626',
                      marginBottom: '16px'
                    }}>
                      {currentSection.title}
                    </Title>
                    <Paragraph style={{ 
                      color: darkReaderEnabled ? '#d9d9d9' : '#595959',
                      fontSize: '16px',
                      marginBottom: '32px'
                    }}>
                      {currentSection.description}
                    </Paragraph>
                    {currentSection.files.length > 0 ? (
                      <List
                        dataSource={currentSection.files}
                        renderItem={(file) => (
                          <List.Item
                            style={{ 
                              padding: '12px 0',
                              cursor: 'pointer',
                              border: 'none'
                            }}
                            onClick={() => handleFileClick(file)}
                          >
                            <Space>
                              <FileTextOutlined style={{ color: '#1890ff' }} />
                              <Text style={{ 
                                color: darkReaderEnabled ? '#40a9ff' : '#1890ff',
                                fontSize: '16px'
                              }}>
                                {file.title}
                              </Text>
                            </Space>
                          </List.Item>
                        )}
                      />
                    ) : (
                      <Text style={{ 
                        color: darkReaderEnabled ? '#d9d9d9' : '#595959',
                        fontStyle: 'italic'
                      }}>
                        Раздел в разработке
                      </Text>
                    )}
                  </div>
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
                      Документация Stroppy Cloud Panel
                    </Title>
                    <Paragraph style={{ 
                      color: darkReaderEnabled ? '#d9d9d9' : '#595959',
                      fontSize: '16px',
                      marginBottom: '32px'
                    }}>
                      Полное руководство по использованию платформы для нагрузочного тестирования 
                      баз данных и облачных приложений
                    </Paragraph>
                    <Row gutter={[24, 24]}>
                      {sections.map((section) => (
                        <Col xs={24} sm={12} lg={8} key={section.id}>
                          <Card
                            hoverable
                            onClick={() => handleSectionClick(section)}
                            style={{ 
                              height: '100%',
                              background: darkReaderEnabled ? '#262626' : '#fafafa',
                              border: darkReaderEnabled ? '1px solid #404040' : '1px solid #e8e8e8'
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
                                color: darkReaderEnabled ? '#ffffff' : '#262626',
                                marginBottom: '8px'
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
              Stroppy Cloud Panel ©2024. Все права защищены.
            </Text>
          </Footer>
        </Layout>
      </Layout>
    </div>
  )
}

export default DocumentationPage
