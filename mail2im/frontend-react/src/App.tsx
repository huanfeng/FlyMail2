import { Routes, Route, Navigate } from 'react-router'
import { useAuthStore } from '@/stores/auth'
import { AppLayout } from '@/layout/AppLayout'
import { LoginPage } from '@/pages/Login'
import { DashboardPage } from '@/pages/Dashboard'
import { AccountsPage } from '@/pages/Accounts'
import { EmailsPage } from '@/pages/Emails'
import { EmailDetailPage } from '@/pages/EmailDetail'
import { EmailStandalonePage, SharePage } from '@/pages/EmailStandalone'
import { LogsPage } from '@/pages/Logs'
import { ChannelsPage } from '@/pages/Channels'
import { ProxiesPage } from '@/pages/Proxies'
import { ClassificationPage } from '@/pages/Classification'
import { TemplatesPage } from '@/pages/Templates'
import { NotificationPolicyPage } from '@/pages/NotificationPolicy'
import { SettingsPage } from '@/pages/Settings'
import { DebugPage } from '@/pages/Debug'

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)
  if (!isAuthenticated) return <Navigate to="/login" replace />
  return <>{children}</>
}

function PublicRoute({ children }: { children: React.ReactNode }) {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)
  if (isAuthenticated) return <Navigate to="/" replace />
  return <>{children}</>
}

export default function App() {
  const loadFromStorage = useAuthStore((s) => s.loadFromStorage)
  loadFromStorage()

  return (
    <Routes>
      <Route path="/login" element={<PublicRoute><LoginPage /></PublicRoute>} />
      <Route path="/email-view/:id" element={<ProtectedRoute><EmailStandalonePage /></ProtectedRoute>} />
      <Route path="/share/:token" element={<SharePage />} />
      <Route path="/" element={<ProtectedRoute><AppLayout /></ProtectedRoute>}>
        <Route index element={<DashboardPage />} />
        <Route path="accounts" element={<AccountsPage />} />
        <Route path="emails" element={<EmailsPage />} />
        <Route path="emails/:id" element={<EmailDetailPage />} />
        <Route path="logs" element={<LogsPage />} />
        <Route path="channels" element={<ChannelsPage />} />
        <Route path="proxies" element={<ProxiesPage />} />
        <Route path="classification" element={<ClassificationPage />} />
        <Route path="templates" element={<TemplatesPage />} />
        <Route path="notification-policy" element={<NotificationPolicyPage />} />
        <Route path="settings" element={<SettingsPage />} />
        <Route path="dev" element={<DebugPage />} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
