import { describe, expect, it } from 'vitest'

import { decodeCandleUpdate, decodeInstruments, decodeRisk, mergeCandle, type Candle } from './contract'

const candle: Candle = {
  symbol: 'BTCUSDT-PERP',
  interval: '1m',
  open_time: '2026-08-10T00:00:00Z',
  close_time: '2026-08-10T00:01:00Z',
  open: '65000.10',
  high: '65001.20',
  low: '64999.90',
  close: '65000.80',
  volume: '1.250',
  trade_count: 3,
  is_final: false,
}

describe('frontend contract boundary', () => {
  it('accepts exact decimal strings and rejects numeric prices', () => {
    expect(decodeCandleUpdate({ version: 1, type: 'candle', data: candle }).data.close).toBe('65000.80')
    expect(() => decodeCandleUpdate({ version: 1, type: 'candle', data: { ...candle, close: 65000.8 } })).toThrow()
  })

  it('rejects unknown contract versions and malformed symbols', () => {
    expect(() => decodeCandleUpdate({ version: 2, type: 'candle', data: candle })).toThrow()
    expect(() => decodeInstruments({ data: [{ symbol: 'btc', price_scale: 2 }] })).toThrow()
  })

  it('replaces duplicate candle timestamps without duplicating history', () => {
    const next = { ...candle, close: '65002.00' }
    expect(mergeCandle([candle], next)).toEqual([next])
  })

  it('requires the server-computed liquidation distance in a risk snapshot', () => {
    const payload = {
      data: {
        symbol: 'BTCUSDT-PERP',
        mode: 'isolated',
        leverage: 10,
        position_size: '0.010',
        entry_price: '50000.00',
        mark_price: '50010.00',
        position_margin: '50.00000000',
        order_margin: '0.00000000',
        wallet_available: '950.00000000',
        unrealized_pnl: '0.10000000',
        margin_balance: '50.10000000',
        maintenance_margin: '2.50000000',
        margin_ratio_bps: 499,
        liquidation_distance_bps: 905,
        risk_version: 3,
        fresh: true,
        as_of: '2026-08-10T00:00:00Z',
      },
    }
    expect(decodeRisk(payload).liquidation_distance_bps).toBe(905)
    const { liquidation_distance_bps: _, ...missing } = payload.data
    expect(() => decodeRisk({ data: missing })).toThrow()
  })
})
