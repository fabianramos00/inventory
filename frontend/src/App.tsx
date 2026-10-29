import { lazy, Suspense } from 'react'
import { HashRouter, Routes, Route, Navigate } from 'react-router-dom'
import { AuthProvider } from '@/context/AuthContext'
import AuthExpiredListener from '@/components/AuthExpiredListener'
import PrivateRoute from '@/components/PrivateRoute'
import SuperuserRoute from '@/components/SuperuserRoute'
import AppLayout from '@/components/layout/AppLayout'
import ErrorBoundary from '@/components/ErrorBoundary'
import Login from '@/pages/Login/Login'

const Dashboard = lazy(() => import('@/pages/Dashboard/Dashboard'))
const Users = lazy(() => import('@/pages/Users/Users'))
const Inventory = lazy(() => import('@/pages/Inventory/Inventory'))
const ProductForm = lazy(() => import('@/pages/ProductForm/ProductForm'))
const Sales = lazy(() => import('@/pages/Sales/Sales'))
const SaleForm = lazy(() => import('@/pages/SaleForm/SaleForm'))
const Orders = lazy(() => import('@/pages/Orders/Orders'))
const OrderForm = lazy(() => import('@/pages/OrderForm/OrderForm'))
const Clients = lazy(() => import('@/pages/Clients/Clients'))
const Providers = lazy(() => import('@/pages/Providers/Providers'))

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
                  <Route path="/" element={<Dashboard />} />
                  <Route path="/inventory" element={<Inventory />} />
                  <Route path="/inventory/create" element={<ProductForm />} />
                  <Route path="/inventory/product/:id" element={<ProductForm />} />
                  <Route path="/sales" element={<Sales />} />
                  <Route path="/sales/create" element={<SaleForm />} />
                  <Route path="/sales/:id" element={<SaleForm />} />
                  <Route path="/orders" element={<Orders />} />
                  <Route path="/orders/create" element={<OrderForm />} />
                  <Route path="/orders/:id" element={<OrderForm />} />
                  <Route path="/clients" element={<Clients />} />
                  <Route path="/providers" element={<Providers />} />
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
