/**
 * Reconnecting candle WebSocket adapter.
 * Frames remain untrusted until decoded by the versioned runtime contract.
 */
import { decodeCandleUpdate, type Candle, type CandleUpdate } from './contract'

/** User-visible connection states emitted by CandleStream. */
export type ConnectionState = 'connecting' | 'live' | 'reconnecting' | 'offline'

type SocketLike = Pick<WebSocket, 'close' | 'onclose' | 'onerror' | 'onmessage' | 'onopen'>
type SocketFactory = (url: string) => SocketLike

/** Owns one socket at a time and reconnects with capped exponential backoff. */
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
        // Untrusted frames are ignored at the contract boundary.
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

/** Builds a same-origin WebSocket URL, preserving HTTPS through wss. */
export function candleWebSocketURL(symbol: string, interval: Candle['interval']): string {
  const scheme = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const query = new URLSearchParams({ symbol, interval })
  return `${scheme}//${window.location.host}/ws/v1/candles?${query}`
}
