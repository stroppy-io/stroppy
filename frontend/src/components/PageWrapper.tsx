import type { ReactNode } from 'react'
import { Layout, Button, Space, Dropdown, Avatar, Typography, Switch } from 'antd'
import { 
  ThunderboltOutlined, 
  CloudOutlined, 
  SettingOutlined, 
  UserOutlined, 
  LogoutOutlined,
  SunOutlined,
  MoonOutlined
} from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../contexts/AuthContext'
import { useTheme } from '../contexts/ThemeContext'

const { Header, Content } = Layout
const { Text } = Typography

interface PageWrapperProps {
  children: ReactNode
}

const PageWrapper: React.FC<PageWrapperProps> = ({ children }) => {
  const { user, logout } = useAuth()
  const { mode, toggleTheme, antdTheme } = useTheme()
  const navigate = useNavigate()

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
      key: 'theme',
      label: (
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', width: '140px' }}>
          <span>Тёмная тема</span>
          <Switch
            size="small"
            checked={mode === 'dark'}
            onChange={toggleTheme}
            checkedChildren={<MoonOutlined />}
            unCheckedChildren={<SunOutlined />}
          />
        </div>
      ),
      onClick: (e: any) => {
        e.domEvent.stopPropagation()
      }
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
          background: antdTheme.components?.Layout?.headerBg || '#fff', 
          padding: '0 16px', 
          height: '56px',
          lineHeight: '56px',
          boxShadow: mode === 'dark' 
            ? '0 1px 4px rgba(255,255,255,.08)' 
            : '0 1px 4px rgba(0,21,41,.08)'
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
              color: mode === 'dark' ? '#ffffff' : '#262626' 
            }}>
              Stroppy Cloud Panel
            </span>
          </div>
          
          <Space size={16}>
            <Button 
              icon={<SettingOutlined />} 
              size="small" 
              type="text" 
              style={{ color: mode === 'dark' ? '#a6a6a6' : '#666' }}
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
        background: antdTheme.components?.Layout?.bodyBg || '#f5f5f5', 
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
    </>
  )
}

export default PageWrapper
