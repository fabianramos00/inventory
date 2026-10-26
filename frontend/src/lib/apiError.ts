import axios from 'axios'

export function getApiErrorCode(error: unknown): string | undefined {
  if (!axios.isAxiosError(error)) return undefined
  const code = (error.response?.data as { code?: unknown } | undefined)?.code
  return typeof code === 'string' ? code : undefined
}

export function getApiErrorDetail(error: unknown): string | undefined {
  if (!axios.isAxiosError(error)) return undefined
  const detail = (error.response?.data as { detail?: unknown } | undefined)?.detail
  return typeof detail === 'string' ? detail : undefined
}
