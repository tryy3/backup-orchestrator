import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import StatusBadge from '../StatusBadge.vue'

describe('StatusBadge', () => {
    it('renders the status text', () => {
        const wrapper = mount(StatusBadge, { props: { status: 'success' } })
        expect(wrapper.text()).toBe('success')
    })

    it('applies green classes for success status', () => {
        const wrapper = mount(StatusBadge, { props: { status: 'success' } })
        expect(wrapper.find('span').classes()).toContain('text-green-400')
    })

    it('applies green classes for active status', () => {
        const wrapper = mount(StatusBadge, { props: { status: 'active' } })
        expect(wrapper.find('span').classes()).toContain('text-green-400')
    })

    it('applies red classes for failed status', () => {
        const wrapper = mount(StatusBadge, { props: { status: 'failed' } })
        expect(wrapper.find('span').classes()).toContain('text-red-400')
    })

    it('applies amber classes for partial status', () => {
        const wrapper = mount(StatusBadge, { props: { status: 'partial' } })
        expect(wrapper.find('span').classes()).toContain('text-amber-400')
    })

    it('applies cyan + animate-pulse for running status', () => {
        const wrapper = mount(StatusBadge, { props: { status: 'running' } })
        const classes = wrapper.find('span').classes()
        expect(classes).toContain('text-cyan-400')
        expect(classes).toContain('animate-pulse')
    })

    it('applies offline styling', () => {
        const wrapper = mount(StatusBadge, { props: { status: 'offline' } })
        expect(wrapper.find('span').classes()).toContain('text-slate-500')
    })

    it('falls back to default for unknown status', () => {
        const wrapper = mount(StatusBadge, { props: { status: 'unknown' } })
        expect(wrapper.find('span').classes()).toContain('text-slate-400')
    })

    it('is case-insensitive for status matching', () => {
        const wrapper = mount(StatusBadge, { props: { status: 'SUCCESS' } })
        expect(wrapper.find('span').classes()).toContain('text-green-400')
    })
})
