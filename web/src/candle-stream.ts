/**
 * 支持重连的 K 线 WebSocket 适配器。
 * 帧在经过带版本的运行时契约解码前始终视为不可信数据。
 */
import { decodeCandleUpdate, type Candle, type CandleUpdate } from './contract'

/** CandleStream 发出的用户可见连接状态。 */
export type ConnectionState = 'connecting' | 'live' | 'reconnecting' | 'offline'

type SocketLike = Pick<WebSocket, 'close' | 'onclose' | 'onerror' | 'onmessage' | 'onopen'>
type SocketFactory = (url: string) => SocketLike

/** 同一时间只管理一个套接字，并使用带上限的指数退避重连。 */
export class CandleStream {
  private socket?: SocketLike
  private stopped = false
  private attempt = 0
  private timer?: ReturnType<typeof setTimeout>

  constructor(
    private readonly path: string,
    private readonly factory: SocketFactory,
    private readonly onUpdate: (update: CandleUpdate) => void,
    private readonly onState: (state: ConnectionState) => void,
  ) {}

  start(): void {
    if (this.stopped || this.socket) return
    this.connect()
  }

  stop(): void {
    this.stopped = true
    if (this.timer) clearTimeout(this.timer)
    this.socket?.close(1000, 'switch or unmount')
    this.socket = undefined
    this.onState('offline')
  }

  private connect(): void {
    if (this.stopped) return
    this.onState(this.attempt === 0 ? 'connecting' : 'reconnecting')
    const socket = this.factory(this.path)
    this.socket = socket
    socket.onopen = () => {
      if (socket !== this.socket || this.stopped) return
      this.attempt = 0
      this.onState('live')
    }
    socket.onmessage = (event) => {
      if (socket !== this.socket || this.stopped || typeof event.data !== 'string') return
      try {
        this.onUpdate(decodeCandleUpdate(JSON.parse(event.data)))
      } catch {
        // 在契约边界忽略不可信帧。
      }
    }
    socket.onerror = () => socket.close()
    socket.onclose = () => {
      if (socket !== this.socket) return
      this.socket = undefined
      if (this.stopped) return
      this.attempt += 1
      const delay = Math.min(5000, 250 * 2 ** Math.min(this.attempt - 1, 5))
      this.onState('reconnecting')
      this.timer = setTimeout(() => this.connect(), delay)
    }
  }
}

/** 构建同源 WebSocket URL，并通过 wss 保持 HTTPS 安全性。 */
export function candleWebSocketURL(symbol: string, interval: Candle['interval']): string {
  const scheme = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const query = new URLSearchParams({ symbol, interval })
  return `${scheme}//${window.location.host}/ws/v1/candles?${query}`
}
