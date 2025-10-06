import React from 'react'
import { Layout, Menu, Button } from 'antd'
import {
  DashboardOutlined,
  SettingOutlined,
  PlayCircleOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  CloudOutlined,
} from '@ant-design/icons'
import { useNavigate, useLocation } from 'react-router-dom'
import { useTranslation } from '../hooks/useTranslation'

const { Sider } = Layout

interface SidebarProps {
  collapsed: boolean
  onCollapse: (collapsed: boolean) => void
}

const Sidebar: React.FC<SidebarProps> = ({ collapsed, onCollapse }) => {
  const navigate = useNavigate()
  const location = useLocation()
  const { t } = useTranslation('common')

  const menuItems = [
    {
      key: '/app/dashboard',
      icon: <DashboardOutlined />,
      label: t('navigation.dashboard'),
    },
    {
      key: '/app/runs',
      icon: <PlayCircleOutlined />,
      label: t('navigation.runs'),
    },
    {
      key: '/app/configurator',
      icon: <SettingOutlined />,
      label: t('navigation.configurator'),
    },
  ]

  const handleMenuClick = ({ key }: { key: string }) => {
    navigate(key)
  }

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

        {/* Кнопка сворачивания */}
        <div style={{ padding: '8px', borderTop: '1px solid #f0f0f0' }}>
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
