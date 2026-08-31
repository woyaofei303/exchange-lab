/**
 * 不可信 API 和 WebSocket 载荷的运行时边界。
 * 金融十进制数刻意保持为字符串，仅在后续显示阶段转换。
 */
export type Instrument = {
  symbol: string
  base_asset: string
  quote_asset: string
  price_scale: number
  quantity_scale: number
  price_step: string
  quantity_step: string
  min_notional: string
}

export type Ticker = {
  symbol: string
  last_price: string
  mark_price: string
  index_price: string
  price_change_24h: string
  volume_24h: string
  updated_at: string
}

export type Candle = {
  symbol: string
  interval: '1m' | '5m' | '1h'
  open_time: string
  close_time: string
  open: string
  high: string
  low: string
  close: string
  volume: string
  trade_count: number
  is_final: boolean
}

export type CandleUpdate = { version: 1; type: 'candle'; data: Candle }

export type User = { id: number; email: string; role: string }
export type Balance = { asset: string; available: string; locked: string; total: string }
export type Position = {
  symbol: string
  size: string
  entry_price: string
  mark_price: string
  realized_pnl: string
  unrealized_pnl: string
  fees: string
  version: number
  updated_at: string
}
export type RiskSnapshot = {
  symbol: string
  mode: 'isolated' | 'cross'
  leverage: number
  position_size: string
  entry_price: string
  mark_price: string
  position_margin: string
  order_margin: string
  wallet_available: string
  unrealized_pnl: string
  margin_balance: string
  maintenance_margin: string
  margin_ratio_bps: number
  liquidation_distance_bps: number
  risk_version: number
  fresh: boolean
  as_of: string
}
export type Order = {
  id: string
  symbol: string
  side: 'buy' | 'sell'
  status: string
  price: string
  quantity: string
  remaining: string
  leverage: number
  reserved_margin: string
}

const symbolPattern = /^[A-Z0-9]{4,24}-PERP$/
const assetPattern = /^[A-Z0-9]{2,12}$/
const decimalPattern = /^(0|[1-9]\d*)(\.\d+)?$/
const signedDecimalPattern = /^-?(0|[1-9]\d*)(\.\d+)?$/

function record(value: unknown, label: string): Record<string, unknown> {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) throw new Error(`invalid ${label}`)
  return value as Record<string, unknown>
}

function text(value: unknown, label: string, pattern?: RegExp): string {
  if (typeof value !== 'string' || value.length === 0 || (pattern && !pattern.test(value)))
    throw new Error(`invalid ${label}`)
  return value
}

function integer(value: unknown, label: string, min = 0, max = Number.MAX_SAFE_INTEGER): number {
  if (!Number.isSafeInteger(value) || (value as number) < min || (value as number) > max)
    throw new Error(`invalid ${label}`)
  return value as number
}

function timestamp(value: unknown, label: string): string {
  const result = text(value, label)
  if (!Number.isFinite(Date.parse(result))) throw new Error(`invalid ${label}`)
  return result
}

function decimal(value: unknown, label: string): string {
  return text(value, label, decimalPattern)
}

function signedDecimal(value: unknown, label: string): string {
  return text(value, label, signedDecimalPattern)
}

function decodeInstrument(value: unknown): Instrument {
  const item = record(value, 'instrument')
  return {
    symbol: text(item.symbol, 'instrument symbol', symbolPattern),
    base_asset: text(item.base_asset, 'base asset', assetPattern),
    quote_asset: text(item.quote_asset, 'quote asset', assetPattern),
    price_scale: integer(item.price_scale, 'price scale', 0, 18),
    quantity_scale: integer(item.quantity_scale, 'quantity scale', 0, 18),
    price_step: decimal(item.price_step, 'price step'),
    quantity_step: decimal(item.quantity_step, 'quantity step'),
    min_notional: decimal(item.min_notional, 'minimum notional'),
  }
}

function decodeTicker(value: unknown): Ticker {
  const item = record(value, 'ticker')
  return {
    symbol: text(item.symbol, 'ticker symbol', symbolPattern),
    last_price: decimal(item.last_price, 'last price'),
    mark_price: decimal(item.mark_price, 'mark price'),
    index_price: decimal(item.index_price, 'index price'),
    price_change_24h: text(item.price_change_24h, 'price change'),
    volume_24h: decimal(item.volume_24h, 'volume'),
    updated_at: timestamp(item.updated_at, 'ticker timestamp'),
  }
}

function decodeCandle(value: unknown): Candle {
  const item = record(value, 'candle')
  const interval = text(item.interval, 'candle interval')
  if (interval !== '1m' && interval !== '5m' && interval !== '1h') throw new Error('invalid candle interval')
  if (typeof item.is_final !== 'boolean') throw new Error('invalid final flag')
  return {
    symbol: text(item.symbol, 'candle symbol', symbolPattern),
    interval,
    open_time: timestamp(item.open_time, 'open time'),
    close_time: timestamp(item.close_time, 'close time'),
    open: decimal(item.open, 'open'),
    high: decimal(item.high, 'high'),
    low: decimal(item.low, 'low'),
    close: decimal(item.close, 'close'),
    volume: decimal(item.volume, 'volume'),
    trade_count: integer(item.trade_count, 'trade count'),
    is_final: item.is_final,
  }
}

function decodeList<T>(value: unknown, label: string, decoder: (item: unknown) => T): T[] {
  const envelope = record(value, label)
  if (!Array.isArray(envelope.data)) throw new Error(`invalid ${label}`)
  return envelope.data.map(decoder)
}

export const decodeInstruments = (value: unknown): Instrument[] => decodeList(value, 'instruments', decodeInstrument)
export const decodeTickers = (value: unknown): Ticker[] => decodeList(value, 'tickers', decodeTicker)
export const decodeCandles = (value: unknown): Candle[] => decodeList(value, 'candles', decodeCandle)

export function decodeUser(value: unknown): User {
  const item = record(record(value, 'user envelope').data, 'user')
  return { id: integer(item.id, 'user id', 1), email: text(item.email, 'email'), role: text(item.role, 'role') }
}
export function decodeBalances(value: unknown): Balance[] {
  return decodeList(value, 'balances', (raw) => {
    const item = record(raw, 'balance')
    return {
      asset: text(item.asset, 'asset', assetPattern),
      available: decimal(item.available, 'available'),
      locked: decimal(item.locked, 'locked'),
      total: decimal(item.total, 'total'),
    }
  })
}
export function decodePositions(value: unknown): Position[] {
  return decodeList(value, 'positions', (raw) => {
    const item = record(raw, 'position')
    return {
      symbol: text(item.symbol, 'symbol', symbolPattern),
      size: signedDecimal(item.size, 'size'),
      entry_price: decimal(item.entry_price, 'entry'),
      mark_price: decimal(item.mark_price, 'mark'),
      realized_pnl: signedDecimal(item.realized_pnl, 'realized pnl'),
      unrealized_pnl: signedDecimal(item.unrealized_pnl, 'unrealized pnl'),
      fees: decimal(item.fees, 'fees'),
      version: integer(item.version, 'version'),
      updated_at: timestamp(item.updated_at, 'updated'),
    }
  })
}
export function decodeRisk(value: unknown): RiskSnapshot {
  const item = record(record(value, 'risk envelope').data, 'risk')
  const mode = text(item.mode, 'mode')
  if (mode !== 'isolated' && mode !== 'cross') throw new Error('invalid mode')
  if (typeof item.fresh !== 'boolean') throw new Error('invalid freshness')
  return {
    symbol: text(item.symbol, 'symbol', symbolPattern),
    mode,
    leverage: integer(item.leverage, 'leverage', 1, 125),
    position_size: signedDecimal(item.position_size, 'size'),
    entry_price: decimal(item.entry_price, 'entry'),
    mark_price: decimal(item.mark_price, 'mark'),
    position_margin: decimal(item.position_margin, 'position margin'),
    order_margin: decimal(item.order_margin, 'order margin'),
    wallet_available: decimal(item.wallet_available, 'available'),
    unrealized_pnl: signedDecimal(item.unrealized_pnl, 'upnl'),
    margin_balance: signedDecimal(item.margin_balance, 'margin balance'),
    maintenance_margin: decimal(item.maintenance_margin, 'maintenance'),
    margin_ratio_bps: integer(item.margin_ratio_bps, 'ratio', 0),
    liquidation_distance_bps: integer(item.liquidation_distance_bps, 'liquidation distance', 0),
    risk_version: integer(item.risk_version, 'version', 1),
    fresh: item.fresh,
    as_of: timestamp(item.as_of, 'as of'),
  }
}
export function decodeOrders(value: unknown): Order[] {
  return decodeList(value, 'orders', (raw) => {
    const item = record(raw, 'order')
    const side = text(item.side, 'side')
    if (side !== 'buy' && side !== 'sell') throw new Error('invalid side')
    return {
      id: text(item.id, 'id'),
      symbol: text(item.symbol, 'symbol', symbolPattern),
      side,
      status: text(item.status, 'status'),
      price: decimal(item.price, 'price'),
      quantity: decimal(item.quantity, 'quantity'),
      remaining: decimal(item.remaining, 'remaining'),
      leverage: integer(item.leverage, 'leverage', 1, 125),
      reserved_margin: decimal(item.reserved_margin, 'reserved'),
    }
  })
}

/** 严格解码唯一受支持的实时 K 线信封版本。 */
export function decodeCandleUpdate(value: unknown): CandleUpdate {
  const envelope = record(value, 'candle update')
  if (envelope.version !== 1 || envelope.type !== 'candle') throw new Error('unsupported candle update')
  return { version: 1, type: 'candle', data: decodeCandle(envelope.data) }
}

/** 按开盘时间替换一个时间桶，并返回按时间顺序排列的历史记录。 */
export function mergeCandle(history: Candle[], incoming: Candle): Candle[] {
  const result = history.filter((item) => item.open_time !== incoming.open_time)
  result.push(incoming)
  return result.sort((left, right) => Date.parse(left.open_time) - Date.parse(right.open_time))
}
