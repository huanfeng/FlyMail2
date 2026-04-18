import { Routes, Route, Navigate } from 'react-router'
import { useAuthStore } from '@/stores/auth'
import { AppLayout } from '@/layout/AppLayout'
import { LoginPage } from '@/pages/Login'

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
      <Route path="/" element={<ProtectedRoute><AppLayout /></ProtectedRoute>}>
        <Route index element={<PlaceholderPage title="Dashboard" />} />
        <Route path="accounts" element={<PlaceholderPage title="Accounts" />} />
        <Route path="emails" element={<PlaceholderPage title="Emails" />} />
        <Route path="emails/:id" element={<PlaceholderPage title="Email Detail" />} />
        <Route path="logs" element={<PlaceholderPage title="Logs" />} />
        <Route path="channels" element={<PlaceholderPage title="Channels" />} />
        <Route path="proxies" element={<PlaceholderPage title="Proxies" />} />
        <Route path="classification" element={<PlaceholderPage title="Classification" />} />
        <Route path="templates" element={<PlaceholderPage title="Templates" />} />
        <Route path="notification-policy" element={<PlaceholderPage title="Notification Policy" />} />
        <Route path="settings" element={<PlaceholderPage title="Settings" />} />
        <Route path="dev" element={<PlaceholderPage title="Debug" />} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}

function PlaceholderPage({ title }: { title: string }) {
  return (
    <div className="flex items-center justify-center h-full">
      <p className="text-muted-foreground text-lg">{title} - Coming soon</p>
    </div>
  )
}
