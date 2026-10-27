import { lazy, Suspense } from 'react'
import { HashRouter, Routes, Route, Navigate } from 'react-router-dom'
import { AuthProvider } from '@/context/AuthContext'
import AuthExpiredListener from '@/components/AuthExpiredListener'
import PrivateRoute from '@/components/PrivateRoute'
import SuperuserRoute from '@/components/SuperuserRoute'
import AppLayout from '@/components/layout/AppLayout'
import ErrorBoundary from '@/components/ErrorBoundary'
import Home from '@/pages/Home/Home'
import Login from '@/pages/Login/Login'

const Users = lazy(() => import('@/pages/Users/Users'))

export default function App() {
  return (
    <AuthProvider>
      <HashRouter>
        <AuthExpiredListener />
        <ErrorBoundary>
          <Suspense fallback={null}>
            <Routes>
              <Route path="/login" element={<Login />} />
              <Route element={<PrivateRoute />}>
                <Route element={<AppLayout />}>
                  <Route path="/" element={<Home />} />
                </Route>
                <Route element={<SuperuserRoute />}>
                  <Route element={<AppLayout />}>
                    <Route path="/users" element={<Users />} />
                  </Route>
                </Route>
              </Route>
              <Route path="*" element={<Navigate to="/" replace />} />
            </Routes>
          </Suspense>
        </ErrorBoundary>
      </HashRouter>
    </AuthProvider>
  )
}
