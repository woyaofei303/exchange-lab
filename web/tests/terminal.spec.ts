import { expect, test } from '@playwright/test'
import { execFileSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'

const repository = fileURLToPath(new URL('../..', import.meta.url))

test.afterEach(() => {
  execFileSync('docker', ['compose', 'start', 'api'], { cwd: repository, stdio: 'ignore' })
})

test('loads market data, switches interval, and recovers its live stream', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('heading', { name: '永续合约实验室' })).toBeVisible()
  await expect(page.getByTestId('last-price')).not.toHaveText('—')
  await expect(page.getByTestId('kline-chart')).toBeVisible()
  await page.getByRole('button', { name: '5m' }).click()
  await expect(page.getByRole('button', { name: '5m' })).toHaveAttribute('aria-pressed', 'true')
  await expect(page.getByTestId('connection-state')).toContainText('实时')

  execFileSync('docker', ['compose', 'stop', 'api'], { cwd: repository, stdio: 'ignore' })
  await expect(page.getByTestId('connection-state')).toContainText('重连中')

  execFileSync('docker', ['compose', 'start', 'api'], { cwd: repository, stdio: 'ignore' })
  await expect(page.getByTestId('connection-state')).toContainText('实时', { timeout: 20_000 })
})
