import React from 'react'
import { 
  Modal, 
  Row, 
  Col, 
  Card, 
  Typography, 
  Tag, 
  Statistic, 
  Divider, 
  Space, 
  Badge,
  Descriptions,
  Alert,
  Progress
} from 'antd'
import {
  DatabaseOutlined,
  ThunderboltOutlined,
  CloudServerOutlined,
  BugOutlined,
  SettingOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  InfoCircleOutlined,
  SwapOutlined,
  CalendarOutlined,
  ClockCircleOutlined,
  UserOutlined
} from '@ant-design/icons'

const { Title, Text } = Typography

interface ComparisonRun {
  id: string
  runId: string
  name: string
  description?: string
  status: 'completed' | 'failed'
  progress: number
  startTime: string
  duration: string
  
  workloadType: string
  workloadProperties: {
    runners: number
    duration?: string
    [key: string]: any
  }
  
  databaseType: string
  databaseVersion: string
  databaseBuild?: string
  
  hardwareConfiguration: {
    id: string
    name: string
    signature?: string
    cpu: { cores: number, model: string }
    memory: { totalGB: number }
    storage: { type: string, capacityGB: number }
    nodeCount: number
  }
  
  deploymentLayout: {
    type: string
    signature: string
    configuration: Record<string, any>
  }
  
  nemesisSignature: {
    signature: string
    nemeses: Array<{ type: string, enabled: boolean }>
  }
}

interface RunComparisonModalProps {
  visible: boolean
  onClose: () => void
  runs: [ComparisonRun, ComparisonRun]
}

const RunComparisonModal: React.FC<RunComparisonModalProps> = ({
  visible,
  onClose,
  runs
}) => {
  if (!runs || runs.length !== 2) {
    return null
  }

  const [run1, run2] = runs

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'completed': return 'success'
      case 'failed': return 'error'
      default: return 'default'
    }
  }

  const getStatusText = (status: string) => {
    switch (status) {
      case 'completed': return 'Завершен'
      case 'failed': return 'Ошибка'
      default: return 'Неизвестно'
    }
  }

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'completed': return <CheckCircleOutlined />
      case 'failed': return <CloseCircleOutlined />
      default: return <InfoCircleOutlined />
    }
  }

  // Подсчет различий
  const getDifferencesCount = () => {
    let count = 0
    
    // Основные поля
    if (run1.runId !== run2.runId) count++
    if (run1.description !== run2.description) count++
    if (run1.status !== run2.status) count++
    if (run1.duration !== run2.duration) count++
    
    // Нагрузка
    if (run1.workloadType !== run2.workloadType) count++
    if (run1.workloadProperties.runners !== run2.workloadProperties.runners) count++
    if (run1.workloadProperties.duration !== run2.workloadProperties.duration) count++
    
    // База данных
    if (run1.databaseType !== run2.databaseType) count++
    if (run1.databaseVersion !== run2.databaseVersion) count++
    if (run1.databaseBuild !== run2.databaseBuild) count++
    
    // Железо
    if (run1.hardwareConfiguration.signature !== run2.hardwareConfiguration.signature) count++
    if (run1.hardwareConfiguration.cpu.cores !== run2.hardwareConfiguration.cpu.cores) count++
    if (run1.hardwareConfiguration.memory.totalGB !== run2.hardwareConfiguration.memory.totalGB) count++
    if (run1.hardwareConfiguration.storage.capacityGB !== run2.hardwareConfiguration.storage.capacityGB) count++
    if (run1.hardwareConfiguration.nodeCount !== run2.hardwareConfiguration.nodeCount) count++
    
    // Развертывание
    if (run1.deploymentLayout.type !== run2.deploymentLayout.type) count++
    if (run1.deploymentLayout.signature !== run2.deploymentLayout.signature) count++
    
    // Немезисы
    if (run1.nemesisSignature.signature !== run2.nemesisSignature.signature) count++
    
    return count
  }

  const renderComparisonRow = (
    label: string, 
    value1: any, 
    value2: any, 
    icon?: React.ReactNode,
    highlight = false
  ) => {
    const isDifferent = JSON.stringify(value1) !== JSON.stringify(value2)
    const displayValue1 = typeof value1 === 'object' ? JSON.stringify(value1) : String(value1)
    const displayValue2 = typeof value2 === 'object' ? JSON.stringify(value2) : String(value2)
    
    return (
      <Descriptions.Item 
        label={
          <Space>
            {icon}
            <Text strong>{label}</Text>
            {isDifferent && highlight && <Badge status="warning" />}
          </Space>
        }
        span={3}
      >
        <Row gutter={[16, 8]}>
          <Col span={12}>
            <Card 
              size="small" 
              style={{ 
                backgroundColor: isDifferent && highlight ? '#fff7e6' : '#f6ffed',
                borderColor: isDifferent && highlight ? '#ffa940' : '#b7eb8f'
              }}
            >
              <Text 
                style={{ 
                  color: isDifferent && highlight ? '#fa8c16' : '#52c41a',
                  fontWeight: isDifferent && highlight ? 600 : 400
                }}
              >
                {displayValue1}
              </Text>
            </Card>
          </Col>
          <Col span={12}>
            <Card 
              size="small" 
              style={{ 
                backgroundColor: isDifferent && highlight ? '#fff7e6' : '#f6ffed',
                borderColor: isDifferent && highlight ? '#ffa940' : '#b7eb8f'
              }}
            >
              <Text 
                style={{ 
                  color: isDifferent && highlight ? '#fa8c16' : '#52c41a',
                  fontWeight: isDifferent && highlight ? 600 : 400
                }}
              >
                {displayValue2}
              </Text>
            </Card>
          </Col>
        </Row>
      </Descriptions.Item>
    )
  }

  const renderWorkloadProperties = (run: ComparisonRun) => {
    const props = run.workloadProperties
    const entries = Object.entries(props).filter(([key]) => key !== 'runners' && key !== 'duration')
    
    if (entries.length === 0) {
      return <Text type="secondary">Нет дополнительных свойств</Text>
    }

    return (
      <div>
        {entries.map(([key, value]) => (
          <div key={key} className="mb-1">
            <Text type="secondary">{key}: </Text>
            <Text>{Array.isArray(value) ? value.join(', ') : String(value)}</Text>
          </div>
        ))}
      </div>
    )
  }

  const differencesCount = getDifferencesCount()

  return (
    <Modal
      title={
        <Space align="center">
          <SwapOutlined />
          <Text strong style={{ fontSize: '18px' }}>Сравнение запусков</Text>
          <Badge count={differencesCount} style={{ backgroundColor: '#faad14' }} />
        </Space>
      }
      open={visible}
      onCancel={onClose}
      footer={null}
      width={1600}
      style={{ top: 20 }}
    >
      <div style={{ maxHeight: '80vh', overflowY: 'auto' }}>
        {/* Заголовки запусков */}
        <Row gutter={24} style={{ marginBottom: 24 }}>
          <Col span={12}>
            <Card 
              style={{ 
                textAlign: 'center',
                background: 'linear-gradient(135deg, #e6f7ff 0%, #bae7ff 100%)',
                border: '2px solid #40a9ff'
              }}
            >
              <Space direction="vertical" size="small">
                <Title level={4} style={{ margin: 0, color: '#096dd9' }}>
                  {run1.name}
                </Title>
                <Space>
                  <Tag 
                    icon={getStatusIcon(run1.status)} 
                    color={getStatusColor(run1.status)}
                    style={{ fontSize: '14px', padding: '4px 12px' }}
                  >
                    {getStatusText(run1.status)}
                  </Tag>
                  <Tag icon={<UserOutlined />} color="blue">ID: {run1.id}</Tag>
                </Space>
                <Progress 
                  percent={run1.progress} 
                  size="small" 
                  status={run1.status === 'completed' ? 'success' : run1.status === 'failed' ? 'exception' : 'active'}
                />
              </Space>
            </Card>
          </Col>
          <Col span={12}>
            <Card 
              style={{ 
                textAlign: 'center',
                background: 'linear-gradient(135deg, #f6ffed 0%, #d9f7be 100%)',
                border: '2px solid #73d13d'
              }}
            >
              <Space direction="vertical" size="small">
                <Title level={4} style={{ margin: 0, color: '#389e0d' }}>
                  {run2.name}
                </Title>
                <Space>
                  <Tag 
                    icon={getStatusIcon(run2.status)} 
                    color={getStatusColor(run2.status)}
                    style={{ fontSize: '14px', padding: '4px 12px' }}
                  >
                    {getStatusText(run2.status)}
                  </Tag>
                  <Tag icon={<UserOutlined />} color="green">ID: {run2.id}</Tag>
                </Space>
                <Progress 
                  percent={run2.progress} 
                  size="small" 
                  status={run2.status === 'completed' ? 'success' : run2.status === 'failed' ? 'exception' : 'active'}
                />
              </Space>
            </Card>
          </Col>
        </Row>

        {/* Статистика различий */}
        {differencesCount > 0 && (
          <Alert
            message={`Обнаружено ${differencesCount} различий между запусками`}
            description="Различающиеся поля выделены оранжевым цветом"
            type="warning"
            showIcon
            style={{ marginBottom: 24 }}
          />
        )}

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
          <Descriptions column={1} bordered size="small">
            {renderComparisonRow('Run ID', run1.runId, run2.runId, <InfoCircleOutlined />, true)}
            {renderComparisonRow('Описание', run1.description || 'Не указано', run2.description || 'Не указано')}
            {renderComparisonRow('Время запуска', run1.startTime, run2.startTime, <CalendarOutlined />)}
            {renderComparisonRow('Длительность', run1.duration, run2.duration, <ClockCircleOutlined />, true)}
            {renderComparisonRow('Прогресс', `${run1.progress}%`, `${run2.progress}%`)}
          </Descriptions>
        </Card>

        {/* Нагрузка */}
        <Card 
          title={
            <Space>
              <ThunderboltOutlined />
              <Text strong>Конфигурация нагрузки</Text>
            </Space>
          }
          style={{ marginBottom: 16 }}
        >
          <Descriptions column={1} bordered size="small">
            {renderComparisonRow('Тип нагрузки', run1.workloadType.toUpperCase(), run2.workloadType.toUpperCase(), <ThunderboltOutlined />, true)}
            {renderComparisonRow('Количество раннеров', run1.workloadProperties.runners, run2.workloadProperties.runners, undefined, true)}
            {renderComparisonRow('Продолжительность теста', run1.workloadProperties.duration || 'Не указано', run2.workloadProperties.duration || 'Не указано', undefined, true)}
          </Descriptions>
          
          {/* Дополнительные свойства нагрузки */}
          <Divider orientation="left" plain>Дополнительные свойства</Divider>
          <Row gutter={16}>
            <Col span={12}>
              <Card size="small" title="Запуск 1">
                {renderWorkloadProperties(run1)}
              </Card>
            </Col>
            <Col span={12}>
              <Card size="small" title="Запуск 2">
                {renderWorkloadProperties(run2)}
              </Card>
            </Col>
          </Row>
        </Card>

        {/* База данных */}
        <Card 
          title={
            <Space>
              <DatabaseOutlined />
              <Text strong>Конфигурация базы данных</Text>
            </Space>
          }
          style={{ marginBottom: 16 }}
        >
          <Descriptions column={1} bordered size="small">
            {renderComparisonRow('Тип СУБД', run1.databaseType.toUpperCase(), run2.databaseType.toUpperCase(), <DatabaseOutlined />, true)}
            {renderComparisonRow('Версия', run1.databaseVersion, run2.databaseVersion, undefined, true)}
            {renderComparisonRow('Сборка', run1.databaseBuild || 'Не указано', run2.databaseBuild || 'Не указано')}
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
          <Descriptions column={1} bordered size="small">
            {renderComparisonRow('Название конфигурации', run1.hardwareConfiguration.name, run2.hardwareConfiguration.name)}
            {renderComparisonRow('Сигнатура', run1.hardwareConfiguration.signature || 'Не указано', run2.hardwareConfiguration.signature || 'Не указано', undefined, true)}
            {renderComparisonRow('Процессор', `${run1.hardwareConfiguration.cpu.cores} ядер`, `${run2.hardwareConfiguration.cpu.cores} ядер`, undefined, true)}
            {renderComparisonRow('Модель CPU', run1.hardwareConfiguration.cpu.model, run2.hardwareConfiguration.cpu.model)}
            {renderComparisonRow('Память', `${run1.hardwareConfiguration.memory.totalGB} GB`, `${run2.hardwareConfiguration.memory.totalGB} GB`, undefined, true)}
            {renderComparisonRow('Тип накопителя', run1.hardwareConfiguration.storage.type.toUpperCase(), run2.hardwareConfiguration.storage.type.toUpperCase())}
            {renderComparisonRow('Объем накопителя', `${run1.hardwareConfiguration.storage.capacityGB} GB`, `${run2.hardwareConfiguration.storage.capacityGB} GB`, undefined, true)}
            {renderComparisonRow('Количество узлов', run1.hardwareConfiguration.nodeCount, run2.hardwareConfiguration.nodeCount, undefined, true)}
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
          <Descriptions column={1} bordered size="small">
            {renderComparisonRow('Тип развертывания', run1.deploymentLayout.type, run2.deploymentLayout.type, undefined, true)}
            {renderComparisonRow('Сигнатура', run1.deploymentLayout.signature, run2.deploymentLayout.signature, undefined, true)}
          </Descriptions>
          
          {/* Конфигурация развертывания */}
          <Divider orientation="left" plain>Параметры развертывания</Divider>
          <Row gutter={16}>
            <Col span={12}>
              <Card size="small" title="Запуск 1">
                {Object.keys(run1.deploymentLayout.configuration).length > 0 ? (
                  Object.entries(run1.deploymentLayout.configuration).map(([key, value]) => (
                    <div key={key} style={{ marginBottom: 8 }}>
                      <Text type="secondary">{key}: </Text>
                      <Tag color="blue">{String(value)}</Tag>
                    </div>
                  ))
                ) : (
                  <Text type="secondary">Дополнительные параметры отсутствуют</Text>
                )}
              </Card>
            </Col>
            <Col span={12}>
              <Card size="small" title="Запуск 2">
                {Object.keys(run2.deploymentLayout.configuration).length > 0 ? (
                  Object.entries(run2.deploymentLayout.configuration).map(([key, value]) => (
                    <div key={key} style={{ marginBottom: 8 }}>
                      <Text type="secondary">{key}: </Text>
                      <Tag color="green">{String(value)}</Tag>
                    </div>
                  ))
                ) : (
                  <Text type="secondary">Дополнительные параметры отсутствуют</Text>
                )}
              </Card>
            </Col>
          </Row>
        </Card>

        {/* Немезисы */}
        <Card 
          title={
            <Space>
              <BugOutlined />
              <Text strong>Конфигурация немезисов</Text>
            </Space>
          }
        >
          <Descriptions column={1} bordered size="small">
            {renderComparisonRow('Сигнатура немезисов', run1.nemesisSignature.signature, run2.nemesisSignature.signature, <BugOutlined />, true)}
          </Descriptions>
          
          {/* Активные немезисы */}
          <Divider orientation="left" plain>Активные немезисы</Divider>
          <Row gutter={16}>
            <Col span={12}>
              <Card size="small" title="Запуск 1">
                <Space wrap>
                  {run1.nemesisSignature.nemeses.length > 0 ? (
                    run1.nemesisSignature.nemeses
                      .filter(n => n.enabled)
                      .map((nemesis, index) => (
                        <Tag key={index} color="red" icon={<BugOutlined />}>
                          {nemesis.type}
                        </Tag>
                      ))
                  ) : (
                    <Text type="secondary">Немезисы отсутствуют</Text>
                  )}
                </Space>
              </Card>
            </Col>
            <Col span={12}>
              <Card size="small" title="Запуск 2">
                <Space wrap>
                  {run2.nemesisSignature.nemeses.length > 0 ? (
                    run2.nemesisSignature.nemeses
                      .filter(n => n.enabled)
                      .map((nemesis, index) => (
                        <Tag key={index} color="red" icon={<BugOutlined />}>
                          {nemesis.type}
                        </Tag>
                      ))
                  ) : (
                    <Text type="secondary">Немезисы отсутствуют</Text>
                  )}
                </Space>
              </Card>
            </Col>
          </Row>
        </Card>

        {/* Итоговая статистика */}
        <Card 
          title="Сводка различий" 
          style={{ 
            marginTop: 16,
            background: 'linear-gradient(135deg, #f0f2f5 0%, #e6f7ff 100%)'
          }}
        >
          <Row gutter={16}>
            <Col span={6}>
              <Statistic
                title="Всего различий"
                value={differencesCount}
                prefix={<SwapOutlined />}
                valueStyle={{ color: differencesCount > 0 ? '#fa8c16' : '#52c41a' }}
              />
            </Col>
            <Col span={6}>
              <Statistic
                title="Железо"
                value={run1.hardwareConfiguration.signature === run2.hardwareConfiguration.signature ? 'Идентично' : 'Различается'}
                valueStyle={{ 
                  color: run1.hardwareConfiguration.signature === run2.hardwareConfiguration.signature ? '#52c41a' : '#ff4d4f',
                  fontSize: '16px'
                }}
              />
            </Col>
            <Col span={6}>
              <Statistic
                title="Нагрузка"
                value={run1.workloadType === run2.workloadType ? 'Идентична' : 'Различается'}
                valueStyle={{ 
                  color: run1.workloadType === run2.workloadType ? '#52c41a' : '#ff4d4f',
                  fontSize: '16px'
                }}
              />
            </Col>
            <Col span={6}>
              <Statistic
                title="Развертывание"
                value={run1.deploymentLayout.signature === run2.deploymentLayout.signature ? 'Идентично' : 'Различается'}
                valueStyle={{ 
                  color: run1.deploymentLayout.signature === run2.deploymentLayout.signature ? '#52c41a' : '#ff4d4f',
                  fontSize: '16px'
                }}
              />
            </Col>
          </Row>
        </Card>
      </div>
    </Modal>
  )
}

export default RunComparisonModal
