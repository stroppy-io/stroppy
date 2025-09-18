import React, { useState } from 'react'
import { 
  Card, Form, Input, InputNumber, Button, Space, Modal, message, Switch, 
  Tag, Steps, Select, Row, Col, Divider, 
  Typography, Layout, Tabs, Alert, Empty, Descriptions, Dropdown, Tooltip, Badge, Timeline
} from 'antd'
import { 
  DownloadOutlined, EyeOutlined, PlusOutlined, DeleteOutlined, DragOutlined, 
  PlayCircleOutlined, InfoCircleOutlined, SettingOutlined,
  DatabaseOutlined, CodeOutlined, BugOutlined, ThunderboltOutlined,
  CopyOutlined, MoreOutlined
} from '@ant-design/icons'

const { Step } = Steps
const { Option } = Select
const { Title, Text } = Typography
const { Content } = Layout
const { TabPane } = Tabs

const ConfigGenerator: React.FC = () => {
  const [configData, setConfigData] = useState({
    version: '',
    runId: '',
    benchmarkName: '',
    seed: '',
    run: {
      driver: {
        driverPluginPath: '',
        url: '',
        driverPluginArgs: [] as string[],
        dbSpecific: [] as Array<{ key: string; string?: string; int32?: number }>
      },
      goExecutor: {
        goMaxProc: '',
        cancelOnError: false
      },
      k6Executor: {
        k6BinaryPath: '',
        k6ScriptPath: '',
        k6SetupTimeout: '',
        k6Vus: '',
        k6MaxVus: '',
        k6Rate: '',
        k6Duration: '',
        k6BinaryArgs: [] as string[],
        otlpExport: {
          otlpGrpcEndpoint: '',
          otlpMetricsPrefix: ''
        }
      },
      steps: [] as Array<{ name: string; executor: string; async: boolean }>,
      logger: {
        logLevel: 'LOG_LEVEL_INFO',
        logMode: 'LOG_MODE_PRODUCTION'
      },
      metadata: {
        example: ''
      }
    },
    benchmark: {
      steps: [] as Array<{ 
        name: string; 
        async: boolean; 
        units: Array<{
          id: string;
          type: 'query' | 'createTable' | 'transaction';
          async: boolean;
          query?: {
            name: string;
            sql: string;
            count: string;
            params: Array<{
              name: string;
              replaceRegex: string;
              generationRule: {
                type: 'string' | 'int' | 'float' | 'bool' | 'uuid' | 'datetime' | 'decimal';
                min?: number;
                max?: number;
                length?: number;
                pattern?: string;
                values?: string[];
              };
            }>;
          };
          createTable?: {
            name: string;
            constraint: string;
            columns: Array<{
              name: string;
              sqlType: string;
              primaryKey?: boolean;
              nullable: boolean;
              defaultValue?: string;
              constraint?: string;
            }>;
            tableIndexes: Array<{
              name: string;
              columns: string[];
              unique: boolean;
              constraint?: string;
            }>;
          };
          transaction?: {
            name: string;
            isolationLevel: string;
            count?: string;
            queries: Array<{
              name: string;
              sql: string;
              count: string;
              params: Array<{
                name: string;
                replaceRegex: string;
                generationRule: {
                  type: 'string' | 'int' | 'float' | 'bool' | 'uuid' | 'datetime' | 'decimal';
                  min?: number;
                  max?: number;
                  length?: number;
                  pattern?: string;
                  values?: string[];
                };
              }>;
            }>;
          };
        }>;
      }>
    }
  })

  const [previewVisible, setPreviewVisible] = useState(false)
  const [yamlContent, setYamlContent] = useState('')
  const [currentStep, setCurrentStep] = useState(0)
  const [form] = Form.useForm()

  const handleFormChange = (field: string, value: any) => {
    setConfigData(prev => ({
      ...prev,
      [field]: value
    }))
  }

  const handleRunChange = (field: string, value: any) => {
    setConfigData(prev => ({
      ...prev,
      run: {
        ...prev.run,
        [field]: value
      }
    }))
  }

  const handleBenchmarkChange = (field: string, value: any) => {
    setConfigData(prev => ({
      ...prev,
      benchmark: {
        ...prev.benchmark,
        [field]: value
      }
    }))
  }

  const handleDriverChange = (field: string, value: any) => {
    setConfigData(prev => ({
      ...prev,
      run: {
        ...prev.run,
        driver: {
          ...prev.run.driver,
          [field]: value
        }
      }
    }))
  }

  const handleGoExecutorChange = (field: string, value: any) => {
    setConfigData(prev => ({
      ...prev,
      run: {
        ...prev.run,
        goExecutor: {
          ...prev.run.goExecutor,
          [field]: value
        }
      }
    }))
  }

  const handleK6ExecutorChange = (field: string, value: any) => {
    setConfigData(prev => ({
      ...prev,
      run: {
        ...prev.run,
        k6Executor: {
          ...prev.run.k6Executor,
          [field]: value
        }
      }
    }))
  }

  const addDriverArg = () => {
    const newArgs = [...configData.run.driver.driverPluginArgs, '']
    handleDriverChange('driverPluginArgs', newArgs)
  }

  const updateDriverArg = (index: number, value: string) => {
    const newArgs = [...configData.run.driver.driverPluginArgs]
    newArgs[index] = value
    handleDriverChange('driverPluginArgs', newArgs)
  }

  const removeDriverArg = (index: number) => {
    const newArgs = configData.run.driver.driverPluginArgs.filter((_, i) => i !== index)
    handleDriverChange('driverPluginArgs', newArgs)
  }

  const addK6Arg = () => {
    const newArgs = [...(configData.run.k6Executor.k6BinaryArgs || []), '']
    handleK6ExecutorChange('k6BinaryArgs', newArgs)
  }

  const updateK6Arg = (index: number, value: string) => {
    const newArgs = [...(configData.run.k6Executor.k6BinaryArgs || [])]
    newArgs[index] = value
    handleK6ExecutorChange('k6BinaryArgs', newArgs)
  }

  const removeK6Arg = (index: number) => {
    const newArgs = (configData.run.k6Executor.k6BinaryArgs || []).filter((_, i) => i !== index)
    handleK6ExecutorChange('k6BinaryArgs', newArgs)
  }


  const addBenchmarkStep = () => {
    const newStep = {
      name: `benchmark_step_${Date.now()}`,
      async: false,
      units: []
    }
    const newSteps = [...configData.benchmark.steps, newStep]
    handleBenchmarkChange('steps', newSteps)
  }

  const removeBenchmarkStep = (index: number) => {
    const newSteps = configData.benchmark.steps.filter((_, i) => i !== index)
    handleBenchmarkChange('steps', newSteps)
  }

  const addOperation = (stepIndex: number, type: 'query' | 'createTable' | 'transaction') => {
    const newOperation = {
      id: `${type}_${Date.now()}`,
      type,
      async: false,
      ...(type === 'query' && {
        query: {
          name: '',
          sql: '',
          count: '1',
          params: []
        }
      }),
      ...(type === 'createTable' && {
        createTable: {
          name: '',
          constraint: '',
          columns: [],
          tableIndexes: []
        }
      }),
      ...(type === 'transaction' && {
        transaction: {
          name: '',
          isolationLevel: 'TX_ISOLATION_LEVEL_READ_COMMITTED',
          count: '1',
          queries: []
        }
      })
    }

    const newSteps = [...configData.benchmark.steps]
    newSteps[stepIndex] = {
      ...newSteps[stepIndex],
      units: [...(newSteps[stepIndex].units || []), newOperation]
    }
    handleBenchmarkChange('steps', newSteps)
  }

  const removeOperation = (stepIndex: number, operationIndex: number) => {
    const newSteps = [...configData.benchmark.steps]
    newSteps[stepIndex] = {
      ...newSteps[stepIndex],
      units: newSteps[stepIndex].units.filter((_, i) => i !== operationIndex)
    }
    handleBenchmarkChange('steps', newSteps)
  }

  const updateOperation = (stepIndex: number, operationIndex: number, operation: any) => {
    console.log('updateOperation called:', { stepIndex, operationIndex, operation })
    const newSteps = [...configData.benchmark.steps]
    newSteps[stepIndex] = {
      ...newSteps[stepIndex],
      units: newSteps[stepIndex].units.map((unit, i) => 
        i === operationIndex ? operation : unit
      )
    }
    console.log('New steps after update:', newSteps)
    handleBenchmarkChange('steps', newSteps)
  }


  const duplicateOperation = (stepIndex: number, operationIndex: number, operation: any) => {
    const newOperation = {
      ...operation,
      id: `${operation.type}_${Date.now()}`,
    }
    const newSteps = [...configData.benchmark.steps]
    newSteps[stepIndex] = {
      ...newSteps[stepIndex],
      units: [...newSteps[stepIndex].units.slice(0, operationIndex + 1), newOperation, ...newSteps[stepIndex].units.slice(operationIndex + 1)]
    }
    handleBenchmarkChange('steps', newSteps)
  }


  const generateYAML = () => {
    // Генерация структуры run
    const runConfig = {
      runId: configData.runId,
      seed: configData.seed,
      driver: {
        driverPluginPath: configData.run.driver.driverPluginPath,
        url: configData.run.driver.url,
        dbSpecific: {
          fields: configData.run.driver.dbSpecific || []
        }
      },
      goExecutor: {
        goMaxProc: configData.run.goExecutor?.goMaxProc,
        cancelOnError: configData.run.goExecutor?.cancelOnError
      },
      k6Executor: {
        k6BinaryPath: configData.run.k6Executor?.k6BinaryPath,
        k6ScriptPath: configData.run.k6Executor?.k6ScriptPath,
        k6SetupTimeout: configData.run.k6Executor?.k6SetupTimeout,
        k6Vus: configData.run.k6Executor?.k6Vus,
        k6MaxVus: configData.run.k6Executor?.k6MaxVus,
        k6Rate: configData.run.k6Executor?.k6Rate,
        k6Duration: configData.run.k6Executor?.k6Duration,
        otlpExport: configData.run.k6Executor?.otlpExport
      },
      steps: configData.run.steps.map(step => ({
        name: step.name,
        executor: step.executor
      })),
      logger: configData.run.logger,
      metadata: configData.run.metadata
    }

    // Генерация структуры benchmark
    const benchmarkConfig = {
      name: configData.benchmarkName,
      steps: configData.benchmark.steps.map(step => ({
        name: step.name,
        units: step.units?.map(unit => {
          const baseUnit = {
            async: unit.async || false
          }

          if (unit.type === 'query' && unit.query) {
            return {
              ...baseUnit,
              query: {
                name: unit.query.name,
                sql: unit.query.sql,
                count: unit.query.count,
                params: unit.query.params?.map(param => ({
                  name: param.name,
                  generationRule: param.generationRule
                })) || []
              }
            }
          }

          if (unit.type === 'createTable' && unit.createTable) {
            return {
              ...baseUnit,
              createTable: {
                name: unit.createTable.name,
                constraint: unit.createTable.constraint,
                columns: unit.createTable.columns?.map(col => ({
                  name: col.name,
                  sqlType: col.sqlType,
                  primaryKey: col.primaryKey || false,
                  nullable: col.nullable || false,
                  constraint: col.constraint || ''
                })) || [],
                tableIndexes: unit.createTable.tableIndexes?.map(idx => ({
                  name: idx.name,
                  columns: idx.columns,
                  unique: idx.unique || false,
                  constraint: idx.constraint || ''
                })) || []
              }
            }
          }

          if (unit.type === 'transaction' && unit.transaction) {
            return {
              ...baseUnit,
              transaction: {
                name: unit.transaction.name,
                isolationLevel: unit.transaction.isolationLevel,
                count: unit.transaction.count || '1',
                queries: unit.transaction.queries?.map(query => ({
                  name: query.name,
                  sql: query.sql,
                  count: query.count,
                  params: query.params?.map(param => ({
                    name: param.name,
                    generationRule: param.generationRule
                  })) || []
                })) || []
              }
            }
          }

          return baseUnit
        }) || [],
        async: step.async || false
      }))
    }

    const fullConfig = {
      version: configData.version,
      run: runConfig,
      benchmark: benchmarkConfig
    }

    return JSON.stringify(fullConfig, null, 2)
  }

  const handlePreview = async () => {
    try {
      const yaml = generateYAML()
      setYamlContent(yaml)
      setPreviewVisible(true)
    } catch (error) {
      message.error('Ошибка при генерации конфигурации: ' + (error as Error).message)
    }
  }

  const handleDownload = async () => {
    try {
      const yaml = generateYAML()
      
      const blob = new Blob([yaml], { type: 'application/json' })
      const url = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = 'stroppy-config.json'
      document.body.appendChild(link)
      link.click()
      document.body.removeChild(link)
      URL.revokeObjectURL(url)
      
      message.success('Конфигурация скачана успешно')
    } catch (error) {
      message.error('Ошибка при скачивании конфигурации: ' + (error as Error).message)
    }
  }


  const configSteps = [
    {
      title: 'Основные параметры',
      icon: <SettingOutlined />,
      description: 'Базовая конфигурация проекта'
    },
    {
      title: 'База данных',
      icon: <DatabaseOutlined />,
      description: 'Настройки подключения к БД'
    },
    {
      title: 'Исполнители',
      icon: <ThunderboltOutlined />,
      description: 'Go и K6 исполнители'
    },
    {
      title: 'Шаги и операции',
      icon: <CodeOutlined />,
      description: 'Конфигурация бенчмарков'
    },
    {
      title: 'Дополнительно',
      icon: <BugOutlined />,
      description: 'Логирование и метаданные'
    }
  ]

  return (
    <Layout className="min-h-screen bg-gray-50">
      <Content className="p-6">
        <div className="max-w-7xl mx-auto">
          {/* Header */}
          <div className="mb-6">
      <div className="flex justify-between items-center mb-4">
              <div>
                <Title level={2} className="mb-2">
                  <SettingOutlined className="mr-2" />
                  Генератор конфигурации Stroppy
                </Title>
                <Text type="secondary">
                  Создайте конфигурацию для нагрузочного тестирования базы данных
                </Text>
              </div>
              <Space size="middle">
          <Button
            icon={<EyeOutlined />}
            onClick={handlePreview}
                  size="large"
          >
            Предварительный просмотр
          </Button>
          <Button
            type="primary"
            icon={<DownloadOutlined />}
            onClick={handleDownload}
                  size="large"
          >
                  Скачать конфигурацию
          </Button>
        </Space>
      </div>

            {/* Progress Steps */}
            <Steps current={currentStep} className="mb-6">
              {configSteps.map((step, index) => (
                <Step
                  key={index}
                  title={step.title}
                  description={step.description}
                  icon={step.icon}
                  onClick={() => setCurrentStep(index)}
                  style={{ cursor: 'pointer' }}
                />
              ))}
            </Steps>
          </div>

          {/* Main Form Content */}
          <Card className="shadow-sm">
            <Form form={form} layout="vertical" size="large">
              {/* Step 0: Основные параметры */}
              {currentStep === 0 && (
                <div>
                  <Title level={4} className="mb-4">
                    <SettingOutlined className="mr-2" />
                    Основные параметры конфигурации
                  </Title>
                  <Alert
                    message="Настройте базовые параметры конфигурации"
                    description="Эти параметры определяют основные настройки вашего тестирования"
                    type="info"
                    showIcon
                    className="mb-4"
                  />
                  
                  <Row gutter={[24, 16]}>
                    <Col xs={24} sm={12} md={6}>
            <Form.Item
                        label="Версия конфигурации"
              required
              tooltip="Версия формата конфигурации"
                        rules={[{ required: true, message: 'Укажите версию!' }]}
            >
              <Input
                value={configData.version}
                onChange={(e) => handleFormChange('version', e.target.value)}
                placeholder="1.0.0"
                          prefix={<CodeOutlined />}
              />
            </Form.Item>
                    </Col>

                    <Col xs={24} sm={12} md={6}>
            <Form.Item
              label="ID запуска"
              required
              tooltip="Уникальный идентификатор запуска"
                        rules={[{ required: true, message: 'Укажите ID запуска!' }]}
            >
              <Input
                value={configData.runId}
                onChange={(e) => handleFormChange('runId', e.target.value)}
                placeholder="run_1234567890"
                          prefix={<PlayCircleOutlined />}
              />
            </Form.Item>
                    </Col>

                    <Col xs={24} sm={12} md={6}>
            <Form.Item
              label="Название бенчмарка"
              required
              tooltip="Название бенчмарка для выполнения"
                        rules={[{ required: true, message: 'Укажите название бенчмарка!' }]}
            >
              <Input
                value={configData.benchmarkName}
                onChange={(e) => handleFormChange('benchmarkName', e.target.value)}
                placeholder="default_benchmark"
                          prefix={<ThunderboltOutlined />}
              />
            </Form.Item>
                    </Col>

                    <Col xs={24} sm={12} md={6}>
            <Form.Item
                        label="Семя генератора"
              required
              tooltip="Семя для генерации случайных данных"
                        rules={[{ required: true, message: 'Укажите семя!' }]}
                      >
                        <InputNumber
                          value={configData.seed ? parseInt(configData.seed) : undefined}
                          onChange={(value) => handleFormChange('seed', value?.toString() || '')}
                placeholder="737567"
                          className="w-full"
                          min={1}
              />
            </Form.Item>
                    </Col>
                  </Row>
          </div>
              )}

              {/* Step 1: База данных */}
              {currentStep === 1 && (
                <div>
                  <Title level={4} className="mb-4">
                    <DatabaseOutlined className="mr-2" />
                    Настройки базы данных
                  </Title>
                  <Alert
                    message="Настройте подключение к базе данных"
                    description="Укажите драйвер и параметры подключения к вашей базе данных"
                    type="info"
                    showIcon
                    className="mb-4"
                  />
                  
                  <Row gutter={[24, 16]}>
                    <Col xs={24} md={12}>
              <Form.Item
                label="Путь к плагину драйвера"
                required
                tooltip="Путь к исполняемому файлу плагина драйвера базы данных"
                        rules={[{ required: true, message: 'Укажите путь к драйверу!' }]}
              >
                <Input
                  value={configData.run.driver.driverPluginPath}
                  onChange={(e) => handleDriverChange('driverPluginPath', e.target.value)}
                  placeholder="/path/to/driver-plugin"
                          prefix={<DatabaseOutlined />}
                />
              </Form.Item>
                    </Col>

                    <Col xs={24} md={12}>
              <Form.Item
                label="URL подключения к базе данных"
                required
                tooltip="Строка подключения к базе данных"
                        rules={[{ required: true, message: 'Укажите URL подключения!' }]}
              >
                <Input
                  value={configData.run.driver.url}
                  onChange={(e) => handleDriverChange('url', e.target.value)}
                  placeholder="postgres://user:password@localhost:5432/database"
                          prefix={<DatabaseOutlined />}
                />
              </Form.Item>
                    </Col>
                  </Row>

                  <Divider>Дополнительные настройки</Divider>

            <Form.Item
              label="Аргументы плагина драйвера"
              tooltip="Дополнительные аргументы для передачи плагину драйвера"
                  >
                    <div className="space-y-2">
                      {configData.run.driver.driverPluginArgs.length === 0 ? (
                        <Empty
                          image={Empty.PRESENTED_IMAGE_SIMPLE}
                          description="Нет аргументов"
                        />
                      ) : (
                        configData.run.driver.driverPluginArgs.map((arg, index) => (
                          <div key={index} className="flex items-center space-x-2">
                    <Input
                      value={arg}
                      onChange={(e) => updateDriverArg(index, e.target.value)}
                      placeholder={`Аргумент ${index + 1}`}
                              prefix={<CodeOutlined />}
                    />
                    <Button
                      type="text"
                      danger
                      icon={<DeleteOutlined />}
                      onClick={() => removeDriverArg(index)}
                    />
                  </div>
                        ))
                      )}
                <Button
                  type="dashed"
                  icon={<PlusOutlined />}
                  onClick={addDriverArg}
                  className="w-full"
                        size="large"
                >
                  Добавить аргумент
                </Button>
              </div>
            </Form.Item>

            <Form.Item
              label="Специфичные настройки БД"
              tooltip="Настройки, специфичные для конкретной базы данных"
                  >
                    <div className="space-y-2">
                {configData.run.driver.dbSpecific.length === 0 ? (
                        <Empty
                          image={Empty.PRESENTED_IMAGE_SIMPLE}
                          description="Нет специфичных настроек"
                        />
                      ) : (
                        configData.run.driver.dbSpecific.map((field, index) => (
                          <Card key={index} size="small" className="bg-gray-50">
                            <Row gutter={[16, 8]} align="middle">
                              <Col flex="200px">
                        <Input
                          value={field.key}
                          onChange={(e) => {
                            const newFields = [...configData.run.driver.dbSpecific]
                            newFields[index] = { ...field, key: e.target.value }
                            handleDriverChange('dbSpecific', newFields)
                          }}
                          placeholder="trace_log_level"
                                  prefix={<SettingOutlined />}
                        />
                              </Col>
                              <Col flex="120px">
                                <Select
                          value={field.string ? 'string' : 'int32'}
                                  onChange={(value) => {
                            const newFields = [...configData.run.driver.dbSpecific]
                                    if (value === 'string') {
                              newFields[index] = { key: field.key, string: field.string || '' }
                            } else {
                              newFields[index] = { key: field.key, int32: field.int32 || 0 }
                            }
                            handleDriverChange('dbSpecific', newFields)
                          }}
                                  className="w-full"
                                >
                                  <Option value="string">
                                    <div className="pl-3">Строка</div>
                                  </Option>
                                  <Option value="int32">
                                    <div className="pl-3">Число</div>
                                  </Option>
                                </Select>
                              </Col>
                              <Col flex="auto">
                        <Input
                          value={field.string || field.int32?.toString() || ''}
                          onChange={(e) => {
                            const newFields = [...configData.run.driver.dbSpecific]
                            if (field.string !== undefined) {
                              newFields[index] = { ...field, string: e.target.value }
                            } else {
                              newFields[index] = { ...field, int32: parseInt(e.target.value) || 0 }
                            }
                            handleDriverChange('dbSpecific', newFields)
                          }}
                          placeholder={field.string !== undefined ? "warn" : "100"}
                        />
                              </Col>
                              <Col flex="40px">
                        <Button
                          type="text"
                          danger
                          icon={<DeleteOutlined />}
                          onClick={() => {
                            const newFields = configData.run.driver.dbSpecific.filter((_, i) => i !== index)
                            handleDriverChange('dbSpecific', newFields)
                          }}
                                />
                              </Col>
                            </Row>
                          </Card>
                        ))
                      )}
                      <Button
                        type="dashed"
                        icon={<PlusOutlined />}
                        onClick={() => {
                          const newField = { key: '', string: '' }
                          handleDriverChange('dbSpecific', [...configData.run.driver.dbSpecific, newField])
                        }}
                        className="w-full"
                        size="large"
                      >
                        Добавить поле конфигурации
                      </Button>
              </div>
            </Form.Item>
          </div>
              )}

              {/* Step 2: Исполнители */}
              {currentStep === 2 && (
                <div>
                  <Title level={4} className="mb-4">
                    <ThunderboltOutlined className="mr-2" />
                    Настройки исполнителей
                  </Title>
                  <Alert
                    message="Настройте Go и K6 исполнители"
                    description="Конфигурация исполнителей определяет параметры выполнения тестов"
                    type="info"
                    showIcon
                    className="mb-4"
                  />
                  
                  <Tabs defaultActiveKey="go" size="large">
                    <TabPane 
                      tab={
                        <span>
                          <ThunderboltOutlined />
                          Go Executor
                        </span>
                      } 
                      key="go"
                    >
                      <Row gutter={[24, 16]}>
                        <Col xs={24} md={12}>
                    <Form.Item
                      label="Максимальное количество процессов"
                      tooltip="Максимальное количество параллельных процессов для Go executor"
                    >
                      <InputNumber
                        value={configData.run.goExecutor?.goMaxProc ? parseInt(configData.run.goExecutor.goMaxProc) : undefined}
                        onChange={(value) => handleGoExecutorChange('goMaxProc', value?.toString() || '')}
                        placeholder="4"
                        className="w-full"
                        min={1}
                        max={100}
                      />
                    </Form.Item>
                        </Col>

                        <Col xs={24} md={12}>
                    <Form.Item
                      label="Отмена при ошибке"
                      tooltip="Останавливать выполнение при первой ошибке"
                    >
                            <div className="flex items-center">
                        <Switch
                          checked={configData.run.goExecutor?.cancelOnError}
                          onChange={(checked) => handleGoExecutorChange('cancelOnError', checked)}
                          checkedChildren="Да"
                          unCheckedChildren="Нет"
                                size="default"
                        />
                              <Text className="ml-2" type="secondary">
                                {configData.run.goExecutor?.cancelOnError ? 'Остановка при ошибке' : 'Продолжать при ошибках'}
                              </Text>
                      </div>
                    </Form.Item>
                        </Col>
                      </Row>
                    </TabPane>

                    <TabPane 
                      tab={
                        <span>
                          <CodeOutlined />
                          K6 Executor
                        </span>
                      } 
                      key="k6"
                    >
                      <Row gutter={[24, 16]}>
                        <Col xs={24} md={12}>
                      <Form.Item
                        label="Путь к k6 бинарному файлу"
                        required
                        tooltip="Путь к исполняемому файлу k6"
                            rules={[{ required: true, message: 'Укажите путь к k6!' }]}
                      >
                        <Input
                          value={configData.run.k6Executor?.k6BinaryPath || ''}
                          onChange={(e) => handleK6ExecutorChange('k6BinaryPath', e.target.value)}
                          placeholder="/usr/local/bin/k6"
                              prefix={<CodeOutlined />}
                        />
                      </Form.Item>
                        </Col>

                        <Col xs={24} md={12}>
                      <Form.Item
                        label="Путь к k6 скрипту"
                        required
                        tooltip="Путь к JavaScript файлу с тестом k6"
                            rules={[{ required: true, message: 'Укажите путь к скрипту!' }]}
                      >
                        <Input
                          value={configData.run.k6Executor?.k6ScriptPath || ''}
                          onChange={(e) => handleK6ExecutorChange('k6ScriptPath', e.target.value)}
                          placeholder="/path/to/test.js"
                              prefix={<CodeOutlined />}
                        />
                      </Form.Item>
                        </Col>
                      </Row>

                      <Divider>Настройки выполнения</Divider>

                      <Row gutter={[24, 16]}>
                        <Col xs={24} sm={12} md={8}>
                      <Form.Item
                            label="Timeout настройки"
                            tooltip="Время ожидания для фазы настройки k6 (секунды)"
                      >
                        <InputNumber
                          value={configData.run.k6Executor?.k6SetupTimeout ? parseInt(configData.run.k6Executor.k6SetupTimeout) : undefined}
                          onChange={(value) => handleK6ExecutorChange('k6SetupTimeout', value?.toString())}
                          placeholder="30"
                          min={1}
                          className="w-full"
                              addonAfter="сек"
                        />
                      </Form.Item>
                        </Col>

                        <Col xs={24} sm={12} md={8}>
                      <Form.Item
                            label="Длительность теста"
                            tooltip="Общая длительность выполнения теста (секунды)"
                      >
                        <InputNumber
                          value={configData.run.k6Executor?.k6Duration ? parseInt(configData.run.k6Executor.k6Duration) : undefined}
                          onChange={(value) => handleK6ExecutorChange('k6Duration', value?.toString())}
                          placeholder="300"
                          min={1}
                          className="w-full"
                              addonAfter="сек"
                        />
                      </Form.Item>
                        </Col>

                        <Col xs={24} sm={12} md={8}>
                          <Form.Item
                            label="Запросов в секунду"
                            tooltip="Скорость выполнения запросов"
                          >
                            <InputNumber
                              value={configData.run.k6Executor?.k6Rate ? parseInt(configData.run.k6Executor.k6Rate) : undefined}
                              onChange={(value) => handleK6ExecutorChange('k6Rate', value?.toString())}
                              placeholder="100"
                              min={1}
                              className="w-full"
                              addonAfter="rps"
                            />
                          </Form.Item>
                        </Col>
                      </Row>

                      <Row gutter={[24, 16]}>
                        <Col xs={24} md={12}>
                      <Form.Item
                        label="Виртуальные пользователи"
                        tooltip="Количество одновременных пользователей для нагрузочного тестирования"
                      >
                        <InputNumber
                          value={configData.run.k6Executor?.k6Vus ? parseInt(configData.run.k6Executor.k6Vus) : undefined}
                          onChange={(value) => handleK6ExecutorChange('k6Vus', value?.toString())}
                          placeholder="10"
                          min={1}
                          className="w-full"
                              addonAfter="VUs"
                        />
                      </Form.Item>
                        </Col>

                        <Col xs={24} md={12}>
                      <Form.Item
                            label="Максимум VU"
                        tooltip="Максимальное количество виртуальных пользователей"
                      >
                        <InputNumber
                          value={configData.run.k6Executor?.k6MaxVus ? parseInt(configData.run.k6Executor.k6MaxVus) : undefined}
                          onChange={(value) => handleK6ExecutorChange('k6MaxVus', value?.toString())}
                          placeholder="20"
                          min={1}
                          className="w-full"
                              addonAfter="VUs"
                        />
                      </Form.Item>
                        </Col>
                      </Row>

                    <Form.Item
                      label="Дополнительные аргументы k6"
                      tooltip="Дополнительные аргументы для передачи k6 бинарному файлу"
                      >
                        <div className="space-y-2">
                          {(configData.run.k6Executor?.k6BinaryArgs || []).length === 0 ? (
                            <Empty
                              image={Empty.PRESENTED_IMAGE_SIMPLE}
                              description="Нет дополнительных аргументов"
                            />
                          ) : (
                            (configData.run.k6Executor?.k6BinaryArgs || []).map((arg, index) => (
                              <div key={index} className="flex items-center space-x-2">
                            <Input
                              value={arg}
                              onChange={(e) => updateK6Arg(index, e.target.value)}
                              placeholder={`Аргумент ${index + 1}`}
                                  prefix={<CodeOutlined />}
                            />
                            <Button
                              type="text"
                              danger
                              icon={<DeleteOutlined />}
                              onClick={() => removeK6Arg(index)}
                            />
                          </div>
                            ))
                          )}
                        <Button
                          type="dashed"
                          icon={<PlusOutlined />}
                          onClick={addK6Arg}
                          className="w-full"
                            size="large"
                        >
                          Добавить аргумент
                        </Button>
                      </div>
                    </Form.Item>
                    </TabPane>
                  </Tabs>
                  </div>
              )}

              {/* Step 3: Шаги и операции */}
              {currentStep === 3 && (
                <div>
                  <div className="flex justify-between items-center mb-6">
                    <div>
                      <Title level={4} className="mb-2">
                        <CodeOutlined className="mr-2" />
                        Шаги бенчмарка
                      </Title>
                      <Text type="secondary">
                        Создайте последовательность операций для тестирования базы данных
                      </Text>
                    </div>
                    <div className="flex items-center space-x-3">
                      <Badge count={configData.benchmark.steps.length} showZero color="#52c41a">
                        <Tag color="green">Всего шагов</Tag>
                      </Badge>
                      <Badge 
                        count={configData.benchmark.steps.reduce((acc, step) => acc + (step.units?.length || 0), 0)} 
                        showZero 
                        color="#1890ff"
                      >
                        <Tag color="blue">Всего операций</Tag>
                      </Badge>
                      <Dropdown
                        menu={{
                          items: [
                            {
                              key: 'add-step',
                              label: 'Добавить шаг',
                              icon: <PlusOutlined />,
                              onClick: addBenchmarkStep
                            },
                            {
                              key: 'clear-all',
                              label: 'Очистить все',
                              icon: <DeleteOutlined />,
                              danger: true,
                              onClick: () => handleBenchmarkChange('steps', [])
                            }
                          ]
                        }}
                      >
                        <Button type="primary" icon={<MoreOutlined />}>
                          Действия
                        </Button>
                      </Dropdown>
                    </div>
                  </div>

                  {configData.benchmark.steps.length === 0 ? (
                    <Card className="text-center py-12">
                      <Empty
                        image={Empty.PRESENTED_IMAGE_SIMPLE}
                        description={
                          <div>
                            <Title level={4} type="secondary">Нет шагов бенчмарка</Title>
                            <Text type="secondary">
                              Создайте первый шаг для начала настройки тестирования
                            </Text>
                          </div>
                        }
                      >
                        <Button 
                          type="primary" 
                          size="large"
                          icon={<PlusOutlined />} 
                          onClick={addBenchmarkStep}
                        >
                          Создать первый шаг
                        </Button>
                      </Empty>
                    </Card>
                  ) : (
                    <div className="space-y-6">
                      {configData.benchmark.steps.map((step, stepIndex) => (
                        <Card 
                          key={stepIndex} 
                          className="border-l-4 border-l-green-400 hover:shadow-lg transition-all duration-200"
                          title={
                            <div className="flex items-center justify-between">
                              <div className="flex items-center space-x-3">
                                <div className="flex items-center justify-center w-8 h-8 bg-green-100 rounded-full text-green-600 font-medium">
                                  {stepIndex + 1}
                                </div>
                                <div className="flex-1">
                                  <Input
                                    value={step.name}
                                    onChange={(e) => {
                                      const newSteps = [...configData.benchmark.steps]
                                      newSteps[stepIndex] = { ...newSteps[stepIndex], name: e.target.value }
                                      handleBenchmarkChange('steps', newSteps)
                                    }}
                                    placeholder="Название шага бенчмарка"
                                    bordered={false}
                                    className="font-medium text-lg"
                                    style={{ padding: 0 }}
                                  />
                                </div>
                              </div>
                              <div className="flex items-center space-x-3">
                                <Tooltip title="Тип выполнения">
                                  <Switch
                                    checked={step.async}
                                    onChange={(checked) => {
                                      const newSteps = [...configData.benchmark.steps]
                                      newSteps[stepIndex] = { ...newSteps[stepIndex], async: checked }
                                      handleBenchmarkChange('steps', newSteps)
                                    }}
                                    checkedChildren="Асинхронно"
                                    unCheckedChildren="Синхронно"
                                  />
                                </Tooltip>
                                <Badge 
                                  count={step.units?.length || 0} 
                                  showZero 
                                  color="#52c41a"
                                  title="Количество операций"
                                />
                                <Dropdown
                                  menu={{
                                    items: [
                                      {
                                        key: 'copy-step',
                                        label: 'Дублировать шаг',
                                        icon: <CopyOutlined />,
                                        onClick: () => {
                                          const newStep = { ...step, name: `${step.name} (копия)` }
                                          const newSteps = [...configData.benchmark.steps]
                                          newSteps.splice(stepIndex + 1, 0, newStep)
                                          handleBenchmarkChange('steps', newSteps)
                                        }
                                      },
                                      {
                                        key: 'delete-step',
                                        label: 'Удалить шаг',
                                        icon: <DeleteOutlined />,
                                        danger: true,
                                        onClick: () => removeBenchmarkStep(stepIndex)
                                      }
                                    ]
                                  }}
                                >
                                  <Button type="text" icon={<MoreOutlined />} />
                                </Dropdown>
                              </div>
                            </div>
                          }
                          extra={
                            <Dropdown
                              menu={{
                                items: [
                                  {
                                    key: 'query',
                                    label: 'SQL Query',
                                    icon: <CodeOutlined />,
                                    onClick: () => addOperation(stepIndex, 'query')
                                  },
                                  {
                                    key: 'table',
                                    label: 'Create Table',
                                    icon: <DatabaseOutlined />,
                                    onClick: () => addOperation(stepIndex, 'createTable')
                                  },
                                  {
                                    key: 'transaction',
                                    label: 'Transaction',
                                    icon: <ThunderboltOutlined />,
                                    onClick: () => addOperation(stepIndex, 'transaction')
                                  }
                                ]
                              }}
                            >
                              <Button type="primary" icon={<PlusOutlined />}>
                                Добавить операцию
                              </Button>
                            </Dropdown>
                          }
                        >
                          {step.units && step.units.length > 0 ? (
                            <div className="space-y-6">
                              {step.units.map((operation, operationIndex) => (
                                <Card 
                                  key={operation.id} 
                                  className="border-l-4"
                                  style={{
                                    borderLeftColor: 
                                      operation.type === 'query' ? '#1890ff' :
                                      operation.type === 'createTable' ? '#52c41a' : '#722ed1'
                                  }}
                                  title={
                                    <div className="flex items-center justify-between">
                                      <div className="flex items-center space-x-3">
                                        <Tag
                                          color={
                                            operation.type === 'query' ? 'blue' :
                                            operation.type === 'createTable' ? 'green' : 'purple'
                                          }
                                          icon={
                                            operation.type === 'query' ? <CodeOutlined /> :
                                            operation.type === 'createTable' ? <DatabaseOutlined /> : <ThunderboltOutlined />
                                          }
                                        >
                                          {operation.type === 'query' ? 'SQL Query' :
                                           operation.type === 'createTable' ? 'Create Table' : 'Transaction'}
                                        </Tag>
                                        <Input
                                          value={operation.id}
                                          onChange={(e) => {
                                            const updatedOperation = { ...operation, id: e.target.value }
                                            updateOperation(stepIndex, operationIndex, updatedOperation)
                                          }}
                                          placeholder="ID операции"
                                          bordered={false}
                                          className="font-medium"
                                          style={{ padding: 0, fontSize: '16px' }}
                                        />
                                      </div>
                                      <div className="flex items-center space-x-3">
                                        <div className="flex items-center space-x-2">
                                          <Text type="secondary">Асинхронно:</Text>
                                          <Switch
                                            checked={operation.async}
                                            onChange={(checked) => {
                                              const updatedOperation = { ...operation, async: checked }
                                              updateOperation(stepIndex, operationIndex, updatedOperation)
                                            }}
                                            size="small"
                                          />
                                        </div>
                                        <Dropdown
                                          menu={{
                                            items: [
                                              {
                                                key: 'copy',
                                                label: 'Дублировать',
                                                icon: <CopyOutlined />,
                                                onClick: () => duplicateOperation(stepIndex, operationIndex, operation)
                                              },
                                              {
                                                key: 'delete',
                                                label: 'Удалить',
                                                icon: <DeleteOutlined />,
                                                danger: true,
                                                onClick: () => removeOperation(stepIndex, operationIndex)
                                              }
                                            ]
                                          }}
                                        >
                                          <Button type="text" icon={<MoreOutlined />} />
                                        </Dropdown>
                                      </div>
                                    </div>
                                  }
                                >
                                  {/* SQL Query Editor */}
                                  {operation.type === 'query' && (
                                    <div className="space-y-4">
                                      <Row gutter={[16, 16]}>
                                        <Col span={12}>
                                          <Form.Item label="Название запроса" className="mb-3">
                                            <Input
                                              value={operation.query?.name || ''}
                                              onChange={(e) => {
                                                const updatedOperation = {
                                                  ...operation,
                                                  query: { ...operation.query, name: e.target.value }
                                                }
                                                updateOperation(stepIndex, operationIndex, updatedOperation)
                                              }}
                                              placeholder="get_user_by_id"
                                              prefix={<CodeOutlined />}
                                            />
                                          </Form.Item>
                                        </Col>
                                        <Col span={12}>
                                          <Form.Item label="Количество выполнений" className="mb-3">
                                            <InputNumber
                                              value={parseInt(operation.query?.count || '1')}
                                              onChange={(value) => {
                                                const updatedOperation = {
                                                  ...operation,
                                                  query: { ...operation.query, count: value?.toString() || '1' }
                                                }
                                                updateOperation(stepIndex, operationIndex, updatedOperation)
                                              }}
                                              min={1}
                                              className="w-full"
                                              placeholder="1"
                                            />
                                          </Form.Item>
                                        </Col>
                                      </Row>
                                      
                                      <Form.Item label="SQL запрос" className="mb-3">
                                        <Input.TextArea
                                          value={operation.query?.sql || ''}
                                          onChange={(e) => {
                                            const updatedOperation = {
                                              ...operation,
                                              query: { ...operation.query, sql: e.target.value }
                                            }
                                            updateOperation(stepIndex, operationIndex, updatedOperation)
                                          }}
                                          rows={4}
                                          placeholder="SELECT * FROM users WHERE id = ${user_id}"
                                        />
                                      </Form.Item>

                                      {/* Parameters */}
                                      <div>
                                        <div className="flex justify-between items-center mb-3">
                                          <Text strong>Параметры запроса</Text>
                                          <Button
                                            type="dashed"
                                            size="small"
                                            icon={<PlusOutlined />}
                                            onClick={() => {
                                              const newParam = {
                                                name: `param_${Date.now()}`,
                                                replaceRegex: '',
                                                generationRule: {
                                                  type: 'string' as const,
                                                  length: 10
                                                }
                                              }
                                              const updatedOperation = {
                                                ...operation,
                                                query: {
                                                  ...operation.query,
                                                  params: [...(operation.query?.params || []), newParam]
                                                }
                                              }
                                              updateOperation(stepIndex, operationIndex, updatedOperation)
                                            }}
                                          >
                                            Добавить параметр
                                          </Button>
                                        </div>
                                        
                                        {operation.query?.params && operation.query.params.length > 0 ? (
                                          <div className="space-y-3">
                                            {operation.query.params.map((param, paramIndex) => (
                                              <Card key={paramIndex} size="small" className="bg-gray-50">
                                                <Row gutter={[12, 12]}>
                                                  <Col span={8}>
                                                    <Form.Item label="Имя параметра" className="mb-2">
                                                      <Input
                                                        value={param.name}
                                                        onChange={(e) => {
                                                          const newParams = [...(operation.query?.params || [])]
                                                          newParams[paramIndex] = { ...param, name: e.target.value }
                                                          const updatedOperation = {
                                                            ...operation,
                                                            query: { ...operation.query, params: newParams }
                                                          }
                                                          updateOperation(stepIndex, operationIndex, updatedOperation)
                                                        }}
                                                        placeholder="user_id"
                                                        size="small"
                                                      />
                                                    </Form.Item>
                                                  </Col>
                                                  <Col span={8}>
                                                    <Form.Item label="Regex замены" className="mb-2">
                                                      <Input
                                                        value={param.replaceRegex}
                                                        onChange={(e) => {
                                                          const newParams = [...(operation.query?.params || [])]
                                                          newParams[paramIndex] = { ...param, replaceRegex: e.target.value }
                                                          const updatedOperation = {
                                                            ...operation,
                                                            query: { ...operation.query, params: newParams }
                                                          }
                                                          updateOperation(stepIndex, operationIndex, updatedOperation)
                                                        }}
                                                        placeholder="${user_id}"
                                                        size="small"
                                                      />
                                                    </Form.Item>
                                                  </Col>
                                                  <Col span={6}>
                                                    <Form.Item label="Тип данных" className="mb-2">
                                                      <Select
                                                        value={param.generationRule.type}
                                                        onChange={(value) => {
                                                          const newParams = [...(operation.query?.params || [])]
                                                          newParams[paramIndex] = {
                                                            ...param,
                                                            generationRule: { ...param.generationRule, type: value }
                                                          }
                                                          const updatedOperation = {
                                                            ...operation,
                                                            query: { ...operation.query, params: newParams }
                                                          }
                                                          updateOperation(stepIndex, operationIndex, updatedOperation)
                                                        }}
                                                        size="small"
                                                        className="w-full"
                                                      >
                                                        <Option value="string">
                                                          <div className="pl-3">String</div>
                                                        </Option>
                                                        <Option value="int">
                                                          <div className="pl-3">Integer</div>
                                                        </Option>
                                                        <Option value="float">
                                                          <div className="pl-3">Float</div>
                                                        </Option>
                                                        <Option value="bool">
                                                          <div className="pl-3">Boolean</div>
                                                        </Option>
                                                        <Option value="uuid">
                                                          <div className="pl-3">UUID</div>
                                                        </Option>
                                                        <Option value="datetime">
                                                          <div className="pl-3">DateTime</div>
                                                        </Option>
                                                        <Option value="decimal">
                                                          <div className="pl-3">Decimal</div>
                                                        </Option>
                                                      </Select>
                                                    </Form.Item>
                                                  </Col>
                                                  <Col span={2}>
                                                    <Form.Item label=" " className="mb-2">
                                                      <Button
                                                        type="text"
                                                        danger
                                                        size="small"
                                                        icon={<DeleteOutlined />}
                                                        onClick={() => {
                                                          const newParams = (operation.query?.params || []).filter((_, i) => i !== paramIndex)
                                                          const updatedOperation = {
                                                            ...operation,
                                                            query: { ...operation.query, params: newParams }
                                                          }
                                                          updateOperation(stepIndex, operationIndex, updatedOperation)
                                                        }}
                                                      />
                                                    </Form.Item>
                                                  </Col>
                                                </Row>
                                                
                                                {/* Generation Rule Details */}
                                                <Row gutter={[12, 12]}>
                                                  {(param.generationRule.type === 'string' || param.generationRule.type === 'int' || param.generationRule.type === 'float') && (
                                                    <>
                                                      {param.generationRule.type === 'string' && (
                                                        <Col span={8}>
                                                          <Form.Item label="Длина" className="mb-2">
                                                            <InputNumber
                                                              value={param.generationRule.length}
                                                              onChange={(value) => {
                                                                const newParams = [...(operation.query?.params || [])]
                                                                newParams[paramIndex] = {
                                                                  ...param,
                                                                  generationRule: { ...param.generationRule, length: value || 10 }
                                                                }
                                                                const updatedOperation = {
                                                                  ...operation,
                                                                  query: { ...operation.query, params: newParams }
                                                                }
                                                                updateOperation(stepIndex, operationIndex, updatedOperation)
                                                              }}
                                                              min={1}
                                                              size="small"
                                                              className="w-full"
                                                            />
                                                          </Form.Item>
                                                        </Col>
                                                      )}
                                                      {(param.generationRule.type === 'int' || param.generationRule.type === 'float') && (
                                                        <>
                                                          <Col span={8}>
                                                            <Form.Item label="Минимум" className="mb-2">
                                                              <InputNumber
                                                                value={param.generationRule.min}
                                                                onChange={(value) => {
                                                                  const newParams = [...(operation.query?.params || [])]
                                                                  newParams[paramIndex] = {
                                                                    ...param,
                                                                    generationRule: { ...param.generationRule, min: value || 0 }
                                                                  }
                                                                  const updatedOperation = {
                                                                    ...operation,
                                                                    query: { ...operation.query, params: newParams }
                                                                  }
                                                                  updateOperation(stepIndex, operationIndex, updatedOperation)
                                                                }}
                                                                size="small"
                                                                className="w-full"
                                                              />
                                                            </Form.Item>
                                                          </Col>
                                                          <Col span={8}>
                                                            <Form.Item label="Максимум" className="mb-2">
                                                              <InputNumber
                                                                value={param.generationRule.max}
                                                                onChange={(value) => {
                                                                  const newParams = [...(operation.query?.params || [])]
                                                                  newParams[paramIndex] = {
                                                                    ...param,
                                                                    generationRule: { ...param.generationRule, max: value || 100 }
                                                                  }
                                                                  const updatedOperation = {
                                                                    ...operation,
                                                                    query: { ...operation.query, params: newParams }
                                                                  }
                                                                  updateOperation(stepIndex, operationIndex, updatedOperation)
                                                                }}
                                                                size="small"
                                                                className="w-full"
                                                              />
                                                            </Form.Item>
                                                          </Col>
                                                        </>
                                                      )}
                                                    </>
                                                  )}
                                                </Row>
                                              </Card>
                                            ))}
                                          </div>
                                        ) : (
                                          <Empty 
                                            image={Empty.PRESENTED_IMAGE_SIMPLE}
                                            description="Нет параметров"
                                            className="py-4"
                                          />
                                        )}
                                      </div>
                                    </div>
                                  )}

                                  {/* Create Table Editor */}
                                  {operation.type === 'createTable' && (
                                    <div className="space-y-4">
                                      <Row gutter={[16, 16]}>
                                        <Col span={12}>
                                          <Form.Item label="Название таблицы" className="mb-3">
                                            <Input
                                              value={operation.createTable?.name || ''}
                                              onChange={(e) => {
                                                const updatedOperation = {
                                                  ...operation,
                                                  createTable: { ...operation.createTable, name: e.target.value }
                                                }
                                                updateOperation(stepIndex, operationIndex, updatedOperation)
                                              }}
                                              placeholder="users"
                                              prefix={<DatabaseOutlined />}
                                            />
                                          </Form.Item>
                                        </Col>
                                        <Col span={12}>
                                          <Form.Item label="Ограничения таблицы" className="mb-3">
                                            <Input
                                              value={operation.createTable?.constraint || ''}
                                              onChange={(e) => {
                                                const updatedOperation = {
                                                  ...operation,
                                                  createTable: { ...operation.createTable, constraint: e.target.value }
                                                }
                                                updateOperation(stepIndex, operationIndex, updatedOperation)
                                              }}
                                              placeholder="PRIMARY KEY (id)"
                                            />
                                          </Form.Item>
                                        </Col>
                                      </Row>

                                      {/* Columns */}
                                      <div>
                                        <div className="flex justify-between items-center mb-3">
                                          <Text strong>Колонки таблицы</Text>
                                          <Button
                                            type="dashed"
                                            size="small"
                                            icon={<PlusOutlined />}
                                            onClick={() => {
                                              const newColumn = {
                                                name: `column_${Date.now()}`,
                                                sqlType: 'VARCHAR(255)',
                                                nullable: true,
                                                primaryKey: false
                                              }
                                              const updatedOperation = {
                                                ...operation,
                                                createTable: {
                                                  ...operation.createTable,
                                                  columns: [...(operation.createTable?.columns || []), newColumn]
                                                }
                                              }
                                              updateOperation(stepIndex, operationIndex, updatedOperation)
                                            }}
                                          >
                                            Добавить колонку
                                          </Button>
                                        </div>
                                        
                                        {operation.createTable?.columns && operation.createTable.columns.length > 0 ? (
                                          <div className="space-y-3">
                                            {operation.createTable.columns.map((column, columnIndex) => (
                                              <Card key={columnIndex} size="small" className="bg-green-50">
                                                <Row gutter={[12, 12]}>
                                                  <Col span={6}>
                                                    <Form.Item label="Имя колонки" className="mb-2">
                                                      <Input
                                                        value={column.name}
                                                        onChange={(e) => {
                                                          const newColumns = [...(operation.createTable?.columns || [])]
                                                          newColumns[columnIndex] = { ...column, name: e.target.value }
                                                          const updatedOperation = {
                                                            ...operation,
                                                            createTable: { ...operation.createTable, columns: newColumns }
                                                          }
                                                          updateOperation(stepIndex, operationIndex, updatedOperation)
                                                        }}
                                                        placeholder="id"
                                                        size="small"
                                                      />
                                                    </Form.Item>
                                                  </Col>
                                                  <Col span={6}>
                                                    <Form.Item label="Тип SQL" className="mb-2">
                                                      <Input
                                                        value={column.sqlType}
                                                        onChange={(e) => {
                                                          const newColumns = [...(operation.createTable?.columns || [])]
                                                          newColumns[columnIndex] = { ...column, sqlType: e.target.value }
                                                          const updatedOperation = {
                                                            ...operation,
                                                            createTable: { ...operation.createTable, columns: newColumns }
                                                          }
                                                          updateOperation(stepIndex, operationIndex, updatedOperation)
                                                        }}
                                                        placeholder="VARCHAR(255)"
                                                        size="small"
                                                      />
                                                    </Form.Item>
                                                  </Col>
                                                  <Col span={4}>
                                                    <Form.Item label="Значение по умолчанию" className="mb-2">
                                                      <Input
                                                        value={column.defaultValue || ''}
                                                        onChange={(e) => {
                                                          const newColumns = [...(operation.createTable?.columns || [])]
                                                          newColumns[columnIndex] = { ...column, defaultValue: e.target.value }
                                                          const updatedOperation = {
                                                            ...operation,
                                                            createTable: { ...operation.createTable, columns: newColumns }
                                                          }
                                                          updateOperation(stepIndex, operationIndex, updatedOperation)
                                                        }}
                                                        placeholder="NULL"
                                                        size="small"
                                                      />
                                                    </Form.Item>
                                                  </Col>
                                                  <Col span={6}>
                                                    <Form.Item label="Ограничения" className="mb-2">
                                                      <Input
                                                        value={column.constraint || ''}
                                                        onChange={(e) => {
                                                          const newColumns = [...(operation.createTable?.columns || [])]
                                                          newColumns[columnIndex] = { ...column, constraint: e.target.value }
                                                          const updatedOperation = {
                                                            ...operation,
                                                            createTable: { ...operation.createTable, columns: newColumns }
                                                          }
                                                          updateOperation(stepIndex, operationIndex, updatedOperation)
                                                        }}
                                                        placeholder="NOT NULL"
                                                        size="small"
                                                      />
                                                    </Form.Item>
                                                  </Col>
                                                  <Col span={2}>
                                                    <Form.Item label=" " className="mb-2">
                                                      <Button
                                                        type="text"
                                                        danger
                                                        size="small"
                                                        icon={<DeleteOutlined />}
                                                        onClick={() => {
                                                          const newColumns = (operation.createTable?.columns || []).filter((_, i) => i !== columnIndex)
                                                          const updatedOperation = {
                                                            ...operation,
                                                            createTable: { ...operation.createTable, columns: newColumns }
                                                          }
                                                          updateOperation(stepIndex, operationIndex, updatedOperation)
                                                        }}
                                                      />
                                                    </Form.Item>
                                                  </Col>
                                                </Row>
                                                
                                                <Row gutter={[12, 12]}>
                                                  <Col span={8}>
                                                    <div className="flex items-center space-x-4">
                                                      <div className="flex items-center space-x-2">
                                                        <Switch
                                                          checked={column.primaryKey || false}
                                                          onChange={(checked) => {
                                                            const newColumns = [...(operation.createTable?.columns || [])]
                                                            newColumns[columnIndex] = { ...column, primaryKey: checked }
                                                            const updatedOperation = {
                                                              ...operation,
                                                              createTable: { ...operation.createTable, columns: newColumns }
                                                            }
                                                            updateOperation(stepIndex, operationIndex, updatedOperation)
                                                          }}
                                                          size="small"
                                                        />
                                                        <Text type="secondary" className="text-xs">Primary Key</Text>
                                                      </div>
                                                      <div className="flex items-center space-x-2">
                                                        <Switch
                                                          checked={column.nullable}
                                                          onChange={(checked) => {
                                                            const newColumns = [...(operation.createTable?.columns || [])]
                                                            newColumns[columnIndex] = { ...column, nullable: checked }
                                                            const updatedOperation = {
                                                              ...operation,
                                                              createTable: { ...operation.createTable, columns: newColumns }
                                                            }
                                                            updateOperation(stepIndex, operationIndex, updatedOperation)
                                                          }}
                                                          size="small"
                                                        />
                                                        <Text type="secondary" className="text-xs">Nullable</Text>
                                                      </div>
                                                    </div>
                                                  </Col>
                                                </Row>
                                              </Card>
                                            ))}
                                          </div>
                                        ) : (
                                          <Empty 
                                            image={Empty.PRESENTED_IMAGE_SIMPLE}
                                            description="Нет колонок"
                                            className="py-4"
                                          />
                                        )}
                                      </div>

                                      {/* Table Indexes */}
                                      <div>
                                        <div className="flex justify-between items-center mb-3">
                                          <Text strong>Индексы таблицы</Text>
                                          <Button
                                            type="dashed"
                                            size="small"
                                            icon={<PlusOutlined />}
                                            onClick={() => {
                                              const newIndex = {
                                                name: `idx_${Date.now()}`,
                                                columns: [],
                                                unique: false
                                              }
                                              const updatedOperation = {
                                                ...operation,
                                                createTable: {
                                                  ...operation.createTable,
                                                  tableIndexes: [...(operation.createTable?.tableIndexes || []), newIndex]
                                                }
                                              }
                                              updateOperation(stepIndex, operationIndex, updatedOperation)
                                            }}
                                          >
                                            Добавить индекс
                                          </Button>
                                        </div>
                                        
                                        {operation.createTable?.tableIndexes && operation.createTable.tableIndexes.length > 0 ? (
                                          <div className="space-y-3">
                                            {operation.createTable.tableIndexes.map((index, indexIndex) => (
                                              <Card key={indexIndex} size="small" className="bg-blue-50">
                                                <Row gutter={[12, 12]}>
                                                  <Col span={8}>
                                                    <Form.Item label="Имя индекса" className="mb-2">
                                                      <Input
                                                        value={index.name}
                                                        onChange={(e) => {
                                                          const newIndexes = [...(operation.createTable?.tableIndexes || [])]
                                                          newIndexes[indexIndex] = { ...index, name: e.target.value }
                                                          const updatedOperation = {
                                                            ...operation,
                                                            createTable: { ...operation.createTable, tableIndexes: newIndexes }
                                                          }
                                                          updateOperation(stepIndex, operationIndex, updatedOperation)
                                                        }}
                                                        placeholder="idx_user_email"
                                                        size="small"
                                                      />
                                                    </Form.Item>
                                                  </Col>
                                                  <Col span={10}>
                                                    <Form.Item label="Колонки (через запятую)" className="mb-2">
                                                      <Input
                                                        value={index.columns.join(', ')}
                                                        onChange={(e) => {
                                                          const newIndexes = [...(operation.createTable?.tableIndexes || [])]
                                                          newIndexes[indexIndex] = { 
                                                            ...index, 
                                                            columns: e.target.value.split(',').map(col => col.trim()).filter(col => col)
                                                          }
                                                          const updatedOperation = {
                                                            ...operation,
                                                            createTable: { ...operation.createTable, tableIndexes: newIndexes }
                                                          }
                                                          updateOperation(stepIndex, operationIndex, updatedOperation)
                                                        }}
                                                        placeholder="email, created_at"
                                                        size="small"
                                                      />
                                                    </Form.Item>
                                                  </Col>
                                                  <Col span={4}>
                                                    <Form.Item label="Уникальный" className="mb-2">
                                                      <Switch
                                                        checked={index.unique}
                                                        onChange={(checked) => {
                                                          const newIndexes = [...(operation.createTable?.tableIndexes || [])]
                                                          newIndexes[indexIndex] = { ...index, unique: checked }
                                                          const updatedOperation = {
                                                            ...operation,
                                                            createTable: { ...operation.createTable, tableIndexes: newIndexes }
                                                          }
                                                          updateOperation(stepIndex, operationIndex, updatedOperation)
                                                        }}
                                                        size="small"
                                                      />
                                                    </Form.Item>
                                                  </Col>
                                                  <Col span={2}>
                                                    <Form.Item label=" " className="mb-2">
                                                      <Button
                                                        type="text"
                                                        danger
                                                        size="small"
                                                        icon={<DeleteOutlined />}
                                                        onClick={() => {
                                                          const newIndexes = (operation.createTable?.tableIndexes || []).filter((_, i) => i !== indexIndex)
                                                          const updatedOperation = {
                                                            ...operation,
                                                            createTable: { ...operation.createTable, tableIndexes: newIndexes }
                                                          }
                                                          updateOperation(stepIndex, operationIndex, updatedOperation)
                                                        }}
                                                      />
                                                    </Form.Item>
                                                  </Col>
                                                </Row>
                                              </Card>
                                            ))}
                                          </div>
                                        ) : (
                                          <Empty 
                                            image={Empty.PRESENTED_IMAGE_SIMPLE}
                                            description="Нет индексов"
                                            className="py-4"
                                          />
                                        )}
                                      </div>
                                    </div>
                                  )}

                                  {/* Transaction Editor */}
                                  {operation.type === 'transaction' && (
                                    <div className="space-y-4">
                                      <Row gutter={[16, 16]}>
                                        <Col span={8}>
                                          <Form.Item label="Название транзакции" className="mb-3">
                                            <Input
                                              value={operation.transaction?.name || ''}
                                              onChange={(e) => {
                                                const updatedOperation = {
                                                  ...operation,
                                                  transaction: { ...operation.transaction, name: e.target.value }
                                                }
                                                updateOperation(stepIndex, operationIndex, updatedOperation)
                                              }}
                                              placeholder="user_transaction"
                                              prefix={<ThunderboltOutlined />}
                                            />
                                          </Form.Item>
                                        </Col>
                                        <Col span={8}>
                                          <Form.Item label="Уровень изоляции" className="mb-3">
                                            <Select
                                              value={operation.transaction?.isolationLevel || 'TX_ISOLATION_LEVEL_READ_COMMITTED'}
                                              onChange={(value) => {
                                                const updatedOperation = {
                                                  ...operation,
                                                  transaction: { ...operation.transaction, isolationLevel: value }
                                                }
                                                updateOperation(stepIndex, operationIndex, updatedOperation)
                                              }}
                                              className="w-full"
                                            >
                                              <Option value="TX_ISOLATION_LEVEL_READ_COMMITTED">
                                                <div className="pl-3">Read Committed</div>
                                              </Option>
                                              <Option value="TX_ISOLATION_LEVEL_REPEATABLE_READ">
                                                <div className="pl-3">Repeatable Read</div>
                                              </Option>
                                              <Option value="TX_ISOLATION_LEVEL_SERIALIZABLE">
                                                <div className="pl-3">Serializable</div>
                                              </Option>
                                            </Select>
                                          </Form.Item>
                                        </Col>
                                        <Col span={8}>
                                          <Form.Item label="Количество выполнений" className="mb-3">
                                            <InputNumber
                                              value={parseInt(operation.transaction?.count || '1')}
                                              onChange={(value) => {
                                                const updatedOperation = {
                                                  ...operation,
                                                  transaction: { ...operation.transaction, count: value?.toString() || '1' }
                                                }
                                                updateOperation(stepIndex, operationIndex, updatedOperation)
                                              }}
                                              min={1}
                                              className="w-full"
                                              placeholder="1"
                                            />
                                          </Form.Item>
                                        </Col>
                                      </Row>

                                      {/* Transaction Queries */}
                                      <div>
                                        <div className="flex justify-between items-center mb-3">
                                          <Text strong>Запросы в транзакции</Text>
                                          <Button
                                            type="dashed"
                                            size="small"
                                            icon={<PlusOutlined />}
                                            onClick={() => {
                                              const newQuery = {
                                                name: `query_${Date.now()}`,
                                                sql: '',
                                                count: '1',
                                                params: []
                                              }
                                              const updatedOperation = {
                                                ...operation,
                                                transaction: {
                                                  ...operation.transaction,
                                                  queries: [...(operation.transaction?.queries || []), newQuery]
                                                }
                                              }
                                              updateOperation(stepIndex, operationIndex, updatedOperation)
                                            }}
                                          >
                                            Добавить запрос
                                          </Button>
                                        </div>
                                        
                                        {operation.transaction?.queries && operation.transaction.queries.length > 0 ? (
                                          <div className="space-y-4">
                                            {operation.transaction.queries.map((query, queryIndex) => (
                                              <Card key={queryIndex} size="small" className="bg-purple-50">
                                                <div className="flex justify-between items-center mb-3">
                                                  <Text strong className="text-purple-600">Запрос #{queryIndex + 1}</Text>
                                                  <Button
                                                    type="text"
                                                    danger
                                                    size="small"
                                                    icon={<DeleteOutlined />}
                                                    onClick={() => {
                                                      const newQueries = (operation.transaction?.queries || []).filter((_, i) => i !== queryIndex)
                                                      const updatedOperation = {
                                                        ...operation,
                                                        transaction: { ...operation.transaction, queries: newQueries }
                                                      }
                                                      updateOperation(stepIndex, operationIndex, updatedOperation)
                                                    }}
                                                  />
                                                </div>
                                                
                                                <Row gutter={[12, 12]}>
                                                  <Col span={12}>
                                                    <Form.Item label="Название запроса" className="mb-2">
                                                      <Input
                                                        value={query.name}
                                                        onChange={(e) => {
                                                          const newQueries = [...(operation.transaction?.queries || [])]
                                                          newQueries[queryIndex] = { ...query, name: e.target.value }
                                                          const updatedOperation = {
                                                            ...operation,
                                                            transaction: { ...operation.transaction, queries: newQueries }
                                                          }
                                                          updateOperation(stepIndex, operationIndex, updatedOperation)
                                                        }}
                                                        placeholder="insert_user"
                                                        size="small"
                                                      />
                                                    </Form.Item>
                                                  </Col>
                                                  <Col span={12}>
                                                    <Form.Item label="Количество выполнений" className="mb-2">
                                                      <InputNumber
                                                        value={parseInt(query.count)}
                                                        onChange={(value) => {
                                                          const newQueries = [...(operation.transaction?.queries || [])]
                                                          newQueries[queryIndex] = { ...query, count: value?.toString() || '1' }
                                                          const updatedOperation = {
                                                            ...operation,
                                                            transaction: { ...operation.transaction, queries: newQueries }
                                                          }
                                                          updateOperation(stepIndex, operationIndex, updatedOperation)
                                                        }}
                                                        min={1}
                                                        size="small"
                                                        className="w-full"
                                                      />
                                                    </Form.Item>
                                                  </Col>
                                                </Row>
                                                
                                                <Form.Item label="SQL запрос" className="mb-3">
                                                  <Input.TextArea
                                                    value={query.sql}
                                                    onChange={(e) => {
                                                      const newQueries = [...(operation.transaction?.queries || [])]
                                                      newQueries[queryIndex] = { ...query, sql: e.target.value }
                                                      const updatedOperation = {
                                                        ...operation,
                                                        transaction: { ...operation.transaction, queries: newQueries }
                                                      }
                                                      updateOperation(stepIndex, operationIndex, updatedOperation)
                                                    }}
                                                    rows={3}
                                                    placeholder="INSERT INTO users (name, email) VALUES (${name}, ${email})"
                                                  />
                                                </Form.Item>
                                                
                                                {/* Query Parameters - simplified for transaction queries */}
                                                <div>
                                                  <div className="flex justify-between items-center mb-2">
                                                    <Text type="secondary" className="text-xs">Параметры запроса</Text>
                                                    <Button
                                                      type="text"
                                                      size="small"
                                                      icon={<PlusOutlined />}
                                                      onClick={() => {
                                                        const newParam = {
                                                          name: `param_${Date.now()}`,
                                                          replaceRegex: '',
                                                          generationRule: {
                                                            type: 'string' as const,
                                                            length: 10
                                                          }
                                                        }
                                                        const newQueries = [...(operation.transaction?.queries || [])]
                                                        newQueries[queryIndex] = { 
                                                          ...query, 
                                                          params: [...(query.params || []), newParam] 
                                                        }
                                                        const updatedOperation = {
                                                          ...operation,
                                                          transaction: { ...operation.transaction, queries: newQueries }
                                                        }
                                                        updateOperation(stepIndex, operationIndex, updatedOperation)
                                                      }}
                                                    >
                                                      +
                                                    </Button>
                                                  </div>
                                                  
                                                  {query.params && query.params.length > 0 && (
                                                    <div className="space-y-2">
                                                      {query.params.map((param, paramIndex) => (
                                                        <div key={paramIndex} className="flex items-center space-x-2">
                                                          <Input
                                                            value={param.name}
                                                            onChange={(e) => {
                                                              const newQueries = [...(operation.transaction?.queries || [])]
                                                              const newParams = [...(query.params || [])]
                                                              newParams[paramIndex] = { ...param, name: e.target.value }
                                                              newQueries[queryIndex] = { ...query, params: newParams }
                                                              const updatedOperation = {
                                                                ...operation,
                                                                transaction: { ...operation.transaction, queries: newQueries }
                                                              }
                                                              updateOperation(stepIndex, operationIndex, updatedOperation)
                                                            }}
                                                            placeholder="name"
                                                            size="small"
                                                            className="flex-1"
                                                          />
                                                          <Input
                                                            value={param.replaceRegex}
                                                            onChange={(e) => {
                                                              const newQueries = [...(operation.transaction?.queries || [])]
                                                              const newParams = [...(query.params || [])]
                                                              newParams[paramIndex] = { ...param, replaceRegex: e.target.value }
                                                              newQueries[queryIndex] = { ...query, params: newParams }
                                                              const updatedOperation = {
                                                                ...operation,
                                                                transaction: { ...operation.transaction, queries: newQueries }
                                                              }
                                                              updateOperation(stepIndex, operationIndex, updatedOperation)
                                                            }}
                                                            placeholder="${name}"
                                                            size="small"
                                                            className="flex-1"
                                                          />
                                                          <Select
                                                            value={param.generationRule.type}
                                                            onChange={(value) => {
                                                              const newQueries = [...(operation.transaction?.queries || [])]
                                                              const newParams = [...(query.params || [])]
                                                              newParams[paramIndex] = {
                                                                ...param,
                                                                generationRule: { ...param.generationRule, type: value }
                                                              }
                                                              newQueries[queryIndex] = { ...query, params: newParams }
                                                              const updatedOperation = {
                                                                ...operation,
                                                                transaction: { ...operation.transaction, queries: newQueries }
                                                              }
                                                              updateOperation(stepIndex, operationIndex, updatedOperation)
                                                            }}
                                                            size="small"
                                                            className="w-24"
                                                          >
                                                            <Option value="string">
                                                              <div className="pl-3">Str</div>
                                                            </Option>
                                                            <Option value="int">
                                                              <div className="pl-3">Int</div>
                                                            </Option>
                                                            <Option value="float">
                                                              <div className="pl-3">Float</div>
                                                            </Option>
                                                            <Option value="uuid">
                                                              <div className="pl-3">UUID</div>
                                                            </Option>
                                                          </Select>
                                                          <Button
                                                            type="text"
                                                            danger
                                                            size="small"
                                                            icon={<DeleteOutlined />}
                                                            onClick={() => {
                                                              const newQueries = [...(operation.transaction?.queries || [])]
                                                              const newParams = (query.params || []).filter((_, i) => i !== paramIndex)
                                                              newQueries[queryIndex] = { ...query, params: newParams }
                                                              const updatedOperation = {
                                                                ...operation,
                                                                transaction: { ...operation.transaction, queries: newQueries }
                                                              }
                                                              updateOperation(stepIndex, operationIndex, updatedOperation)
                                                            }}
                                                          />
                                                        </div>
                                                      ))}
                                                    </div>
                                                  )}
                                                </div>
                                              </Card>
                                            ))}
                                          </div>
                                        ) : (
                                          <Empty 
                                            image={Empty.PRESENTED_IMAGE_SIMPLE}
                                            description="Нет запросов в транзакции"
                                            className="py-4"
                                          />
                                        )}
                                      </div>
                                    </div>
                                  )}
                                </Card>
                              ))}
                            </div>
                          ) : (
                            <Empty 
                              image={Empty.PRESENTED_IMAGE_SIMPLE}
                              description="Нет операций в этом шаге"
                            >
                              <Dropdown
                                menu={{
                                  items: [
                                    {
                                      key: 'query',
                                      label: 'SQL Query',
                                      icon: <CodeOutlined />,
                                      onClick: () => addOperation(stepIndex, 'query')
                                    },
                                    {
                                      key: 'table',
                                      label: 'Create Table',
                                      icon: <DatabaseOutlined />,
                                      onClick: () => addOperation(stepIndex, 'createTable')
                                    },
                                    {
                                      key: 'transaction',
                                      label: 'Transaction',
                                      icon: <ThunderboltOutlined />,
                                      onClick: () => addOperation(stepIndex, 'transaction')
                                    }
                                  ]
                                }}
                              >
                                <Button type="dashed" icon={<PlusOutlined />}>
                                  Добавить первую операцию
                                </Button>
                              </Dropdown>
                            </Empty>
                          )}
                        </Card>
                      ))}
                      
                      <Card className="border-2 border-dashed border-gray-300 hover:border-green-400 transition-colors">
                        <div className="text-center py-8">
                          <Button 
                            type="primary" 
                            size="large"
                            icon={<PlusOutlined />} 
                            onClick={addBenchmarkStep}
                          >
                            Добавить новый шаг
                          </Button>
                        </div>
                      </Card>
                    </div>
                  )}

                  {/* Timeline визуализация */}
                  {configData.benchmark.steps.length > 0 && (
                    <Card title="Последовательность выполнения" className="mt-6">
                      <Timeline mode="left">
                        {configData.benchmark.steps.map((step, index) => (
                          <Timeline.Item
                            key={`benchmark-${index}`}
                            color="green"
                            dot={<DragOutlined className="text-green-500" />}
                          >
                            <div className="flex items-center space-x-2">
                              <Text strong>{step.name}</Text>
                              <Tag color="green">Шаг {index + 1}</Tag>
                              {step.async && <Tag color="orange">Асинхронно</Tag>}
                              <Badge count={step.units?.length || 0} showZero />
                            </div>
                          </Timeline.Item>
                        ))}
                      </Timeline>
                    </Card>
                  )}
                </div>
              )}

              {/* Step 4: Дополнительные настройки */}
              {currentStep === 4 && (
                <div>
                  <Title level={4} className="mb-4">
                    <BugOutlined className="mr-2" />
                    Дополнительные настройки
                  </Title>
                  <Alert
                    message="Настройте логирование и метаданные"
                    description="Конфигурация системы логирования и дополнительных метаданных"
                    type="info"
                    showIcon
                    className="mb-4"
                  />

                  <Row gutter={[24, 16]}>
                    <Col xs={24} md={12}>
                      <Card title="Настройки логирования" className="h-full">
                <Form.Item
                  label="Уровень логирования"
                  tooltip="Уровень детализации логов"
                >
                          <Select
                    value={configData.run.logger?.logLevel || 'LOG_LEVEL_INFO'}
                            onChange={(value) => handleRunChange('logger', { 
                      ...configData.run.logger, 
                              logLevel: value 
                            })}
                            size="large"
                          >
                            <Option value="LOG_LEVEL_DEBUG">
                              <div className="flex items-center pl-3">
                                <BugOutlined className="mr-2" />
                                DEBUG
                              </div>
                            </Option>
                            <Option value="LOG_LEVEL_INFO">
                              <div className="flex items-center pl-3">
                                <InfoCircleOutlined className="mr-2" />
                                INFO
                              </div>
                            </Option>
                            <Option value="LOG_LEVEL_WARN">
                              <div className="flex items-center pl-3">
                                <InfoCircleOutlined className="mr-2" />
                                WARN
                              </div>
                            </Option>
                            <Option value="LOG_LEVEL_ERROR">
                              <div className="flex items-center pl-3">
                                <InfoCircleOutlined className="mr-2" />
                                ERROR
                              </div>
                            </Option>
                          </Select>
                </Form.Item>

                <Form.Item
                  label="Режим логирования"
                  tooltip="Режим работы системы логирования"
                >
                          <Select
                    value={configData.run.logger?.logMode || 'LOG_MODE_PRODUCTION'}
                            onChange={(value) => handleRunChange('logger', { 
                      ...configData.run.logger, 
                              logMode: value 
                            })}
                            size="large"
                          >
                            <Option value="LOG_MODE_DEVELOPMENT">
                              <div className="flex items-center pl-3">
                                <BugOutlined className="mr-2" />
                                DEVELOPMENT
                              </div>
                            </Option>
                            <Option value="LOG_MODE_PRODUCTION">
                              <div className="flex items-center pl-3">
                                <SettingOutlined className="mr-2" />
                                PRODUCTION
                              </div>
                            </Option>
                          </Select>
                </Form.Item>
            </Card>
                    </Col>

                    <Col xs={24} md={12}>
                      <Card title="Метаданные" className="h-full">
                <Form.Item
                          label="Дополнительные метаданные"
                  tooltip="Дополнительные метаданные для конфигурации"
                >
                          <Input.TextArea
                    value={configData.run.metadata?.example || ''}
                    onChange={(e) => handleRunChange('metadata', { 
                      ...configData.run.metadata, 
                      example: e.target.value 
                    })}
                            placeholder="Введите дополнительные метаданные..."
                            rows={4}
                            size="large"
                  />
                </Form.Item>

                        <Descriptions title="Сводка конфигурации" column={1} size="small">
                          <Descriptions.Item label="Версия">
                            {configData.version || 'Не указана'}
                          </Descriptions.Item>
                          <Descriptions.Item label="ID запуска">
                            {configData.runId || 'Не указан'}
                          </Descriptions.Item>
                          <Descriptions.Item label="Бенчмарк">
                            {configData.benchmarkName || 'Не указан'}
                          </Descriptions.Item>
                          <Descriptions.Item label="Запрошенных шагов">
                            {configData.run.steps.length}
                          </Descriptions.Item>
                          <Descriptions.Item label="Шагов бенчмарка">
                            {configData.benchmark.steps.length}
                          </Descriptions.Item>
                        </Descriptions>
            </Card>
                    </Col>
                  </Row>
          </div>
              )}

              {/* Navigation Buttons */}
              <div className="flex justify-between mt-8">
                <Button
                  size="large"
                  disabled={currentStep === 0}
                  onClick={() => setCurrentStep(currentStep - 1)}
                >
                  Назад
                </Button>
                
                <div>
                  <Text type="secondary" className="mr-4">
                    Шаг {currentStep + 1} из {configSteps.length}
                  </Text>
                </div>
                
                <Button
                  type="primary"
                  size="large"
                  disabled={currentStep === configSteps.length - 1}
                  onClick={() => setCurrentStep(currentStep + 1)}
                >
                  Далее
                </Button>
              </div>
            </Form>
          </Card>
        </div>
      </Content>

      {/* Modal for preview */}
      <Modal
        title={
          <div>
            <EyeOutlined className="mr-2" />
            Предварительный просмотр конфигурации
          </div>
        }
        open={previewVisible}
        onCancel={() => setPreviewVisible(false)}
        footer={[
          <Button key="close" onClick={() => setPreviewVisible(false)}>
            Закрыть
          </Button>,
          <Button key="download" type="primary" icon={<DownloadOutlined />} onClick={handleDownload}>
            Скачать конфигурацию
          </Button>
        ]}
        width={900}
      >
        <Card>
          <pre className="bg-gray-100 p-4 rounded text-sm overflow-auto max-h-96 whitespace-pre-wrap">
            {yamlContent}
          </pre>
        </Card>
      </Modal>

    </Layout>
  )
}

export default ConfigGenerator