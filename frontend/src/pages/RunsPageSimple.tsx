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
  message
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
} from '@ant-design/icons'
import { useState, useEffect } from 'react'
import RunCreationForm from '../components/RunCreationForm'
import type { RunCreationFormData } from '../types/run-creation'
import { apiClient, getErrorMessage } from '../services/api'
import type { Run } from '../services/api'

const { Title, Paragraph } = Typography

const RunsPage: React.FC = () => {
  const [selectedRowKeys, setSelectedRowKeys] = useState<React.Key[]>([])
  const [modalVisible, setModalVisible] = useState(false)
  const [selectedRun, setSelectedRun] = useState<Run | null>(null)
  const [createRunModalVisible, setCreateRunModalVisible] = useState(false)
  const [runs, setRuns] = useState<Run[]>([])
  const [loading, setLoading] = useState(true)
  const [pagination, setPagination] = useState({
    current: 1,
    pageSize: 20,
    total: 0
  })

  // Загрузка запусков с сервера
  const fetchRuns = async (page: number = 1, limit: number = 20) => {
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

  // Загружаем данные при монтировании компонента
  useEffect(() => {
    fetchRuns()
  }, [])

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

      message.success(`Запуск "${formData.name}" создан успешно!`)
      setCreateRunModalVisible(false)
      fetchRuns(pagination.current, pagination.pageSize)
    } catch (error) {
      message.error(`Ошибка создания запуска: ${getErrorMessage(error)}`)
    }
  }

  const columns = [
    {
      title: 'ID и название',
      dataIndex: 'name',
      key: 'name',
      render: (text: string, record: Run) => {
        let config: any = {}
        try {
          config = JSON.parse(record.config || '{}')
        } catch (e) {
          config = {}
        }
        
        return (
          <div>
            <div style={{ fontSize: '12px', color: '#8c8c8c', fontFamily: 'monospace', marginBottom: '4px' }}>
              run-{record.id}
            </div>
            <div style={{ fontWeight: 500 }}>{text}</div>
            {record.description && (
              <div style={{ fontSize: '14px', color: '#8c8c8c', marginTop: '4px' }}>
                {record.description}
              </div>
            )}
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginTop: '8px' }}>
              {config.workloadType && <Tag color="blue">{config.workloadType.toUpperCase()}</Tag>}
              {config.databaseType && <Tag color="green">{config.databaseType}</Tag>}
            </div>
          </div>
        )
      }
    },
    {
      title: 'Статус',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => (
        <div>
          <Tag color={getStatusColor(status)} icon={getStatusIcon(status)}>
            {getStatusText(status)}
          </Tag>
        </div>
      )
    },
    {
      title: 'Время',
      key: 'time',
      render: (record: Run) => {
        const createdAt = new Date(record.created_at).toLocaleString('ru-RU')
        const updatedAt = new Date(record.updated_at).toLocaleString('ru-RU')
        
        return (
          <div>
            <div style={{ fontSize: '14px' }}>Создан: {createdAt}</div>
            <div style={{ fontSize: '12px', color: '#8c8c8c', marginTop: '4px' }}>
              <ClockCircleOutlined style={{ marginRight: '4px' }} />
              Обновлен: {updatedAt}
            </div>
          </div>
        )
      }
    },
    {
      title: 'Действия',
      key: 'actions',
      render: (record: Run) => (
        <Space>
          <Button 
            type="text" 
            icon={<EyeOutlined />}
            onClick={() => handleRunAction('view', record)}
          />
          <Button 
            type="text" 
            icon={<DeleteOutlined />}
            danger
            onClick={() => handleRunAction('delete', record)}
          />
        </Space>
      )
    }
  ]

  const rowSelection = {
    selectedRowKeys,
    onChange: setSelectedRowKeys,
  }

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
      <div style={{ marginBottom: '16px', flexShrink: 0 }}>
        <Title level={2} style={{ marginBottom: '8px' }}>Запуски</Title>
        <Paragraph style={{ color: '#666', fontSize: '14px', margin: 0 }}>
          Управление и мониторинг тестовых запусков
        </Paragraph>
      </div>

      {/* Статистика */}
      <Row gutter={[12, 12]} style={{ marginBottom: '16px', flexShrink: 0 }}>
        <Col xs={24} sm={6}>
          <Card size="small" style={{ textAlign: 'center' }}>
            <Statistic
              title="Всего запусков"
              value={pagination.total}
              prefix={<PlayCircleOutlined />}
              valueStyle={{ fontSize: '20px' }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={6}>
          <Card size="small" style={{ textAlign: 'center' }}>
            <Statistic
              title="Завершено"
              value={runs.filter(r => r.status === 'completed').length}
              valueStyle={{ color: '#52c41a', fontSize: '20px' }}
              prefix={<CheckCircleOutlined />}
            />
          </Card>
        </Col>
        <Col xs={24} sm={6}>
          <Card size="small" style={{ textAlign: 'center' }}>
            <Statistic
              title="Ошибки"
              value={runs.filter(r => r.status === 'failed').length}
              valueStyle={{ color: '#ff4d4f', fontSize: '20px' }}
              prefix={<ExclamationCircleOutlined />}
            />
          </Card>
        </Col>
      </Row>

      {/* Таблица запусков */}
      <Card style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden', minHeight: 0 }}>
        <div style={{ marginBottom: '16px', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
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
              onClick={() => fetchRuns(pagination.current, pagination.pageSize)}
            >
              Обновить
            </Button>
          </Space>
        </div>

        <div style={{ flex: 1, overflow: 'hidden' }}>
          <Table
            rowSelection={rowSelection}
            columns={columns}
            dataSource={runs}
            rowKey="id"
            size="small"
            loading={loading}
            pagination={{
              current: pagination.current,
              pageSize: pagination.pageSize,
              total: pagination.total,
              showSizeChanger: true,
              showQuickJumper: true,
              showTotal: (total, range) => 
                `${range[0]}-${range[1]} из ${total} запусков`,
              size: 'small',
              onChange: (page, pageSize) => {
                fetchRuns(page, pageSize)
              }
            }}
          />
        </div>
      </Card>

      {/* Модальное окно деталей */}
      <Modal
        title="Детали запуска"
        open={modalVisible}
        onCancel={() => setModalVisible(false)}
        footer={null}
        width={800}
      >
        {selectedRun && (
          <div>
            <Title level={4}>{selectedRun.name}</Title>
            <p><strong>Описание:</strong> {selectedRun.description}</p>
            <p><strong>Статус:</strong> <Tag color={getStatusColor(selectedRun.status)}>{getStatusText(selectedRun.status)}</Tag></p>
            <p><strong>Создан:</strong> {new Date(selectedRun.created_at).toLocaleString('ru-RU')}</p>
            <p><strong>Обновлен:</strong> {new Date(selectedRun.updated_at).toLocaleString('ru-RU')}</p>
            <div>
              <strong>Конфигурация:</strong>
              <pre style={{ background: '#f5f5f5', padding: '10px', borderRadius: '4px', marginTop: '8px' }}>
                {JSON.stringify(JSON.parse(selectedRun.config || '{}'), null, 2)}
              </pre>
            </div>
          </div>
        )}
      </Modal>

      {/* Модальное окно создания запуска */}
      <Modal
        title="Создание нового запуска"
        open={createRunModalVisible}
        onCancel={() => setCreateRunModalVisible(false)}
        footer={null}
        width={1200}
        style={{ top: 20 }}
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
