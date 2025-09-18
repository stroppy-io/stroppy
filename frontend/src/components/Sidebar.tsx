import { Layout, Menu, Button, Avatar, Dropdown, Typography } from 'antd'
import {
  SettingOutlined,
  PlayCircleOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  CloudOutlined,
  UserOutlined,
  LogoutOutlined,
} from '@ant-design/icons'
import { useNavigate, useLocation } from 'react-router-dom'
import { useAuth } from '../contexts/AuthContext'

const { Text } = Typography

const { Sider } = Layout

interface SidebarProps {
  collapsed: boolean
  onCollapse: (collapsed: boolean) => void
}

const Sidebar: React.FC<SidebarProps> = ({ collapsed, onCollapse }) => {
  const navigate = useNavigate()
  const location = useLocation()
  const { user, logout } = useAuth()

  const menuItems = [
    {
      key: '/configurator',
      icon: <SettingOutlined />,
      label: 'Конфигуратор',
    },
    {
      key: '/runs',
      icon: <PlayCircleOutlined />,
      label: 'Запуски',
    },
  ]

  const handleMenuClick = ({ key }: { key: string }) => {
    navigate(key)
  }

  const handleLogout = async () => {
    await logout()
    navigate('/login')
  }

  const userMenuItems = [
    {
      key: 'profile',
      icon: <UserOutlined />,
      label: 'Профиль',
    },
    {
      type: 'divider' as const,
    },
    {
      key: 'logout',
      icon: <LogoutOutlined />,
      label: 'Выйти',
      onClick: handleLogout,
    },
  ]

  return (
    <Sider
      trigger={null}
      collapsible
      collapsed={collapsed}
      width={200}
      collapsedWidth={60}
      style={{
        background: '#fff',
        boxShadow: '2px 0 8px rgba(0,0,0,0.1)',
        borderRight: '1px solid #f0f0f0'
      }}
    >
      <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
        {/* Логотип и заголовок */}
        <div style={{ 
          display: 'flex', 
          alignItems: 'center', 
          justifyContent: 'center', 
          padding: '12px',
          borderBottom: '1px solid #f0f0f0'
        }}>
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
              <CloudOutlined style={{ color: 'white', fontSize: '18px' }} />
            </div>
            {!collapsed && (
              <span style={{ fontSize: '16px', fontWeight: 'bold', color: '#262626' }}>
                Stroppy
              </span>
            )}
          </div>
        </div>

        {/* Навигационное меню */}
        <div style={{ flex: 1, paddingTop: '8px' }}>
          <Menu
            mode="inline"
            selectedKeys={[location.pathname]}
            items={menuItems}
            onClick={handleMenuClick}
            style={{ 
              border: 'none',
              background: 'transparent',
              fontSize: '14px'
            }}
          />
        </div>

        {/* Информация о пользователе */}
        <div style={{ padding: '8px', borderTop: '1px solid #f0f0f0' }}>
          {!collapsed && user && (
            <div style={{ marginBottom: '8px' }}>
              <Dropdown
                menu={{ items: userMenuItems }}
                trigger={['click']}
                placement="topRight"
              >
                <div style={{ 
                  display: 'flex', 
                  alignItems: 'center', 
                  gap: '8px', 
                  padding: '8px',
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
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <Text style={{ fontSize: '12px', color: '#666' }}>
                      {user.username}
                    </Text>
                  </div>
                </div>
              </Dropdown>
            </div>
          )}
          
          {/* Кнопка сворачивания */}
          <Button
            type="text"
            icon={collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
            onClick={() => onCollapse(!collapsed)}
            size="small"
            style={{ 
              width: '100%', 
              height: '32px',
              color: '#666',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center'
            }}
          />
        </div>
      </div>
    </Sider>
  )
}

export default Sidebar
