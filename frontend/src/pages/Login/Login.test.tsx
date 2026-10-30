import { fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import Login from './Login'

const mocks = vi.hoisted(() => ({
  loginRequest: vi.fn(),
}))

vi.mock('@/lib/api/auth', () => ({
  authApi: { login: mocks.loginRequest },
}))

vi.mock('@/context/AuthContext', () => ({
  useAuth: () => ({ login: vi.fn() }),
}))

function submit() {
  fireEvent.change(screen.getByLabelText('Correo electrónico'), {
    target: { value: 'admin@example.com' },
  })
  fireEvent.change(screen.getByLabelText('Contraseña'), {
    target: { value: 'admin12345' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Ingresar' }))
}

describe('Login error messages', () => {
  beforeEach(() => {
    mocks.loginRequest.mockReset()
    render(
      <MemoryRouter>
        <Login />
      </MemoryRouter>,
    )
  })

  it.each([
    ['401 credentials', { isAxiosError: true, response: { status: 401 } }, 'Correo o contraseña incorrectos.'],
    ['network/CORS failure', { isAxiosError: true }, 'No se pudo conectar con el servidor. Intenta de nuevo.'],
    ['unexpected server error', { isAxiosError: true, response: { status: 500 } }, 'Ocurrió un error inesperado. Intenta de nuevo.'],
  ])('shows the right message for %s', async (_name, error, expected) => {
    mocks.loginRequest.mockRejectedValue(error)

    submit()

    expect((await screen.findByRole('alert')).textContent).toBe(expected)
  })
})
