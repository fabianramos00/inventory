import type { FieldConfig } from '@/components/CreateFormModal/CreateFormModal'

export const CLIENT_FIELDS: FieldConfig[] = [
  { key: 'name', label: 'Nombre del Cliente', placeholder: 'Ej. Fabian Rodríguez', required: true },
  { key: 'identity_card', label: 'Cédula', placeholder: 'Ej. 1052416157', type: 'text' },
  { key: 'email', label: 'Correo Electrónico', placeholder: 'Ej. fabian@gmail.com', type: 'email' },
  { key: 'phone', label: 'Teléfono', placeholder: 'Ej. 3133120107', type: 'tel' },
]

export const PROVIDER_FIELDS: FieldConfig[] = [
  { key: 'name', label: 'Nombre del Proveedor', placeholder: 'Ej. Distribuidora Metálica', required: true },
  { key: 'contact_info', label: 'Persona de Contacto', placeholder: 'Ej. Juan Pérez', type: 'text' },
  { key: 'email', label: 'Correo Electrónico', placeholder: 'Ej. correo@proveedor.co', type: 'email' },
  { key: 'phone', label: 'Teléfono', placeholder: 'Ej. +57 300 123 4567', type: 'tel' },
]

export function buildClientCreatePayload(values: Record<string, string>) {
  return {
    name: values.name,
    ...(values.identity_card && { identity_card: values.identity_card }),
    ...(values.email && { email: values.email }),
    ...(values.phone && { phone: values.phone }),
  }
}

export function buildProviderCreatePayload(values: Record<string, string>) {
  return {
    name: values.name,
    ...(values.contact_info && { contact_info: values.contact_info }),
    ...(values.email && { email: values.email }),
    ...(values.phone && { phone: values.phone }),
  }
}
