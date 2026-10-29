import { useEffect, useState } from 'react'
import { Database } from 'lucide-react'
import { fetchHealth, type DatabaseMode } from '@/lib/api/health'
import styles from './OfflineDatabaseBanner.module.css'

export default function OfflineDatabaseBanner() {
  const [database, setDatabase] = useState<DatabaseMode | null>(null)

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

  if (database !== 'sqlite') return null

  return (
    <div className={styles.banner} role="status">
      <Database size={18} aria-hidden="true" className={styles.icon} />
      <p className={styles.message}>
        Modo local: los datos se guardan en este equipo y no se sincronizan con
        otros dispositivos. El archivo local no está cifrado; protege el equipo y
        realiza copias de seguridad.
      </p>
    </div>
  )
}
