import React, { type ReactNode } from 'react'
import { Layout } from 'antd'

const { Content } = Layout

interface PageWrapperProps {
  children: ReactNode
}

const PageWrapper: React.FC<PageWrapperProps> = ({ children }) => {
  return (
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
  )
}

export default PageWrapper
