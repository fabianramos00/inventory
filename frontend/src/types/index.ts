// ─── Auth ────────────────────────────────────────────────────────────────────
export interface LoginPayload {
  email: string
  password: string
}
export interface AuthResponse {
  access_token: string
  token_type: string
  refresh_token: string
  user: User
}

// ─── User ────────────────────────────────────────────────────────────────────
export interface User {
  id: number
  full_name: string
  email: string
  is_superuser: boolean
  is_active: boolean
}

// ─── Category / Material ─────────────────────────────────────────────────────
export interface Category {
  id: number
  code: string
  name: string
  description?: string
}

// ─── Product / Inventory ─────────────────────────────────────────────────────
export interface Product {
  id: number
  name: string
  sku: string
  category: Category | null
  price: number
  // Extended fields returned by the API
  stock_quantity?: number
  low_stock_threshold?: number
  size_value?: string | null
  brand_id?: number
  category_id?: number
  material_id?: number
  measurement_unit_id?: number | null
  measurement_unit?: { id: number; name: string; abbreviation: string } | null
  material?: { id: number; name: string } | null
  brand?: { id: number; name: string } | null
  cost?: number
  sale_price?: number
  // Legacy aliases kept for compatibility with older payloads
  stock?: number
  min_stock?: number
  unit?: string
}

// ─── Provider / Supplier ─────────────────────────────────────────────────────
export interface Provider {
  id: number
  name: string
  contact_info: string | null
  email: string | null
  phone: string | null
}

// ─── Client ───────────────────────────────────────────────────────────────────
export interface Client {
  id: number
  name: string
  identity_card?: string | null
  email?: string | null
  phone?: string | null
  created_at: string
  updated_at: string | null
}

// ─── Order ───────────────────────────────────────────────────────────────────
export type OrderStatus = 'pending' | 'sent' | 'received' | 'cancelled'
export type OrderPaymentStatus = 'pending' | 'paid'

export interface OrderItem {
  id: number
  order_id: number
  product_id: number
  quantity: number
  unit_cost: number
  subtotal: number
  supplier_sku: string | null
  product_name: string
  product_sku: string
}

export interface Order {
  id: number
  provider_id: number
  status: OrderStatus
  payment_status: OrderPaymentStatus
  order_date: string
  total_amount: number
  provider: {
    id: number
    name: string
    contact_info: string | null
    email: string | null
    phone: string | null
  }
}

// ─── Sale ────────────────────────────────────────────────────────────────────
export interface SaleItem {
  id: number
  sale_id: number
  product_id: number
  quantity: number
  delivered_quantity: number
  unit_price: number
  subtotal: number
  product_name: string
  product_sku: string
  brand_name: string | null
  material_name: string | null
  category_name: string | null
}

export type PaymentStatus = 'pending' | 'paid' | 'partial'
export type DeliveryStatus = 'pending' | 'partial' | 'delivered'
export type PaymentMethod = 'cash' | 'credit' | 'debit' | null

export interface Sale {
  id: number
  total_amount: number
  debt_amount: number
  payment_status: PaymentStatus
  delivery_status: DeliveryStatus
  payment_method: PaymentMethod
  amount_paid: number
  client_id: number | null
  created_at: string
  updated_at: string | null
  created_by_id: number
  created_by: Pick<User, 'id' | 'full_name' | 'email' | 'is_active' | 'is_superuser'>
  client: { id: number; name: string } | null
}

// ─── Pagination ───────────────────────────────────────────────────────────────
export interface PaginatedResponse<T> {
  items: T[]
  total: number
  page: number
  size: number
  pages: number
}

export interface CursorPaginatedResponse<T> {
  items: T[]
  has_next: boolean
  skip: number
  limit: number
}

// ─── Dropdown / Filter Options ────────────────────────────────────────────────
export interface FilterOption {
  id: number
  name: string
  abbreviation?: string
  description?: string
  logo_url?: string
}

// ─── Sale Creation Form ───────────────────────────────────────────────────────
export interface CreateSaleInput {
  payment_method: PaymentMethod
  amount_paid: number
  client_id: number | null
  items: {
    product_id: number
    quantity: number
    delivered_quantity: number
    unit_price: number | null
  }[]
}

// ─── Order Creation Form ──────────────────────────────────────────────────────
export interface CreateOrderInput {
  provider_id: number
  status: OrderStatus
  payment_status: OrderPaymentStatus
  items: {
    product_id: number
    quantity: number
    unit_cost: number
    supplier_sku?: string
  }[]
}

// ─── Product Creation Form ────────────────────────────────────────────────────
export interface CreateProductInput {
  name: string
  sku?: string
  brand_id: number
  category_id: number
  material_id: number
  size_value?: string | null
  measurement_unit_id?: number | null
  price: number
  cost: number
  stock_quantity: number
  low_stock_threshold: number
}

// ─── InventoryStats ───────────────────────────────────────────────────────────────
export interface InventoryStatsData {
  total_products: number
  total_stock: number
  low_stock_count: number
  total_inventory_value: number
}
