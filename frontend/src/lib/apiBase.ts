export const DEFAULT_API_BASE_URL = 'http://127.0.0.1:8787/api/v1'

let apiBaseUrl = DEFAULT_API_BASE_URL

export function getApiBaseUrl() {
  return apiBaseUrl
}

export function setApiBaseUrl(url: string) {
  apiBaseUrl = url
}
