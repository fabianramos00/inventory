import { Navigate, Outlet } from 'react-router-dom'
import { useAuth } from '@/context/AuthContext'

export default function SuperuserRoute() {
  const { user } = useAuth()
  return user?.is_superuser ? <Outlet /> : <Navigate to="/" replace />
}
