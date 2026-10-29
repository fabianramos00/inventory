import { describe, expect, it } from 'vitest'
import { getAPIErrorMessage, getApiErrorCode, getApiErrorDetail } from './apiError'

function axiosError(data: unknown) {
  return Object.assign(new Error('Request failed'), {
    isAxiosError: true,
    response: { data },
  })
}

describe('apiError helpers', () => {
  it('reads code and detail from an Axios-shaped error', () => {
    const error = axiosError({ code: 'FOREIGN_KEY_VIOLATION', detail: 'Cannot delete' })
    expect(getApiErrorCode(error)).toBe('FOREIGN_KEY_VIOLATION')
    expect(getApiErrorDetail(error)).toBe('Cannot delete')
  })

  it('ignores plain Error instances', () => {
    const error = new Error('boom')
    expect(getApiErrorCode(error)).toBeUndefined()
    expect(getApiErrorDetail(error)).toBeUndefined()
  })

  it('ignores unknown values and malformed payloads', () => {
    expect(getApiErrorCode('nope')).toBeUndefined()
    expect(getApiErrorDetail(undefined)).toBeUndefined()
    expect(getApiErrorCode(axiosError({ code: 42 }))).toBeUndefined()
    expect(getApiErrorDetail(axiosError({ detail: { message: 'x' } }))).toBeUndefined()
  })
})

describe('getAPIErrorMessage', () => {
  it('returns the backend detail for an Axios error', () => {
    const error = axiosError({ detail: 'A client with this identity card already exists.' })
    expect(getAPIErrorMessage(error, 'Error al guardar')).toBe('A client with this identity card already exists.')
  })

  it('falls back to the error message when there is no detail', () => {
    expect(getAPIErrorMessage(new Error('boom'), 'Error al guardar')).toBe('boom')
    expect(getAPIErrorMessage(axiosError({ detail: 42 }), 'Error al guardar')).toBe('Request failed')
  })

  it('uses the fallback for unknown values', () => {
    expect(getAPIErrorMessage(undefined, 'Error al guardar')).toBe('Error al guardar')
  })
})
