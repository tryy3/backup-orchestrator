import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { subscribe, connectWebSocket, disconnectWebSocket, wsConnectionStatus } from '../websocket'

class MockWebSocket {
    static instances: MockWebSocket[] = []

    readyState = 0 // CONNECTING
    onopen: (() => void) | null = null
    onmessage: ((event: { data: string }) => void) | null = null
    onclose: (() => void) | null = null
    onerror: (() => void) | null = null

    constructor(public url: string) {
        MockWebSocket.instances.push(this)
    }

    close() {
        this.readyState = 3 // CLOSED
        this.onclose?.()
    }

    simulateOpen() {
        this.readyState = 1 // OPEN
        this.onopen?.()
    }

    simulateMessage(data: unknown) {
        this.onmessage?.({ data: JSON.stringify(data) })
    }

    static OPEN = 1
    static CONNECTING = 0
    static CLOSING = 2
    static CLOSED = 3
}

describe('websocket', () => {
    beforeEach(() => {
        vi.useFakeTimers()
        MockWebSocket.instances = []
        vi.stubGlobal('WebSocket', MockWebSocket)
        // Ensure clean state
        disconnectWebSocket()
    })

    afterEach(() => {
        disconnectWebSocket()
        vi.useRealTimers()
        vi.unstubAllGlobals()
    })

    describe('connectWebSocket', () => {
        it('sets status to "connecting" then "connected" on open', () => {
            expect(wsConnectionStatus.value).toBe('disconnected')
            connectWebSocket()
            expect(wsConnectionStatus.value).toBe('connecting')

            MockWebSocket.instances[0].simulateOpen()
            expect(wsConnectionStatus.value).toBe('connected')
        })

        it('does not create duplicate connections', () => {
            connectWebSocket()
            MockWebSocket.instances[0].simulateOpen()
            connectWebSocket() // should no-op
            expect(MockWebSocket.instances).toHaveLength(1)
        })
    })

    describe('disconnectWebSocket', () => {
        it('sets status to "disconnected" and clears socket', () => {
            connectWebSocket()
            MockWebSocket.instances[0].simulateOpen()
            expect(wsConnectionStatus.value).toBe('connected')

            disconnectWebSocket()
            expect(wsConnectionStatus.value).toBe('disconnected')
        })
    })

    describe('subscribe / message dispatch', () => {
        it('delivers messages to subscribers', () => {
            const callback = vi.fn()
            subscribe('job.created', callback)

            connectWebSocket()
            MockWebSocket.instances[0].simulateOpen()
            MockWebSocket.instances[0].simulateMessage({
                type: 'job.created',
                payload: { id: 'j1', agent_id: 'a1' },
            })

            expect(callback).toHaveBeenCalledWith({ id: 'j1', agent_id: 'a1' })
        })

        it('does not deliver messages for unsubscribed types', () => {
            const callback = vi.fn()
            subscribe('job.created', callback)

            connectWebSocket()
            MockWebSocket.instances[0].simulateOpen()
            MockWebSocket.instances[0].simulateMessage({
                type: 'job.completed',
                payload: { id: 'j1' },
            })

            expect(callback).not.toHaveBeenCalled()
        })

        it('unsubscribe stops delivery', () => {
            const callback = vi.fn()
            const unsub = subscribe('agent.heartbeat', callback)

            connectWebSocket()
            const ws = MockWebSocket.instances[0]
            ws.simulateOpen()

            ws.simulateMessage({ type: 'agent.heartbeat', payload: { agent_id: 'a1', timestamp: 't1' } })
            expect(callback).toHaveBeenCalledTimes(1)

            unsub()

            ws.simulateMessage({ type: 'agent.heartbeat', payload: { agent_id: 'a1', timestamp: 't2' } })
            expect(callback).toHaveBeenCalledTimes(1) // still 1, not called again
        })

        it('ignores malformed JSON messages', () => {
            const callback = vi.fn()
            subscribe('job.created', callback)

            connectWebSocket()
            const ws = MockWebSocket.instances[0]
            ws.simulateOpen()

            // Trigger onmessage with invalid JSON directly
            ws.onmessage?.({ data: 'not json{{{' })
            expect(callback).not.toHaveBeenCalled()
        })
    })

    describe('reconnection', () => {
        it('schedules reconnect with exponential backoff on close', () => {
            connectWebSocket()
            const ws = MockWebSocket.instances[0]
            ws.simulateOpen()
            expect(wsConnectionStatus.value).toBe('connected')

            // Simulate unexpected close (triggers reconnect)
            ws.readyState = 3
            ws.onclose?.()

            expect(wsConnectionStatus.value).toBe('disconnected')

            // First reconnect after 1s (1000 * 2^0)
            vi.advanceTimersByTime(1000)
            expect(MockWebSocket.instances).toHaveLength(2)
        })
    })
})
