/**
 * Typed browser client for the Exchange Lab HTTP API.
 * Every response crosses the runtime decoders in contract.ts before UI use.
 */
import {
  decodeBalances,
  decodeCandles,
  decodeInstruments,
  decodeOrders,
  decodePositions,
  decodeRisk,
  decodeTickers,
  decodeUser,
  type Balance,
  type Candle,
  type Instrument,
  type Order,
  type Position,
  type RiskSnapshot,
  type Ticker,
  type User,
} from './contract'

/** Represents a sanitized HTTP failure without exposing a response body. */
export class APIError extends Error {
  constructor(
    readonly status: number,
    message: string,
  ) {
    super(message)
  }
}

async function request(path: string, signal?: AbortSignal, init?: RequestInit): Promise<unknown> {
  const response = await fetch(path, {
    credentials: 'same-origin',
    ...init,
    headers: { Accept: 'application/json', ...(init?.headers ?? {}) },
    signal,
  })
  if (!response.ok) throw new APIError(response.status, `request failed (${response.status})`)
  return response.json()
}

async function mutate(path: string, body: unknown, idempotencyKey?: string): Promise<unknown> {
  return request(path, undefined, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', ...(idempotencyKey ? { 'Idempotency-Key': idempotencyKey } : {}) },
    body: JSON.stringify(body),
  })
}

export async function getInstruments(signal?: AbortSignal): Promise<Instrument[]> {
  return decodeInstruments(await request('/v1/instruments', signal))
}

export async function getTicker(symbol: string, signal?: AbortSignal): Promise<Ticker | undefined> {
  return decodeTickers(await request(`/v1/tickers?symbol=${encodeURIComponent(symbol)}`, signal))[0]
}

const intervalSeconds = { '1m': 60, '5m': 300, '1h': 3600 } as const

/** Loads the latest bounded interval range using UTC-aligned bucket boundaries. */
export async function getCandles(
  symbol: string,
  interval: Candle['interval'],
  signal?: AbortSignal,
): Promise<Candle[]> {
  const limit = 240
  const duration = intervalSeconds[interval]
  const endSeconds = Math.floor(Date.now() / 1000 / duration) * duration + duration
  const startSeconds = endSeconds - limit * duration
  const query = new URLSearchParams({
    symbol,
    interval,
    start: new Date(startSeconds * 1000).toISOString(),
    end: new Date(endSeconds * 1000).toISOString(),
    limit: String(limit),
  })
  return decodeCandles(await request(`/v1/candles?${query}`, signal))
}

export async function register(email: string, password: string): Promise<User> {
  return decodeUser(await mutate('/v1/auth/register', { email, password }))
}
export async function login(email: string, password: string): Promise<User> {
  return decodeUser(await mutate('/v1/auth/login', { email, password }))
}
export async function me(signal?: AbortSignal): Promise<User> {
  return decodeUser(await request('/v1/me', signal))
}
export async function faucet(amount: string): Promise<void> {
  await mutate('/v1/faucet', { amount }, crypto.randomUUID())
}
export async function getBalances(signal?: AbortSignal): Promise<Balance[]> {
  return decodeBalances(await request('/v1/balances', signal))
}
export async function getPositions(signal?: AbortSignal): Promise<Position[]> {
  return decodePositions(await request('/v1/positions', signal))
}
export async function getRisk(symbol: string, signal?: AbortSignal): Promise<RiskSnapshot> {
  return decodeRisk(await request(`/v1/risk?symbol=${encodeURIComponent(symbol)}`, signal))
}
export async function getOrders(signal?: AbortSignal): Promise<Order[]> {
  return decodeOrders(await request('/v1/orders?limit=50', signal))
}
export async function placeOrder(input: {
  symbol: string
  side: 'buy' | 'sell'
  price: string
  quantity: string
  leverage: number
}): Promise<void> {
  await mutate(
    '/v1/orders',
    { ...input, type: 'limit', client_order_id: `web-${crypto.randomUUID()}` },
    crypto.randomUUID(),
  )
}
export async function setLeverage(symbol: string, leverage: number): Promise<void> {
  await mutate('/v1/leverage', { symbol, leverage })
}
export async function setMarginMode(mode: 'isolated' | 'cross'): Promise<void> {
  await mutate('/v1/margin-mode', { mode })
}
export async function adjustMargin(symbol: string, amount: string): Promise<void> {
  await mutate('/v1/margin/adjust', { symbol, amount }, crypto.randomUUID())
}
export async function createConditional(input: {
  symbol: string
  type: 'take_profit' | 'stop_loss'
  trigger_source: 'mark' | 'last'
  trigger_direction: 'lte' | 'gte'
  trigger_price: string
  quantity: string
}): Promise<void> {
  await mutate('/v1/conditional-orders', input, crypto.randomUUID())
}
export async function getFunding(signal?: AbortSignal): Promise<unknown> {
  return request('/v1/funding-rates?symbol=BTCUSDT-PERP', signal)
}
export async function getLiquidations(signal?: AbortSignal): Promise<unknown> {
  return request('/v1/liquidations', signal)
}
export async function getInsurance(signal?: AbortSignal): Promise<unknown> {
  return request('/v1/insurance-fund', signal)
}
