import React, { useState, useEffect, useMemo } from 'react'
import { 
  Layout, 
  Typography, 
  Button, 
  Row, 
  Col, 
  Card, 
  Space, 
  Divider,
  Statistic,
  Avatar,
  List,
  Tag,
  Modal,
  Descriptions,
  Progress,
} from 'antd'
import { 
  RocketOutlined,
  CheckCircleOutlined,
  ArrowRightOutlined,
  PlayCircleOutlined,
  CloudOutlined,
  DatabaseOutlined,
  ApiOutlined,
  TrophyOutlined,
  ReloadOutlined,
  EyeOutlined,
  ExclamationCircleOutlined,
  ClockCircleOutlined,
  CalendarOutlined,
  UserOutlined,
  SettingOutlined,
  ThunderboltOutlined,
  GlobalOutlined
} from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from '../../common/hooks/useTranslation'
import { useAuth } from '../../common/contexts/AuthContext'
import { useTheme } from '../../common/contexts/ThemeContext'
import Header from '../components/Header'
import dayjs from 'dayjs'
import { apiClient } from '../../common/services/api'
import type { Run } from '../../common/services/api'
// import RunComparisonModal from '../components/RunComparisonModal' // Убираем для публичного просмотра

const { Content, Footer } = Layout
const { Title, Paragraph, Text } = Typography
// const { Option } = Select // Убираем для публичного просмотра
// const { RangePicker } = DatePicker // Убираем для публичного просмотра

// interface TopRun extends Run {
//   rank: number;
//   score: number;
// }

const LandingPage: React.FC = () => {
  const navigate = useNavigate()
  const { t } = useTranslation('landing')
  const { isAuthenticated } = useAuth()
  const { darkReaderEnabled } = useTheme()
  // Убираем состояния связанные со скроллом
  
  // Состояния для Dashboard функциональности
  const [runs, setRuns] = useState<Run[]>([])
  const [loading, setLoading] = useState(false)
  // Убираем filterOptions для публичного просмотра
  // const [filterOptions, setFilterOptions] = useState<FilterOptionsResponse>({
  //   statuses: [],
  //   load_types: [],
  //   databases: [],
  //   deployment_schemas: [],
  //   hardware_configs: []
  // })
  
  // Состояния для модальных окон
  const [selectedRun, setSelectedRun] = useState<Run | null>(null)
  const [modalVisible, setModalVisible] = useState(false)
  // const [comparisonModalVisible, setComparisonModalVisible] = useState(false) // Убираем для публичного просмотра
  // Убираем неиспользуемые состояния для публичного просмотра
  // const [selectedRowKeys, setSelectedRowKeys] = useState<React.Key[]>([])
  // const [filters, setFilters] = useState({
  //   status: '',
  //   dateRange: null as [dayjs.Dayjs | null, dayjs.Dayjs | null] | null,
  //   sortBy: 'tps_avg' as 'tps_avg' | 'created_at' | 'duration',
  //   loadType: '',
  //   database: '',
  //   deploymentSchema: '',
  //   hardwareConfig: '',
  //   tpsMin: null as number | null,
  //   tpsMax: null as number | null
  // })

  const handleGetStarted = () => {
    navigate('/register')
  }

  const handleLogin = () => {
    navigate('/login')
  }

  const handleGoToPanel = () => {
    navigate('/app/dashboard')
  }



  // Убираем функции связанные со скроллом

  // Убираем все useEffect связанные со скроллом

  const features = [
    {
      icon: <CloudOutlined style={{ fontSize: '32px', color: '#1890ff' }} />,
      title: t('features.cloud.title'),
      description: t('features.cloud.description')
    },
    {
      icon: <GlobalOutlined style={{ fontSize: '32px', color: '#52c41a' }} />,
      title: t('features.kubernetes.title'),
      description: t('features.kubernetes.description')
    },
    {
      icon: <DatabaseOutlined style={{ fontSize: '32px', color: '#fa8c16' }} />,
      title: t('features.databases.title'),
      description: t('features.databases.description')
    },
    {
      icon: <EyeOutlined style={{ fontSize: '32px', color: '#eb2f96' }} />,
      title: t('features.monitoring.title'),
      description: t('features.monitoring.description')
    }
  ]

  const stats = [
    { title: t('stats.databases'), value: '4', icon: <DatabaseOutlined /> },
    { title: t('stats.clouds'), value: '2', icon: <CloudOutlined /> },
    { title: t('stats.tests'), value: '100+', icon: <RocketOutlined /> },
    { title: t('stats.clusters'), value: '50+', icon: <GlobalOutlined /> }
  ]

  const benefits = [
    t('about.benefits.1'),
    t('about.benefits.2'),
    t('about.benefits.3'),
    t('about.benefits.4'),
    t('about.benefits.5'),
    t('about.benefits.6')
  ]

  // Функции для работы с данными запусков
  const fetchRuns = async () => {
    try {
      setLoading(true)
      const response = await apiClient.getTopRuns(10)
      setRuns(response.runs)
    } catch (error) {
      console.error('Ошибка загрузки данных:', error)
    } finally {
      setLoading(false)
    }
  }

  // Убираем fetchFilterOptions для публичного просмотра
  // const fetchFilterOptions = async () => {
  //   try {
  //     const options = await apiClient.getFilterOptions()
  //     setFilterOptions(options)
  //   } catch (error) {
  //     console.error('Ошибка загрузки опций фильтров:', error)
  //   }
  // }

  // Загружаем данные при монтировании компонента
  useEffect(() => {
    fetchRuns()
    // fetchFilterOptions() // Убираем для публичного просмотра
  }, [])

  // Убираем функцию parseConfig для публичного просмотра
  // const parseConfig = (configString: string) => {
  //   try {
  //     const config = JSON.parse(configString)
  //     return {
  //       load_type: config.load_type || '',
  //       database: config.database || '',
  //       deployment_schema: config.deployment_schema || '',
  //       hardware_config: config.hardware_config || ''
  //     }
  //   } catch {
  //     return {
  //       load_type: '',
  //       database: '',
  //       deployment_schema: '',
  //       hardware_config: ''
  //     }
  //   }
  // }

  // Убираем функции фильтрации для публичного просмотра
  // const clearAllFilters = () => {
  //   setFilters({
  //     status: '',
  //     dateRange: null,
  //     sortBy: 'tps_avg',
  //     loadType: '',
  //     database: '',
  //     deploymentSchema: '',
  //     hardwareConfig: '',
  //     tpsMin: null,
  //     tpsMax: null
  //   })
  // }

  // Вспомогательные функции для работы со статусами
  const getStatusColor = (status: string) => {
    switch (status) {
      case 'running': return 'processing'
      case 'completed': return 'success'
      case 'failed': return 'error'
      case 'cancelled': return 'warning'
      case 'pending': return 'default'
      default: return 'default'
    }
  }

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'running': return <PlayCircleOutlined />
      case 'completed': return <CheckCircleOutlined />
      case 'failed': return <ExclamationCircleOutlined />
      case 'pending': return <ClockCircleOutlined />
      default: return <ClockCircleOutlined />
    }
  }

  const getStatusText = (status: string) => {
    switch (status) {
      case 'running': return 'Выполняется'
      case 'completed': return 'Завершен'
      case 'failed': return 'Ошибка'
      case 'cancelled': return 'Отменен'
      case 'pending': return 'Ожидает'
      default: return 'Неизвестно'
    }
  }

  // Убираем функцию convertRunForComparison для публичного просмотра

  // Убираем функцию сравнения для публичного просмотра
  // const handleCompareRuns = () => {
  //   if (selectedRowKeys.length !== 2) {
  //     message.warning('Выберите ровно 2 запуска для сравнения');
  //     return;
  //   }
  //   
  //   const selectedRuns = runs.filter(run => selectedRowKeys.includes(run.id));
  //   if (selectedRuns.length === 2) {
  //     setComparisonModalVisible(true);
  //   }
  // }

  // Функция для просмотра деталей запуска
  const handleViewRun = (run: Run) => {
    setSelectedRun(run);
    setModalVisible(true);
  }

  // Фильтрация и сортировка данных
  // Показываем только завершенные запуски в топах
  const filteredAndSortedRuns = useMemo(() => {
    if (!runs || !Array.isArray(runs)) return []
    return runs
      .filter(run => run.status === 'completed') // Фильтруем только завершенные запуски
      .map((run, index) => ({
        ...run,
        rank: index + 1,
        score: run.tps_metrics?.average || 0
      }))
  }, [runs])

  // ТОП 10 запусков
  const topRuns = filteredAndSortedRuns.slice(0, 10)


  return (
    <>
      <Header showDocsButton={true} />

      {/* Основной контент */}
      <Layout style={{ minHeight: '100vh' }}>
        <Content style={{ marginTop: '60px' }}>
        {/* Hero Section */}
        <section 
          id="hero"
          className="hero-gradient"
          style={{ 
            color: 'white',
            padding: '80px 24px',
            textAlign: 'center'
          }}
        >
          <div style={{ maxWidth: '1200px', margin: '0 auto' }}>
            <Title level={1} style={{ color: 'white', fontSize: '3.5rem', marginBottom: '24px' }}>
              {t('hero.title')}
            </Title>
            <Paragraph style={{ 
              color: 'rgba(255,255,255,0.9)', 
              fontSize: '1.25rem', 
              maxWidth: '600px',
              margin: '0 auto 40px'
            }}>
              {t('hero.subtitle')}
            </Paragraph>
            <Space size="large">
              <Button 
                type="primary" 
                size="large" 
                icon={<RocketOutlined />}
                onClick={handleGetStarted}
                className="animated-button"
                style={{ 
                  height: '48px',
                  padding: '0 32px',
                  fontSize: '16px',
                  background: '#fff',
                  color: '#1890ff',
                  border: '2px solid #fff',
                  fontWeight: '600',
                  borderRadius: '8px',
                  transition: 'all 0.3s ease',
                  boxShadow: '0 4px 15px rgba(0,0,0,0.2)'
                }}
                onMouseEnter={(e) => {
                  e.currentTarget.style.background = '#f0f8ff'
                  e.currentTarget.style.color = '#096dd9'
                  e.currentTarget.style.transform = 'translateY(-2px)'
                  e.currentTarget.style.boxShadow = '0 6px 20px rgba(0,0,0,0.3)'
                }}
                onMouseLeave={(e) => {
                  e.currentTarget.style.background = '#fff'
                  e.currentTarget.style.color = '#1890ff'
                  e.currentTarget.style.transform = 'translateY(0)'
                  e.currentTarget.style.boxShadow = '0 4px 15px rgba(0,0,0,0.2)'
                }}
              >
                {t('hero.getStarted')}
              </Button>
              <Button 
                size="large" 
                icon={<PlayCircleOutlined />}
                className="animated-button"
                style={{ 
                  height: '48px',
                  padding: '0 32px',
                  fontSize: '16px',
                  color: '#1890ff',
                  borderColor: '#1890ff',
                  background: 'rgba(255,255,255,0.95)',
                  backdropFilter: 'blur(10px)',
                  fontWeight: '600',
                  borderRadius: '8px',
                  border: '2px solid #1890ff',
                  transition: 'all 0.3s ease',
                  boxShadow: '0 4px 15px rgba(0,0,0,0.2)'
                }}
                onMouseEnter={(e) => {
                  e.currentTarget.style.background = '#1890ff'
                  e.currentTarget.style.color = 'white'
                  e.currentTarget.style.borderColor = '#1890ff'
                  e.currentTarget.style.transform = 'translateY(-2px)'
                  e.currentTarget.style.boxShadow = '0 6px 20px rgba(24, 144, 255, 0.4)'
                }}
                onMouseLeave={(e) => {
                  e.currentTarget.style.background = 'rgba(255,255,255,0.95)'
                  e.currentTarget.style.color = '#1890ff'
                  e.currentTarget.style.borderColor = '#1890ff'
                  e.currentTarget.style.transform = 'translateY(0)'
                  e.currentTarget.style.boxShadow = '0 4px 15px rgba(0,0,0,0.2)'
                }}
              >
                {t('hero.watchDemo')}
              </Button>
            </Space>
          </div>
        </section>

        {/* Stats Section */}
        <section id="stats" style={{ padding: '60px 24px', background: '#fafafa' }}>
          <div style={{ maxWidth: '1200px', margin: '0 auto' }}>
            <Row gutter={[32, 32]}>
              {stats.map((stat, index) => (
                <Col xs={12} sm={6} key={index}>
                  <div className="stat-item" style={{ textAlign: 'center' }}>
                    <Avatar 
                      size={64} 
                      icon={stat.icon}
                      style={{ 
                        background: 'linear-gradient(135deg, #1890ff, #096dd9)',
                        marginBottom: '16px'
                      }}
                    />
                    <Statistic
                      title={stat.title}
                      value={stat.value}
                      valueStyle={{ color: '#1890ff', fontSize: '2rem' }}
                    />
                  </div>
                </Col>
              ))}
            </Row>
          </div>
        </section>

        {/* Features Section */}
        <section id="features" style={{ padding: '80px 24px' }}>
          <div style={{ maxWidth: '1200px', margin: '0 auto' }}>
            <div style={{ textAlign: 'center', marginBottom: '60px' }}>
              <Title level={2}>{t('features.title')}</Title>
              <Paragraph style={{ fontSize: '1.1rem', color: '#666', maxWidth: '600px', margin: '0 auto' }}>
                {t('features.subtitle')}
              </Paragraph>
            </div>
            
            <Row gutter={[32, 32]}>
              {features.map((feature, index) => (
                <Col xs={24} sm={12} lg={6} key={index}>
                  <Card 
                    hoverable
                    className="feature-card"
                    style={{ 
                      height: '100%',
                      textAlign: 'center',
                      border: '1px solid #f0f0f0',
                      borderRadius: '12px'
                    }}
                    bodyStyle={{ padding: '32px 24px' }}
                  >
                    <div style={{ marginBottom: '20px' }}>
                      {feature.icon}
                    </div>
                    <Title level={4} style={{ marginBottom: '12px' }}>
                      {feature.title}
                    </Title>
                    <Paragraph style={{ color: '#666', margin: 0 }}>
                      {feature.description}
                    </Paragraph>
                  </Card>
                </Col>
              ))}
            </Row>
          </div>
        </section>

        {/* Top Runs Section */}
        <section id="topRuns" style={{ padding: '80px 24px', background: 'linear-gradient(135deg, #f5f7fa 0%, #c3cfe2 100%)' }}>
          <div style={{ maxWidth: '1200px', margin: '0 auto' }}>
            <div style={{ textAlign: 'center', marginBottom: '60px' }}>
              <Title level={2} style={{ 
                display: 'flex', 
                alignItems: 'center', 
                justifyContent: 'center', 
                gap: '16px',
                color: darkReaderEnabled ? '#ffffff' : '#262626',
                marginBottom: '16px'
              }}>
                <Avatar 
                  size={64} 
                  icon={<TrophyOutlined />}
                  style={{ 
                    background: 'linear-gradient(135deg, #faad14, #ffc53d)',
                    border: '3px solid #fff',
                    boxShadow: '0 4px 12px rgba(250, 173, 20, 0.3)'
                  }}
                />
                Топ запусков
              </Title>
              <Paragraph style={{ 
                fontSize: '1.2rem', 
                color: darkReaderEnabled ? '#d9d9d9' : '#595959', 
                maxWidth: '700px', 
                margin: '0 auto',
                fontWeight: '400'
              }}>
                Посмотрите на лучшие результаты тестирования производительности
              </Paragraph>
            </div>
            
            {loading ? (
              <div style={{ textAlign: 'center', padding: '60px 0' }}>
                <div style={{ fontSize: '48px', marginBottom: '16px' }}>⚡</div>
                <Text style={{ fontSize: '16px', color: darkReaderEnabled ? '#d9d9d9' : '#666' }}>Загружаем топы...</Text>
              </div>
            ) : topRuns.length === 0 ? (
              <Card 
                style={{ 
                  textAlign: 'center', 
                  padding: '60px 24px',
                  background: 'rgba(255, 255, 255, 0.9)',
                  borderRadius: '16px',
                  border: 'none',
                  boxShadow: '0 8px 32px rgba(0, 0, 0, 0.1)'
                }}
              >
                <div style={{ fontSize: '64px', marginBottom: '24px' }}>📊</div>
                <Title level={3} style={{ color: darkReaderEnabled ? '#d9d9d9' : '#595959', marginBottom: '16px' }}>
                  Нет данных
                </Title>
                <Paragraph style={{ color: darkReaderEnabled ? '#bfbfbf' : '#8c8c8c', fontSize: '16px' }}>
                  Запуски не найдены. Создайте первый запуск для отображения в топах.
                </Paragraph>
              </Card>
            ) : (
              <Row gutter={[32, 32]}>
                {topRuns.slice(0, 6).map((run) => (
                  <Col xs={24} sm={12} lg={8} key={run.id}>
                    <Card 
                      hoverable
                      className="top-run-card"
                      style={{ 
                        height: '100%',
                        border: 'none',
                        borderRadius: '16px',
                        background: run.rank <= 3 
                          ? 'linear-gradient(135deg, #fff9e6 0%, #fff2cc 100%)'
                          : 'rgba(255, 255, 255, 0.95)',
                        boxShadow: run.rank <= 3 
                          ? '0 8px 32px rgba(250, 173, 20, 0.2)'
                          : '0 4px 20px rgba(0, 0, 0, 0.08)',
                        transition: 'all 0.3s ease',
                        cursor: 'pointer',
                        position: 'relative'
                      }}
                      bodyStyle={{ padding: '24px' }}
                      onClick={() => handleViewRun(run)}
                    >
                      <div style={{ display: 'flex', alignItems: 'center', marginBottom: '16px' }}>
                        <div style={{ 
                          width: '40px', 
                          height: '40px', 
                          borderRadius: '50%',
                          background: run.rank <= 3 
                            ? 'linear-gradient(135deg, #faad14, #ffc53d)'
                            : 'linear-gradient(135deg, #1890ff, #40a9ff)',
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'center',
                          marginRight: '12px',
                          color: 'white',
                          fontWeight: 'bold',
                          fontSize: '16px',
                          boxShadow: '0 2px 8px rgba(0, 0, 0, 0.15)'
                        }}>
                          {run.rank <= 3 ? '🏆' : `#${run.rank}`}
                        </div>
                        <div style={{ flex: 1 }}>
                          <Title level={4} style={{ 
                            margin: 0, 
                            color: darkReaderEnabled ? '#ffffff' : '#262626',
                            fontSize: '16px',
                            lineHeight: '1.4'
                          }}>
                            {run.name}
                          </Title>
                          <Text type="secondary" style={{ fontSize: '12px' }}>
                            {dayjs(run.created_at).format('DD.MM.YYYY HH:mm')}
                          </Text>
                        </div>
                      </div>

                      <div style={{ marginBottom: '16px' }}>
                        <Tag 
                          color={getStatusColor(run.status)}
                          style={{ 
                            borderRadius: '20px',
                            padding: '4px 12px',
                            fontSize: '12px',
                            fontWeight: '500'
                          }}
                        >
                          {getStatusIcon(run.status)} {getStatusText(run.status)}
                        </Tag>
                      </div>

                      <div style={{ 
                        background: 'rgba(24, 144, 255, 0.1)',
                        borderRadius: '12px',
                        padding: '16px',
                        textAlign: 'center'
                      }}>
                        <div style={{ 
                          fontSize: '24px', 
                          fontWeight: 'bold', 
                          color: '#1890ff',
                          marginBottom: '4px'
                        }}>
                          {run.tps_metrics?.average?.toFixed(2) || 'N/A'}
                        </div>
                        <Text style={{ 
                          fontSize: '12px', 
                          color: '#8c8c8c',
                          textTransform: 'uppercase',
                          letterSpacing: '0.5px'
                        }}>
                          TPS (средний)
                        </Text>
                      </div>

                      {run.rank <= 3 && (
                        <div style={{
                          position: 'absolute',
                          top: '-8px',
                          right: '-8px',
                          background: 'linear-gradient(135deg, #faad14, #ffc53d)',
                          borderRadius: '50%',
                          width: '32px',
                          height: '32px',
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'center',
                          color: 'white',
                          fontSize: '16px',
                          boxShadow: '0 4px 12px rgba(250, 173, 20, 0.4)'
                        }}>
                          {run.rank === 1 ? '🥇' : run.rank === 2 ? '🥈' : '🥉'}
                        </div>
                      )}
                    </Card>
                  </Col>
                ))}
              </Row>
            )}

            {/* Информационная панель */}
            <Card 
              style={{ 
                marginTop: '40px',
                background: 'rgba(255, 255, 255, 0.9)',
                borderRadius: '16px',
                border: 'none',
                boxShadow: '0 8px 32px rgba(0, 0, 0, 0.1)'
              }}
              bodyStyle={{ padding: '32px' }}
            >
              <Row gutter={[32, 32]} align="middle">
                <Col xs={24} md={16}>
                  <Space direction="vertical" size="middle" style={{ width: '100%' }}>
                    <div>
                      <Title level={4} style={{ margin: 0, color: darkReaderEnabled ? '#ffffff' : '#262626' }}>
                        📈 Публичный просмотр топов
                      </Title>
                      <Paragraph style={{ color: darkReaderEnabled ? '#bfbfbf' : '#8c8c8c', margin: '8px 0 0 0' }}>
                        Здесь показаны лучшие результаты завершенных тестов производительности. 
                        Для полного доступа к функциям фильтрации и управления запусками войдите в систему.
                      </Paragraph>
                    </div>
                    
                    <Row gutter={[16, 16]}>
                      <Col span={12}>
                        <div style={{ 
                          background: '#f6f8fa',
                          borderRadius: '8px',
                          padding: '12px',
                          textAlign: 'center'
                        }}>
                          <Text strong style={{ color: '#1890ff' }}>Сортировка</Text>
                          <br />
                          <Text style={{ fontSize: '12px', color: darkReaderEnabled ? '#bfbfbf' : '#8c8c8c' }}>
                            Завершенные по TPS ↓
                          </Text>
                        </div>
                      </Col>
                      <Col span={12}>
                        <div style={{ 
                          background: '#f6f8fa',
                          borderRadius: '8px',
                          padding: '12px',
                          textAlign: 'center'
                        }}>
                          <Text strong style={{ color: '#52c41a' }}>Показано</Text>
                          <br />
                          <Text style={{ fontSize: '12px', color: darkReaderEnabled ? '#bfbfbf' : '#8c8c8c' }}>
                            {filteredAndSortedRuns.length} завершенных
                          </Text>
                        </div>
                      </Col>
                    </Row>
                  </Space>
                </Col>
                
                <Col xs={24} md={8}>
                  <Space direction="vertical" style={{ width: '100%' }}>
                    <Button 
                      type="primary"
                      icon={<ReloadOutlined />} 
                      onClick={fetchRuns}
                      loading={loading}
                      style={{ 
                        width: '100%',
                        height: '48px',
                        borderRadius: '8px',
                        background: 'linear-gradient(135deg, #1890ff, #40a9ff)',
                        border: 'none',
                        fontSize: '16px',
                        fontWeight: '500'
                      }}
                    >
                      Обновить данные
                    </Button>
                    {isAuthenticated && (
                      <Button 
                        type="default"
                        onClick={handleGoToPanel}
                        style={{ 
                          width: '100%',
                          height: '48px',
                          borderRadius: '8px',
                          fontSize: '16px',
                          fontWeight: '500'
                        }}
                      >
                        Перейти в панель
                      </Button>
                    )}
                  </Space>
                </Col>
              </Row>
            </Card>
          </div>
        </section>

        {/* About Section */}
        <section id="about" style={{ padding: '80px 24px', background: '#fafafa' }}>
          <div style={{ maxWidth: '1200px', margin: '0 auto' }}>
            <Row gutter={[48, 48]} align="middle">
              <Col xs={24} lg={12}>
                <Title level={2}>{t('about.title')}</Title>
                <Paragraph style={{ fontSize: '1.1rem', color: '#666', marginBottom: '24px' }}>
                  {t('about.description')}
                </Paragraph>
                <List
                  dataSource={benefits}
                  renderItem={(item) => (
                    <List.Item style={{ border: 'none', padding: '8px 0' }}>
                      <CheckCircleOutlined style={{ color: '#52c41a', marginRight: '12px' }} />
                      <Text>{item}</Text>
                    </List.Item>
                  )}
                />
              </Col>
              <Col xs={24} lg={12}>
                <div style={{ 
                  background: 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)',
                  borderRadius: '16px',
                  padding: '40px',
                  color: 'white',
                  textAlign: 'center'
                }}>
                  <DatabaseOutlined style={{ fontSize: '64px', marginBottom: '24px' }} />
                  <Title level={3} style={{ color: 'white', marginBottom: '16px' }}>
                    {t('about.techTitle')}
                  </Title>
                  <Paragraph style={{ color: 'rgba(255,255,255,0.9)', margin: 0 }}>
                    {t('about.techDescription')}
                  </Paragraph>
                </div>
              </Col>
            </Row>
          </div>
        </section>

        {/* CTA Section */}
        <section 
          id="cta"
          className="cta-gradient"
          style={{ 
            padding: '80px 24px',
            color: 'white',
            textAlign: 'center'
          }}
        >
          <div style={{ maxWidth: '800px', margin: '0 auto' }}>
            <Title level={2} style={{ color: 'white', marginBottom: '24px' }}>
              {t('cta.title')}
            </Title>
            <Paragraph style={{ 
              color: 'rgba(255,255,255,0.9)', 
              fontSize: '1.1rem',
              marginBottom: '40px'
            }}>
              {t('cta.description')}
            </Paragraph>
            <Space size="large">
              <Button 
                type="primary" 
                size="large" 
                icon={<ArrowRightOutlined />}
                onClick={handleGetStarted}
                className="animated-button"
                style={{ 
                  height: '48px',
                  padding: '0 32px',
                  fontSize: '16px',
                  background: '#fff',
                  color: '#1890ff',
                  border: '2px solid #fff',
                  fontWeight: '600',
                  borderRadius: '8px',
                  transition: 'all 0.3s ease',
                  boxShadow: '0 4px 15px rgba(0,0,0,0.2)'
                }}
                onMouseEnter={(e) => {
                  e.currentTarget.style.background = '#f0f8ff'
                  e.currentTarget.style.color = '#096dd9'
                  e.currentTarget.style.transform = 'translateY(-2px)'
                  e.currentTarget.style.boxShadow = '0 6px 20px rgba(0,0,0,0.3)'
                }}
                onMouseLeave={(e) => {
                  e.currentTarget.style.background = '#fff'
                  e.currentTarget.style.color = '#1890ff'
                  e.currentTarget.style.transform = 'translateY(0)'
                  e.currentTarget.style.boxShadow = '0 4px 15px rgba(0,0,0,0.2)'
                }}
              >
                {t('cta.getStarted')}
              </Button>
              <Button 
                size="large" 
                icon={<ApiOutlined />}
                onClick={handleLogin}
                className="animated-button"
                style={{ 
                  height: '48px',
                  padding: '0 32px',
                  fontSize: '16px',
                  color: '#1890ff',
                  borderColor: '#1890ff',
                  background: 'rgba(255,255,255,0.95)',
                  backdropFilter: 'blur(10px)',
                  fontWeight: '600',
                  borderRadius: '8px',
                  border: '2px solid #1890ff',
                  transition: 'all 0.3s ease',
                  boxShadow: '0 4px 15px rgba(0,0,0,0.2)'
                }}
                onMouseEnter={(e) => {
                  e.currentTarget.style.background = '#1890ff'
                  e.currentTarget.style.color = 'white'
                  e.currentTarget.style.borderColor = '#1890ff'
                  e.currentTarget.style.transform = 'translateY(-2px)'
                  e.currentTarget.style.boxShadow = '0 6px 20px rgba(24, 144, 255, 0.4)'
                }}
                onMouseLeave={(e) => {
                  e.currentTarget.style.background = 'rgba(255,255,255,0.95)'
                  e.currentTarget.style.color = '#1890ff'
                  e.currentTarget.style.borderColor = '#1890ff'
                  e.currentTarget.style.transform = 'translateY(0)'
                  e.currentTarget.style.boxShadow = '0 4px 15px rgba(0,0,0,0.2)'
                }}
              >
                {t('cta.existingUser')}
              </Button>
            </Space>
          </div>
        </section>

        {/* Footer */}
        <Footer style={{ 
          background: '#001529', 
          color: 'white',
          textAlign: 'center',
          padding: '40px 24px'
        }}>
          <div style={{ maxWidth: '1200px', margin: '0 auto' }}>
            <Row gutter={[32, 32]}>
              <Col xs={24} sm={8}>
                <div>
                  <Title level={4} style={{ color: 'white', marginBottom: '16px' }}>
                    Stroppy Cloud Panel
                  </Title>
                  <Paragraph style={{ color: 'rgba(255,255,255,0.7)' }}>
                    {t('footer.description')}
                  </Paragraph>
                </div>
              </Col>
              <Col xs={24} sm={8}>
                <div>
                  <Title level={5} style={{ color: 'white', marginBottom: '16px' }}>
                    {t('footer.links.title')}
                  </Title>
                  <Space direction="vertical" size="small">
                    <Button type="link" style={{ color: 'rgba(255,255,255,0.7)', padding: 0 }}>
                      {t('footer.links.documentation')}
                    </Button>
                    <Button type="link" style={{ color: 'rgba(255,255,255,0.7)', padding: 0 }}>
                      {t('footer.links.support')}
                    </Button>
                    <Button type="link" style={{ color: 'rgba(255,255,255,0.7)', padding: 0 }}>
                      {t('footer.links.community')}
                    </Button>
                  </Space>
                </div>
              </Col>
              <Col xs={24} sm={8}>
                <div>
                  <Title level={5} style={{ color: 'white', marginBottom: '16px' }}>
                    {t('footer.contact.title')}
                  </Title>
                  <Space direction="vertical" size="small">
                    <Text style={{ color: 'rgba(255,255,255,0.7)' }}>
                      {t('footer.contact.email')}
                    </Text>
                    <Text style={{ color: 'rgba(255,255,255,0.7)' }}>stroppy.io</Text>
                  </Space>
                </div>
                </Col>
              </Row>
              <Divider style={{ borderColor: 'rgba(255,255,255,0.2)', margin: '32px 0 16px' }} />
              <Text style={{ color: 'rgba(255,255,255,0.7)' }}>
                {t('footer.copyright')}
              </Text>
            </div>
          </Footer>

        {/* Убираем кнопку "Наверх" */}
      </Content>

      {/* Модальное окно деталей запуска */}
      <Modal
        title={
          <Space align="center">
            <EyeOutlined />
            <Text strong style={{ fontSize: '18px' }}>Детали запуска</Text>
          </Space>
        }
        open={modalVisible}
        onCancel={() => setModalVisible(false)}
        footer={null}
        width={1400}
        style={{ top: 20 }}
      >
        {selectedRun && (() => {
          let config;
          try {
            config = JSON.parse(selectedRun.config);
          } catch {
            config = {};
          }
          
          // Вычисляем прогресс на основе статуса
          const getProgress = () => {
            switch (selectedRun.status) {
              case 'pending': return 0;
              case 'running': return 50;
              case 'completed': return 100;
              case 'failed': return 75;
              case 'cancelled': return 25;
              default: return 0;
            }
          };

          // Вычисляем длительность
          const getDuration = () => {
            if (selectedRun.completed_at && selectedRun.started_at) {
              const duration = dayjs(selectedRun.completed_at).diff(dayjs(selectedRun.started_at), 'minute');
              return `${duration} мин`;
            }
            return 'Не завершен';
          };
          
          return (
            <div style={{ maxHeight: '80vh', overflowY: 'auto' }}>
              {/* Заголовок запуска */}
              <Card 
                style={{ 
                  textAlign: 'center',
                  background: 'linear-gradient(135deg, #e6f7ff 0%, #bae7ff 100%)',
                  border: '2px solid #40a9ff',
                  marginBottom: 24
                }}
              >
                <Space direction="vertical" size="small">
                  <Title level={3} style={{ margin: 0, color: '#096dd9' }}>
                    {selectedRun.name}
                  </Title>
                  <Space>
                    <Tag 
                      icon={getStatusIcon(selectedRun.status)} 
                      color={getStatusColor(selectedRun.status)}
                      style={{ fontSize: '16px', padding: '6px 16px' }}
                    >
                      {getStatusText(selectedRun.status)}
                    </Tag>
                    <Tag icon={<UserOutlined />} color="blue">ID: {selectedRun.id}</Tag>
                  </Space>
                  <Progress 
                    percent={getProgress()} 
                    size="small" 
                    status={selectedRun.status === 'completed' ? 'success' : selectedRun.status === 'failed' ? 'exception' : 'active'}
                  />
                </Space>
              </Card>

              {/* Основная информация */}
              <Card 
                title={
                  <Space>
                    <SettingOutlined />
                    <Text strong>Основная информация</Text>
                  </Space>
                }
                style={{ marginBottom: 16 }}
              >
                <Descriptions column={2} bordered size="small">
                  <Descriptions.Item label="ID запуска" span={1}>
                    <Tag color="blue">#{selectedRun.id}</Tag>
                  </Descriptions.Item>
                  <Descriptions.Item label="Run ID" span={1}>
                    <Tag color="cyan">run-{selectedRun.id}</Tag>
                  </Descriptions.Item>
                  <Descriptions.Item label="Статус" span={1}>
                    <Tag icon={getStatusIcon(selectedRun.status)} color={getStatusColor(selectedRun.status)}>
                      {getStatusText(selectedRun.status)}
                    </Tag>
                  </Descriptions.Item>
                  <Descriptions.Item label="Прогресс" span={1}>
                    <Progress 
                      percent={getProgress()} 
                      size="small" 
                      status={selectedRun.status === 'completed' ? 'success' : selectedRun.status === 'failed' ? 'exception' : 'active'}
                      style={{ width: '100px' }}
                    />
                  </Descriptions.Item>
                  <Descriptions.Item label="Описание" span={2}>
                    {selectedRun.description || <Text type="secondary">Нет описания</Text>}
                  </Descriptions.Item>
                  <Descriptions.Item label="Время запуска" span={1}>
                    <Space>
                      <CalendarOutlined />
                      {dayjs(selectedRun.created_at).format('DD.MM.YYYY HH:mm:ss')}
                    </Space>
                  </Descriptions.Item>
                  <Descriptions.Item label="Обновлен" span={1}>
                    <Space>
                      <ClockCircleOutlined />
                      {dayjs(selectedRun.updated_at).format('DD.MM.YYYY HH:mm:ss')}
                    </Space>
                  </Descriptions.Item>
                  {selectedRun.started_at && (
                    <Descriptions.Item label="Запущен" span={1}>
                      <Space>
                        <PlayCircleOutlined />
                        {dayjs(selectedRun.started_at).format('DD.MM.YYYY HH:mm:ss')}
                      </Space>
                    </Descriptions.Item>
                  )}
                  {selectedRun.completed_at && (
                    <Descriptions.Item label="Завершен" span={1}>
                      <Space>
                        <CheckCircleOutlined />
                        {dayjs(selectedRun.completed_at).format('DD.MM.YYYY HH:mm:ss')}
                      </Space>
                    </Descriptions.Item>
                  )}
                  <Descriptions.Item label="Длительность" span={2}>
                    <Space>
                      <ClockCircleOutlined />
                      <Text strong>{getDuration()}</Text>
                    </Space>
                  </Descriptions.Item>
                </Descriptions>
              </Card>

              {/* TPS Метрики */}
              <Card 
                title={
                  <Space>
                    <ThunderboltOutlined />
                    <Text strong>TPS Метрики</Text>
                  </Space>
                }
                style={{ marginBottom: 16 }}
              >
                <Descriptions column={2} bordered size="small">
                  {selectedRun.tps_metrics?.max !== undefined ? (
                    <Descriptions.Item label="Максимальный TPS" span={1}>
                      <Tag color="green">{selectedRun.tps_metrics.max.toFixed(2)}</Tag>
                    </Descriptions.Item>
                  ) : (
                    <Descriptions.Item label="Максимальный TPS" span={1}>
                      <Text type="secondary">Не указано</Text>
                    </Descriptions.Item>
                  )}
                  {selectedRun.tps_metrics?.min !== undefined ? (
                    <Descriptions.Item label="Минимальный TPS" span={1}>
                      <Tag color="red">{selectedRun.tps_metrics.min.toFixed(2)}</Tag>
                    </Descriptions.Item>
                  ) : (
                    <Descriptions.Item label="Минимальный TPS" span={1}>
                      <Text type="secondary">Не указано</Text>
                    </Descriptions.Item>
                  )}
                  {selectedRun.tps_metrics?.average !== undefined ? (
                    <Descriptions.Item label="Средний TPS" span={1}>
                      <Tag color="blue">{selectedRun.tps_metrics.average.toFixed(2)}</Tag>
                    </Descriptions.Item>
                  ) : (
                    <Descriptions.Item label="Средний TPS" span={1}>
                      <Text type="secondary">Не указано</Text>
                    </Descriptions.Item>
                  )}
                  {selectedRun.tps_metrics?.['95p'] !== undefined ? (
                    <Descriptions.Item label="95-й процентиль TPS" span={1}>
                      <Tag color="purple">{selectedRun.tps_metrics['95p'].toFixed(2)}</Tag>
                    </Descriptions.Item>
                  ) : (
                    <Descriptions.Item label="95-й процентиль TPS" span={1}>
                      <Text type="secondary">Не указано</Text>
                    </Descriptions.Item>
                  )}
                  {selectedRun.tps_metrics?.['99p'] !== undefined ? (
                    <Descriptions.Item label="99-й процентиль TPS" span={2}>
                      <Tag color="orange">{selectedRun.tps_metrics['99p'].toFixed(2)}</Tag>
                    </Descriptions.Item>
                  ) : (
                    <Descriptions.Item label="99-й процентиль TPS" span={2}>
                      <Text type="secondary">Не указано</Text>
                    </Descriptions.Item>
                  )}
                </Descriptions>
              </Card>

              {/* Полная конфигурация */}
              <Card title="Полная конфигурация" size="small">
                <pre style={{ 
                  background: '#f5f5f5', 
                  padding: 12, 
                  borderRadius: 4, 
                  fontSize: '12px',
                  maxHeight: '300px',
                  overflow: 'auto',
                  margin: 0
                }}>
                  {JSON.stringify(config, null, 2)}
                </pre>
              </Card>
            </div>
          );
        })()}
      </Modal>

      {/* Убираем модальное окно сравнения для публичного просмотра */}
      {/* {selectedRowKeys.length === 2 && (
        <RunComparisonModal
          visible={comparisonModalVisible}
          onClose={() => setComparisonModalVisible(false)}
          runs={[
            convertRunForComparison(runs.find(run => run.id === selectedRowKeys[0])!),
            convertRunForComparison(runs.find(run => run.id === selectedRowKeys[1])!)
          ] as [any, any]}
        />
      )} */}

      </Layout>
    </>
  )
}

export default LandingPage
