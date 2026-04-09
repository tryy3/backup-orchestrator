import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { relativeTime, formatDate, formatDuration, durationBetween, formatBytes } from '../time'

describe('relativeTime', () => {
    beforeEach(() => {
        vi.useFakeTimers()
        vi.setSystemTime(new Date('2025-06-15T12:00:00Z'))
    })

    afterEach(() => {
        vi.useRealTimers()
    })

    it('returns "Never" for null/undefined', () => {
        expect(relativeTime(null)).toBe('Never')
        expect(relativeTime(undefined)).toBe('Never')
    })

    it('returns "Just now" for less than 5 seconds ago', () => {
        expect(relativeTime('2025-06-15T11:59:57Z')).toBe('Just now')
    })

    it('returns seconds for <60s', () => {
        expect(relativeTime('2025-06-15T11:59:30Z')).toBe('30s ago')
    })

    it('returns minutes for <60min', () => {
        expect(relativeTime('2025-06-15T11:45:00Z')).toBe('15 min ago')
    })

    it('returns hours for <24h', () => {
        expect(relativeTime('2025-06-15T06:00:00Z')).toBe('6h ago')
    })

    it('returns days for <30d', () => {
        expect(relativeTime('2025-06-10T12:00:00Z')).toBe('5d ago')
    })

    it('falls back to formatted date for >=30d', () => {
        const result = relativeTime('2025-04-01T12:00:00Z')
        // Should be a formatted date string, not relative
        expect(result).toContain('2025')
    })
})

describe('formatDate', () => {
    it('returns "-" for null/undefined', () => {
        expect(formatDate(null)).toBe('-')
        expect(formatDate(undefined)).toBe('-')
    })

    it('formats a valid date', () => {
        const result = formatDate('2025-06-15T14:30:00Z')
        // Verify it contains expected parts (locale-dependent formatting)
        expect(result).toContain('2025')
        expect(result).toContain('Jun')
        expect(result).toContain('15')
    })
})

describe('formatDuration', () => {
    it('returns "-" for null/undefined/zero/negative', () => {
        expect(formatDuration(null)).toBe('-')
        expect(formatDuration(undefined)).toBe('-')
        expect(formatDuration(0)).toBe('-')
        expect(formatDuration(-100)).toBe('-')
    })

    it('formats seconds', () => {
        expect(formatDuration(5000)).toBe('5s')
        expect(formatDuration(45000)).toBe('45s')
    })

    it('formats minutes and seconds', () => {
        expect(formatDuration(90_000)).toBe('1m 30s')
        expect(formatDuration(300_000)).toBe('5m 0s')
    })

    it('formats hours and minutes', () => {
        expect(formatDuration(3_660_000)).toBe('1h 1m')
        expect(formatDuration(7_200_000)).toBe('2h 0m')
    })
})

describe('durationBetween', () => {
    it('returns null when either date is missing', () => {
        expect(durationBetween(null, '2025-06-15T12:00:00Z')).toBeNull()
        expect(durationBetween('2025-06-15T12:00:00Z', null)).toBeNull()
        expect(durationBetween(null, null)).toBeNull()
    })

    it('returns the difference in ms', () => {
        expect(durationBetween('2025-06-15T12:00:00Z', '2025-06-15T12:05:00Z')).toBe(300_000)
    })

    it('can return negative for reversed dates', () => {
        expect(durationBetween('2025-06-15T12:05:00Z', '2025-06-15T12:00:00Z')).toBe(-300_000)
    })
})

describe('formatBytes', () => {
    it('returns "0 B" for zero', () => {
        expect(formatBytes(0)).toBe('0 B')
    })

    it('formats bytes', () => {
        expect(formatBytes(512)).toBe('512 B')
    })

    it('formats kilobytes', () => {
        expect(formatBytes(1024)).toBe('1.0 KB')
        expect(formatBytes(1536)).toBe('1.5 KB')
    })

    it('formats megabytes', () => {
        expect(formatBytes(1_048_576)).toBe('1.0 MB')
    })

    it('formats gigabytes', () => {
        expect(formatBytes(1_073_741_824)).toBe('1.0 GB')
    })
})
