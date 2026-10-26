import { Component, type ReactNode } from 'react'
import styles from './ErrorBoundary.module.css'

interface Props {
  children: ReactNode
}

interface State {
  hasError: boolean
}

export default class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false }

  static getDerivedStateFromError(): State {
    return { hasError: true }
  }

  render() {
    if (this.state.hasError) {
      return (
        <div className={styles.fallback}>
          <p className={styles.message}>Algo salió mal.</p>
          <a href="/" className={styles.link}>
            Volver al inicio
          </a>
        </div>
      )
    }
    return this.props.children
  }
}
