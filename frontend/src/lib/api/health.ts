import axios from 'axios'
import { getApiBaseUrl } from '@/lib/apiBase'

export type DatabaseMode = 'sqlite' | 'postgres'

export interface HealthResponse {
  status: string
  database: DatabaseMode
}

export function getHealthUrl() {
  return `${getApiBaseUrl().replace(/\/api\/v1\/?$/, '')}/healthz`
}

export async function fetchHealth() {
  const res = await axios.get<HealthResponse>(getHealthUrl())
  return res.data
}
