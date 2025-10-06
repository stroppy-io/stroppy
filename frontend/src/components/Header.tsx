import React from 'react'
import { Layout, Select, Space, Typography, Button, Dropdown, Avatar, Switch } from 'antd'
import { 
  GlobalOutlined, 
  UserOutlined, 
  LogoutOutlined,
  BellOutlined,
  SettingOutlined,
  EyeOutlined,
  ThunderboltOutlined
} from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../contexts/AuthContext'
import { useTheme } from '../contexts/ThemeContext'
import { useTranslation } from '../hooks/useTranslation'

const { Header: AntHeader } = Layout
const { Option } = Select
const { Text } = Typography

interface HeaderProps {
  collapsed?: boolean
}

const Header: React.FC<HeaderProps> = ({ collapsed }) => {
  const navigate = useNavigate()
  const { user, logout } = useAuth()
  const { darkReaderEnabled, toggleDarkReader } = useTheme()
  const { changeLanguage, getCurrentLanguage, getAvailableLanguages } = useTranslation()

  const handleLogout = async () => {
    await logout()
    navigate('/login')
  }

  const { t } = useTranslation('common')

  const userMenuItems = [
    {
      key: 'profile',
      icon: <UserOutlined />,
      label: (
        <div style={{ padding: '4px 0' }}>
          <Text strong>{user?.username}</Text>
          <br />
          <Text type="secondary" style={{ fontSize: '12px' }}>
            Администратор
          </Text>
        </div>
      ),
      disabled: true
    },
    {
      type: 'divider' as const
    },
    {
      key: 'logout',
      icon: <LogoutOutlined />,
      label: t('navigation.logout'),
      onClick: handleLogout,
      danger: true
    }
  ]

  return (
    <>
    <AntHeader
      style={{
        background: '#fff',
        padding: '0 24px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        boxShadow: '0 2px 8px rgba(0,0,0,0.1)',
        borderBottom: '1px solid #f0f0f0',
        height: '64px',
        lineHeight: '64px',
        position: 'sticky',
        top: 0,
        zIndex: 1000
      }}
    >
      {/* Левая часть - логотип и название */}
      <div style={{ display: 'flex', alignItems: 'center', gap: '16px' }}>
        {!collapsed && (
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <div style={{
              width: '32px',
              height: '32px',
              background: 'linear-gradient(135deg, #1890ff, #096dd9)',
              borderRadius: '8px',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center'
            }}>
              <GlobalOutlined style={{ color: 'white', fontSize: '18px' }} />
            </div>
            <Text strong style={{ fontSize: '18px', color: '#262626' }}>
              Stroppy Cloud Panel
            </Text>
          </div>
        )}
      </div>

      {/* Правая часть - кнопки, переключатель языка, уведомления, профиль */}
      <div style={{ display: 'flex', alignItems: 'center', gap: '16px' }}>
        {/* Кнопка настроек */}
        <Button 
          icon={<SettingOutlined />} 
          size="small" 
          type="text" 
          style={{ color: '#666' }}
        >
          Настройки
        </Button>

        {/* Кнопка запуска */}
        <Button type="primary" icon={<ThunderboltOutlined />} size="small">
          Запустить
        </Button>

        {/* Переключатель темы */}
        <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
          <EyeOutlined style={{ color: '#666', fontSize: '16px' }} />
          <Switch
            size="small"
            checked={darkReaderEnabled}
            onChange={toggleDarkReader}
            checkedChildren="🌙"
            unCheckedChildren="☀️"
          />
        </div>

        {/* Переключатель языка */}
        <Select
          value={getCurrentLanguage()}
          onChange={changeLanguage}
          size="middle"
          style={{ width: 120 }}
          suffixIcon={<GlobalOutlined />}
          bordered={false}
        >
          {getAvailableLanguages().map(lang => (
            <Option key={lang.code} value={lang.code}>
              <Space>
                <span>{lang.nativeName}</span>
              </Space>
            </Option>
          ))}
        </Select>

        {/* Уведомления */}
        <Button
          type="text"
          icon={<BellOutlined />}
          size="large"
          style={{ 
            color: '#666',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center'
          }}
        />

        {/* Профиль пользователя */}
        {user && (
          <Dropdown
            menu={{ items: userMenuItems }}
            trigger={['click']}
            placement="bottomRight"
          >
            <div style={{ 
              display: 'flex', 
              alignItems: 'center', 
              gap: '8px', 
              padding: '8px 12px',
              borderRadius: '6px',
              cursor: 'pointer',
              transition: 'background-color 0.2s',
            }}
            onMouseEnter={(e) => {
              e.currentTarget.style.backgroundColor = '#f5f5f5'
            }}
            onMouseLeave={(e) => {
              e.currentTarget.style.backgroundColor = 'transparent'
            }}
            >
              <Avatar size="small" icon={<UserOutlined />} />
              <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-start' }}>
                <Text style={{ fontSize: '14px', fontWeight: 500, color: '#262626', lineHeight: '20px' }}>
                  {user.username}
                </Text>
                <Text style={{ fontSize: '12px', color: '#8c8c8c', lineHeight: '16px' }}>
                  Администратор
                </Text>
              </div>
            </div>
          </Dropdown>
        )}
      </div>
    </AntHeader>
  </>
  )
}

export default Header
