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
import { RunsDetailPage } from '../pages/panel/runs-detail-page'
import { RunsCreatePage } from '../pages/panel/runs-create-page'
import { RunsComparePage } from '../pages/panel/runs-compare-page'
import { AutomationsPage } from '../pages/panel/automations-page'
import { AutomationsCreatePage } from '../pages/panel/automations-create-page'
import { AutomationsDetailPage } from '../pages/panel/automations-detail-page'
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
          <Route path="automations">
            <Route index element={<AutomationsPage />} />
            <Route path="new" element={<AutomationsCreatePage />} />
            <Route path=":automationId" element={<AutomationsDetailPage />} />
          </Route>
          <Route path="runs" element={<RunsPage />} />
          <Route path="runs/new" element={<RunsCreatePage />} />
          <Route path="runs/compare" element={<RunsComparePage />} />
          <Route path="runs/:runId" element={<RunsDetailPage />} />
        </Route>

        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </BrowserRouter>
  )
}
