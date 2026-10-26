import { HashRouter, Routes, Route, Navigate } from 'react-router-dom'
import { AuthProvider } from '@/context/AuthContext'
import AuthExpiredListener from '@/components/AuthExpiredListener'
import PrivateRoute from '@/components/PrivateRoute'
import ErrorBoundary from '@/components/ErrorBoundary'
import Home from '@/pages/Home/Home'
import Login from '@/pages/Login/Login'

export default function App() {
  return (
    <AuthProvider>
      <HashRouter>
        <AuthExpiredListener />
        <ErrorBoundary>
          <Routes>
            <Route path="/login" element={<Login />} />
            <Route element={<PrivateRoute />}>
              <Route path="/" element={<Home />} />
            </Route>
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </ErrorBoundary>
      </HashRouter>
    </AuthProvider>
  )
}
