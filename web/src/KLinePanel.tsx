/** 历史/实时 K 线图适配器；Number 转换仅用于显示。 */
import { useEffect, useId, useState } from 'react'
import { dispose, init, type KLineData } from 'klinecharts'

import { getCandles } from './api'
import { CandleStream, candleWebSocketURL, type ConnectionState } from './candle-stream'
import type { Candle } from './contract'

type Props = {
  symbol: string
  interval: Candle['interval']
  onState: (state: ConnectionState) => void
}

function chartCandle(candle: Candle): KLineData {
  // Number 转换仅限可视化适配器；API/领域值仍保持为精确字符串。
  return {
    timestamp: Date.parse(candle.open_time),
    open: Number(candle.open),
    high: Number(candle.high),
    low: Number(candle.low),
    close: Number(candle.close),
    volume: Number(candle.volume),
  }
}

/** 将一个交易对/周期绑定到历史加载和可释放的实时数据流。 */
export function KLinePanel({ symbol, interval, onState }: Props) {
  const id = `kline-${useId().replaceAll(':', '')}`
  const [error, setError] = useState('')

  useEffect(() => {
    let active = true
    const subscriptions = new Map<string, CandleStream>()
    const chart = init(id)
    if (!chart) {
      setError('图表初始化失败')
      return
    }
    chart.setSymbol({ ticker: symbol })
    chart.setPeriod(interval === '1h' ? { span: 1, type: 'hour' } : { span: Number(interval[0]), type: 'minute' })
    chart.setDataLoader({
      async getBars({ callback }) {
        try {
          const bars = (await getCandles(symbol, interval)).map(chartCandle)
          if (active) callback(bars, false)
        } catch {
          if (active) {
            setError('历史 K 线加载失败')
            callback([], false)
          }
        }
      },
      subscribeBar({ callback }) {
        const key = `${symbol}:${interval}`
        const stream = new CandleStream(
          candleWebSocketURL(symbol, interval),
          (url) => new WebSocket(url),
          (update) => callback(chartCandle(update.data)),
          onState,
        )
        subscriptions.set(key, stream)
        stream.start()
      },
      unsubscribeBar() {
        for (const stream of subscriptions.values()) stream.stop()
        subscriptions.clear()
      },
    })
    return () => {
      active = false
      for (const stream of subscriptions.values()) stream.stop()
      subscriptions.clear()
      dispose(id)
    }
  }, [id, interval, onState, symbol])

  return (
    <section
      className="relative min-h-[604px] overflow-hidden rounded-b-[10px] border border-[#242a32] bg-[rgba(17,21,27,0.94)] shadow-[0_18px_46px_rgba(0,0,0,0.22)]"
      aria-label="K 线图"
    >
      {error && (
        <div
          className="absolute inset-x-2.5 top-2.5 z-[2] rounded-[6px] bg-[rgba(246,70,93,0.13)] px-3 py-[9px] text-[#ffb4b4]"
          role="alert"
        >
          {error}
        </div>
      )}
      <div id={id} className="h-[602px] w-full" data-testid="kline-chart" />
    </section>
  )
}
