import { afterEach, describe, expect, it, vi } from 'vitest'

import { CandleStream, type ConnectionState } from './candle-stream'

class FakeSocket {
  onclose: ((event: CloseEvent) => void) | null = null
  onerror: ((event: Event) => void) | null = null
  onmessage: ((event: MessageEvent) => void) | null = null
  onopen: ((event: Event) => void) | null = null
  close = vi.fn(() => this.onclose?.({} as CloseEvent))
}

afterEach(() => vi.useRealTimers())

describe('CandleStream', () => {
  it('reports reconnect and creates a replacement socket after disconnect', () => {
    vi.useFakeTimers()
    const sockets: FakeSocket[] = []
    const states: ConnectionState[] = []
    const stream = new CandleStream(
      '/ws',
      () => {
        const socket = new FakeSocket()
        sockets.push(socket)
        return socket
      },
      () => undefined,
      (state) => states.push(state),
    )

    stream.start()
    sockets[0].onopen?.({} as Event)
    sockets[0].onclose?.({} as CloseEvent)
    vi.advanceTimersByTime(250)

    expect(sockets).toHaveLength(2)
    expect(states).toEqual(['connecting', 'live', 'reconnecting', 'reconnecting'])
    stream.stop()
  })

  it('ignores malformed and duplicate old-socket messages after stop', () => {
    const socket = new FakeSocket()
    const updates: string[] = []
    const stream = new CandleStream(
      '/ws',
      () => socket,
      (update) => updates.push(update.data.close),
      () => undefined,
    )
    stream.start()
    socket.onmessage?.({ data: '{bad json' } as MessageEvent)
    stream.stop()
    socket.onmessage?.({ data: '{}' } as MessageEvent)
    expect(updates).toEqual([])
  })
})
