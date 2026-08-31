import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './tests',
  timeout: 30_000,
  retries: 0,
  workers: 1,
  use: {
    baseURL: process.env.WEB_BASE_URL ?? 'http://127.0.0.1:3000',
    trace: 'retain-on-failure',
  },
  outputDir: '../output-tdd/playwright/week-21/results',
  reporter: [['list'], ['html', { outputFolder: '../output-tdd/playwright/week-21/report', open: 'never' }]],
})
