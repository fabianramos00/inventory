import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { bootstrap } from '@/bootstrap'
import './index.css'
import App from './App.tsx'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 5 * 60 * 1000,
      retry: 2,
      refetchOnWindowFocus: false,
      refetchOnReconnect: true,
    },
    mutations: {
      retry: 1,
    },
  },
})

function renderStartupError() {
  const root = document.getElementById('root')
  if (!root) return

  const message = document.createElement('p')
  message.setAttribute('role', 'alert')
  message.textContent = 'No se pudo iniciar la aplicación. Reinicia la aplicación.'
  root.replaceChildren(message)
}

async function start() {
  try {
    await bootstrap()
  } catch {
    renderStartupError()
    return
  }

  createRoot(document.getElementById('root')!).render(
    <StrictMode>
      <QueryClientProvider client={queryClient}>
        <App />
      </QueryClientProvider>
    </StrictMode>,
  )
}

void start()
