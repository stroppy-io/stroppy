import React, { useState } from 'react'
import { Modal, Slider, Button, Space, Typography, Divider, Switch, Card } from 'antd'
import { EyeOutlined, DownloadOutlined, CloudOutlined } from '@ant-design/icons'
import { useTheme } from '../../common/contexts/ThemeContext'

const { Title, Text } = Typography

interface DarkReaderSettingsProps {
  visible: boolean
  onClose: () => void
}

const DarkReaderSettings: React.FC<DarkReaderSettingsProps> = ({ visible, onClose }) => {
  const { 
    darkReaderEnabled, 
    toggleDarkReader, 
    updateDarkReaderConfig,
    followSystemTheme,
    stopFollowingSystem,
    exportCSS
  } = useTheme()

  const [config, setConfig] = useState({
    brightness: 100,
    contrast: 90,
    sepia: 10
  })

  const handleConfigChange = (key: string, value: number) => {
    const newConfig = { ...config, [key]: value }
    setConfig(newConfig)
    updateDarkReaderConfig(newConfig)
  }

  const handleExportCSS = async () => {
    try {
      const css = await exportCSS()
      const blob = new Blob([css], { type: 'text/css' })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = 'darkreader-generated.css'
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      URL.revokeObjectURL(url)
    } catch (error) {
      console.error('Failed to export CSS:', error)
    }
  }

  const resetToDefaults = () => {
    const defaultConfig = { brightness: 100, contrast: 90, sepia: 10 }
    setConfig(defaultConfig)
    updateDarkReaderConfig(defaultConfig)
  }

  return (
    <Modal
      title={
        <Space>
          <EyeOutlined />
          <span>Настройки Dark Reader</span>
        </Space>
      }
      open={visible}
      onCancel={onClose}
      footer={[
        <Button key="export" icon={<DownloadOutlined />} onClick={handleExportCSS}>
          Экспорт CSS
        </Button>,
        <Button key="reset" onClick={resetToDefaults}>
          Сброс
        </Button>,
        <Button key="close" type="primary" onClick={onClose}>
          Закрыть
        </Button>
      ]}
      width={500}
    >
      <Space direction="vertical" style={{ width: '100%' }} size="large">
        {/* Основные настройки */}
        <Card size="small">
          <Space direction="vertical" style={{ width: '100%' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <Text strong>Включить Dark Reader</Text>
              <Switch 
                checked={darkReaderEnabled} 
                onChange={toggleDarkReader}
                checkedChildren="Вкл"
                unCheckedChildren="Выкл"
              />
            </div>
            
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <Text strong>Следовать системной теме</Text>
              <Button 
                type="text" 
                icon={<CloudOutlined />}
                onClick={darkReaderEnabled ? stopFollowingSystem : followSystemTheme}
                size="small"
              >
                {darkReaderEnabled ? 'Остановить' : 'Включить'}
              </Button>
            </div>
          </Space>
        </Card>

        <Divider />

        {/* Настройки фильтров */}
        <div>
          <Title level={5}>Настройки фильтров</Title>
          
          <div style={{ marginBottom: '16px' }}>
            <Text>Яркость: {config.brightness}%</Text>
            <Slider
              min={0}
              max={200}
              value={config.brightness}
              onChange={(value) => handleConfigChange('brightness', value)}
              disabled={!darkReaderEnabled}
            />
          </div>

          <div style={{ marginBottom: '16px' }}>
            <Text>Контрастность: {config.contrast}%</Text>
            <Slider
              min={0}
              max={200}
              value={config.contrast}
              onChange={(value) => handleConfigChange('contrast', value)}
              disabled={!darkReaderEnabled}
            />
          </div>

          <div style={{ marginBottom: '16px' }}>
            <Text>Сепия: {config.sepia}%</Text>
            <Slider
              min={0}
              max={100}
              value={config.sepia}
              onChange={(value) => handleConfigChange('sepia', value)}
              disabled={!darkReaderEnabled}
            />
          </div>
        </div>

        <Divider />

        {/* Информация */}
        <Card size="small" style={{ backgroundColor: '#f5f5f5' }}>
          <Text type="secondary" style={{ fontSize: '12px' }}>
            Dark Reader применяет темную тему ко всему сайту, включая сторонние элементы.
            Настройки сохраняются автоматически и применяются ко всем страницам.
          </Text>
        </Card>
      </Space>
    </Modal>
  )
}

export default DarkReaderSettings
