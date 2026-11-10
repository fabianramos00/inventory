import process from 'node:process'
import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  // ponytail: explicit allowlist; new specs are invisible until registered here.
  testMatch: [
    'auth.spec.ts',
    'users.spec.ts',
    'inventory.spec.ts',
    'sales.spec.ts',
    'orders.spec.ts',
    'dashboard.spec.ts',
    'floating-menu.spec.ts',
    'kpi-strip.spec.ts',
    'filters.spec.ts',
    'keyboard.spec.ts',
    'client-debt.spec.ts',
    'lists.spec.ts',
  ],
  globalSetup: './e2e/global-setup.ts',
  timeout: 60_000,
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: 0,
  reporter: 'list',
  use: {
    baseURL: 'http://127.0.0.1:4173',
    trace: 'retain-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  webServer: [
    {
      command: 'cd .. && go run ./cmd/seed -superuser && go run ./cmd/server',
      url: 'http://127.0.0.1:8787/healthz',
      reuseExistingServer: false,
      timeout: 120_000,
      env: {
        PORT: '8787',
        DATABASE_URL: process.env.DATABASE_URL
          ?? process.env.INVENTORY_TEST_DATABASE_URL
          ?? 'postgresql://inventory:inventory@127.0.0.1:5432/inventory_desktop_test',
        DB_MODE: process.env.DB_MODE ?? 'postgres',
        SECRET_KEY: 'e2e-test-secret-0123456789abcdef',
        BCRYPT_ROUNDS: '4',
        AUTO_MIGRATE: 'true',
        SEED_EMAIL: 'admin@example.com',
        SEED_PASSWORD: 'admin12345',
        SEED_NAME: 'Admin User',
        ALLOW_DEV_ORIGINS: 'true',
      },
    },
    {
      command: 'pnpm exec vite --port 4173 --strictPort',
      url: 'http://127.0.0.1:4173',
      reuseExistingServer: false,
      env: { VITE_API_URL: 'http://127.0.0.1:8787/api/v1' },
    },
  ],
})
