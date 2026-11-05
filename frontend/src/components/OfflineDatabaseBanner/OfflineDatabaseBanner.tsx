import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { Database } from 'lucide-react'
import { fetchHealth, type DatabaseMode } from '@/lib/api/health'
import styles from './OfflineDatabaseBanner.module.css'

const BANNER_HEIGHT_VARIABLE = '--offline-banner-height'

export default function OfflineDatabaseBanner() {
  const [database, setDatabase] = useState<DatabaseMode | null>(null)
  const bannerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    let active = true
    fetchHealth()
      .then((health) => {
        if (active) setDatabase(health.database)
      })
      .catch(() => {
        if (active) setDatabase(null)
      })
    return () => {
      active = false
    }
  }, [])

  useLayoutEffect(() => {
    const root = document.documentElement
    const banner = bannerRef.current
    if (database !== 'sqlite' || !banner) {
      root.style.removeProperty(BANNER_HEIGHT_VARIABLE)
      return
    }

    const updateHeight = () => {
      root.style.setProperty(
        BANNER_HEIGHT_VARIABLE,
        `${banner.getBoundingClientRect().height}px`,
      )
    }

    updateHeight()
    if (typeof ResizeObserver === 'undefined') {
      return () => root.style.removeProperty(BANNER_HEIGHT_VARIABLE)
    }

    const observer = new ResizeObserver(updateHeight)
    observer.observe(banner)
    return () => {
      observer.disconnect()
      root.style.removeProperty(BANNER_HEIGHT_VARIABLE)
    }
  }, [database])

  if (database !== 'sqlite') return null

  return (
    <div ref={bannerRef} className={styles.banner} role="status">
      <Database size={18} aria-hidden="true" className={styles.icon} />
      <p className={styles.message}>
        Modo local: los datos se guardan en este equipo y no se sincronizan con
        otros dispositivos. El archivo local no está cifrado; protege el equipo y
        realiza copias de seguridad.
      </p>
    </div>
  )
}
