/** Main read-only market shell plus the authenticated contract workflow. */
import { useCallback, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'

import { getInstruments, getTicker } from './api'
import type { ConnectionState } from './candle-stream'
import type { Candle } from './contract'
import { KLinePanel } from './KLinePanel'
import { TradingPanel } from './TradingPanel'

const pageClass =
  'min-h-screen min-w-[320px] bg-[radial-gradient(circle_at_25%_0%,#172136_0,#0b0e11_38%,#07090c_100%)] font-sans text-[#eaecef] [font-synthesis:none]'
const panelClass = 'border border-[#242a32] bg-[rgba(17,21,27,0.94)] shadow-[0_18px_46px_rgba(0,0,0,0.22)]'
const marketCellClass =
  'flex min-h-[74px] flex-col justify-center gap-[7px] border-r border-[#242a32] p-3 min-[621px]:px-[18px] min-[621px]:py-[14px]'
const marketValueClass = 'text-[15px] text-[#eaecef] [font-variant-numeric:tabular-nums]'

const statusText: Record<ConnectionState, string> = {
  connecting: '连接中',
  live: '实时',
  reconnecting: '重连中',
  offline: '已断开',
}

const statusClass: Record<ConnectionState, { container: string; dot: string }> = {
  connecting: { container: 'text-[#aeb4bc]', dot: 'bg-[#7a8491]' },
  live: { container: 'text-[#2ebd85]', dot: 'bg-[#2ebd85]' },
  reconnecting: { container: 'text-[#f0b90b]', dot: 'bg-[#f0b90b]' },
  offline: { container: 'text-[#aeb4bc]', dot: 'bg-[#7a8491]' },
}

function shiftDecimal(value: string, scale: number, deltaAtoms: bigint): string {
  const [whole, fraction = ''] = value.split('.')
  const atoms =
    BigInt(whole) * 10n ** BigInt(scale) + BigInt((fraction + '0'.repeat(scale)).slice(0, scale)) + deltaAtoms
  const absolute = atoms < 0n ? -atoms : atoms
  const padded = absolute.toString().padStart(scale + 1, '0')
  const result = scale === 0 ? padded : `${padded.slice(0, -scale)}.${padded.slice(-scale)}`
  return atoms < 0n ? `-${result}` : result
}

export default function App() {
  const [symbol, setSymbol] = useState('BTCUSDT-PERP')
  const [interval, setInterval] = useState<Candle['interval']>('1m')
  const [connection, setConnection] = useState<ConnectionState>('connecting')
  const onConnection = useCallback((state: ConnectionState) => setConnection(state), [])
  const instruments = useQuery({
    queryKey: ['instruments'],
    queryFn: ({ signal }) => getInstruments(signal),
    staleTime: 60_000,
  })
  const ticker = useQuery({
    queryKey: ['ticker', symbol],
    queryFn: ({ signal }) => getTicker(symbol, signal),
    refetchInterval: 1000,
  })
  const selected = instruments.data?.find((item) => item.symbol === symbol)
  const depth = useMemo(() => {
    if (!ticker.data || !selected) return []
    const lastPrice = ticker.data.last_price
    return Array.from({ length: 6 }, (_, index) => {
      const step = BigInt(10 * (index + 1))
      return {
        ask: shiftDecimal(lastPrice, selected.price_scale, step),
        bid: shiftDecimal(lastPrice, selected.price_scale, -step),
        quantity: `${index + 1}.${String((index + 3) * 137).padStart(3, '0')}`,
      }
    })
  }, [selected, ticker.data])

  if (instruments.isPending)
    return <main className={`${pageClass} grid place-items-center text-[#aeb4bc]`}>正在加载交易终端…</main>
  if (instruments.isError || !instruments.data?.length)
    return <main className={`${pageClass} grid place-items-center text-[#f6465d]`}>交易对加载失败，请检查 API</main>

  return (
    <div className={pageClass}>
      <div className="mx-auto w-full max-w-[1480px] p-3 min-[621px]:p-5">
        <header className="flex items-center justify-between px-1 pt-2 pb-5">
          <div>
            <p className="m-0 text-[11px] font-[750] tracking-[0.18em] text-[#f0b90b]">EXCHANGE LAB</p>
            <h1 className="mt-0.5 mb-0 text-lg font-[620] tracking-[0.02em] min-[621px]:text-[22px]">永续合约实验室</h1>
          </div>
          <div
            className={`flex items-center gap-2 rounded-full border border-[#2b3139] px-3 py-[7px] text-xs ${statusClass[connection].container}`}
            data-testid="connection-state"
          >
            <span
              className={`h-[7px] w-[7px] rounded-full shadow-[0_0_8px_currentColor] ${statusClass[connection].dot}`}
            />
            {statusText[connection]}
          </div>
        </header>

        <section
          className={`${panelClass} mb-3 grid grid-cols-2 overflow-hidden rounded-[10px] min-[621px]:grid-cols-3 min-[981px]:grid-cols-[minmax(190px,1.35fr)_repeat(5,minmax(130px,1fr))]`}
        >
          <label className={`${marketCellClass} text-xs text-[#848e9c]`}>
            交易对
            <select
              className="border-0 bg-transparent p-0 text-[17px] font-[650] text-white outline-0"
              value={symbol}
              onChange={(event) => setSymbol(event.target.value)}
              aria-label="交易对"
            >
              {instruments.data.map((item) => (
                <option key={item.symbol}>{item.symbol}</option>
              ))}
            </select>
          </label>
          <div className={marketCellClass}>
            <span className="text-xs text-[#848e9c]">最新价</span>
            <strong className={marketValueClass} data-testid="last-price">
              {ticker.data?.last_price ?? '—'}
            </strong>
          </div>
          <div className={marketCellClass}>
            <span className="text-xs text-[#848e9c]">标记价格</span>
            <strong className={marketValueClass}>{ticker.data?.mark_price ?? '—'}</strong>
          </div>
          <div className={marketCellClass}>
            <span className="text-xs text-[#848e9c]">指数价格</span>
            <strong className={marketValueClass}>{ticker.data?.index_price ?? '—'}</strong>
          </div>
          <div className={marketCellClass}>
            <span className="text-xs text-[#848e9c]">24h 成交量</span>
            <strong className={marketValueClass}>{ticker.data?.volume_24h ?? '—'}</strong>
          </div>
          <div className={marketCellClass}>
            <span className="text-xs text-[#848e9c]">数据时间</span>
            <strong className="text-[13px] text-[#eaecef] [font-variant-numeric:tabular-nums]">
              {ticker.data ? new Date(ticker.data.updated_at).toLocaleTimeString() : '—'}
            </strong>
          </div>
        </section>

        <main className="grid grid-cols-1 gap-3 min-[981px]:grid-cols-[minmax(0,1fr)_310px]">
          <div className="min-w-0">
            <nav
              className="flex h-[42px] items-center gap-1.5 rounded-t-[10px] border border-b-0 border-[#242a32] bg-[#11151b] px-3"
              aria-label="K 线周期"
            >
              {(['1m', '5m', '1h'] as const).map((item) => {
                const active = item === interval
                return (
                  <button
                    type="button"
                    key={item}
                    className={`cursor-pointer rounded-[5px] border-0 px-[11px] py-[5px] hover:bg-[#f0b90b] hover:text-[#0b0e11] ${active ? 'bg-[#f0b90b] text-[#0b0e11]' : 'bg-transparent text-[#848e9c]'}`}
                    aria-pressed={active}
                    onClick={() => setInterval(item)}
                  >
                    {item}
                  </button>
                )
              })}
            </nav>
            <KLinePanel symbol={symbol} interval={interval} onState={onConnection} />
          </div>
          <aside
            className={`${panelClass} min-h-0 rounded-[10px] p-[18px] min-[981px]:min-h-[646px]`}
            aria-label="模拟深度"
          >
            <div className="flex items-baseline justify-between border-b border-[#242a32] pb-[14px]">
              <h2 className="m-0 text-[15px] font-semibold">模拟深度</h2>
              <span className="text-[11px] text-[#697480]">价格 / 数量</span>
            </div>
            <div className="grid gap-px py-3 [font-variant-numeric:tabular-nums]">
              {[...depth].reverse().map((level) => (
                <div
                  className="flex justify-between bg-[linear-gradient(to_left,rgba(246,70,93,0.08),transparent_66%)] px-[7px] py-[5px] text-xs"
                  key={level.ask}
                >
                  <span className="text-[#f6465d]">{level.ask}</span>
                  <span>{level.quantity}</span>
                </div>
              ))}
            </div>
            <div className="border-y border-[#242a32] px-[7px] py-3 text-xl font-[650] text-[#f0b90b] [font-variant-numeric:tabular-nums]">
              {ticker.data?.last_price ?? '—'}
            </div>
            <div className="grid gap-px py-3 [font-variant-numeric:tabular-nums]">
              {depth.map((level) => (
                <div
                  className="flex justify-between bg-[linear-gradient(to_left,rgba(46,189,133,0.08),transparent_66%)] px-[7px] py-[5px] text-xs"
                  key={level.bid}
                >
                  <span className="text-[#2ebd85]">{level.bid}</span>
                  <span>{level.quantity}</span>
                </div>
              ))}
            </div>
            <p className="mt-[18px] text-center text-[11px] text-[#697480]">教学模拟数据 · 不连接真实资金</p>
          </aside>
        </main>
        <TradingPanel symbol={symbol} markPrice={ticker.data?.mark_price ?? '—'} />
      </div>
    </div>
  )
}
