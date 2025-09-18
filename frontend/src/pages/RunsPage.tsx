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
  Flex
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
  UserOutlined
} from '@ant-design/icons'
import { useState, useEffect, useMemo } from 'react'
import type { ColumnsType, TableProps } from 'antd/es/table'
import dayjs from 'dayjs'
import RunCreationForm from '../components/RunCreationForm'
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

  // Загрузка запусков с сервера
  const fetchRuns = async (page: number = 1, limit: number = 50) => {
    try {
      setLoading(true)
      const response = await apiClient.getRuns(page, limit)
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

  // Фильтрованные данные
  const filteredRuns = useMemo(() => {
    let filtered = [...runs];

    // Применяем поиск по тексту
    if (filters.searchText) {
      const searchLower = filters.searchText.toLowerCase();
      filtered = filtered.filter(run => 
        run.name.toLowerCase().includes(searchLower) ||
        run.description.toLowerCase().includes(searchLower)
      );
    }

    // Применяем фильтр по статусу
    if (filters.status) {
      filtered = filtered.filter(run => run.status === filters.status);
    }

    // Применяем фильтр по дате
    if (filters.dateRange && filters.dateRange[0] && filters.dateRange[1]) {
      const [startDate, endDate] = filters.dateRange;
      filtered = filtered.filter(run => {
        const createdAt = dayjs(run.created_at);
        return createdAt.isAfter(startDate.startOf('day')) && 
               createdAt.isBefore(endDate.endOf('day'));
      });
    }

    return filtered;
  }, [runs, filters]);

  // Загружаем данные при монтировании компонента
  useEffect(() => {
    fetchRuns(1, 1000) // Загружаем все данные сразу для клиентской фильтрации
  }, [])

  // Функции для работы с фильтрами
  const handleFilterChange = (key: keyof FilterState, value: any) => {
    setFilters(prev => ({ ...prev, [key]: value }));
  };

  const clearFilters = () => {
    setFilters({
      searchText: '',
      status: '',
      dateRange: null
    });
  };

  const hasActiveFilters = filters.searchText || filters.status || filters.dateRange;

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
    const total = filteredRuns.length;
    const pending = filteredRuns.filter(r => r.status === 'pending').length;
    const running = filteredRuns.filter(r => r.status === 'running').length;
    const completed = filteredRuns.filter(r => r.status === 'completed').length;
    const failed = filteredRuns.filter(r => r.status === 'failed').length;

    return { total, pending, running, completed, failed };
  }, [filteredRuns]);

  const tableProps: TableProps<Run> = {
    rowSelection,
    columns,
    dataSource: filteredRuns,
    rowKey: "id",
    size: "small",
    loading,
    pagination: {
      current: pagination.current,
      pageSize: pagination.pageSize,
      total: filteredRuns.length,
      showSizeChanger: true,
      showQuickJumper: true,
      showTotal: (total, range) => 
        `${range[0]}-${range[1]} из ${total} запусков`,
      size: 'small',
      pageSizeOptions: ['20', '50', '100', '200'],
      onChange: (page, pageSize) => {
        setPagination(prev => ({
          ...prev,
          current: page,
          pageSize: pageSize || prev.pageSize
        }));
      }
    },
    scroll: { x: 'max-content', y: 'calc(100vh - 350px)' }
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
                type="primary" 
                icon={<PlusOutlined />}
                onClick={() => setCreateRunModalVisible(true)}
              >
                Новый запуск
              </Button>
              <Button 
                icon={<ReloadOutlined />}
                loading={loading}
                onClick={() => fetchRuns(1, 1000)}
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
          overflow: 'hidden',
          minHeight: 0 
        }}
        bodyStyle={{ 
          flex: 1, 
          display: 'flex', 
          flexDirection: 'column', 
          overflow: 'hidden',
          padding: '16px'
        }}
      >
        <Table {...tableProps} />
      </Card>

      {/* Модальное окно деталей */}
      <Modal
        title="Детали запуска"
        open={modalVisible}
        onCancel={() => setModalVisible(false)}
        footer={[
          <Button key="close" onClick={() => setModalVisible(false)}>
            Закрыть
          </Button>
        ]}
        width={800}
      >
        {selectedRun && (
          <div>
            <Row gutter={16}>
              <Col span={12}>
                <p><strong>ID:</strong> {selectedRun.id}</p>
                <p><strong>Название:</strong> {selectedRun.name}</p>
                <p><strong>Описание:</strong> {selectedRun.description || 'Нет описания'}</p>
                <p><strong>Пользователь ID:</strong> {selectedRun.user_id}</p>
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
              </Col>
            </Row>
            
            <div style={{ marginTop: 16 }}>
              <strong>Конфигурация:</strong>
              <pre style={{ 
                background: '#f5f5f5', 
                padding: 12, 
                borderRadius: 4, 
                marginTop: 8,
                fontSize: '12px',
                maxHeight: '200px',
                overflow: 'auto'
              }}>
                {JSON.stringify(JSON.parse(selectedRun.config), null, 2)}
              </pre>
            </div>
            
            {selectedRun.result && (
              <div style={{ marginTop: 16 }}>
                <strong>Результат:</strong>
                <pre style={{ 
                  background: '#f5f5f5', 
                  padding: 12, 
                  borderRadius: 4, 
                  marginTop: 8,
                  fontSize: '12px',
                  maxHeight: '200px',
                  overflow: 'auto'
                }}>
                  {JSON.stringify(JSON.parse(selectedRun.result), null, 2)}
                </pre>
              </div>
            )}
          </div>
        )}
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
    </div>
  )
}

export default RunsPage