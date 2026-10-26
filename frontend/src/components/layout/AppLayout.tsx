import { useState, useEffect, useRef } from 'react'
import { Outlet, useLocation } from 'react-router-dom'
import { Menu } from 'lucide-react'
import Sidebar from './Sidebar'
import styles from './AppLayout.module.css'

export default function AppLayout() {
  const [isMobileMenuOpen, setIsMobileMenuOpen] = useState(false)
  const [isSidebarCollapsed, setIsSidebarCollapsed] = useState(false)
  const menuButtonRef = useRef<HTMLButtonElement>(null)
  const location = useLocation()

  useEffect(() => {
    const saved = localStorage.getItem('sidebarCollapsed')
    if (saved !== null) {
      try {
        // eslint-disable-next-line react-hooks/set-state-in-effect -- hydrate persisted sidebar state on mount
        setIsSidebarCollapsed(JSON.parse(saved))
      } catch { /* ignore corrupted value */ }
    }
  }, [])

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- navigation always dismisses the mobile sheet
    setIsMobileMenuOpen(false)
  }, [location.pathname])

  useEffect(() => {
    if (!isMobileMenuOpen) return
    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === 'Escape') {
        setIsMobileMenuOpen(false)
        menuButtonRef.current?.focus()
      }
    }
    document.addEventListener('keydown', handleKeyDown)
    return () => {
      document.body.style.overflow = previousOverflow
      document.removeEventListener('keydown', handleKeyDown)
    }
  }, [isMobileMenuOpen])

  const closeMobileMenu = (returnFocus = true) => {
    setIsMobileMenuOpen(false)
    if (returnFocus) menuButtonRef.current?.focus()
  }

  const toggleSidebar = () => {
    const newState = !isSidebarCollapsed
    setIsSidebarCollapsed(newState)
    localStorage.setItem('sidebarCollapsed', JSON.stringify(newState))
  }

  return (
    <div className={styles.shell}>
      <a href="#main-content" className={styles.skipLink}>Saltar al contenido</a>

      <header className={styles.mobileBar}>
        <button
          type="button"
          ref={menuButtonRef}
          className={styles.menuButton}
          onClick={() => setIsMobileMenuOpen(true)}
          aria-label="Abrir menú"
          aria-expanded={isMobileMenuOpen}
          aria-controls="app-sidebar"
        >
          <Menu size={20} />
        </button>
        <span className={styles.mobileBrand}>GestiónStock</span>
      </header>

      {isMobileMenuOpen && (
        <div className={styles.sheetScrim} onClick={() => closeMobileMenu()} aria-hidden="true" />
      )}

      <Sidebar
        isMobileMenuOpen={isMobileMenuOpen}
        closeMobileMenu={closeMobileMenu}
        isCollapsed={isSidebarCollapsed}
        toggleCollapse={toggleSidebar}
      />

      <main className={styles.main} data-collapsed={isSidebarCollapsed}>
        <div id="main-content" className={styles.content} tabIndex={-1}>
          <Outlet />
        </div>
      </main>
    </div>
  )
}
