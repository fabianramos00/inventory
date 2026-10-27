import process from 'node:process'
import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  // ponytail: full-stack specs only; the legacy mocked suite still runs manually.
  testMatch: ['auth.spec.ts', 'users.spec.ts'],
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
        DATABASE_URL: 'postgresql://inventory:inventory@localhost:5433/inventory_desktop_test',
        SECRET_KEY: 'e2e-test-secret-0123456789abcdef',
        BCRYPT_ROUNDS: '4',
        AUTO_MIGRATE: 'true',
        SEED_EMAIL: 'admin@example.com',
        SEED_PASSWORD: 'admin12345',
        SEED_NAME: 'Admin User',
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
