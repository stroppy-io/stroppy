import React, { type ReactNode } from 'react'
import { Layout, Button, Space, Dropdown, Avatar, Typography, Switch } from 'antd'
import { 
  ThunderboltOutlined, 
  CloudOutlined, 
  SettingOutlined, 
  UserOutlined, 
  LogoutOutlined,
  EyeOutlined
} from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../contexts/AuthContext'
import { useTheme } from '../contexts/ThemeContext'
import DarkReaderSettings from './DarkReaderSettings'

const { Header, Content } = Layout
const { Text } = Typography

interface PageWrapperProps {
  children: ReactNode
}

const PageWrapper: React.FC<PageWrapperProps> = ({ children }) => {
  const { user, logout } = useAuth()
  const { 
    darkReaderEnabled, 
    toggleDarkReader,
    followSystemTheme,
    stopFollowingSystem 
  } = useTheme()
  const navigate = useNavigate()
  
  const [darkReaderSettingsVisible, setDarkReaderSettingsVisible] = React.useState(false)

  const handleLogout = async () => {
    await logout()
    navigate('/login')
  }

  const userMenuItems = [
    {
      key: 'profile',
      icon: <UserOutlined />,
      label: (
        <div style={{ padding: '4px 0' }}>
          <Text strong>{user?.username}</Text>
          <br />
          <Text type="secondary" style={{ fontSize: '12px' }}>
            Профиль пользователя
          </Text>
        </div>
      ),
      disabled: true
    },
    {
      type: 'divider' as const
    },
    {
      key: 'darkreader',
      label: (
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', width: '160px' }}>
          <span>Dark Reader</span>
          <Switch
            size="small"
            checked={darkReaderEnabled}
            onChange={toggleDarkReader}
            checkedChildren={<EyeOutlined />}
            unCheckedChildren={<EyeOutlined />}
          />
        </div>
      ),
      onClick: (e: any) => {
        e.domEvent.stopPropagation()
      }
    },
    {
      key: 'darkreader-settings',
      icon: <SettingOutlined />,
      label: 'Настройки Dark Reader',
      onClick: () => setDarkReaderSettingsVisible(true)
    },
    {
      key: 'system-theme',
      icon: <CloudOutlined />,
      label: darkReaderEnabled ? 'Остановить следование системной теме' : 'Следовать системной теме',
      onClick: darkReaderEnabled ? stopFollowingSystem : followSystemTheme
    },
    {
      type: 'divider' as const
    },
    {
      key: 'logout',
      icon: <LogoutOutlined />,
      label: 'Выйти из системы',
      onClick: handleLogout,
      danger: true
    }
  ]

  return (
    <>
      <Header 
        style={{ 
          background: '#fff', 
          padding: '0 16px', 
          height: '56px',
          lineHeight: '56px',
          boxShadow: '0 1px 4px rgba(0,21,41,.08)'
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', height: '100%' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
            <div style={{
              width: '28px',
              height: '28px',
              background: 'linear-gradient(135deg, #1890ff, #096dd9)',
              borderRadius: '6px',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center'
            }}>
              <CloudOutlined style={{ color: 'white', fontSize: '14px' }} />
            </div>
            <span style={{ 
              fontSize: '18px', 
              fontWeight: 'bold', 
              color: '#262626' 
            }}>
              Stroppy Cloud Panel
            </span>
          </div>
          
          <Space size={16}>
            <Button 
              icon={<SettingOutlined />} 
              size="small" 
              type="text" 
              style={{ color: '#666' }}
            >
              Настройки
            </Button>
            <Button type="primary" icon={<ThunderboltOutlined />} size="small">
              Запустить
            </Button>
            
            {/* Меню пользователя */}
            <Dropdown
              menu={{ items: userMenuItems }}
              placement="bottomRight"
              trigger={['click']}
            >
              <div style={{ 
                display: 'flex', 
                alignItems: 'center', 
                gap: '8px', 
                cursor: 'pointer',
                padding: '4px 8px',
                borderRadius: '6px',
                transition: 'background-color 0.2s'
              }}
              onMouseEnter={(e) => {
                e.currentTarget.style.backgroundColor = '#f0f0f0'
              }}
              onMouseLeave={(e) => {
                e.currentTarget.style.backgroundColor = 'transparent'
              }}
              >
                <Avatar size="small" icon={<UserOutlined />} />
                <Text style={{ fontSize: '14px', fontWeight: 500 }}>
                  {user?.username}
                </Text>
              </div>
            </Dropdown>
          </Space>
        </div>
      </Header>
      
      <Content style={{ 
        background: '#f5f5f5', 
        padding: '24px',
        overflow: 'hidden',
        flex: 1,
        display: 'flex',
        flexDirection: 'column'
      }}>
        <div style={{ flex: 1, overflow: 'auto', minHeight: 0 }}>
          {children}
        </div>
      </Content>
      
      {/* Модальное окно настроек Dark Reader */}
      <DarkReaderSettings
        visible={darkReaderSettingsVisible}
        onClose={() => setDarkReaderSettingsVisible(false)}
      />
    </>
  )
}

export default PageWrapper
