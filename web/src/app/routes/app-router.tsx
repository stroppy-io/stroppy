import { Navigate, Route, BrowserRouter, Routes } from 'react-router-dom'
import { LandingLayout } from '../layouts/landing-layout'
import { PanelLayout } from '../layouts/panel-layout'
import { LandingHomePage } from '../pages/landing/home-page'
import { LandingDocsPage } from '../pages/landing/docs-page'
import { LandingLoginPage } from '../pages/landing/login-page'
import { LandingRegisterPage } from '../pages/landing/register-page'
import { DashboardPage } from '../pages/panel/dashboard-page'
import { ConfiguratorPage } from '../pages/panel/configurator-page'
import { RunsPage } from '../pages/panel/runs-page'
import { ProtectedRoute } from './protected-route'

export const AppRouter = () => {
  return (
    <BrowserRouter>
      <Routes>
        <Route element={<LandingLayout />}>
          <Route index element={<LandingHomePage />} />
          <Route path="/docs" element={<LandingDocsPage />} />
          <Route path="/login" element={<LandingLoginPage />} />
          <Route path="/register" element={<LandingRegisterPage />} />
        </Route>

        <Route
          path="/app"
          element={
            <ProtectedRoute>
              <PanelLayout />
            </ProtectedRoute>
          }
        >
          <Route index element={<Navigate to="dashboard" replace />} />
          <Route path="dashboard" element={<DashboardPage />} />
          <Route path="configurator" element={<ConfiguratorPage />} />
          <Route path="runs" element={<RunsPage />} />
        </Route>

        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </BrowserRouter>
  )
}

