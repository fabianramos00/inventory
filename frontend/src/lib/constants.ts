export const PAGE_LIMIT = 10

// --- Sales ---
export const paymentStatusLabel: Record<string, string> = {
  pending: 'Pendiente',
  paid: 'Pagado',
  partial: 'Parcial',
}

export const paymentStatusBadge: Record<string, string> = {
  pending: 'badge--neutral',
  paid: 'badge--success',
  partial: 'badge--warning',
}

export const deliveryStatusLabel: Record<string, string> = {
  pending: 'Pendiente',
  partial: 'Parcial',
  delivered: 'Entregado',
}

export const deliveryStatusBadge: Record<string, string> = {
  pending: 'badge--neutral',
  partial: 'badge--warning',
  delivered: 'badge--success',
}

export const paymentMethodLabel: Record<string, string> = {
  cash: 'Efectivo',
  credit: 'Crédito',
  debit: 'Débito',
}

// --- Orders ---
export const orderStatusLabel: Record<string, string> = {
  pending: 'Pendiente',
  sent: 'Enviado',
  received: 'Recibido',
  delivered: 'Recibido',
  cancelled: 'Cancelado',
}

export const orderStatusBadge: Record<string, string> = {
  pending: 'badge--neutral',
  sent: 'badge--blue',
  received: 'badge--success',
  delivered: 'badge--success',
  cancelled: 'badge--neutral',
}

export const orderPaymentStatusLabel: Record<string, string> = {
  pending: 'Pendiente',
  paid: 'Pagado',
}

export const orderPaymentStatusBadge: Record<string, string> = {
  pending: 'badge--neutral',
  paid: 'badge--success',
}

// --- Products ---
export const productStatusMap: Record<string, string> = {
  low_stock: 'badge--warning',
  in_stock: 'badge--success',
  out_of_stock: 'badge--destructive',
}

export const productStatusLabel: Record<string, string> = {
  low_stock: 'Bajo mínimo',
  in_stock: 'En stock',
  out_of_stock: 'Sin stock',
}
