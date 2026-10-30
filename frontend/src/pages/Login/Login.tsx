import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import axios from 'axios'
import { Eye, EyeOff, Loader2 } from 'lucide-react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useAuth } from '@/context/AuthContext'
import { authApi } from '@/lib/api/auth'
import loginPhoto from '@/assets/login.jpg'
import styles from './Login.module.css'

const loginSchema = z.object({
  email: z.string().min(1, 'El correo electrónico es requerido').email('Correo electrónico inválido'),
  password: z.string().min(1, 'La contraseña es requerida'),
})

type LoginFormInputs = z.infer<typeof loginSchema>

export default function Login() {
  const { login } = useAuth()
  const navigate = useNavigate()

  const [showPassword, setShowPassword] = useState(false)
  const [loading, setLoading] = useState(false)
  const [apiError, setApiError] = useState<string | null>(null)

  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<LoginFormInputs>({
    resolver: zodResolver(loginSchema),
    defaultValues: {
      email: '',
      password: '',
    },
  })

  async function onSubmit(data: LoginFormInputs) {
    setApiError(null)
    setLoading(true)
    try {
      const res = await authApi.login({ email: data.email, password: data.password })
      login(res.data.access_token, res.data.user, res.data.refresh_token)
      navigate('/', { replace: true })
    } catch (error) {
      if (axios.isAxiosError(error) && error.response?.status === 401) {
        setApiError('Correo o contraseña incorrectos.')
      } else if (axios.isAxiosError(error) && !error.response) {
        setApiError('No se pudo conectar con el servidor. Intenta de nuevo.')
      } else {
        setApiError('Ocurrió un error inesperado. Intenta de nuevo.')
      }
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className={styles.container}>
      <div className={styles.shell}>
        <div className={styles.photoPanel}>
          <img
            src={loginPhoto}
            alt="Ferretería J&S"
            className={styles.photo}
            width={1024}
            height={600}
            fetchPriority="high"
            decoding="async"
          />
          <div className={styles.photoScrim} />

          <div className={styles.photoTop}>
            <div className={styles.photoMark}>GS</div>
            <span className={styles.photoChip}>GestiónStock</span>
          </div>

          <div className={styles.photoBottom}>
            <p className={styles.photoEyebrow}>Inventario · Ventas · Pedidos</p>
            <h1 className={styles.photoTitle}>Ferretería J&S</h1>
            <p className={styles.photoSubtitle}>Soluciones en Construcción y Minería</p>
          </div>
        </div>

        <div className={styles.card}>
          <div className={styles.cardBrand}>
            <div className={styles.cardBrandMark}>GS</div>
            <span className={styles.cardBrandName}>GestiónStock</span>
          </div>

          <div className={styles.formHeader}>
            <h2 className={styles.formTitle}>Bienvenido</h2>
            <p className={styles.formSubtitle}>Ingresa tus datos para continuar.</p>
          </div>

          <form onSubmit={handleSubmit(onSubmit)} className={styles.formBody}>
            <div className={styles.inputGroup}>
              <label htmlFor="email" className={styles.inputLabel}>
                Correo electrónico
              </label>
              <input
                id="email"
                type="email"
                className={styles.inputField}
                placeholder="Ingresa tu correo"
                aria-invalid={errors.email ? true : undefined}
                aria-describedby={errors.email ? 'email-error' : undefined}
                {...register('email')}
                autoComplete="email"
              />
              {errors.email && (
                <p id="email-error" className={styles.errorMessage}>
                  {errors.email.message}
                </p>
              )}
            </div>

            <div className={styles.inputGroup}>
              <label htmlFor="password" className={styles.inputLabel}>
                Contraseña
              </label>
              <div className={styles.inputWrapper}>
                <input
                  id="password"
                  type={showPassword ? 'text' : 'password'}
                  className={`${styles.inputField} ${styles.inputFieldPassword}`}
                  placeholder="••••••••"
                  aria-invalid={errors.password ? true : undefined}
                  aria-describedby={errors.password ? 'password-error' : undefined}
                  {...register('password')}
                  autoComplete="current-password"
                />
                <button
                  type="button"
                  className={styles.passwordToggle}
                  onClick={() => setShowPassword(!showPassword)}
                  aria-label={showPassword ? 'Ocultar contraseña' : 'Mostrar contraseña'}
                  aria-pressed={showPassword}
                  title={showPassword ? 'Ocultar contraseña' : 'Mostrar contraseña'}
                >
                  {showPassword ? <EyeOff size={18} strokeWidth={2} /> : <Eye size={18} strokeWidth={2} />}
                </button>
              </div>
              {errors.password && (
                <p id="password-error" className={styles.errorMessage}>
                  {errors.password.message}
                </p>
              )}
            </div>

            {apiError && (
              <p className={styles.apiError} role="alert">
                {apiError}
              </p>
            )}

            <button type="submit" disabled={loading} className={styles.submitBtn}>
              {loading ? (
                <>
                  <Loader2 size={18} className={styles.spinner} />
                  Iniciando sesión...
                </>
              ) : (
                'Ingresar'
              )}
            </button>
          </form>

          <p className={styles.footer}>Solo personal autorizado</p>
        </div>
      </div>
    </div>
  )
}
