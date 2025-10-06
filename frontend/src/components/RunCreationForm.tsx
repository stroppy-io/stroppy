import React, { useState, useEffect } from 'react'
import {
  Form,
  Input,
  Select,
  Button,
  Card,
  Row,
  Col,
  Space,
  Divider,
  InputNumber,
  Switch,
  Typography,
  Collapse,
  Tag,
  Tooltip,
  Alert,
  message
} from 'antd'
import {
  PlusOutlined,
  DeleteOutlined,
  SettingOutlined,
  ThunderboltOutlined,
  DatabaseOutlined,
  CloudServerOutlined,
  BugOutlined,
  CheckCircleOutlined,
  ExclamationCircleOutlined
} from '@ant-design/icons'
import type {
  RunCreationFormData,
  WorkloadType,
  DatabaseType,
  DeploymentLayoutType,
  HardwareConfiguration,
  NemesisConfig,
  WorkloadProperties,
  TPSMetrics
} from '../types/run-creation'
import {
  WORKLOAD_TYPES,
  DATABASE_TYPES,
  NEMESIS_TYPES,
  generateHardwareSignature,
  generateDeploymentSignature,
  generateNemesisSignature
} from '../types/run-creation'
import { useTranslation } from '../hooks/useTranslation'

const { Title } = Typography
const { Panel } = Collapse
const { TextArea } = Input
const { Option } = Select

interface RunCreationFormProps {
  onSubmit: (data: RunCreationFormData) => void
  onCancel: () => void
  initialData?: Partial<RunCreationFormData>
}

const RunCreationForm: React.FC<RunCreationFormProps> = ({
  onSubmit,
  onCancel,
  initialData
}) => {
  const [form] = Form.useForm()
  const [selectedWorkloadType, setSelectedWorkloadType] = useState<WorkloadType>('pgbench')
  const { t } = useTranslation('configurator')
  const [selectedDatabaseType, setSelectedDatabaseType] = useState<DatabaseType>('postgres')
  const [hardwareConfigs, setHardwareConfigs] = useState<HardwareConfiguration[]>([])
  const [selectedHardwareConfig, setSelectedHardwareConfig] = useState<string>()
  const [nemeses, setNemeses] = useState<NemesisConfig[]>([])
  const [deploymentLayoutType, setDeploymentLayoutType] = useState<DeploymentLayoutType>('single-node')
  const [tpsMetrics, setTpsMetrics] = useState<TPSMetrics>({})

  // Инициализация формы
  useEffect(() => {
    if (initialData) {
      form.setFieldsValue(initialData)
      if (initialData.workloadType) setSelectedWorkloadType(initialData.workloadType)
      if (initialData.databaseType) setSelectedDatabaseType(initialData.databaseType)
      if (initialData.hardwareConfiguration) {
        setHardwareConfigs([initialData.hardwareConfiguration])
        setSelectedHardwareConfig(initialData.hardwareConfiguration.id)
      }
      if (initialData.nemesisSignature?.nemeses) {
        setNemeses(initialData.nemesisSignature.nemeses)
      }
      if (initialData.deploymentLayout?.type) {
        setDeploymentLayoutType(initialData.deploymentLayout.type)
      }
      if (initialData.tpsMetrics) {
        setTpsMetrics(initialData.tpsMetrics)
      }
    }
  }, [initialData, form])

  // Получение доступных версий БД
  const getAvailableVersions = () => {
    const dbType = DATABASE_TYPES.find(db => db.value === selectedDatabaseType)
    return dbType?.availableVersions || []
  }

  // Получение доступных схем развертывания
  const getAvailableDeploymentLayouts = () => {
    const dbType = DATABASE_TYPES.find(db => db.value === selectedDatabaseType)
    return dbType?.availableDeploymentLayouts || ['single-node']
  }

  // Добавление новой конфигурации железа
  const addHardwareConfig = () => {
    const newConfig: HardwareConfiguration = {
      id: `hw-${Date.now()}`,
      name: `Конфигурация ${hardwareConfigs.length + 1}`,
      cpu: { cores: 4, model: 'Intel Xeon' },
      memory: { totalGB: 16 },
      storage: { type: 'ssd', capacityGB: 500 },
      network: {},
      nodeCount: 1
    }
    setHardwareConfigs([...hardwareConfigs, newConfig])
  }

  // Обновление конфигурации железа
  const updateHardwareConfig = (id: string, updates: Partial<HardwareConfiguration>) => {
    setHardwareConfigs(configs =>
      configs.map(config =>
        config.id === id ? { ...config, ...updates } : config
      )
    )
  }

  // Удаление конфигурации железа
  const removeHardwareConfig = (id: string) => {
    setHardwareConfigs(configs => configs.filter(config => config.id !== id))
    if (selectedHardwareConfig === id) {
      setSelectedHardwareConfig(undefined)
    }
  }

  // Добавление немезиса
  const addNemesis = () => {
    const newNemesis: NemesisConfig = {
      type: 'node-failure',
      enabled: true,
      parameters: {}
    }
    setNemeses([...nemeses, newNemesis])
  }

  // Обновление немезиса
  const updateNemesis = (index: number, updates: Partial<NemesisConfig>) => {
    setNemeses(current =>
      current.map((nemesis, i) =>
        i === index ? { ...nemesis, ...updates } : nemesis
      )
    )
  }

  // Удаление немезиса
  const removeNemesis = (index: number) => {
    setNemeses(current => current.filter((_, i) => i !== index))
  }

  // Обновление TPS метрик
  const updateTpsMetrics = (field: keyof TPSMetrics, value: number | undefined) => {
    setTpsMetrics(prev => ({
      ...prev,
      [field]: value
    }))
  }

  // Отправка формы
  const handleSubmit = async (values: any) => {
    try {
      const selectedHwConfig = hardwareConfigs.find(hw => hw.id === selectedHardwareConfig)
      if (!selectedHwConfig) {
        message.error('Выберите конфигурацию железа')
        return
      }

      const formData: RunCreationFormData = {
        runId: values.runId,
        name: values.name,
        description: values.description,
        status: values.status,
        workloadType: selectedWorkloadType,
        databaseType: selectedDatabaseType,
        databaseVersion: {
          version: values.databaseVersion,
          build: values.databaseBuild
        },
        hardwareConfiguration: selectedHwConfig,
        deploymentLayout: {
          type: deploymentLayoutType,
          signature: generateDeploymentSignature(values.deploymentConfiguration || {}, deploymentLayoutType),
          configuration: values.deploymentConfiguration || {}
        },
        nemesisSignature: {
          signature: generateNemesisSignature(nemeses),
          nemeses: nemeses
        },
        workloadProperties: {
          runners: values.runners || 1,
          duration: values.duration,
          ...getWorkloadSpecificProperties(values)
        },
        tpsMetrics: tpsMetrics
      }

      onSubmit(formData)
      message.success('Запуск создан успешно!')
    } catch (error) {
      message.error('Ошибка при создании запуска')
      console.error(error)
    }
  }

  // Получение свойств специфичных для типа нагрузки
  const getWorkloadSpecificProperties = (values: any): Partial<WorkloadProperties> => {
    switch (selectedWorkloadType) {
      case 'pgbench':
        return {
          pgbenchProperties: {
            scaleFactor: values.pgbench_scaleFactor || 1,
            clients: values.pgbench_clients || 1,
            threads: values.pgbench_threads,
            transactions: values.pgbench_transactions,
            customScript: values.pgbench_customScript
          }
        }
      case 'ycsb':
        return {
          ycsbProperties: {
            recordCount: values.ycsb_recordCount || 1000,
            operationCount: values.ycsb_operationCount || 1000,
            workload: values.ycsb_workload || 'workloada',
            readProportion: values.ycsb_readProportion,
            updateProportion: values.ycsb_updateProportion,
            insertProportion: values.ycsb_insertProportion,
            scanProportion: values.ycsb_scanProportion
          }
        }
      case 'tpc-h':
        return {
          tpchProperties: {
            scaleFactor: values.tpch_scaleFactor || 1,
            streams: values.tpch_streams,
            queries: values.tpch_queries
          }
        }
      case 'tpc-c':
        return {
          tpccProperties: {
            warehouses: values.tpcc_warehouses || 1,
            terminals: values.tpcc_terminals,
            rampupTime: values.tpcc_rampupTime,
            measureTime: values.tpcc_measureTime
          }
        }
      default:
        return {
          customProperties: values.customProperties || {}
        }
    }
  }

  // Рендер полей для конкретного типа нагрузки
  const renderWorkloadSpecificFields = () => {
    switch (selectedWorkloadType) {
      case 'pgbench':
        return (
          <>
            <Row gutter={16}>
              <Col span={12}>
                <Form.Item
                  name="pgbench_scaleFactor"
                  label="Масштабный коэффициент"
                  tooltip="Количество строк в таблице pgbench_accounts"
                >
                  <InputNumber min={1} placeholder="1" style={{ width: '100%' }} />
                </Form.Item>
              </Col>
              <Col span={12}>
                <Form.Item
                  name="pgbench_clients"
                  label="Количество клиентов"
                  tooltip="Количество одновременных клиентских соединений"
                >
                  <InputNumber min={1} placeholder="1" style={{ width: '100%' }} />
                </Form.Item>
              </Col>
            </Row>
            <Row gutter={16}>
              <Col span={12}>
                <Form.Item
                  name="pgbench_threads"
                  label="Количество потоков"
                  tooltip="Количество рабочих потоков"
                >
                  <InputNumber min={1} style={{ width: '100%' }} />
                </Form.Item>
              </Col>
              <Col span={12}>
                <Form.Item
                  name="pgbench_transactions"
                  label="Количество транзакций"
                  tooltip="Общее количество транзакций для выполнения"
                >
                  <InputNumber min={1} style={{ width: '100%' }} />
                </Form.Item>
              </Col>
            </Row>
            <Form.Item
              name="pgbench_customScript"
              label="Пользовательский скрипт"
              tooltip="Путь к пользовательскому скрипту pgbench"
            >
              <Input placeholder="Путь к скрипту (необязательно)" />
            </Form.Item>
          </>
        )
      case 'ycsb':
        return (
          <>
            <Row gutter={16}>
              <Col span={12}>
                <Form.Item
                  name="ycsb_recordCount"
                  label="Количество записей"
                  tooltip="Количество записей для загрузки в БД"
                >
                  <InputNumber min={1} placeholder="1000" style={{ width: '100%' }} />
                </Form.Item>
              </Col>
              <Col span={12}>
                <Form.Item
                  name="ycsb_operationCount"
                  label="Количество операций"
                  tooltip="Количество операций для выполнения"
                >
                  <InputNumber min={1} placeholder="1000" style={{ width: '100%' }} />
                </Form.Item>
              </Col>
            </Row>
            <Row gutter={16}>
              <Col span={12}>
                <Form.Item
                  name="ycsb_workload"
                  label="Тип нагрузки"
                  tooltip="Предустановленный тип нагрузки YCSB"
                >
                  <Select placeholder="Выберите тип нагрузки">
                    <Option value="workloada">
                      <div className="pl-3">Workload A (50% read, 50% update)</div>
                    </Option>
                    <Option value="workloadb">
                      <div className="pl-3">Workload B (95% read, 5% update)</div>
                    </Option>
                    <Option value="workloadc">
                      <div className="pl-3">Workload C (100% read)</div>
                    </Option>
                    <Option value="workloadd">
                      <div className="pl-3">Workload D (95% read, 5% insert)</div>
                    </Option>
                    <Option value="workloade">
                      <div className="pl-3">Workload E (95% scan, 5% insert)</div>
                    </Option>
                    <Option value="workloadf">
                      <div className="pl-3">Workload F (50% read, 50% read-modify-write)</div>
                    </Option>
                  </Select>
                </Form.Item>
              </Col>
            </Row>
          </>
        )
      case 'tpc-h':
        return (
          <>
            <Row gutter={16}>
              <Col span={12}>
                <Form.Item
                  name="tpch_scaleFactor"
                  label="Масштабный коэффициент"
                  tooltip="Размер базы данных в GB"
                >
                  <InputNumber min={1} placeholder="1" style={{ width: '100%' }} />
                </Form.Item>
              </Col>
              <Col span={12}>
                <Form.Item
                  name="tpch_streams"
                  label="Количество потоков"
                  tooltip="Количество параллельных потоков выполнения запросов"
                >
                  <InputNumber min={1} style={{ width: '100%' }} />
                </Form.Item>
              </Col>
            </Row>
            <Form.Item
              name="tpch_queries"
              label="Запросы для выполнения"
              tooltip="Список запросов TPC-H для выполнения"
            >
              <Select mode="multiple" placeholder="Выберите запросы">
                {Array.from({ length: 22 }, (_, i) => (
                  <Option key={`Q${i + 1}`} value={`Q${i + 1}`}>
                    <div className="pl-3">
                      Query {i + 1}
                    </div>
                  </Option>
                ))}
              </Select>
            </Form.Item>
          </>
        )
      case 'tpc-c':
        return (
          <>
            <Row gutter={16}>
              <Col span={12}>
                <Form.Item
                  name="tpcc_warehouses"
                  label="Количество складов"
                  tooltip="Количество складов в базе данных TPC-C"
                >
                  <InputNumber min={1} placeholder="1" style={{ width: '100%' }} />
                </Form.Item>
              </Col>
              <Col span={12}>
                <Form.Item
                  name="tpcc_terminals"
                  label="Количество терминалов"
                  tooltip="Количество терминалов на склад"
                >
                  <InputNumber min={1} style={{ width: '100%' }} />
                </Form.Item>
              </Col>
            </Row>
            <Row gutter={16}>
              <Col span={12}>
                <Form.Item
                  name="tpcc_rampupTime"
                  label="Время разогрева"
                  tooltip="Время разогрева перед началом измерений"
                >
                  <Input placeholder="5m" />
                </Form.Item>
              </Col>
              <Col span={12}>
                <Form.Item
                  name="tpcc_measureTime"
                  label="Время измерений"
                  tooltip="Время выполнения измерений"
                >
                  <Input placeholder="10m" />
                </Form.Item>
              </Col>
            </Row>
          </>
        )
      default:
        return (
          <Form.Item
            name="customProperties"
            label="Пользовательские свойства"
            tooltip="JSON объект с пользовательскими свойствами нагрузки"
          >
            <TextArea rows={4} placeholder='{"key": "value"}' />
          </Form.Item>
        )
    }
  }

  return (
    <Form
      form={form}
      layout="vertical"
      onFinish={handleSubmit}
      initialValues={{
        workloadType: 'pgbench',
        databaseType: 'postgres',
        status: 'completed',
        runners: 1
      }}
    >
      {/* Основная информация */}
      <Card title={<><SettingOutlined className="mr-2" />{t('form.basicInfo')}</>} className="mb-4">
        <Row gutter={16}>
          <Col span={6}>
            <Form.Item
              name="runId"
              label={t('fields.runId')}
              rules={[{ required: true, message: t('validation.runIdRequired') }]}
            >
              <Input placeholder={t('placeholders.runId')} />
            </Form.Item>
          </Col>
          <Col span={6}>
            <Form.Item
              name="name"
              label={t('fields.runName')}
              rules={[{ required: true, message: t('validation.runNameRequired') }]}
            >
              <Input placeholder={t('placeholders.runName')} />
            </Form.Item>
          </Col>
          <Col span={6}>
            <Form.Item name="description" label={t('fields.runDescription')}>
              <Input placeholder={t('placeholders.runDescription')} />
            </Form.Item>
          </Col>
          <Col span={6}>
            <Form.Item
              name="status"
              label="Статус запуска"
              rules={[{ required: true, message: 'Выберите статус запуска' }]}
            >
              <Select placeholder="Выберите статус">
                <Option value="completed">
                  <div className="flex items-center pl-3">
                    <CheckCircleOutlined className="text-green-500 mr-2" />
                    Завершен
                  </div>
                </Option>
                <Option value="failed">
                  <div className="flex items-center pl-3">
                    <ExclamationCircleOutlined className="text-red-500 mr-2" />
                    Ошибка
                  </div>
                </Option>
              </Select>
            </Form.Item>
          </Col>
        </Row>
      </Card>

      {/* Тип нагрузки */}
      <Card title={<><ThunderboltOutlined className="mr-2" />Тип нагрузки</>} className="mb-4">
        <Form.Item
          name="workloadType"
          label="Выберите тип нагрузки"
          rules={[{ required: true, message: 'Выберите тип нагрузки' }]}
        >
          <Select
            placeholder="Выберите тип нагрузки"
            onChange={(value) => setSelectedWorkloadType(value)}
          >
            {WORKLOAD_TYPES.map(workload => (
              <Option key={workload.value} value={workload.value}>
                <div className="pl-3">
                  <div>{workload.label}</div>
                  <small style={{ color: '#666' }}>{workload.description}</small>
                </div>
              </Option>
            ))}
          </Select>
        </Form.Item>
        
        <Divider />
        <Title level={5}>Свойства нагрузки</Title>
        <Row gutter={16}>
          <Col span={12}>
            <Form.Item
              name="runners"
              label="Количество раннеров"
              tooltip="Количество параллельных процессов выполнения нагрузки"
              rules={[{ required: true, message: 'Укажите количество раннеров' }]}
            >
              <InputNumber min={1} placeholder="1" style={{ width: '100%' }} />
            </Form.Item>
          </Col>
          <Col span={12}>
            <Form.Item
              name="duration"
              label="Продолжительность"
              tooltip="Продолжительность выполнения теста (например: 10m, 1h)"
            >
              <Input placeholder="10m" />
            </Form.Item>
          </Col>
        </Row>
        
        {renderWorkloadSpecificFields()}
      </Card>

      {/* База данных */}
      <Card title={<><DatabaseOutlined className="mr-2" />База данных</>} className="mb-4">
        <Row gutter={16}>
          <Col span={8}>
            <Form.Item
              name="databaseType"
              label="Тип базы данных"
              rules={[{ required: true, message: 'Выберите тип базы данных' }]}
            >
              <Select
                placeholder="Выберите тип БД"
                onChange={(value) => setSelectedDatabaseType(value)}
              >
                {DATABASE_TYPES.map(db => (
                  <Option key={db.value} value={db.value}>
                    <div className="pl-3">
                      <div>{db.label}</div>
                      <small style={{ color: '#666' }}>{db.description}</small>
                    </div>
                  </Option>
                ))}
              </Select>
            </Form.Item>
          </Col>
          <Col span={8}>
            <Form.Item
              name="databaseVersion"
              label="Версия"
              rules={[{ required: true, message: 'Выберите версию БД' }]}
            >
              <Select placeholder="Выберите версию">
                {getAvailableVersions().map(version => (
                  <Option key={version.version} value={version.version}>
                    <div className="pl-3">
                      {version.description || version.version}
                    </div>
                  </Option>
                ))}
              </Select>
            </Form.Item>
          </Col>
          <Col span={8}>
            <Form.Item name="databaseBuild" label="Сборка">
              <Input placeholder="Номер сборки (необязательно)" />
            </Form.Item>
          </Col>
        </Row>
      </Card>

      {/* Схема развертывания */}
      <Card title={<><CloudServerOutlined className="mr-2" />Схема развертывания</>} className="mb-4">
        <Form.Item
          name="deploymentLayoutType"
          label="Тип развертывания"
          rules={[{ required: true, message: 'Выберите тип развертывания' }]}
        >
          <Select
            placeholder="Выберите тип развертывания"
            onChange={(value) => setDeploymentLayoutType(value)}
          >
            {getAvailableDeploymentLayouts().map(layout => (
              <Option key={layout} value={layout}>
                <div className="pl-3">
                  {layout}
                </div>
              </Option>
            ))}
          </Select>
        </Form.Item>

        {deploymentLayoutType !== 'single-node' && (
          <Collapse>
            <Panel header="Конфигурация развертывания" key="deployment-config">
              <Row gutter={16}>
                {(deploymentLayoutType === 'master-replica' || deploymentLayoutType === 'cluster') && (
                  <>
                    <Col span={12}>
                      <Form.Item name={['deploymentConfiguration', 'masters']} label="Мастеры">
                        <InputNumber min={1} placeholder="1" style={{ width: '100%' }} />
                      </Form.Item>
                    </Col>
                    <Col span={12}>
                      <Form.Item name={['deploymentConfiguration', 'replicas']} label="Реплики">
                        <InputNumber min={0} placeholder="0" style={{ width: '100%' }} />
                      </Form.Item>
                    </Col>
                  </>
                )}
                {deploymentLayoutType === 'distributed' && (
                  <>
                    <Col span={8}>
                      <Form.Item name={['deploymentConfiguration', 'segments']} label="Сегменты">
                        <InputNumber min={1} placeholder="2" style={{ width: '100%' }} />
                      </Form.Item>
                    </Col>
                    <Col span={8}>
                      <Form.Item name={['deploymentConfiguration', 'coordinators']} label="Координаторы">
                        <InputNumber min={1} placeholder="1" style={{ width: '100%' }} />
                      </Form.Item>
                    </Col>
                    <Col span={8}>
                      <Form.Item name={['deploymentConfiguration', 'witnesses']} label="Свидетели">
                        <InputNumber min={0} placeholder="0" style={{ width: '100%' }} />
                      </Form.Item>
                    </Col>
                  </>
                )}
                {deploymentLayoutType === 'sharded' && (
                  <Col span={12}>
                    <Form.Item name={['deploymentConfiguration', 'shards']} label="Шарды">
                      <InputNumber min={1} placeholder="2" style={{ width: '100%' }} />
                    </Form.Item>
                  </Col>
                )}
              </Row>
            </Panel>
          </Collapse>
        )}
      </Card>

      {/* Конфигурация железа */}
      <Card title={<><CloudServerOutlined className="mr-2" />Конфигурация железа</>} className="mb-4">
        <div className="mb-4">
          <Button type="dashed" onClick={addHardwareConfig} icon={<PlusOutlined />} block>
            Добавить конфигурацию железа
          </Button>
        </div>

        {hardwareConfigs.map((config) => (
          <Card
            key={config.id}
            size="small"
            className="mb-2"
            title={
              <div className="flex justify-between items-center">
                <span>{config.name}</span>
                <div>
                  <Tag color="blue">{generateHardwareSignature(config)}</Tag>
                  <Button
                    type="text"
                    size="small"
                    danger
                    icon={<DeleteOutlined />}
                    onClick={() => removeHardwareConfig(config.id)}
                  />
                </div>
              </div>
            }
            extra={
              <Switch
                checked={selectedHardwareConfig === config.id}
                onChange={(checked) => {
                  setSelectedHardwareConfig(checked ? config.id : undefined)
                }}
                checkedChildren="Выбрано"
                unCheckedChildren="Не выбрано"
              />
            }
          >
            <Row gutter={16}>
              <Col span={12}>
                <Input
                  placeholder="Название конфигурации"
                  value={config.name}
                  onChange={(e) => updateHardwareConfig(config.id, { name: e.target.value })}
                />
              </Col>
              <Col span={12}>
                <InputNumber
                  placeholder="Количество узлов"
                  value={config.nodeCount}
                  min={1}
                  style={{ width: '100%' }}
                  onChange={(value) => updateHardwareConfig(config.id, { nodeCount: value || 1 })}
                />
              </Col>
            </Row>
            <Row gutter={16} className="mt-2">
              <Col span={6}>
                <InputNumber
                  placeholder="Ядра CPU"
                  value={config.cpu.cores}
                  min={1}
                  style={{ width: '100%' }}
                  onChange={(value) => updateHardwareConfig(config.id, {
                    cpu: { ...config.cpu, cores: value || 1 }
                  })}
                />
              </Col>
              <Col span={6}>
                <InputNumber
                  placeholder="Память (GB)"
                  value={config.memory.totalGB}
                  min={1}
                  style={{ width: '100%' }}
                  onChange={(value) => updateHardwareConfig(config.id, {
                    memory: { ...config.memory, totalGB: value || 1 }
                  })}
                />
              </Col>
              <Col span={6}>
                <Select
                  placeholder="Тип накопителя"
                  value={config.storage.type}
                  style={{ width: '100%' }}
                  onChange={(value) => updateHardwareConfig(config.id, {
                    storage: { ...config.storage, type: value }
                  })}
                >
                  <Option value="ssd">
                    <div className="pl-3">SSD</div>
                  </Option>
                  <Option value="nvme">
                    <div className="pl-3">NVMe</div>
                  </Option>
                  <Option value="hdd">
                    <div className="pl-3">HDD</div>
                  </Option>
                  <Option value="mixed">
                    <div className="pl-3">Смешанный</div>
                  </Option>
                </Select>
              </Col>
              <Col span={6}>
                <InputNumber
                  placeholder="Объем (GB)"
                  value={config.storage.capacityGB}
                  min={1}
                  style={{ width: '100%' }}
                  onChange={(value) => updateHardwareConfig(config.id, {
                    storage: { ...config.storage, capacityGB: value || 1 }
                  })}
                />
              </Col>
            </Row>
          </Card>
        ))}

        {selectedHardwareConfig && (
          <Alert
            message="Выбранная конфигурация железа"
            description={`Конфигурация: ${generateHardwareSignature(
              hardwareConfigs.find(hw => hw.id === selectedHardwareConfig)!
            )}`}
            type="info"
            showIcon
            className="mt-2"
          />
        )}
      </Card>

      {/* Немезисы */}
      <Card title={<><BugOutlined className="mr-2" />Немезисы</>} className="mb-4">
        <div className="mb-4">
          <Button type="dashed" onClick={addNemesis} icon={<PlusOutlined />} block>
            Добавить немезис
          </Button>
        </div>

        {nemeses.map((nemesis, index) => (
          <Card
            key={index}
            size="small"
            className="mb-2"
            title={
              <div className="flex justify-between items-center">
                <span>Немезис {index + 1}</span>
                <Button
                  type="text"
                  size="small"
                  danger
                  icon={<DeleteOutlined />}
                  onClick={() => removeNemesis(index)}
                />
              </div>
            }
          >
            <Row gutter={16}>
              <Col span={12}>
                <Select
                  placeholder="Тип немезиса"
                  value={nemesis.type}
                  style={{ width: '100%' }}
                  onChange={(value) => updateNemesis(index, { type: value })}
                >
                  {NEMESIS_TYPES.map(type => (
                    <Option key={type.value} value={type.value}>
                      <div className="pl-3">
                        <Tooltip title={type.description}>
                          {type.label}
                        </Tooltip>
                      </div>
                    </Option>
                  ))}
                </Select>
              </Col>
              <Col span={12}>
                <Switch
                  checked={nemesis.enabled}
                  onChange={(checked) => updateNemesis(index, { enabled: checked })}
                  checkedChildren="Включен"
                  unCheckedChildren="Отключен"
                />
              </Col>
            </Row>
          </Card>
        ))}

        {nemeses.length > 0 && (
          <Alert
            message="Сигнатура немезисов"
            description={`Сигнатура: ${generateNemesisSignature(nemeses)}`}
            type="info"
            showIcon
            className="mt-2"
          />
        )}
      </Card>

      {/* TPS Метрики */}
      <Card title={<><ThunderboltOutlined className="mr-2" />TPS Метрики</>} className="mb-4">
        <Alert
          message="Метрики производительности"
          description="Укажите метрики TPS (Transactions Per Second) для данного запуска. Все поля необязательны."
          type="info"
          showIcon
          className="mb-4"
        />
        
        <Row gutter={16}>
          <Col span={12}>
            <Form.Item
              name="tps_max"
              label="Максимальный TPS"
              tooltip="Максимальное количество транзакций в секунду"
            >
              <InputNumber
                min={0}
                step={0.1}
                placeholder="0.0"
                style={{ width: '100%' }}
                value={tpsMetrics.max}
                onChange={(value) => updateTpsMetrics('max', value || undefined)}
              />
            </Form.Item>
          </Col>
          <Col span={12}>
            <Form.Item
              name="tps_min"
              label="Минимальный TPS"
              tooltip="Минимальное количество транзакций в секунду"
            >
              <InputNumber
                min={0}
                step={0.1}
                placeholder="0.0"
                style={{ width: '100%' }}
                value={tpsMetrics.min}
                onChange={(value) => updateTpsMetrics('min', value || undefined)}
              />
            </Form.Item>
          </Col>
        </Row>
        
        <Row gutter={16}>
          <Col span={12}>
            <Form.Item
              name="tps_average"
              label="Средний TPS"
              tooltip="Среднее количество транзакций в секунду"
            >
              <InputNumber
                min={0}
                step={0.1}
                placeholder="0.0"
                style={{ width: '100%' }}
                value={tpsMetrics.average}
                onChange={(value) => updateTpsMetrics('average', value || undefined)}
              />
            </Form.Item>
          </Col>
          <Col span={12}>
            <Form.Item
              name="tps_95p"
              label="95-й процентиль TPS"
              tooltip="95-й процентиль количества транзакций в секунду"
            >
              <InputNumber
                min={0}
                step={0.1}
                placeholder="0.0"
                style={{ width: '100%' }}
                value={tpsMetrics['95p']}
                onChange={(value) => updateTpsMetrics('95p', value || undefined)}
              />
            </Form.Item>
          </Col>
        </Row>
        
        <Row gutter={16}>
          <Col span={12}>
            <Form.Item
              name="tps_99p"
              label="99-й процентиль TPS"
              tooltip="99-й процентиль количества транзакций в секунду"
            >
              <InputNumber
                min={0}
                step={0.1}
                placeholder="0.0"
                style={{ width: '100%' }}
                value={tpsMetrics['99p']}
                onChange={(value) => updateTpsMetrics('99p', value || undefined)}
              />
            </Form.Item>
          </Col>
        </Row>
      </Card>

      {/* Действия */}
      <Card>
        <div className="flex justify-end">
          <Space>
            <Button onClick={onCancel}>{t('actions.cancel')}</Button>
            <Button type="primary" htmlType="submit">
              {t('actions.createRun')}
            </Button>
          </Space>
        </div>
      </Card>
    </Form>
  )
}

export default RunCreationForm
