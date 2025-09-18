import { useState } from 'react'
import { BrowserRouter as Router, Routes, Route, Navigate } from 'react-router-dom'
import { Layout, ConfigProvider } from 'antd'
import { AuthProvider } from './contexts/AuthContext'
import { ThemeProvider, useTheme } from './contexts/ThemeContext'
import ProtectedRoute from './components/ProtectedRoute'
import Sidebar from './components/Sidebar'
import PageWrapper from './components/PageWrapper'
import ConfiguratorPage from './pages/ConfiguratorPage'
import RunsPage from './pages/RunsPage'
import LoginPage from './pages/LoginPage'
import RegisterPage from './pages/RegisterPage'

const { Footer } = Layout

// Внутренний компонент с доступом к ThemeContext
const AppContent: React.FC = () => {
  const [collapsed, setCollapsed] = useState(false)
  const { antdTheme } = useTheme()

  return (
    <ConfigProvider theme={antdTheme}>
      <AuthProvider>
        <Router>
          <Routes>
            {/* Публичные маршруты */}
            <Route path="/login" element={<LoginPage />} />
            <Route path="/register" element={<RegisterPage />} />
            
            {/* Защищенные маршруты */}
            <Route path="/*" element={
              <ProtectedRoute>
                <Layout style={{ height: '100vh', overflow: 'hidden' }}>
                  <Sidebar collapsed={collapsed} onCollapse={setCollapsed} />
                  
                  <Layout style={{ display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
                    <PageWrapper>
                      <Routes>
                        <Route path="/" element={<Navigate to="/runs" replace />} />
                        <Route path="/configurator" element={<ConfiguratorPage />} />
                        <Route path="/runs" element={<RunsPage />} />
                      </Routes>
                    </PageWrapper>
                    
                    <Footer style={{ 
                      textAlign: 'center', 
                      padding: '8px 16px', 
                      fontSize: '12px', 
                      color: '#999',
                      height: '32px',
                      lineHeight: '16px',
                      flexShrink: 0
                    }}>
                      Stroppy Cloud Panel ©2024
                    </Footer>
                  </Layout>
                </Layout>
              </ProtectedRoute>
            } />
          </Routes>
        </Router>
      </AuthProvider>
    </ConfigProvider>
  )
}

function App() {
  return (
    <ThemeProvider>
      <AppContent />
    </ThemeProvider>
  )
}

export default App
