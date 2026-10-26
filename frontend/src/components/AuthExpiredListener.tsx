import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '@/context/AuthContext'

export default function AuthExpiredListener() {
  const { logout } = useAuth()
  const navigate = useNavigate()

  useEffect(() => {
    function handle() {
      logout()
      navigate('/login', { replace: true })
    }
    window.addEventListener('auth:expired', handle)
    return () => window.removeEventListener('auth:expired', handle)
  }, [logout, navigate])

  return null
}
