export function bogotaDayBounds(startDate?: string, endDate?: string) {
  return {
    start_date: startDate ? new Date(`${startDate}T00:00:00-05:00`).toISOString() : undefined,
    end_date: endDate ? new Date(`${endDate}T23:59:59.999-05:00`).toISOString() : undefined,
  }
}
