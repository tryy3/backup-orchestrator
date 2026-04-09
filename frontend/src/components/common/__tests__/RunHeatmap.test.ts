import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import RunHeatmap from '../RunHeatmap.vue'
import type { HeatmapRun } from '../RunHeatmap.vue'

// Stub vue-router since the component uses useRouter
vi.mock('vue-router', () => ({
    useRouter: () => ({
        push: vi.fn(),
    }),
}))

function makeRun(overrides: Partial<HeatmapRun> = {}): HeatmapRun {
    return {
        id: 'run-1',
        status: 'success',
        started_at: '2025-06-15T10:00:00Z',
        finished_at: '2025-06-15T10:05:00Z',
        ...overrides,
    }
}

describe('RunHeatmap', () => {
    it('renders empty cells when there are no runs', () => {
        const wrapper = mount(RunHeatmap, {
            props: { runs: [], maxRuns: 10 },
        })
        const cells = wrapper.findAll('[class*="bg-surface-700"]')
        expect(cells).toHaveLength(10)
    })

    it('renders run cells for completed runs', () => {
        const runs = [
            makeRun({ id: 'r1', status: 'success' }),
            makeRun({ id: 'r2', status: 'failed' }),
        ]
        const wrapper = mount(RunHeatmap, {
            props: { runs, maxRuns: 5 },
        })
        const heatmapCells = wrapper.findAll('[data-heatmap-cell]')
        expect(heatmapCells).toHaveLength(2)
    })

    it('filters out planned and running runs from display', () => {
        const runs = [
            makeRun({ id: 'r1', status: 'success' }),
            makeRun({ id: 'r2', status: 'planned' }),
            makeRun({ id: 'r3', status: 'running' }),
        ]
        const wrapper = mount(RunHeatmap, {
            props: { runs },
        })
        const heatmapCells = wrapper.findAll('[data-heatmap-cell]')
        expect(heatmapCells).toHaveLength(1) // only 'success'
    })

    it('limits displayed runs to maxRuns', () => {
        const runs = Array.from({ length: 50 }, (_, i) =>
            makeRun({ id: `r${i}`, started_at: `2025-06-${String(i + 1).padStart(2, '0')}T10:00:00Z` }),
        )
        const wrapper = mount(RunHeatmap, {
            props: { runs, maxRuns: 10 },
        })
        const heatmapCells = wrapper.findAll('[data-heatmap-cell]')
        expect(heatmapCells).toHaveLength(10)
    })

    it('applies correct color classes based on run status', () => {
        const runs = [
            makeRun({ id: 'r1', status: 'success', started_at: '2025-06-01T10:00:00Z' }),
            makeRun({ id: 'r2', status: 'failed', started_at: '2025-06-02T10:00:00Z' }),
            makeRun({ id: 'r3', status: 'partial', started_at: '2025-06-03T10:00:00Z' }),
        ]
        const wrapper = mount(RunHeatmap, {
            props: { runs },
        })
        const cells = wrapper.findAll('[data-heatmap-cell]')
        expect(cells[0].classes()).toContain('bg-green-500')
        expect(cells[1].classes()).toContain('bg-red-500')
        expect(cells[2].classes()).toContain('bg-amber-500')
    })

    it('shows tooltip on hover', async () => {
        const runs = [makeRun({ id: 'r1', status: 'success' })]
        const wrapper = mount(RunHeatmap, {
            props: { runs, maxRuns: 1 },
        })

        expect(wrapper.find('.whitespace-pre').exists()).toBe(false)

        await wrapper.find('[data-heatmap-cell]').trigger('mouseenter')
        expect(wrapper.find('.whitespace-pre').exists()).toBe(true)
        expect(wrapper.find('.whitespace-pre').text()).toContain('Success')
    })

    it('hides tooltip on mouse leave', async () => {
        const runs = [makeRun({ id: 'r1', status: 'success' })]
        const wrapper = mount(RunHeatmap, {
            props: { runs, maxRuns: 1 },
        })

        const cell = wrapper.find('[data-heatmap-cell]')
        await cell.trigger('mouseenter')
        expect(wrapper.find('.whitespace-pre').exists()).toBe(true)

        await cell.trigger('mouseleave')
        expect(wrapper.find('.whitespace-pre').exists()).toBe(false)
    })

    it('defaults maxRuns to 30', () => {
        const runs = Array.from({ length: 35 }, (_, i) =>
            makeRun({ id: `r${i}`, started_at: `2025-06-${String(i + 1).padStart(2, '0')}T10:00:00Z` }),
        )
        const wrapper = mount(RunHeatmap, {
            props: { runs },
        })
        const heatmapCells = wrapper.findAll('[data-heatmap-cell]')
        expect(heatmapCells).toHaveLength(30)
    })
})
