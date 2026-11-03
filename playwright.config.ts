import { defineConfig, devices } from '@playwright/test'

// Runs against the Vite dev server with the core stub, headless, so no window reaches the user's screen.
export default defineConfig({
  testDir: 'e2e',
  outputDir: 'test-results',
  timeout: 30_000,
  forbidOnly: true,
  reporter: 'list',
  use: {
    baseURL: 'http://127.0.0.1:1421',
    headless: true,
    viewport: { width: 1180, height: 760 },
    locale: 'ru-RU',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'], viewport: { width: 1180, height: 760 } } }],
  webServer: {
    command: 'npx vite --port 1421 --strictPort',
    url: 'http://127.0.0.1:1421',
    reuseExistingServer: false,
    timeout: 60_000,
  },
})
