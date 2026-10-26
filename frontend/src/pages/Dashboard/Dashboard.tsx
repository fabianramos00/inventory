import { Link } from 'react-router-dom'
import PageHeader from '@/components/PageHeader/PageHeader'
import DataCard from '@/components/DataCard/DataCard'
import {
  Tooltip,
  XAxis, YAxis, CartesianGrid,
  LineChart, Line, ResponsiveContainer,
} from 'recharts'
import { type SalesTrendData } from '@/lib/api/dashboard'
import DataTable from '@/components/DataTable/DataTable'
import KpiStrip, { type KpiCardConfig } from '@/components/KpiStrip/KpiStrip'
import ErrorState from '@/components/ErrorState/ErrorState'
import StockMeter from '@/components/StockMeter/StockMeter'
import { useDashboard } from '@/hooks/useDashboard'
import { orderStatusLabel } from '@/lib/constants'
import { formatCurrency, formatNumber } from '@/lib/utils'
import styles from './Dashboard.module.css'

const ORDER_STATUS_ORDER = ['pending', 'sent', 'received', 'cancelled']

function normalizeOrderStatus(status: string) {
  return status === 'delivered' ? 'received' : status
}

function getSpanishDayName(dateStr: string): string {
  const date = new Date(dateStr + 'T00:00:00')
  const dayIndex = date.getUTCDay()
  const days = ['Dom', 'Lun', 'Mar', 'Mié', 'Jue', 'Vie', 'Sáb']
  return days[dayIndex]
}

interface VentasTooltipProps {
  active?: boolean
  label?: string
  payload?: ReadonlyArray<{ value?: number | string }>
}

function VentasTooltip({ active, label, payload }: VentasTooltipProps) {
  if (!active || !payload?.length) return null
  return (
    <div className={styles.tooltip}>
      <span className={styles.tooltipLabel}>{label}</span>
      <span className={styles.tooltipValue}>{formatCurrency(Number(payload[0]?.value ?? 0))}</span>
    </div>
  )
}

export default function Dashboard() {
  const { summary, trend, topProducts, orderStatus, lowStock, lowStockTotal, isLoading, errors, refetch } = useDashboard()

  if (isLoading) {
    return (
      <div className={styles.container}>
        <PageHeader activeLabel="Resumen" title="Dashboard" />
        <div className={styles.skeletonKpiStrip} data-lead="true">
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className={`${styles.skeletonKpiCell} ${i === 0 ? styles.skeletonKpiCellLead : ''}`}>
              <span className={styles.skeletonBarShort} />
              <span className={styles.skeletonBar} />
            </div>
          ))}
        </div>
        <div className={styles.mainGrid}>
          <div className={`${styles.skeletonCard} ${styles.skeletonTrend}`} />
          <div className={styles.skeletonRail}>
            <div className={styles.skeletonCard} />
            <div className={styles.skeletonCard} />
          </div>
        </div>
        <div className={`${styles.skeletonCard} ${styles.skeletonTable}`} />
      </div>
    )
  }

  const unpaidSales = summary?.pending_payment_sales ?? 0
  const lowStockItems = lowStock ?? []
  const lowStockCount = lowStockTotal ?? lowStockItems.length
  const pendingOrders = summary?.pending_orders ?? 0

  const finding = errors.summary && !summary
    ? undefined
    : unpaidSales > 0
      ? `${formatNumber(unpaidSales)} ventas sin pagar requieren seguimiento.`
      : lowStockCount > 0
        ? `${formatNumber(lowStockCount)} productos bajo el mínimo.`
        : 'Sin pendientes: ventas y stock al día.'

  const contextualAction = unpaidSales > 0
    ? { to: '/sales', label: 'Revisar ventas sin pagar' }
    : lowStockCount > 0
      ? { to: '/inventory', label: 'Ver stock bajo' }
      : null

  const kpiCards: KpiCardConfig[] = [
    { label: 'Ventas de hoy', value: formatCurrency(summary?.total_sales_today ?? 0), lead: true },
    { label: 'Ventas sin pagar', value: formatNumber(unpaidSales) },
    { label: 'Pedidos pendientes', value: formatNumber(pendingOrders) },
    { label: 'Stock bajo', value: formatNumber(lowStockCount) },
  ]

  const weeklyData = (trend && trend.length > 0)
    ? trend.map((item: SalesTrendData) => ({
        dia: getSpanishDayName(item.date),
        ventas: item.sales_amount,
      }))
    : []

  const weekTotal = weeklyData.reduce((sum, item) => sum + item.ventas, 0)

  const renderLineDot = (props: { cx?: number; cy?: number; index?: number }) => {
    if (props.index !== weeklyData.length - 1) return <g />
    return (
      <circle
        cx={props.cx}
        cy={props.cy}
        r={3}
        strokeWidth={1}
        className={styles.lineDotLast}
      />
    )
  }

  const mergedOrderStatus = (orderStatus ?? []).reduce<Record<string, { status: string; count: number; percentage: number }>>((acc, item) => {
    const status = normalizeOrderStatus(item.status)
    const current = acc[status]
    acc[status] = current
      ? { ...current, count: current.count + item.count, percentage: current.percentage + item.percentage }
      : { status, count: item.count, percentage: item.percentage }
    return acc
  }, {})

  const orderStatusRows = Object.values(mergedOrderStatus)
    .sort((a, b) => ORDER_STATUS_ORDER.indexOf(a.status) - ORDER_STATUS_ORDER.indexOf(b.status))
    .map((item) => ({
      status: item.status,
      label: orderStatusLabel[item.status] ?? item.status,
      count: item.count,
      percentage: item.percentage,
    }))

  const lowStockData = lowStockItems.slice(0, 3).map((item) => ({
    sku: item.sku,
    name: item.name,
    stock: item.stock_quantity ?? 0,
    min: item.low_stock_threshold || 0,
    unit: item.measurement_unit?.abbreviation || item.unit || 'pzas',
  }))

  const topProductsData = (topProducts ?? []).slice(0, 5).map((p) => ({
    sku: p.product_sku,
    name: p.product_name,
    brand: p.brand_name,
    ventas: p.total_quantity_sold,
    revenue: p.total_sales_amount,
  }))

  return (
    <div className={styles.container}>
      <PageHeader
        activeLabel="Resumen"
        title="Dashboard"
        finding={finding}
        action={contextualAction && (
          <Link className={styles.contextAction} to={contextualAction.to}>
            {contextualAction.label}
          </Link>
        )}
      />

      {errors.summary && !summary ? (
        <ErrorState
          className={styles.summaryError}
          message="No se pudieron cargar los indicadores."
          onRetry={refetch.summary}
        />
      ) : (
        <KpiStrip cards={kpiCards} />
      )}

      <div className={styles.mainGrid}>
        <DataCard title="Ventas de los últimos 7 días" className={styles.trendCard}>
          <div className={styles.chartBody}>
            {!trend || trend.length === 0 ? (
              errors.trend ? (
                <ErrorState message="No se pudo cargar la tendencia de ventas." onRetry={refetch.trend} />
              ) : (
                <p className={styles.cardEmpty}>Sin datos de tendencia de ventas.</p>
              )
            ) : (
              <div className={styles.chart}>
                <ResponsiveContainer width="100%" height={210}>
                  <LineChart data={weeklyData} margin={{ top: 12, right: 12, left: 0, bottom: 0 }}>
                    <CartesianGrid vertical={false} strokeDasharray="3 3" />
                    <XAxis
                      dataKey="dia"
                      axisLine={false}
                      tickLine={false}
                      minTickGap={12}
                      tick={{ fontSize: 12 }}
                      dy={8}
                    />
                    <YAxis
                      allowDecimals={false}
                      axisLine={false}
                      tickLine={false}
                      width={48}
                      tickCount={4}
                      tick={{ fontSize: 12 }}
                      tickFormatter={(value: number) => formatNumber(value)}
                    />
                    <Tooltip content={<VentasTooltip />} cursor={{ stroke: 'var(--border-strong)' }} />
                    <Line
                      type="linear"
                      dataKey="ventas"
                      stroke="currentColor"
                      strokeWidth={2}
                      dot={renderLineDot}
                      activeDot={{ r: 4 }}
                      isAnimationActive={false}
                    />
                  </LineChart>
                </ResponsiveContainer>
              </div>
            )}
          </div>
          {weeklyData.length > 0 && (
            <div className={styles.chartFooter}>
              <span className={styles.chartFooterLabel}>Total 7 días</span>
              <span className={styles.chartFooterValue}>{formatCurrency(weekTotal)}</span>
            </div>
          )}
        </DataCard>

        <div className={styles.rail}>
          <DataCard title="Pedidos" className={styles.railCard}>
            {orderStatusRows.length === 0 ? (
              errors.orderStatus ? (
                <ErrorState message="No se pudo cargar el estado de pedidos." onRetry={refetch.orderStatus} />
              ) : (
                <p className={styles.cardEmpty}>Sin datos de estado de pedidos.</p>
              )
            ) : (
              <div className={styles.statusList}>
                {orderStatusRows.map((row) => (
                  <div key={row.status} className={styles.statusRow}>
                    <span className={styles.statusLabel}>
                      {row.status === 'pending' && row.count > 0 && <span className={styles.attentionDot} />}
                      {row.label}
                    </span>
                    <span className={styles.statusMeta}>
                      <span className={styles.statusCount}>{formatNumber(row.count)}</span>
                      <span className={styles.statusPct}>{formatNumber(row.percentage)}%</span>
                    </span>
                  </div>
                ))}
              </div>
            )}
          </DataCard>

          <DataCard
            title="Stock bajo"
            className={styles.railCard}
            action={<Link className={styles.cardLink} to="/inventory">Ver inventario</Link>}
          >
            {lowStockData.length === 0 ? (
              errors.lowStock ? (
                <ErrorState message="No se pudo cargar el stock bajo." onRetry={refetch.lowStock} />
              ) : (
                <p className={styles.cardEmpty}>Nada bajo el mínimo</p>
              )
            ) : (
              <div className={styles.stockList}>
                {lowStockData.map((item) => {
                  const tone = item.stock === 0 ? 'danger' : 'warning'
                  return (
                    <div key={item.sku} className={styles.stockRow}>
                      <div className={styles.stockTop}>
                        <span className={styles.stockName}>{item.name}</span>
                        <span className={styles.stockSku}>{item.sku}</span>
                      </div>
                      <StockMeter
                        value={item.stock}
                        max={item.min || item.stock || 1}
                        tone={tone}
                        label={`${item.name}: ${item.stock} ${item.unit} de mínimo ${item.min}`}
                      />
                    </div>
                  )
                })}
              </div>
            )}
          </DataCard>
        </div>
      </div>

      <DataCard title="Top productos">
        <DataTable
          loading={false}
          empty={topProductsData.length === 0}
          error={errors.topProducts && !topProducts}
          errorText="No se pudieron cargar los productos vendidos."
          onRetry={refetch.topProducts}
          emptyText="Sin productos vendidos."
        >
          <thead>
            <tr>
              <th scope="col">Producto</th>
              <th scope="col">Marca</th>
              <th scope="col" className="num">Unidades</th>
              <th scope="col" className="num">Ingresos</th>
            </tr>
          </thead>
          <tbody>
            {topProductsData.map((p) => (
              <tr key={p.sku}>
                <td>
                  <span className={styles.productName}>{p.name}</span>
                  <span className={styles.productSku}>{p.sku}</span>
                </td>
                <td><span className={styles.brandText}>{p.brand}</span></td>
                <td className="num">{formatNumber(p.ventas)}</td>
                <td className="num">{formatCurrency(p.revenue)}</td>
              </tr>
            ))}
          </tbody>
        </DataTable>
      </DataCard>
    </div>
  )
}
