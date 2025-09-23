import React, { useState, useEffect, useMemo } from 'react'
import { 
  Card, 
  Row, 
  Col, 
  Select, 
  DatePicker, 
  Space, 
  Typography, 
  Tag,
  Button,
  Alert,
  InputNumber,
  Modal,
  Descriptions,
  Progress,
  Divider,
  Statistic,
  message
} from 'antd'
import { 
  TrophyOutlined,
  ReloadOutlined,
  EyeOutlined,
  SwapOutlined,
  CheckCircleOutlined,
  ExclamationCircleOutlined,
  ClockCircleOutlined,
  PlayCircleOutlined,
  CalendarOutlined,
  UserOutlined,
  SettingOutlined,
  ThunderboltOutlined,
  DatabaseOutlined,
  CloudServerOutlined,
  BugOutlined
} from '@ant-design/icons'
import dayjs from 'dayjs'
import { apiClient } from '../services/api'
import type { Run, FilterOptionsResponse } from '../services/api'
import RunComparisonModal from '../components/RunComparisonModal'

const { Title, Text } = Typography
const { Option } = Select
const { RangePicker } = DatePicker


interface TopRun extends Run {
  rank: number;
  score: number;
}


const DashboardPage: React.FC = () => {
  const [runs, setRuns] = useState<Run[]>([])
  const [loading, setLoading] = useState(true)
  const [filterOptions, setFilterOptions] = useState<FilterOptionsResponse>({
    statuses: [],
    load_types: [],
    databases: [],
    deployment_schemas: [],
    hardware_configs: []
  })
  
  // Состояния для модальных окон
  const [selectedRun, setSelectedRun] = useState<Run | null>(null)
  const [modalVisible, setModalVisible] = useState(false)
  const [comparisonModalVisible, setComparisonModalVisible] = useState(false)
  const [selectedRowKeys, setSelectedRowKeys] = useState<React.Key[]>([])
  const [filters, setFilters] = useState({
    status: '',
    dateRange: null as [dayjs.Dayjs | null, dayjs.Dayjs | null] | null,
    sortBy: 'tps_avg' as 'tps_avg' | 'created_at' | 'duration',
    loadType: '',
    database: '',
    deploymentSchema: '',
    hardwareConfig: '',
    tpsMin: null as number | null,
    tpsMax: null as number | null
  })

  // Загрузка данных
  const fetchRuns = async () => {
    try {
      setLoading(true)
      const response = await apiClient.getRuns(1, 1000) // Загружаем больше данных для анализа
      setRuns(response.runs)
    } catch (error) {
      console.error('Ошибка загрузки данных:', error)
    } finally {
      setLoading(false)
    }
  }

  // Загрузка опций фильтров
  const fetchFilterOptions = async () => {
    try {
      const options = await apiClient.getFilterOptions()
      setFilterOptions(options)
    } catch (error) {
      console.error('Ошибка загрузки опций фильтров:', error)
    }
  }

  useEffect(() => {
    fetchRuns()
    fetchFilterOptions()
  }, [])

  // Функция для извлечения данных из config JSON
  const parseConfig = (configString: string) => {
    try {
      const config = JSON.parse(configString)
      return {
        load_type: config.load_type || '',
        database: config.database || '',
        deployment_schema: config.deployment_schema || '',
        hardware_config: config.hardware_config || ''
      }
    } catch {
      return {
        load_type: '',
        database: '',
        deployment_schema: '',
        hardware_config: ''
      }
    }
  }

  // Функция для очистки всех фильтров
  const clearAllFilters = () => {
    setFilters({
      status: '',
      dateRange: null,
      sortBy: 'tps_avg',
      loadType: '',
      database: '',
      deploymentSchema: '',
      hardwareConfig: '',
      tpsMin: null,
      tpsMax: null
    })
  }

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

  // Функция для преобразования API Run в тип для сравнения
  const convertRunForComparison = (run: Run) => {
    let config;
    try {
      config = JSON.parse(run.config);
    } catch {
      config = {};
    }

    return {
      id: run.id.toString(),
      runId: `run-${run.id}`,
      name: run.name,
      description: run.description,
      status: run.status === 'completed' ? 'completed' as const : 'failed' as const,
      progress: run.status === 'completed' ? 100 : 0,
      startTime: dayjs(run.created_at).format('DD.MM.YYYY HH:mm'),
      duration: run.completed_at 
        ? dayjs(run.completed_at).diff(dayjs(run.started_at || run.created_at), 'minute') + ' мин'
        : 'Не завершен',
      
      workloadType: config.workloadType || 'custom',
      workloadProperties: {
        runners: config.workloadProperties?.runners || 1,
        duration: config.workloadProperties?.duration || 'Не указано',
        ...config.workloadProperties
      },
      
      databaseType: config.databaseType || 'postgres',
      databaseVersion: config.databaseVersion?.version || 'Не указано',
      databaseBuild: config.databaseVersion?.build,
      
      hardwareConfiguration: config.hardwareConfiguration ? {
        ...config.hardwareConfiguration,
        signature: config.hardwareConfiguration.signature || `${config.hardwareConfiguration.cpu?.cores || 0}c-${config.hardwareConfiguration.memory?.totalGB || 0}gb-${config.hardwareConfiguration.storage?.type || 'ssd'}-${config.hardwareConfiguration.storage?.capacityGB || 0}gb-${config.hardwareConfiguration.nodeCount || 1}n`
      } : {
        id: 'unknown',
        name: 'Неизвестная конфигурация',
        signature: '0c-0gb-ssd-0gb-1n',
        cpu: { cores: 0, model: 'Неизвестно' },
        memory: { totalGB: 0 },
        storage: { type: 'ssd', capacityGB: 0 },
        nodeCount: 1
      },
      
      deploymentLayout: config.deploymentLayout || {
        type: 'single-node',
        signature: 'single-node',
        configuration: {}
      },
      
      nemesisSignature: config.nemesisSignature || {
        signature: 'none',
        nemeses: []
      },
      
      tpsMetrics: run.tps_metrics || config.tpsMetrics
    };
  }

  // Функция для открытия сравнения
  const handleCompareRuns = () => {
    if (selectedRowKeys.length !== 2) {
      message.warning('Выберите ровно 2 запуска для сравнения');
      return;
    }
    
    const selectedRuns = runs.filter(run => selectedRowKeys.includes(run.id));
    if (selectedRuns.length === 2) {
      setComparisonModalVisible(true);
    }
  }

  // Функция для просмотра деталей запуска
  const handleViewRun = (run: Run) => {
    setSelectedRun(run);
    setModalVisible(true);
  }

  // Фильтрация и сортировка данных
  const filteredAndSortedRuns = useMemo(() => {
    let filtered = runs

    // Фильтр по статусу
    if (filters.status) {
      filtered = filtered.filter(run => run.status === filters.status)
    }

    // Фильтр по дате
    if (filters.dateRange && filters.dateRange[0] && filters.dateRange[1]) {
      const startDate = filters.dateRange[0].startOf('day')
      const endDate = filters.dateRange[1].endOf('day')
      filtered = filtered.filter(run => {
        const runDate = dayjs(run.created_at)
        return runDate.isAfter(startDate) && runDate.isBefore(endDate)
      })
    }

    // Фильтр по типу нагрузки
    if (filters.loadType) {
      filtered = filtered.filter(run => {
        const config = parseConfig(run.config)
        return config.load_type === filters.loadType
      })
    }

    // Фильтр по базе данных
    if (filters.database) {
      filtered = filtered.filter(run => {
        const config = parseConfig(run.config)
        return config.database === filters.database
      })
    }

    // Фильтр по схеме развертывания
    if (filters.deploymentSchema) {
      filtered = filtered.filter(run => {
        const config = parseConfig(run.config)
        return config.deployment_schema === filters.deploymentSchema
      })
    }

    // Фильтр по конфигурации железа
    if (filters.hardwareConfig) {
      filtered = filtered.filter(run => {
        const config = parseConfig(run.config)
        return config.hardware_config === filters.hardwareConfig
      })
    }

    // Фильтр по TPS (диапазон)
    if (filters.tpsMin !== null || filters.tpsMax !== null) {
      filtered = filtered.filter(run => {
        const tps = run.tps_metrics?.average || 0
        if (filters.tpsMin !== null && tps < filters.tpsMin) return false
        if (filters.tpsMax !== null && tps > filters.tpsMax) return false
        return true
      })
    }

    // Сортировка и ранжирование
    const sorted = filtered.sort((a, b) => {
      switch (filters.sortBy) {
        case 'tps_avg':
          const aTps = a.tps_metrics?.average || 0
          const bTps = b.tps_metrics?.average || 0
          return bTps - aTps
        case 'created_at':
          return dayjs(b.created_at).diff(dayjs(a.created_at))
        case 'duration':
          const aDuration = a.completed_at && a.started_at 
            ? dayjs(a.completed_at).diff(dayjs(a.started_at), 'minute')
            : 0
          const bDuration = b.completed_at && b.started_at 
            ? dayjs(b.completed_at).diff(dayjs(b.started_at), 'minute')
            : 0
          return bDuration - aDuration
        default:
          return 0
      }
    })

    // Добавляем ранги и очки
    return sorted.map((run, index) => {
      let score = 0
      switch (filters.sortBy) {
        case 'tps_avg':
          score = run.tps_metrics?.average || 0
          break
        case 'created_at':
          score = dayjs().diff(dayjs(run.created_at), 'hour')
          break
        case 'duration':
          score = run.completed_at && run.started_at 
            ? dayjs(run.completed_at).diff(dayjs(run.started_at), 'minute')
            : 0
          break
      }

      return {
        ...run,
        rank: index + 1,
        score
      } as TopRun
    })
  }, [runs, filters])

  // ТОП 10 запусков
  const topRuns = filteredAndSortedRuns.slice(0, 10)



  const getRankIcon = (rank: number) => {
    switch (rank) {
      case 1: return '🥇'
      case 2: return '🥈'
      case 3: return '🥉'
      default: return `#${rank}`
    }
  }

  return (
    <div style={{ padding: '24px', background: '#f5f5f5', minHeight: '100vh' }}>
      {/* Заголовок */}
      <div style={{ marginBottom: 24 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
          <Title level={2} style={{ margin: 0, display: 'flex', alignItems: 'center', gap: 12 }}>
            <TrophyOutlined style={{ color: '#faad14' }} />
            ТОП Запусков
          </Title>
          <Space>
            <Button 
              type={selectedRowKeys.length === 2 ? 'primary' : 'default'}
              icon={<SwapOutlined />}
              disabled={selectedRowKeys.length !== 2}
              onClick={handleCompareRuns}
              title={selectedRowKeys.length === 2 ? 'Сравнить выбранные запуски' : 'Выберите ровно 2 запуска для сравнения'}
              style={{
                backgroundColor: selectedRowKeys.length === 2 ? '#52c41a' : undefined,
                borderColor: selectedRowKeys.length === 2 ? '#52c41a' : undefined,
                animation: selectedRowKeys.length === 2 ? 'pulse 2s infinite' : undefined
              }}
            >
              Сравнить ({selectedRowKeys.length}/2)
            </Button>
          </Space>
        </div>
        <Text type="secondary">Анализ и рейтинг лучших запусков тестирования</Text>
      </div>

      {/* ТОП запусков с фильтрами */}
      <Card 
        title={
          <Space>
            <TrophyOutlined />
            <span>ТОП 10 Запусков</span>
            <Tag color="green">Найдено: {filteredAndSortedRuns.length}</Tag>
          </Space>
        }
        loading={loading}
        style={{ marginBottom: 16 }}
      >
        <Row gutter={24}>
          {/* Основной контент - ТОП запусков */}
          <Col span={16}>
            <div style={{ height: '600px', overflowY: 'auto' }}>
        {topRuns.length === 0 ? (
          <Alert
            message="Нет данных"
            description="Не найдено запусков, соответствующих выбранным фильтрам"
            type="info"
            showIcon
          />
        ) : (
          <Space direction="vertical" style={{ width: '100%' }} size="middle">
            {topRuns.map((run) => (
              <Card 
                key={run.id} 
                size="small"
                style={{ 
                  border: run.rank <= 3 ? '2px solid #faad14' : '1px solid #d9d9d9',
                  background: run.rank <= 3 ? '#fffbe6' : 'white'
                }}
              >
                <Row align="middle" justify="space-between">
                  <Col>
                    <Space>
                      <input
                        type="checkbox"
                        checked={selectedRowKeys.includes(run.id)}
                        onChange={(e) => {
                          if (e.target.checked) {
                            setSelectedRowKeys([...selectedRowKeys, run.id]);
                          } else {
                            setSelectedRowKeys(selectedRowKeys.filter(key => key !== run.id));
                          }
                        }}
                        style={{ marginRight: 8 }}
                      />
                      <Text strong style={{ fontSize: '18px' }}>
                        {getRankIcon(run.rank)}
                      </Text>
                      <div>
                        <Text strong>{run.name}</Text>
                        <br />
                        <Text type="secondary" style={{ fontSize: '12px' }}>
                          ID: {run.id} • {dayjs(run.created_at).format('DD.MM.YYYY HH:mm')}
                        </Text>
                      </div>
                    </Space>
                  </Col>
                  <Col>
                    <Space direction="vertical" align="end">
                      <Space>
                        <Button
                          type="text"
                          icon={<EyeOutlined />}
                          size="small"
                          onClick={() => handleViewRun(run)}
                          title="Просмотр деталей"
                        />
                        <Tag color={getStatusColor(run.status)}>
                          {getStatusText(run.status)}
                        </Tag>
                      </Space>
                      <div>
                        {filters.sortBy === 'tps_avg' && (
                          <Text strong style={{ color: '#1890ff' }}>
                            TPS: {run.tps_metrics?.average?.toFixed(2) || 'N/A'}
                          </Text>
                        )}
                        {filters.sortBy === 'created_at' && (
                          <Text strong style={{ color: '#52c41a' }}>
                            {dayjs().diff(dayjs(run.created_at), 'day')} дн. назад
                          </Text>
                        )}
                        {filters.sortBy === 'duration' && (
                          <Text strong style={{ color: '#722ed1' }}>
                            {run.completed_at && run.started_at 
                              ? `${dayjs(run.completed_at).diff(dayjs(run.started_at), 'minute')} мин`
                              : 'N/A'}
                          </Text>
                        )}
                      </div>
                    </Space>
                  </Col>
                </Row>
              </Card>
            ))}
          </Space>
        )}
            </div>
          </Col>

          {/* Фильтры - правая колонка */}
          <Col span={8}>
            <div style={{ padding: '16px', background: '#fafafa', borderRadius: '6px', height: '600px', overflowY: 'auto' }}>
              <Title level={5} style={{ marginBottom: 16 }}>Фильтры</Title>
              
              <Space direction="vertical" size="middle" style={{ width: '100%' }}>
                <div>
                  <Text strong>Период:</Text>
                  <RangePicker
                    value={filters.dateRange}
                    onChange={(dates) => setFilters(prev => ({ ...prev, dateRange: dates }))}
                    style={{ width: '100%', marginTop: 4 }}
                    format="DD.MM.YYYY"
                  />
                </div>

                <div>
                  <Text strong>Статус:</Text>
                  <Select
                    value={filters.status}
                    onChange={(value) => setFilters(prev => ({ ...prev, status: value }))}
                    placeholder="Выберите статус"
                    style={{ width: '100%', marginTop: 4 }}
                    allowClear
                  >
                    {filterOptions.statuses.map(status => (
                      <Option key={status} value={status}>
                        {status}
                      </Option>
                    ))}
                  </Select>
                </div>

                <div>
                  <Text strong>Тип нагрузки:</Text>
                  <Select
                    value={filters.loadType}
                    onChange={(value) => setFilters(prev => ({ ...prev, loadType: value }))}
                    placeholder="Выберите тип нагрузки"
                    style={{ width: '100%', marginTop: 4 }}
                    allowClear
                  >
                    {filterOptions.load_types.map(loadType => (
                      <Option key={loadType} value={loadType}>
                        {loadType}
                      </Option>
                    ))}
                  </Select>
                </div>

                <div>
                  <Text strong>База данных:</Text>
                  <Select
                    value={filters.database}
                    onChange={(value) => setFilters(prev => ({ ...prev, database: value }))}
                    placeholder="Выберите БД"
                    style={{ width: '100%', marginTop: 4 }}
                    allowClear
                  >
                    {filterOptions.databases.map(database => (
                      <Option key={database} value={database}>
                        {database}
                      </Option>
                    ))}
                  </Select>
                </div>

                <div>
                  <Text strong>Схема развертывания:</Text>
                  <Select
                    value={filters.deploymentSchema}
                    onChange={(value) => setFilters(prev => ({ ...prev, deploymentSchema: value }))}
                    placeholder="Выберите схему"
                    style={{ width: '100%', marginTop: 4 }}
                    allowClear
                  >
                    {filterOptions.deployment_schemas.map(schema => (
                      <Option key={schema} value={schema}>
                        {schema}
                      </Option>
                    ))}
                  </Select>
                </div>

                <div>
                  <Text strong>Конфигурация железа:</Text>
                  <Select
                    value={filters.hardwareConfig}
                    onChange={(value) => setFilters(prev => ({ ...prev, hardwareConfig: value }))}
                    placeholder="Выберите конфигурацию"
                    style={{ width: '100%', marginTop: 4 }}
                    allowClear
                  >
                    {filterOptions.hardware_configs.map(config => (
                      <Option key={config} value={config}>
                        {config}
                      </Option>
                    ))}
                  </Select>
                </div>

                <div>
                  <Text strong>TPS диапазон:</Text>
                  <div style={{ display: 'flex', gap: 8, marginTop: 4 }}>
                    <InputNumber
                      value={filters.tpsMin}
                      onChange={(value) => setFilters(prev => ({ ...prev, tpsMin: value }))}
                      placeholder="От"
                      style={{ flex: 1 }}
                      min={0}
                    />
                    <InputNumber
                      value={filters.tpsMax}
                      onChange={(value) => setFilters(prev => ({ ...prev, tpsMax: value }))}
                      placeholder="До"
                      style={{ flex: 1 }}
                      min={0}
                    />
                  </div>
                </div>

                <div style={{ marginTop: 16 }}>
                  <Space direction="vertical" style={{ width: '100%' }}>
                    <Button 
                      type={selectedRowKeys.length === 2 ? 'primary' : 'default'}
                      icon={<SwapOutlined />}
                      disabled={selectedRowKeys.length !== 2}
                      onClick={handleCompareRuns}
                      style={{ width: '100%' }}
                      title={selectedRowKeys.length === 2 ? 'Сравнить выбранные запуски' : 'Выберите ровно 2 запуска для сравнения'}
                    >
                      Сравнить ({selectedRowKeys.length}/2)
                    </Button>
                    <Button 
                      icon={<ReloadOutlined />} 
                      onClick={fetchRuns}
                      loading={loading}
                      style={{ width: '100%' }}
                    >
                      Обновить
                    </Button>
                    <Button 
                      onClick={clearAllFilters}
                      type="default"
                      style={{ width: '100%' }}
                    >
                      Очистить фильтры
                    </Button>
                  </Space>
                </div>
              </Space>
            </div>
          </Col>
        </Row>
      </Card>

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

              {/* Конфигурация нагрузки */}
              <Card 
                title={
                  <Space>
                    <ThunderboltOutlined />
                    <Text strong>Конфигурация нагрузки</Text>
                  </Space>
                }
                style={{ marginBottom: 16 }}
              >
                <Descriptions column={2} bordered size="small">
                  <Descriptions.Item label="Тип нагрузки" span={1}>
                    <Tag color="blue">{(config.workloadType || 'custom').toUpperCase()}</Tag>
                  </Descriptions.Item>
                  <Descriptions.Item label="Количество раннеров" span={1}>
                    {config.workloadProperties?.runners || 1}
                  </Descriptions.Item>
                  <Descriptions.Item label="Продолжительность теста" span={2}>
                    {config.workloadProperties?.duration || 'Не указано'}
                  </Descriptions.Item>
                </Descriptions>
                
                {/* Дополнительные свойства нагрузки */}
                {config.workloadProperties && Object.keys(config.workloadProperties).length > 0 && (
                  <>
                    <Divider orientation="left" plain>Дополнительные свойства</Divider>
                    <div style={{ marginTop: 8 }}>
                      {Object.entries(config.workloadProperties)
                        .filter(([key]) => !['runners', 'duration'].includes(key))
                        .map(([key, value]) => (
                          <div key={key} style={{ marginBottom: 8 }}>
                            <Text type="secondary">{key}: </Text>
                            <Tag color="blue">{typeof value === 'object' ? JSON.stringify(value) : String(value)}</Tag>
                          </div>
                        ))}
                      {Object.keys(config.workloadProperties).filter(key => !['runners', 'duration'].includes(key)).length === 0 && (
                        <Text type="secondary">Дополнительные свойства отсутствуют</Text>
                      )}
                    </div>
                  </>
                )}
              </Card>

              {/* Конфигурация базы данных */}
              <Card 
                title={
                  <Space>
                    <DatabaseOutlined />
                    <Text strong>Конфигурация базы данных</Text>
                  </Space>
                }
                style={{ marginBottom: 16 }}
              >
                <Descriptions column={2} bordered size="small">
                  <Descriptions.Item label="Тип СУБД" span={1}>
                    <Tag color="green">{(config.databaseType || 'postgres').toUpperCase()}</Tag>
                  </Descriptions.Item>
                  <Descriptions.Item label="Версия" span={1}>
                    {config.databaseVersion?.version || 'Не указано'}
                  </Descriptions.Item>
                  <Descriptions.Item label="Сборка" span={2}>
                    {config.databaseVersion?.build || <Text type="secondary">Не указано</Text>}
                  </Descriptions.Item>
                </Descriptions>
              </Card>

              {/* Конфигурация железа */}
              <Card 
                title={
                  <Space>
                    <CloudServerOutlined />
                    <Text strong>Конфигурация железа</Text>
                  </Space>
                }
                style={{ marginBottom: 16 }}
              >
                <Descriptions column={2} bordered size="small">
                  <Descriptions.Item label="Название конфигурации" span={1}>
                    {config.hardwareConfiguration?.name || 'Неизвестная конфигурация'}
                  </Descriptions.Item>
                  <Descriptions.Item label="Количество узлов" span={1}>
                    {config.hardwareConfiguration?.nodeCount || 1}
                  </Descriptions.Item>
                  <Descriptions.Item label="Процессор" span={1}>
                    {config.hardwareConfiguration?.cpu?.cores || 0} ядер
                  </Descriptions.Item>
                  <Descriptions.Item label="Модель CPU" span={1}>
                    {config.hardwareConfiguration?.cpu?.model || 'Неизвестно'}
                  </Descriptions.Item>
                  <Descriptions.Item label="Память" span={1}>
                    {config.hardwareConfiguration?.memory?.totalGB || 0} GB
                  </Descriptions.Item>
                  <Descriptions.Item label="Тип накопителя" span={1}>
                    {(config.hardwareConfiguration?.storage?.type || 'ssd').toUpperCase()}
                  </Descriptions.Item>
                  <Descriptions.Item label="Объем накопителя" span={1}>
                    {config.hardwareConfiguration?.storage?.capacityGB || 0} GB
                  </Descriptions.Item>
                  <Descriptions.Item label="Сигнатура" span={1}>
                    <Tag color="orange">
                      {config.hardwareConfiguration?.signature || 
                       `${config.hardwareConfiguration?.cpu?.cores || 0}c-${config.hardwareConfiguration?.memory?.totalGB || 0}gb-${config.hardwareConfiguration?.storage?.type || 'ssd'}-${config.hardwareConfiguration?.storage?.capacityGB || 0}gb-${config.hardwareConfiguration?.nodeCount || 1}n`}
                    </Tag>
                  </Descriptions.Item>
                </Descriptions>
              </Card>

              {/* Схема развертывания */}
              <Card 
                title={
                  <Space>
                    <CloudServerOutlined />
                    <Text strong>Схема развертывания</Text>
                  </Space>
                }
                style={{ marginBottom: 16 }}
              >
                <Descriptions column={2} bordered size="small">
                  <Descriptions.Item label="Тип развертывания" span={1}>
                    <Tag color="cyan">{config.deploymentLayout?.type || 'single-node'}</Tag>
                  </Descriptions.Item>
                  <Descriptions.Item label="Сигнатура" span={1}>
                    <Tag color="purple">{config.deploymentLayout?.signature || 'single-node'}</Tag>
                  </Descriptions.Item>
                </Descriptions>
                
                {/* Конфигурация развертывания */}
                <Divider orientation="left" plain>Параметры развертывания</Divider>
                <div style={{ marginTop: 8 }}>
                  {config.deploymentLayout?.configuration && Object.keys(config.deploymentLayout.configuration).length > 0 ? (
                    Object.entries(config.deploymentLayout.configuration).map(([key, value]) => (
                      <div key={key} style={{ marginBottom: 8 }}>
                        <Text type="secondary">{key}: </Text>
                        <Tag color="blue">{String(value)}</Tag>
                      </div>
                    ))
                  ) : (
                    <Text type="secondary">Дополнительные параметры отсутствуют</Text>
                  )}
                </div>
              </Card>

              {/* Конфигурация немезисов */}
              <Card 
                title={
                  <Space>
                    <BugOutlined />
                    <Text strong>Конфигурация немезисов</Text>
                  </Space>
                }
                style={{ marginBottom: 16 }}
              >
                <Descriptions column={1} bordered size="small">
                  <Descriptions.Item label="Сигнатура немезисов">
                    <Tag color="red">{config.nemesisSignature?.signature || 'none'}</Tag>
                  </Descriptions.Item>
                </Descriptions>
                
                {/* Активные немезисы */}
                <Divider orientation="left" plain>Активные немезисы</Divider>
                <div style={{ marginTop: 8 }}>
                  {config.nemesisSignature?.nemeses && config.nemesisSignature.nemeses.length > 0 ? (
                    <Space wrap>
                      {config.nemesisSignature.nemeses
                        .filter((n: any) => n.enabled)
                        .map((nemesis: any, index: number) => (
                          <Tag key={index} color="red" icon={<BugOutlined />}>
                            {nemesis.type}
                          </Tag>
                        ))}
                      {config.nemesisSignature.nemeses.filter((n: any) => n.enabled).length === 0 && (
                        <Text type="secondary">Нет активных немезисов</Text>
                      )}
                    </Space>
                  ) : (
                    <Text type="secondary">Немезисы отсутствуют</Text>
                  )}
                </div>
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

              {/* Итоговая статистика */}
              <Card 
                title="Сводка конфигурации" 
                style={{ 
                  marginTop: 16,
                  background: 'linear-gradient(135deg, #f0f2f5 0%, #e6f7ff 100%)'
                }}
              >
                <Row gutter={16}>
                  <Col span={6}>
                    <Statistic
                      title="Статус запуска"
                      value={getStatusText(selectedRun.status)}
                      prefix={getStatusIcon(selectedRun.status)}
                      valueStyle={{ 
                        color: selectedRun.status === 'completed' ? '#52c41a' : 
                               selectedRun.status === 'failed' ? '#ff4d4f' : '#1890ff',
                        fontSize: '16px'
                      }}
                    />
                  </Col>
                  <Col span={6}>
                    <Statistic
                      title="Тип нагрузки"
                      value={(config.workloadType || 'custom').toUpperCase()}
                      prefix={<ThunderboltOutlined />}
                      valueStyle={{ color: '#1890ff', fontSize: '16px' }}
                    />
                  </Col>
                  <Col span={6}>
                    <Statistic
                      title="Тип СУБД"
                      value={(config.databaseType || 'postgres').toUpperCase()}
                      prefix={<DatabaseOutlined />}
                      valueStyle={{ color: '#52c41a', fontSize: '16px' }}
                    />
                  </Col>
                  <Col span={6}>
                    <Statistic
                      title="Узлов"
                      value={config.hardwareConfiguration?.nodeCount || 1}
                      prefix={<CloudServerOutlined />}
                      valueStyle={{ color: '#722ed1', fontSize: '16px' }}
                    />
                  </Col>
                </Row>
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

      {/* Модальное окно сравнения запусков */}
      {selectedRowKeys.length === 2 && (
        <RunComparisonModal
          visible={comparisonModalVisible}
          onClose={() => setComparisonModalVisible(false)}
          runs={[
            convertRunForComparison(runs.find(run => run.id === selectedRowKeys[0])!),
            convertRunForComparison(runs.find(run => run.id === selectedRowKeys[1])!)
          ] as [any, any]}
        />
      )}

    </div>
  )
}

export default DashboardPage
