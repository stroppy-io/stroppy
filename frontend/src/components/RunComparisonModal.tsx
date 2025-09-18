import React from 'react'
import { Modal, Row, Col, Card, Typography, Tag, Statistic } from 'antd'
import {
  DatabaseOutlined,
  ThunderboltOutlined,
  CloudServerOutlined,
  BugOutlined,
  SettingOutlined
} from '@ant-design/icons'

const { Title, Text } = Typography

interface Run {
  id: string
  runId: string
  name: string
  description?: string
  status: 'completed' | 'failed' // | 'running' | 'paused' - убрали лишние статусы
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
    signature: string
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
  runs: [Run, Run]
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
      // case 'running': return 'processing'
      case 'completed': return 'success'
      case 'failed': return 'error'
      // case 'paused': return 'warning'
      default: return 'default'
    }
  }

  const getStatusText = (status: string) => {
    switch (status) {
      // case 'running': return 'Выполняется'
      case 'completed': return 'Завершен'
      case 'failed': return 'Ошибка'
      // case 'paused': return 'Приостановлен'
      default: return 'Неизвестно'
    }
  }

  const renderComparisonField = (label: string, value1: any, value2: any, highlight = false) => {
    const isDifferent = JSON.stringify(value1) !== JSON.stringify(value2)
    
    return (
      <Row gutter={16} className="mb-2">
        <Col span={6} className="font-medium text-gray-600">
          {label}:
        </Col>
        <Col span={9}>
          <Text className={isDifferent && highlight ? 'text-orange-600 font-medium' : ''}>
            {typeof value1 === 'object' ? JSON.stringify(value1) : value1}
          </Text>
        </Col>
        <Col span={9}>
          <Text className={isDifferent && highlight ? 'text-orange-600 font-medium' : ''}>
            {typeof value2 === 'object' ? JSON.stringify(value2) : value2}
          </Text>
        </Col>
      </Row>
    )
  }

  const renderWorkloadProperties = (run: Run) => {
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

  return (
    <Modal
      title={
        <div className="flex items-center">
          <SettingOutlined className="mr-2" />
          Сравнение запусков
        </div>
      }
      open={visible}
      onCancel={onClose}
      footer={null}
      width={1400}
      style={{ top: 20 }}
    >
      <div className="space-y-6">
        {/* Заголовки запусков */}
        <Row gutter={16}>
          <Col span={6}></Col>
          <Col span={9}>
            <Card size="small" className="text-center bg-blue-50">
              <Title level={4} className="mb-2">{run1.name}</Title>
              <Tag color={getStatusColor(run1.status)}>
                {getStatusText(run1.status)}
              </Tag>
            </Card>
          </Col>
          <Col span={9}>
            <Card size="small" className="text-center bg-green-50">
              <Title level={4} className="mb-2">{run2.name}</Title>
              <Tag color={getStatusColor(run2.status)}>
                {getStatusText(run2.status)}
              </Tag>
            </Card>
          </Col>
        </Row>

        {/* Основная информация */}
        <Card title={<><SettingOutlined className="mr-2" />Основная информация</>}>
          {renderComparisonField('Run ID', run1.runId, run2.runId, true)}
          {renderComparisonField('Описание', run1.description || 'Не указано', run2.description || 'Не указано')}
          {renderComparisonField('Время запуска', run1.startTime, run2.startTime)}
          {renderComparisonField('Длительность', run1.duration, run2.duration)}
          {renderComparisonField('Прогресс', `${run1.progress}%`, `${run2.progress}%`)}
        </Card>

        {/* Тип нагрузки */}
        <Card title={<><ThunderboltOutlined className="mr-2" />Нагрузка</>}>
          {renderComparisonField('Тип нагрузки', run1.workloadType.toUpperCase(), run2.workloadType.toUpperCase(), true)}
          {renderComparisonField('Количество раннеров', run1.workloadProperties.runners, run2.workloadProperties.runners, true)}
          {renderComparisonField('Продолжительность теста', run1.workloadProperties.duration || 'Не указано', run2.workloadProperties.duration || 'Не указано', true)}
          
          <Row gutter={16} className="mt-4">
            <Col span={6} className="font-medium text-gray-600">
              Специфичные свойства:
            </Col>
            <Col span={9}>
              {renderWorkloadProperties(run1)}
            </Col>
            <Col span={9}>
              {renderWorkloadProperties(run2)}
            </Col>
          </Row>
        </Card>

        {/* База данных */}
        <Card title={<><DatabaseOutlined className="mr-2" />База данных</>}>
          {renderComparisonField('Тип БД', run1.databaseType, run2.databaseType, true)}
          {renderComparisonField('Версия', run1.databaseVersion, run2.databaseVersion, true)}
          {renderComparisonField('Сборка', run1.databaseBuild || 'Не указано', run2.databaseBuild || 'Не указано')}
        </Card>

        {/* Конфигурация железа */}
        <Card title={<><CloudServerOutlined className="mr-2" />Конфигурация железа</>}>
          {renderComparisonField('Название конфигурации', run1.hardwareConfiguration.name, run2.hardwareConfiguration.name)}
          {renderComparisonField('Сигнатура', run1.hardwareConfiguration.signature, run2.hardwareConfiguration.signature, true)}
          {renderComparisonField('Процессор', `${run1.hardwareConfiguration.cpu.cores} ядер (${run1.hardwareConfiguration.cpu.model})`, `${run2.hardwareConfiguration.cpu.cores} ядер (${run2.hardwareConfiguration.cpu.model})`, true)}
          {renderComparisonField('Память', `${run1.hardwareConfiguration.memory.totalGB} GB`, `${run2.hardwareConfiguration.memory.totalGB} GB`, true)}
          {renderComparisonField('Накопитель', `${run1.hardwareConfiguration.storage.type.toUpperCase()} ${run1.hardwareConfiguration.storage.capacityGB} GB`, `${run2.hardwareConfiguration.storage.type.toUpperCase()} ${run2.hardwareConfiguration.storage.capacityGB} GB`, true)}
          {renderComparisonField('Количество узлов', run1.hardwareConfiguration.nodeCount, run2.hardwareConfiguration.nodeCount, true)}
        </Card>

        {/* Схема развертывания */}
        <Card title={<><CloudServerOutlined className="mr-2" />Схема развертывания</>}>
          {renderComparisonField('Тип развертывания', run1.deploymentLayout.type, run2.deploymentLayout.type, true)}
          {renderComparisonField('Сигнатура', run1.deploymentLayout.signature, run2.deploymentLayout.signature, true)}
          
          <Row gutter={16} className="mt-4">
            <Col span={6} className="font-medium text-gray-600">
              Конфигурация:
            </Col>
            <Col span={9}>
              <div className="space-y-1">
                {Object.entries(run1.deploymentLayout.configuration).map(([key, value]) => (
                  <div key={key}>
                    <Text type="secondary">{key}: </Text>
                    <Text>{String(value)}</Text>
                  </div>
                ))}
              </div>
            </Col>
            <Col span={9}>
              <div className="space-y-1">
                {Object.entries(run2.deploymentLayout.configuration).map(([key, value]) => (
                  <div key={key}>
                    <Text type="secondary">{key}: </Text>
                    <Text>{String(value)}</Text>
                  </div>
                ))}
              </div>
            </Col>
          </Row>
        </Card>

        {/* Немезисы */}
        <Card title={<><BugOutlined className="mr-2" />Немезисы</>}>
          {renderComparisonField('Сигнатура немезисов', run1.nemesisSignature.signature, run2.nemesisSignature.signature, true)}
          
          <Row gutter={16} className="mt-4">
            <Col span={6} className="font-medium text-gray-600">
              Активные немезисы:
            </Col>
            <Col span={9}>
              <div className="space-y-1">
                {run1.nemesisSignature.nemeses.length > 0 ? (
                  run1.nemesisSignature.nemeses
                    .filter(n => n.enabled)
                    .map((nemesis, index) => (
                      <Tag key={index} color="red">
                        {nemesis.type}
                      </Tag>
                    ))
                ) : (
                  <Text type="secondary">Немезисы отсутствуют</Text>
                )}
              </div>
            </Col>
            <Col span={9}>
              <div className="space-y-1">
                {run2.nemesisSignature.nemeses.length > 0 ? (
                  run2.nemesisSignature.nemeses
                    .filter(n => n.enabled)
                    .map((nemesis, index) => (
                      <Tag key={index} color="red">
                        {nemesis.type}
                      </Tag>
                    ))
                ) : (
                  <Text type="secondary">Немезисы отсутствуют</Text>
                )}
              </div>
            </Col>
          </Row>
        </Card>

        {/* Статистика различий */}
        <Card title="Сводка различий" className="bg-gray-50">
          <Row gutter={16}>
            <Col span={8}>
              <Statistic
                title="Различия в конфигурации железа"
                value={run1.hardwareConfiguration.signature === run2.hardwareConfiguration.signature ? 0 : 1}
                suffix="/ 1"
                valueStyle={{ color: run1.hardwareConfiguration.signature === run2.hardwareConfiguration.signature ? '#52c41a' : '#ff4d4f' }}
              />
            </Col>
            <Col span={8}>
              <Statistic
                title="Различия в типе нагрузки"
                value={run1.workloadType === run2.workloadType ? 0 : 1}
                suffix="/ 1"
                valueStyle={{ color: run1.workloadType === run2.workloadType ? '#52c41a' : '#ff4d4f' }}
              />
            </Col>
            <Col span={8}>
              <Statistic
                title="Различия в схеме развертывания"
                value={run1.deploymentLayout.signature === run2.deploymentLayout.signature ? 0 : 1}
                suffix="/ 1"
                valueStyle={{ color: run1.deploymentLayout.signature === run2.deploymentLayout.signature ? '#52c41a' : '#ff4d4f' }}
              />
            </Col>
          </Row>
        </Card>
      </div>
    </Modal>
  )
}

export default RunComparisonModal
