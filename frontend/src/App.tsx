import { useState } from 'react'
import { BrowserRouter as Router, Routes, Route, Navigate } from 'react-router-dom'
import { Layout } from 'antd'
import { AuthProvider } from './common/contexts/AuthContext'
import { ThemeProvider } from './common/contexts/ThemeContext'
import ProtectedRoute from './panel/components/ProtectedRoute'
import Sidebar from './panel/components/Sidebar'
import Header from './panel/components/Header'
import PageWrapper from './panel/components/PageWrapper'
import DashboardPage from './panel/pages/DashboardPage'
import ConfiguratorPage from './panel/pages/ConfiguratorPage'
import RunsPage from './panel/pages/RunsPage'
import LoginPage from './landing/pages/LoginPage'
import RegisterPage from './landing/pages/RegisterPage'
import LandingPage from './landing/pages/LandingPage'
import DocumentationPage from './landing/pages/DocumentationPage'

const { Footer } = Layout

// Внутренний компонент
const AppContent: React.FC = () => {
  const [collapsed, setCollapsed] = useState(false)

  return (
    <AuthProvider>
      <Router>
        <Routes>
          {/* Публичные маршруты */}
          <Route path="/" element={<LandingPage />} />
          <Route path="/login" element={<LoginPage />} />
          <Route path="/register" element={<RegisterPage />} />
          <Route path="/docs" element={<DocumentationPage />} />
          <Route path="/docs/:sectionId" element={<DocumentationPage />} />
          <Route path="/docs/:sectionId/:fileId" element={<DocumentationPage />} />
          
          {/* Защищенные маршруты */}
          <Route path="/app/*" element={
            <ProtectedRoute>
              <Layout style={{ height: '100vh', overflow: 'hidden' }}>
                <Sidebar collapsed={collapsed} onCollapse={setCollapsed} />
                
                <Layout style={{ display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
                  <Header collapsed={collapsed} />
                  
                  <PageWrapper>
                    <Routes>
                      <Route path="/" element={<Navigate to="/app/dashboard" replace />} />
                      <Route path="/dashboard" element={<DashboardPage />} />
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
