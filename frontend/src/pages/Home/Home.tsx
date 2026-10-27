import { useNavigate } from 'react-router-dom'
import { LogOut } from 'lucide-react'
import { useAuth } from '@/context/AuthContext'
import { authApi } from '@/lib/api/auth'
import styles from './Home.module.css'

// ponytail: Phase 2 placeholder, replace with Dashboard in Phase 3
export default function Home() {
  const { user, refreshToken, logout } = useAuth()
  const navigate = useNavigate()

  function handleLogout() {
    authApi.logout(refreshToken).catch(() => {})
    logout()
    navigate('/login', { replace: true })
  }

  return (
    <div className={styles.page}>
      <section className={styles.card} aria-labelledby="home-title">
        <div className={styles.brand}>
          <span className={styles.brandMark} aria-hidden="true">GS</span>
          <span className={styles.brandName}>GestiónStock</span>
        </div>

        <div className={styles.content}>
          <p className={styles.eyebrow}>Sesión iniciada</p>
          <h1 id="home-title" className={styles.title}>Hola, {user?.full_name ?? 'Usuario'}</h1>

          <dl className={styles.details}>
            <div className={styles.detailRow}>
              <dt>Nombre</dt>
              <dd>{user?.full_name ?? '—'}</dd>
            </div>
            <div className={styles.detailRow}>
              <dt>Correo</dt>
              <dd>{user?.email ?? '—'}</dd>
            </div>
          </dl>

          <p className={styles.note}>El panel principal estará disponible en la siguiente fase.</p>
        </div>

        <button type="button" className={styles.logout} onClick={handleLogout}>
          <LogOut size={16} aria-hidden="true" />
          Cerrar sesión
        </button>
      </section>
    </div>
  )
}
