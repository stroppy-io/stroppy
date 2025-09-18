import { 
  Card, 
  Table, 
  Button, 
  Space, 
  Tag, 
  Typography, 
  Row, 
  Col, 
  Statistic,
  Modal,
  message,
  Input,
  Select,
  DatePicker,
  Tooltip,
  Flex,
  Pagination
} from 'antd'
import { 
  PlayCircleOutlined, 
  ReloadOutlined,
  EyeOutlined,
  DeleteOutlined,
  CheckCircleOutlined,
  ExclamationCircleOutlined,
  PlusOutlined,
  ClockCircleOutlined,
  ClearOutlined,
  UserOutlined,
  SwapOutlined,
  SettingOutlined,
  ThunderboltOutlined,
  DatabaseOutlined,
  CloudServerOutlined,
  BugOutlined
} from '@ant-design/icons'
import { useState, useEffect, useMemo } from 'react'
import type { ColumnsType, TableProps } from 'antd/es/table'
import dayjs from 'dayjs'
import RunCreationForm from '../components/RunCreationForm'
import RunComparisonModal from '../components/RunComparisonModal'
import type { RunCreationFormData } from '../types/run-creation'
import { apiClient, getErrorMessage } from '../services/api'
import type { Run } from '../services/api'

const { Title, Paragraph, Text } = Typography
const { Option } = Select
const { RangePicker } = DatePicker
const { Search } = Input

interface FilterState {
  searchText: string;
  status: string;
  dateRange: [dayjs.Dayjs | null, dayjs.Dayjs | null] | null;
}

const RunsPage: React.FC = () => {
  const [selectedRowKeys, setSelectedRowKeys] = useState<React.Key[]>([])
  const [modalVisible, setModalVisible] = useState(false)
  const [selectedRun, setSelectedRun] = useState<Run | null>(null)
  const [createRunModalVisible, setCreateRunModalVisible] = useState(false)
  const [comparisonModalVisible, setComparisonModalVisible] = useState(false)
  const [runs, setRuns] = useState<Run[]>([])
  const [loading, setLoading] = useState(true)
  const [pagination, setPagination] = useState({
    current: 1,
    pageSize: 50,
    total: 0
  })
  
  // Состояние фильтров
  const [filters, setFilters] = useState<FilterState>({
    searchText: '',
    status: '',
    dateRange: null
  })

  // Загрузка запусков с сервера с фильтрами
  const fetchRuns = async (
    page: number = 1, 
    limit: number = 20,
    searchText?: string,
    status?: string,
    dateFrom?: string,
    dateTo?: string
  ) => {
    try {
      setLoading(true)
      const response = await apiClient.getRuns(page, limit, searchText, status, dateFrom, dateTo)
      setRuns(response.runs)
      setPagination({
        current: response.page,
        pageSize: response.limit,
        total: response.total
      })
    } catch (error) {
      message.error(`Ошибка загрузки запусков: ${getErrorMessage(error)}`)
    } finally {
      setLoading(false)
    }
  }

  // Вспомогательная функция для загрузки данных с текущими фильтрами
  const fetchRunsWithCurrentFilters = (page?: number, pageSize?: number) => {
    const currentPage = page || pagination.current;
    const currentPageSize = pageSize || pagination.pageSize;
    
    let dateFrom, dateTo;
    if (filters.dateRange && filters.dateRange[0] && filters.dateRange[1]) {
      dateFrom = filters.dateRange[0].format('YYYY-MM-DD');
      dateTo = filters.dateRange[1].format('YYYY-MM-DD');
    }

    fetchRuns(
      currentPage,
      currentPageSize,
      filters.searchText || undefined,
      filters.status || undefined,
      dateFrom,
      dateTo
    );
  };

  // Загружаем данные при монтировании компонента
  useEffect(() => {
    fetchRunsWithCurrentFilters(1);
  }, []);


  // Функции для работы с фильтрами
  const handleFilterChange = (key: keyof FilterState, value: any) => {
    const newFilters = { ...filters, [key]: value };
    setFilters(newFilters);
    
    // Сбрасываем пагинацию и загружаем данные с новыми фильтрами
    setPagination(prev => ({ ...prev, current: 1 }));
    
    // Применяем фильтры с задержкой для лучшего UX
    setTimeout(() => {
      fetchRunsWithCurrentFilters(1);
    }, 300);
  };

  const clearFilters = () => {
    setFilters({
      searchText: '',
      status: '',
      dateRange: null
    });
    setPagination(prev => ({ ...prev, current: 1 }));
    
    // Загружаем данные без фильтров
    fetchRuns(1, pagination.pageSize);
  };

  const hasActiveFilters = filters.searchText || filters.status || filters.dateRange;

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
      }
    };
  };

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
  };

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

  const handleRunAction = async (action: string, record: Run) => {
    switch (action) {
      case 'view':
        setSelectedRun(record)
        setModalVisible(true)
        break
      case 'delete':
        Modal.confirm({
          title: 'Подтвердите удаление',
          content: `Вы уверены, что хотите удалить запуск "${record.name}"?`,
          okText: 'Удалить',
          cancelText: 'Отмена',
          okType: 'danger',
          onOk: async () => {
            try {
              await apiClient.deleteRun(record.id)
              message.success(`Запуск "${record.name}" удален`)
              fetchRuns(pagination.current, pagination.pageSize)
            } catch (error) {
              message.error(`Ошибка удаления: ${getErrorMessage(error)}`)
            }
          }
        })
        break
    }
  }

  const handleCreateRun = async (formData: RunCreationFormData) => {
    try {
      const config = JSON.stringify({
        workloadType: formData.workloadType,
        workloadProperties: formData.workloadProperties,
        databaseType: formData.databaseType,
        databaseVersion: formData.databaseVersion,
        hardwareConfiguration: formData.hardwareConfiguration,
        deploymentLayout: formData.deploymentLayout,
        nemesisSignature: formData.nemesisSignature
      })

      await apiClient.createRun({
        name: formData.name,
        description: formData.description || '',
        config
      })

      message.success('Запуск успешно создан')
      setCreateRunModalVisible(false)
      fetchRuns(pagination.current, pagination.pageSize)
    } catch (error) {
      message.error(`Ошибка создания запуска: ${getErrorMessage(error)}`)
    }
  }

  const columns: ColumnsType<Run> = [
    {
      title: 'ID',
      dataIndex: 'id',
      key: 'id',
      width: 80,
      sorter: (a, b) => a.id - b.id,
      render: (id) => <Text code>#{id}</Text>
    },
    {
      title: 'Название',
      dataIndex: 'name',
      key: 'name',
      sorter: (a, b) => a.name.localeCompare(b.name),
      render: (name, record) => (
        <div>
          <Text strong>{name}</Text>
          {record.description && (
            <div>
              <Text type="secondary" style={{ fontSize: '12px' }}>
                {record.description.length > 50 
                  ? `${record.description.substring(0, 50)}...` 
                  : record.description}
              </Text>
            </div>
          )}
        </div>
      )
    },
    {
      title: 'Статус',
      dataIndex: 'status',
      key: 'status',
      width: 120,
      sorter: (a, b) => a.status.localeCompare(b.status),
      render: (status) => (
        <Tag icon={getStatusIcon(status)} color={getStatusColor(status)}>
          {getStatusText(status)}
        </Tag>
      )
    },
    {
      title: 'Пользователь',
      dataIndex: 'user_id',
      key: 'user_id',
      width: 100,
      sorter: (a, b) => a.user_id - b.user_id,
      render: (userId) => (
        <Tag icon={<UserOutlined />} color="blue">
          ID: {userId}
        </Tag>
      )
    },
    {
      title: 'Создан',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 150,
      sorter: (a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime(),
      render: (date) => (
        <Tooltip title={dayjs(date).format('DD.MM.YYYY HH:mm:ss')}>
          <Text>{dayjs(date).format('DD.MM HH:mm')}</Text>
        </Tooltip>
      )
    },
    {
      title: 'Обновлен',
      dataIndex: 'updated_at',
      key: 'updated_at',
      width: 150,
      sorter: (a, b) => new Date(a.updated_at).getTime() - new Date(b.updated_at).getTime(),
      render: (date) => (
        <Tooltip title={dayjs(date).format('DD.MM.YYYY HH:mm:ss')}>
          <Text>{dayjs(date).format('DD.MM HH:mm')}</Text>
        </Tooltip>
      )
    },
    {
      title: 'Действия',
      key: 'actions',
      width: 120,
      render: (_, record) => (
        <Space size="small">
          <Button
            type="text"
            icon={<EyeOutlined />}
            size="small"
            onClick={() => handleRunAction('view', record)}
            title="Просмотр"
          />
          <Button
            type="text"
            icon={<DeleteOutlined />}
            size="small"
            danger
            onClick={() => handleRunAction('delete', record)}
            title="Удалить"
          />
        </Space>
      )
    }
  ]

  const rowSelection = {
    selectedRowKeys,
    onChange: (newSelectedRowKeys: React.Key[]) => {
      setSelectedRowKeys(newSelectedRowKeys)
    }
  }

  // Подсчет статистики
  const stats = useMemo(() => {
    const total = pagination.total; // Используем общее количество с сервера
    const pending = runs.filter(r => r.status === 'pending').length;
    const running = runs.filter(r => r.status === 'running').length;
    const completed = runs.filter(r => r.status === 'completed').length;
    const failed = runs.filter(r => r.status === 'failed').length;

    return { total, pending, running, completed, failed };
  }, [runs, pagination.total]);

  const tableProps: TableProps<Run> = {
    rowSelection,
    columns,
    dataSource: runs, // Используем данные с сервера напрямую
    rowKey: "id",
    size: "small",
    loading,
    pagination: false, // Отключаем встроенную пагинацию
    scroll: { x: 'max-content', y: 'calc(100vh - 450px)' }
  };

  return (
    <div style={{ 
      height: '100vh', 
      display: 'flex', 
      flexDirection: 'column', 
      padding: '24px',
      gap: '16px',
      overflow: 'hidden'
    }}>
      {/* Заголовок */}
      <div>
        <Title level={2} style={{ margin: 0 }}>
          Запуски тестирования
        </Title>
        <Paragraph type="secondary" style={{ margin: '8px 0 0 0' }}>
          Управление и мониторинг всех запусков тестирования в системе
        </Paragraph>
      </div>

      {/* Статистика */}
      <Row gutter={16}>
        <Col span={4}>
          <Card size="small">
            <Statistic
              title="Всего"
              value={stats.total}
              prefix={<PlayCircleOutlined />}
            />
          </Card>
        </Col>
        <Col span={4}>
          <Card size="small">
            <Statistic
              title="Ожидают"
              value={stats.pending}
              prefix={<ClockCircleOutlined />}
              valueStyle={{ color: '#8c8c8c' }}
            />
          </Card>
        </Col>
        <Col span={4}>
          <Card size="small">
            <Statistic
              title="Выполняются"
              value={stats.running}
              prefix={<PlayCircleOutlined />}
              valueStyle={{ color: '#1890ff' }}
            />
          </Card>
        </Col>
        <Col span={4}>
          <Card size="small">
            <Statistic
              title="Завершены"
              value={stats.completed}
              prefix={<CheckCircleOutlined />}
              valueStyle={{ color: '#52c41a' }}
            />
          </Card>
        </Col>
        <Col span={4}>
          <Card size="small">
            <Statistic
              title="Ошибки"
              value={stats.failed}
              prefix={<ExclamationCircleOutlined />}
              valueStyle={{ color: '#ff4d4f' }}
            />
          </Card>
        </Col>
      </Row>

      {/* Фильтры и управление */}
      <Card size="small">
        <Flex gap="middle" align="center" wrap="wrap">
          <Search
            placeholder="Поиск по названию или описанию..."
            value={filters.searchText}
            onChange={(e) => handleFilterChange('searchText', e.target.value)}
            style={{ width: 300 }}
            allowClear
          />
          
          <Select
            placeholder="Статус"
            value={filters.status || undefined}
            onChange={(value) => handleFilterChange('status', value)}
            style={{ width: 150 }}
            allowClear
          >
            <Option value="pending">Ожидает</Option>
            <Option value="running">Выполняется</Option>
            <Option value="completed">Завершен</Option>
            <Option value="failed">Ошибка</Option>
            <Option value="cancelled">Отменен</Option>
          </Select>

          <RangePicker
            placeholder={['Дата от', 'Дата до']}
            value={filters.dateRange}
            onChange={(dates) => handleFilterChange('dateRange', dates)}
            style={{ width: 250 }}
            format="DD.MM.YYYY"
          />

          {hasActiveFilters && (
            <Button
              icon={<ClearOutlined />}
              onClick={clearFilters}
              title="Очистить фильтры"
            >
              Очистить
            </Button>
          )}

          <div style={{ marginLeft: 'auto' }}>
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
              <Button 
                type="primary" 
                icon={<PlusOutlined />}
                onClick={() => setCreateRunModalVisible(true)}
              >
                Новый запуск
              </Button>
              <Button 
                icon={<ReloadOutlined />}
                loading={loading}
                onClick={() => fetchRunsWithCurrentFilters()}
              >
                Обновить
              </Button>
            </Space>
          </div>
        </Flex>
      </Card>

      {/* Таблица запусков */}
      <Card 
        style={{ 
          flex: 1, 
          display: 'flex', 
          flexDirection: 'column', 
          minHeight: 0,
          overflow: 'hidden'
        }}
        bodyStyle={{ 
          flex: 1, 
          display: 'flex', 
          flexDirection: 'column', 
          padding: '16px',
          overflow: 'hidden'
        }}
      >
        <Table 
          {...tableProps} 
          style={{ height: '100%' }}
        />
        
        {/* Пагинация */}
        <div style={{ 
          padding: '16px 0', 
          borderTop: '1px solid #f0f0f0',
          display: 'flex',
          justifyContent: 'center'
        }}>
          <Pagination
            current={pagination.current}
            pageSize={pagination.pageSize}
            total={pagination.total}
            showSizeChanger
            showQuickJumper
            showTotal={(total, range) => 
              `${range[0]}-${range[1]} из ${total} запусков`
            }
            pageSizeOptions={['20', '50', '100', '200']}
            onChange={(page, pageSize) => {
              setPagination(prev => ({
                ...prev,
                current: page,
                pageSize: pageSize || prev.pageSize
              }));
              // Загружаем новую страницу с сервера
              fetchRunsWithCurrentFilters(page, pageSize);
            }}
          />
        </div>
      </Card>

      {/* Модальное окно деталей */}
      <Modal
        title={
          <div style={{ display: 'flex', alignItems: 'center' }}>
            <EyeOutlined style={{ marginRight: 8 }} />
            Детали запуска
          </div>
        }
        open={modalVisible}
        onCancel={() => setModalVisible(false)}
        footer={[
          <Button key="close" onClick={() => setModalVisible(false)}>
            Закрыть
          </Button>
        ]}
        width={1200}
        style={{ top: 20 }}
      >
        {selectedRun && (() => {
          let config;
          try {
            config = JSON.parse(selectedRun.config);
          } catch {
            config = {};
          }
          
          return (
            <div style={{ maxHeight: '80vh', overflowY: 'auto' }}>
              {/* Основная информация */}
              <Card title={<><SettingOutlined style={{ marginRight: 8 }} />Основная информация</>} size="small" style={{ marginBottom: 16 }}>
                <Row gutter={16}>
                  <Col span={12}>
                    <p><strong>ID:</strong> <Tag color="blue">#{selectedRun.id}</Tag></p>
                    <p><strong>Название:</strong> {selectedRun.name}</p>
                    <p><strong>Описание:</strong> {selectedRun.description || <Text type="secondary">Нет описания</Text>}</p>
                    <p><strong>Пользователь ID:</strong> <Tag icon={<UserOutlined />} color="purple">{selectedRun.user_id}</Tag></p>
                  </Col>
                  <Col span={12}>
                    <p><strong>Статус:</strong> 
                      <Tag icon={getStatusIcon(selectedRun.status)} color={getStatusColor(selectedRun.status)} style={{ marginLeft: 8 }}>
                        {getStatusText(selectedRun.status)}
                      </Tag>
                    </p>
                    <p><strong>Создан:</strong> {dayjs(selectedRun.created_at).format('DD.MM.YYYY HH:mm:ss')}</p>
                    <p><strong>Обновлен:</strong> {dayjs(selectedRun.updated_at).format('DD.MM.YYYY HH:mm:ss')}</p>
                    {selectedRun.started_at && (
                      <p><strong>Запущен:</strong> {dayjs(selectedRun.started_at).format('DD.MM.YYYY HH:mm:ss')}</p>
                    )}
                    {selectedRun.completed_at && (
                      <p><strong>Завершен:</strong> {dayjs(selectedRun.completed_at).format('DD.MM.YYYY HH:mm:ss')}</p>
                    )}
                    {selectedRun.completed_at && selectedRun.started_at && (
                      <p><strong>Длительность:</strong> {dayjs(selectedRun.completed_at).diff(dayjs(selectedRun.started_at), 'minute')} мин</p>
                    )}
                  </Col>
                </Row>
              </Card>

              {/* Тип нагрузки */}
              {config.workloadType && (
                <Card title={<><ThunderboltOutlined style={{ marginRight: 8 }} />Нагрузка</>} size="small" style={{ marginBottom: 16 }}>
                  <Row gutter={16}>
                    <Col span={8}>
                      <p><strong>Тип:</strong> <Tag color="blue">{config.workloadType.toUpperCase()}</Tag></p>
                    </Col>
                    <Col span={8}>
                      <p><strong>Раннеры:</strong> {config.workloadProperties?.runners || 'Не указано'}</p>
                    </Col>
                    <Col span={8}>
                      <p><strong>Длительность:</strong> {config.workloadProperties?.duration || 'Не указано'}</p>
                    </Col>
                  </Row>
                  {config.workloadProperties && Object.keys(config.workloadProperties).length > 2 && (
                    <div style={{ marginTop: 12 }}>
                      <strong>Дополнительные свойства:</strong>
                      <div style={{ marginTop: 8 }}>
                        {Object.entries(config.workloadProperties)
                          .filter(([key]) => !['runners', 'duration'].includes(key))
                          .map(([key, value]) => (
                            <Tag key={key} style={{ marginBottom: 4 }}>
                              {key}: {typeof value === 'object' ? JSON.stringify(value) : String(value)}
                            </Tag>
                          ))}
                      </div>
                    </div>
                  )}
                </Card>
              )}

              {/* База данных */}
              {config.databaseType && (
                <Card title={<><DatabaseOutlined style={{ marginRight: 8 }} />База данных</>} size="small" style={{ marginBottom: 16 }}>
                  <Row gutter={16}>
                    <Col span={8}>
                      <p><strong>Тип:</strong> <Tag color="green">{config.databaseType.toUpperCase()}</Tag></p>
                    </Col>
                    <Col span={8}>
                      <p><strong>Версия:</strong> {config.databaseVersion?.version || 'Не указано'}</p>
                    </Col>
                    <Col span={8}>
                      <p><strong>Сборка:</strong> {config.databaseVersion?.build || <Text type="secondary">Не указано</Text>}</p>
                    </Col>
                  </Row>
                </Card>
              )}

              {/* Конфигурация железа */}
              {config.hardwareConfiguration && (
                <Card title={<><CloudServerOutlined style={{ marginRight: 8 }} />Конфигурация железа</>} size="small" style={{ marginBottom: 16 }}>
                  <Row gutter={16}>
                    <Col span={12}>
                      <p><strong>Название:</strong> {config.hardwareConfiguration.name || 'Не указано'}</p>
                      <p><strong>Узлы:</strong> {config.hardwareConfiguration.nodeCount || 1}</p>
                      <p><strong>Процессор:</strong> {config.hardwareConfiguration.cpu?.cores || 0} ядер ({config.hardwareConfiguration.cpu?.model || 'Неизвестно'})</p>
                    </Col>
                    <Col span={12}>
                      <p><strong>Память:</strong> {config.hardwareConfiguration.memory?.totalGB || 0} GB</p>
                      <p><strong>Накопитель:</strong> {config.hardwareConfiguration.storage?.type?.toUpperCase() || 'SSD'} {config.hardwareConfiguration.storage?.capacityGB || 0} GB</p>
                      {config.hardwareConfiguration.signature && (
                        <p><strong>Сигнатура:</strong> <Tag color="orange">{config.hardwareConfiguration.signature}</Tag></p>
                      )}
                    </Col>
                  </Row>
                </Card>
              )}

              {/* Схема развертывания */}
              {config.deploymentLayout && (
                <Card title={<><CloudServerOutlined style={{ marginRight: 8 }} />Схема развертывания</>} size="small" style={{ marginBottom: 16 }}>
                  <Row gutter={16}>
                    <Col span={12}>
                      <p><strong>Тип:</strong> <Tag color="cyan">{config.deploymentLayout.type}</Tag></p>
                      {config.deploymentLayout.signature && (
                        <p><strong>Сигнатура:</strong> <Tag color="purple">{config.deploymentLayout.signature}</Tag></p>
                      )}
                    </Col>
                    <Col span={12}>
                      {config.deploymentLayout.configuration && Object.keys(config.deploymentLayout.configuration).length > 0 && (
                        <div>
                          <strong>Конфигурация:</strong>
                          <div style={{ marginTop: 4 }}>
                            {Object.entries(config.deploymentLayout.configuration).map(([key, value]) => (
                              <div key={key}>
                                <Text type="secondary">{key}: </Text>
                                <Text>{String(value)}</Text>
                              </div>
                            ))}
                          </div>
                        </div>
                      )}
                    </Col>
                  </Row>
                </Card>
              )}

              {/* Немезисы */}
              {config.nemesisSignature && (
                <Card title={<><BugOutlined style={{ marginRight: 8 }} />Немезисы</>} size="small" style={{ marginBottom: 16 }}>
                  <Row gutter={16}>
                    <Col span={12}>
                      <p><strong>Сигнатура:</strong> <Tag color="red">{config.nemesisSignature.signature}</Tag></p>
                    </Col>
                    <Col span={12}>
                      {config.nemesisSignature.nemeses && config.nemesisSignature.nemeses.length > 0 ? (
                        <div>
                          <strong>Активные немезисы:</strong>
                          <div style={{ marginTop: 4 }}>
                            {config.nemesisSignature.nemeses
                              .filter((n: any) => n.enabled)
                              .map((nemesis: any, index: number) => (
                                <Tag key={index} color="volcano" style={{ marginBottom: 4 }}>
                                  {nemesis.type}
                                </Tag>
                              ))}
                          </div>
                        </div>
                      ) : (
                        <Text type="secondary">Немезисы отсутствуют</Text>
                      )}
                    </Col>
                  </Row>
                </Card>
              )}
              
              {/* Результат */}
              {selectedRun.result && (
                <Card title="Результат выполнения" size="small" style={{ marginBottom: 16 }}>
                  <pre style={{ 
                    background: '#f5f5f5', 
                    padding: 12, 
                    borderRadius: 4, 
                    fontSize: '12px',
                    maxHeight: '300px',
                    overflow: 'auto',
                    margin: 0
                  }}>
                    {JSON.stringify(JSON.parse(selectedRun.result), null, 2)}
                  </pre>
                </Card>
              )}

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

      {/* Модальное окно создания запуска */}
      <Modal
        title="Создать новый запуск"
        open={createRunModalVisible}
        onCancel={() => setCreateRunModalVisible(false)}
        footer={null}
        width={800}
        destroyOnClose
      >
        <RunCreationForm 
          onSubmit={handleCreateRun}
          onCancel={() => setCreateRunModalVisible(false)}
        />
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

export default RunsPage