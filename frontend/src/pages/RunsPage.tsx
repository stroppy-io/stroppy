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
  Pagination,
  Progress,
  Descriptions,
  Divider
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
  BugOutlined,
  CalendarOutlined
} from '@ant-design/icons'
import React, { useState, useEffect, useMemo, useCallback } from 'react'
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

interface SortState {
  field: string;
  order: 'asc' | 'desc';
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

  // Состояние сортировки
  const [sortState, setSortState] = useState<SortState>({
    field: 'created_at',
    order: 'desc'
  })

  // Обычная таблица без виртуализации

  // Загрузка запусков с сервера с фильтрами и сортировкой
  const fetchRuns = async (
    page: number = 1, 
    limit: number = 20,
    searchText?: string,
    status?: string,
    dateFrom?: string,
    dateTo?: string,
    sortBy?: string,
    sortOrder?: string
  ) => {
    try {
      setLoading(true)
      const response = await apiClient.getRuns(page, limit, searchText, status, dateFrom, dateTo, sortBy, sortOrder)
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

  // Вспомогательная функция для загрузки данных с текущими фильтрами и сортировкой
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
      dateTo,
      sortState.field,
      sortState.order
    );
  };

  // Функция для применения фильтров
  const applyFilters = useCallback((filtersToApply: FilterState) => {
    const currentPageSize = pagination.pageSize;
    
    let dateFrom, dateTo;
    if (filtersToApply.dateRange && filtersToApply.dateRange[0] && filtersToApply.dateRange[1]) {
      dateFrom = filtersToApply.dateRange[0].format('YYYY-MM-DD');
      dateTo = filtersToApply.dateRange[1].format('YYYY-MM-DD');
    }

    fetchRuns(
      1,
      currentPageSize,
      filtersToApply.searchText || undefined,
      filtersToApply.status || undefined,
      dateFrom,
      dateTo,
      sortState.field,
      sortState.order
    );
  }, [pagination.pageSize, sortState.field, sortState.order]);

  // Загружаем данные при монтировании компонента
  useEffect(() => {
    fetchRunsWithCurrentFilters(1);
  }, []);

  // Debounce для поиска
  useEffect(() => {
    if (filters.searchText !== undefined) {
      const timeoutId = setTimeout(() => {
        applyFilters(filters);
      }, 500);
      
      return () => clearTimeout(timeoutId);
    }
  }, [filters.searchText, applyFilters, filters]);

  // Функции для работы с фильтрами
  const handleFilterChange = (key: keyof FilterState, value: any) => {
    const newFilters = { ...filters, [key]: value };
    setFilters(newFilters);
    
    // Сбрасываем пагинацию
    setPagination(prev => ({ ...prev, current: 1 }));
    
    // Для поиска debounce обрабатывается в useEffect, для остальных фильтров - немедленно
    if (key !== 'searchText') {
      applyFilters(newFilters);
    }
  };

  const clearFilters = () => {
    setFilters({
      searchText: '',
      status: '',
      dateRange: null
    });
    setPagination(prev => ({ ...prev, current: 1 }));
    
    // Загружаем данные без фильтров
    fetchRuns(1, pagination.pageSize, undefined, undefined, undefined, undefined, sortState.field, sortState.order);
  };

  const hasActiveFilters = filters.searchText || filters.status || filters.dateRange;

  // Обработка сортировки
  const handleSort = (field: string, order: 'asc' | 'desc') => {
    setSortState({ field, order });
    setPagination(prev => ({ ...prev, current: 1 }));
    
    // Загружаем данные с новой сортировкой немедленно
    const currentPageSize = pagination.pageSize;
    
    let dateFrom, dateTo;
    if (filters.dateRange && filters.dateRange[0] && filters.dateRange[1]) {
      dateFrom = filters.dateRange[0].format('YYYY-MM-DD');
      dateTo = filters.dateRange[1].format('YYYY-MM-DD');
    }

    fetchRuns(
      1,
      currentPageSize,
      filters.searchText || undefined,
      filters.status || undefined,
      dateFrom,
      dateTo,
      field,
      order
    );
  };

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
      sorter: true,
      sortOrder: sortState.field === 'id' ? (sortState.order === 'asc' ? 'ascend' : 'descend') : null,
      onHeaderCell: () => ({
        onClick: () => {
          const newOrder = sortState.field === 'id' && sortState.order === 'asc' ? 'desc' : 'asc';
          handleSort('id', newOrder);
        }
      }),
      render: (id) => <Text code>#{id}</Text>
    },
    {
      title: 'Название',
      dataIndex: 'name',
      key: 'name',
      sorter: true,
      sortOrder: sortState.field === 'name' ? (sortState.order === 'asc' ? 'ascend' : 'descend') : null,
      onHeaderCell: () => ({
        onClick: () => {
          const newOrder = sortState.field === 'name' && sortState.order === 'asc' ? 'desc' : 'asc';
          handleSort('name', newOrder);
        }
      }),
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
      sorter: true,
      sortOrder: sortState.field === 'status' ? (sortState.order === 'asc' ? 'ascend' : 'descend') : null,
      onHeaderCell: () => ({
        onClick: () => {
          const newOrder = sortState.field === 'status' && sortState.order === 'asc' ? 'desc' : 'asc';
          handleSort('status', newOrder);
        }
      }),
      render: (status) => (
        <Tag icon={getStatusIcon(status)} color={getStatusColor(status)}>
          {getStatusText(status)}
        </Tag>
      )
    },
    {
      title: 'Создан',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 150,
      sorter: true,
      sortOrder: sortState.field === 'created_at' ? (sortState.order === 'asc' ? 'ascend' : 'descend') : null,
      onHeaderCell: () => ({
        onClick: () => {
          const newOrder = sortState.field === 'created_at' && sortState.order === 'asc' ? 'desc' : 'asc';
          handleSort('created_at', newOrder);
        }
      }),
      render: (date) => (
        <Tooltip title={dayjs(date).format('DD.MM.YYYY HH:mm:ss')}>
          <Text>{dayjs(date).format('DD.MM HH:mm')}</Text>
        </Tooltip>
      )
    },
    {
      title: 'TPS Avg',
      dataIndex: 'tps_metrics',
      key: 'tps_average',
      width: 100,
      sorter: true,
      sortOrder: sortState.field === 'tps_avg' ? (sortState.order === 'asc' ? 'ascend' : 'descend') : null,
      onHeaderCell: () => ({
        onClick: () => {
          const newOrder = sortState.field === 'tps_avg' && sortState.order === 'asc' ? 'desc' : 'asc';
          handleSort('tps_avg', newOrder);
        }
      }),
      render: (tpsMetrics) => (
        tpsMetrics?.average ? (
          <Tag color="blue">{tpsMetrics.average.toFixed(1)}</Tag>
        ) : (
          <Text type="secondary">-</Text>
        )
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

// ============================================================================
// КОД ВИРТУАЛИЗАЦИИ ТАБЛИЦЫ (для будущего использования)
// ============================================================================
// 
// Для включения виртуализации:
// 1. Установить: npm install @tanstack/react-virtual
// 2. Добавить импорт: import { useVirtualizer } from '@tanstack/react-virtual'
// 3. Заменить обычную таблицу на VirtualizedTable
// 4. Убрать импорт Table из antd
//
// ============================================================================

/*
// Виртуализированная таблица
const VirtualizedTable = () => {
  const parentRef = React.useRef<HTMLDivElement>(null);
  
  const virtualizer = useVirtualizer({
    count: runs.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 50, // Высота строки
    overscan: 10, // Количество дополнительных строк для рендеринга
  });

  // Функция для рендеринга заголовка с сортировкой
  const renderSortableHeader = (title: string, field: string, width: string) => {
    const isActive = sortState.field === field;
    const isAsc = isActive && sortState.order === 'asc';
    const isDesc = isActive && sortState.order === 'desc';
    
    return (
      <div
        style={{
          width,
          padding: '8px 16px',
          fontWeight: 'bold',
          backgroundColor: '#fafafa',
          borderBottom: '1px solid #f0f0f0',
          cursor: 'pointer',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          userSelect: 'none',
        }}
        onClick={() => {
          const newOrder = isActive && sortState.order === 'asc' ? 'desc' : 'asc';
          handleSort(field, newOrder);
        }}
      >
        <span>{title}</span>
        <div style={{ display: 'flex', flexDirection: 'column', fontSize: '10px' }}>
          <span style={{ color: isAsc ? '#1890ff' : '#d9d9d9' }}>▲</span>
          <span style={{ color: isDesc ? '#1890ff' : '#d9d9d9' }}>▼</span>
        </div>
      </div>
    );
  };

  return (
    <div style={{ height: 'calc(100vh - 450px)', display: 'flex', flexDirection: 'column' }}>
      <div style={{ display: 'flex', borderBottom: '2px solid #f0f0f0' }}>
        {renderSortableHeader('ID', 'id', '80px')}
        {renderSortableHeader('Название', 'name', 'flex: 1')}
        {renderSortableHeader('Статус', 'status', '120px')}
        {renderSortableHeader('Создан', 'created_at', '150px')}
        {renderSortableHeader('TPS Avg', 'tps_avg', '100px')}
        <div style={{ width: '120px', padding: '8px 16px', fontWeight: 'bold', backgroundColor: '#fafafa', borderBottom: '1px solid #f0f0f0' }}>
          Действия
        </div>
      </div>
      
      <div
        ref={parentRef}
        style={{
          flex: 1,
          overflow: 'auto',
        }}
      >
        <div
          style={{
            height: `${virtualizer.getTotalSize()}px`,
            width: '100%',
            position: 'relative',
          }}
        >
          {virtualizer.getVirtualItems().map((virtualItem) => {
            const run = runs[virtualItem.index];
            return (
              <div
                key={virtualItem.key}
                style={{
                  position: 'absolute',
                  top: 0,
                  left: 0,
                  width: '100%',
                  height: `${virtualItem.size}px`,
                  transform: `translateY(${virtualItem.start}px)`,
                  borderBottom: '1px solid #f0f0f0',
                  display: 'flex',
                  alignItems: 'center',
                  padding: '8px 16px',
                  backgroundColor: selectedRowKeys.includes(run.id) ? '#e6f7ff' : 'white',
                  cursor: 'pointer',
                }}
                onClick={() => {
                  const newKeys = selectedRowKeys.includes(run.id)
                    ? selectedRowKeys.filter(key => key !== run.id)
                    : [...selectedRowKeys, run.id];
                  setSelectedRowKeys(newKeys);
                }}
              >
                <div style={{ width: '80px' }}>
                  <Text code>#{run.id}</Text>
                </div>
                <div style={{ flex: 1, marginLeft: '16px' }}>
                  <Text strong>{run.name}</Text>
                  {run.description && (
                    <div>
                      <Text type="secondary" style={{ fontSize: '12px' }}>
                        {run.description.length > 50 
                          ? `${run.description.substring(0, 50)}...` 
                          : run.description}
                      </Text>
                    </div>
                  )}
                </div>
                <div style={{ width: '120px', marginLeft: '16px' }}>
                  <Tag icon={getStatusIcon(run.status)} color={getStatusColor(run.status)}>
                    {getStatusText(run.status)}
                  </Tag>
                </div>
                <div style={{ width: '150px', marginLeft: '16px' }}>
                  <Tooltip title={dayjs(run.created_at).format('DD.MM.YYYY HH:mm:ss')}>
                    <Text>{dayjs(run.created_at).format('DD.MM HH:mm')}</Text>
                  </Tooltip>
                </div>
                <div style={{ width: '100px', marginLeft: '16px' }}>
                  {run.tps_metrics?.average ? (
                    <Tag color="blue">{run.tps_metrics.average.toFixed(1)}</Tag>
                  ) : (
                    <Text type="secondary">-</Text>
                  )}
                </div>
                <div style={{ width: '120px', marginLeft: '16px' }}>
                  <Space size="small">
                    <Button
                      type="text"
                      icon={<EyeOutlined />}
                      size="small"
                      onClick={(e) => {
                        e.stopPropagation();
                        handleRunAction('view', run);
                      }}
                      title="Просмотр"
                    />
                    <Button
                      type="text"
                      icon={<DeleteOutlined />}
                      size="small"
                      danger
                      onClick={(e) => {
                        e.stopPropagation();
                        handleRunAction('delete', run);
                      }}
                      title="Удалить"
                    />
                  </Space>
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
};

// Для использования виртуализации заменить:
// <Table {...tableProps} style={{ height: '100%' }} />
// на:
// <VirtualizedTable />
*/