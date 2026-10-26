import { defineConfig, configDefaults } from 'vitest/config'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'path'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    exclude: [...configDefaults.exclude, 'e2e/**'],
  },
  build: {
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes('node_modules')) return
          if (/\/react(-dom|-router-dom)?\//.test(id)) return 'vendor-react'
          if (id.includes('recharts')) return 'vendor-charts'
          if (/(react-hook-form|zod|@hookform\/resolvers)/.test(id)) return 'vendor-forms'
        },
      },
    },
  },
})
