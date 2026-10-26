import { NavLink, useNavigate } from 'react-router-dom'
import {
  LayoutDashboard,
  Package,
  Truck,
  ContactRound,
  ShoppingCart,
  ClipboardList,
  Users,
  ChevronLeft,
  ChevronRight,
  LogOut,
  X,
} from 'lucide-react'
import { useAuth } from '@/context/AuthContext'
import { authApi } from '@/lib/api/auth'
import styles from './Sidebar.module.css'

const navGroups = [
  {
    label: 'Operación',
    items: [
      { to: '/', label: 'Dashboard', icon: LayoutDashboard },
      { to: '/sales', label: 'Ventas', icon: ShoppingCart },
      { to: '/orders', label: 'Pedidos', icon: ClipboardList },
      { to: '/inventory', label: 'Inventario', icon: Package },
    ],
  },
  {
    label: 'Directorio',
    items: [
      { to: '/providers', label: 'Proveedores', icon: Truck },
      { to: '/clients', label: 'Clientes', icon: ContactRound },
      { to: '/users', label: 'Usuarios', icon: Users },
    ],
  },
]

interface SidebarProps {
  isMobileMenuOpen?: boolean
  closeMobileMenu?: (returnFocus?: boolean) => void
  isCollapsed: boolean
  toggleCollapse: () => void
}

export default function Sidebar({ isMobileMenuOpen, closeMobileMenu, isCollapsed, toggleCollapse }: SidebarProps) {
  const { user, refreshToken, logout } = useAuth()
  const navigate = useNavigate()

  async function handleLogout() {
    authApi.logout(refreshToken).catch(() => {})
    logout()
    navigate('/login', { replace: true })
  }

  const userInitial = user?.full_name?.charAt(0).toUpperCase() ?? 'U'
  const displayName = user?.full_name ?? 'Usuario'
  const displayRole = user?.is_superuser ? 'Admin' : 'Usuario'

  return (
    <aside
      id="app-sidebar"
      className={`${styles.sidebar} ${isMobileMenuOpen ? styles.sidebarOpen : ''}`}
      data-collapsed={isCollapsed}
      aria-label="Navegación principal"
    >
      <div className={styles.mobileHeader}>
        <div className={styles.mobileBrandGroup}>
          <div className={styles.brandMark}>GS</div>
          <div>
            <div className={styles.mobileBrandName}>GestiónStock</div>
            <div className={styles.mobileBrandSub}>Ferretería J&S</div>
          </div>
        </div>
        <button
          type="button"
          className={styles.mobileClose}
          onClick={() => closeMobileMenu?.()}
          aria-label="Cerrar menú"
        >
          <X size={18} />
        </button>
      </div>

      <div className={styles.logoHeader}>
        <div className={styles.brandMark}>GS</div>
        <div className={styles.brandText}>
          <div className={styles.brandName}>GestiónStock</div>
          <div className={styles.brandSub}>Ferretería J&S</div>
        </div>
        <button
          type="button"
          onClick={toggleCollapse}
          className={styles.collapseBtn}
          title={isCollapsed ? 'Expandir' : 'Contraer'}
          aria-label={isCollapsed ? 'Expandir menú lateral' : 'Contraer menú lateral'}
        >
          {isCollapsed ? <ChevronRight size={16} /> : <ChevronLeft size={16} />}
        </button>
      </div>

      <nav className={styles.nav}>
        {navGroups.map((group) => {
          const items = group.items.filter((item) => item.to !== '/users' || user?.is_superuser)
          if (items.length === 0) return null
          return (
            <div key={group.label} className={styles.navGroup}>
              <div className={styles.menuLabel}>{group.label}</div>
              <ul className={styles.navList}>
                {items.map((item) => (
                  <li key={item.to}>
                    <NavLink
                      to={item.to}
                      end={item.to === '/'}
                      onClick={() => closeMobileMenu?.(false)}
                      className={({ isActive }) =>
                        `${styles.navLink} ${isActive ? styles.navLinkActive : ''}`
                      }
                      title={isCollapsed ? item.label : undefined}
                    >
                      {({ isActive }) => (
                        <>
                          <item.icon
                            size={18}
                            strokeWidth={isActive ? 1.8 : 1.5}
                            className={styles.navIcon}
                          />
                          <span className={styles.navLabel}>{item.label}</span>
                        </>
                      )}
                    </NavLink>
                  </li>
                ))}
              </ul>
            </div>
          )
        })}
      </nav>

      <div className={styles.footer}>
        <div className={styles.userWell} title={isCollapsed ? displayName : undefined}>
          <div className={styles.avatar}>{userInitial}</div>
          <div className={styles.userMeta}>
            <div className={styles.userName}>{displayName}</div>
            <div className={styles.userRole}>{displayRole}</div>
          </div>
          <button
            type="button"
            onClick={handleLogout}
            className={styles.logoutButton}
            aria-label="Cerrar sesión"
            title="Cerrar sesión"
          >
            <LogOut size={16} />
          </button>
        </div>
      </div>
    </aside>
  )
}
