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
const Inventory = lazy(() => import('@/pages/Inventory/Inventory'))
const ProductForm = lazy(() => import('@/pages/ProductForm/ProductForm'))
const Sales = lazy(() => import('@/pages/Sales/Sales'))
const SaleForm = lazy(() => import('@/pages/SaleForm/SaleForm'))
const Clients = lazy(() => import('@/pages/Clients/Clients'))

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
                  <Route path="/inventory" element={<Inventory />} />
                  <Route path="/inventory/create" element={<ProductForm />} />
                  <Route path="/inventory/product/:id" element={<ProductForm />} />
                  <Route path="/sales" element={<Sales />} />
                  <Route path="/sales/create" element={<SaleForm />} />
                  <Route path="/sales/:id" element={<SaleForm />} />
                  <Route path="/clients" element={<Clients />} />
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
