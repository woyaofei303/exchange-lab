import { expect, test, type Browser, type BrowserContext } from '@playwright/test'
import { execFileSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'

const repository = fileURLToPath(new URL('../..', import.meta.url))
const password = 'playwright derivative password 2026'

function sql(query: string): string {
  return execFileSync(
    'docker',
    ['compose', 'exec', '-T', 'postgres', 'psql', '-U', 'exchange', '-d', 'exchange', '-Atc', query],
    {
      cwd: repository,
      encoding: 'utf8',
    },
  ).trim()
}

async function post(context: BrowserContext, path: string, data: unknown, key?: string) {
  const response = await context.request.post(path, {
    data,
    headers: key ? { 'Idempotency-Key': key } : undefined,
  })
  expect(response.ok(), `${path}: ${await response.text()}`).toBeTruthy()
  return response.json()
}

async function createTrader(context: BrowserContext, email: string, key: string) {
  await post(context, '/v1/auth/register', { email, password })
  await post(context, '/v1/auth/login', { email, password })
  await post(context, '/v1/faucet', { amount: '10000.00000000' }, `faucet-${key}`)
  await post(context, '/v1/leverage', { symbol: 'BTCUSDT-PERP', leverage: 10 })
}

async function clearStalePlaywrightOrders(browser: Browser) {
  const rows = sql(
    "SELECT u.email||'|'||o.id FROM orders o JOIN users u ON u.id=o.user_id WHERE o.status='open' AND u.email LIKE 'pw-%@example.test' ORDER BY o.created_at",
  )
  if (!rows) return
  for (const row of rows.split('\n')) {
    const [email, orderID] = row.split('|')
    const context = await browser.newContext()
    try {
      await post(context, '/v1/auth/login', { email, password })
      const response = await context.request.post(`/v1/orders/${orderID}/cancel`, {
        headers: { 'Idempotency-Key': `pw-cleanup-${orderID}` },
      })
      expect(response.ok(), `cleanup ${orderID}: ${await response.text()}`).toBeTruthy()
    } finally {
      await context.close()
    }
  }
}

test('two users open, restore, and close a perpetual position', async ({ browser }) => {
  const suffix = `${Date.now()}-${process.pid}`
  const seller = await browser.newContext()
  const buyer = await browser.newContext()
  try {
    await clearStalePlaywrightOrders(browser)
    await createTrader(seller, `pw-seller-${suffix}@example.test`, `seller-${suffix}`)
    await createTrader(buyer, `pw-buyer-${suffix}@example.test`, `buyer-${suffix}`)

    const mark = Number(
      sql(
        "SELECT mark_price_atoms FROM mark_prices WHERE symbol='BTCUSDT-PERP' ORDER BY created_at DESC,id DESC LIMIT 1",
      ),
    )
    const maxBid = Number(sql("SELECT COALESCE(max(price_atoms),0) FROM matching_book_orders WHERE side='buy'"))
    const minAsk = Number(sql("SELECT COALESCE(min(price_atoms),0) FROM matching_book_orders WHERE side='sell'"))
    const atoms = maxBid > 0 ? maxBid + 10 : mark
    expect(minAsk === 0 || atoms < minAsk).toBeTruthy()
    const price = `${Math.floor(atoms / 100)}.${String(atoms % 100).padStart(2, '0')}`
    const order = (side: 'buy' | 'sell', id: string) => ({
      symbol: 'BTCUSDT-PERP',
      side,
      type: 'limit',
      price,
      quantity: '0.010',
      leverage: 10,
      client_order_id: id,
    })

    const openSell = await post(seller, '/v1/orders', order('sell', `pw-open-sell-${suffix}`), `pw-open-sell-${suffix}`)
    await expect
      .poll(() => sql(`SELECT count(*) FROM matching_book_orders WHERE order_id='${openSell.data.id}'`))
      .toBe('1')
    await post(buyer, '/v1/orders', order('buy', `pw-open-buy-${suffix}`), `pw-open-buy-${suffix}`)

    await expect
      .poll(async () => {
        const response = await buyer.request.get('/v1/positions')
        return (await response.json()).data?.[0]?.size
      })
      .toBe('0.010')

    const buyerPage = await buyer.newPage()
    await buyerPage.goto('/')
    await expect(buyerPage.getByRole('heading', { name: '仓位与风险' })).toBeVisible()
    await expect(buyerPage.getByTestId('risk-card')).toContainText('0.010')
    await buyerPage.reload()
    await expect(buyerPage.getByTestId('risk-card')).toContainText('0.010')

    const closeSell = await post(
      buyer,
      '/v1/orders',
      order('sell', `pw-close-sell-${suffix}`),
      `pw-close-sell-${suffix}`,
    )
    await expect
      .poll(() => sql(`SELECT count(*) FROM matching_book_orders WHERE order_id='${closeSell.data.id}'`))
      .toBe('1')
    await post(seller, '/v1/orders', order('buy', `pw-close-buy-${suffix}`), `pw-close-buy-${suffix}`)
    await expect
      .poll(async () => {
        const response = await buyer.request.get('/v1/positions')
        return (await response.json()).data?.[0]?.size
      })
      .toBe('0.000')
  } finally {
    await seller.close()
    await buyer.close()
  }
})
