<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useAgentsStore } from '../../stores/agents'
import { usePlansStore } from '../../stores/plans'

const route = useRoute()
const agentsStore = useAgentsStore()
const plansStore = usePlansStore()

interface Breadcrumb {
  label: string
  to?: string
}

const breadcrumbs = computed<Breadcrumb[]>(() => {
  const name = route.name as string
  if (!name || name === 'fleet-overview') return []

  if (name === 'agent-inspect') {
    return [
      { label: 'Fleet Overview', to: '/' },
      { label: agentsStore.current?.name ?? String(route.params.id) },
    ]
  }

  if (name === 'plan-history') {
    const agentName = agentsStore.current?.name ?? String(route.params.id)
    const planName = plansStore.current?.name ?? String(route.params.planId)
    return [
      { label: 'Fleet Overview', to: '/' },
      { label: agentName, to: `/agents/${route.params.id}` },
      { label: planName },
    ]
  }

  if (name === 'job-detail' || name === 'job-console') {
    return [{ label: 'Fleet Overview', to: '/' }]
  }

  if (name === 'plan-new') return [{ label: 'Plans', to: '/plans' }, { label: 'New Plan' }]
  if (name === 'plan-edit') return [{ label: 'Plans', to: '/plans' }, { label: 'Edit Plan' }]
  if (name === 'plan-detail') return [{ label: 'Plans', to: '/plans' }, { label: 'Plan Details' }]
  if (name?.startsWith('plan')) return [{ label: 'Plans', to: '/plans' }]

  if (name === 'repository-new') return [{ label: 'Repositories', to: '/repositories' }, { label: 'New Repository' }]
  if (name === 'repository-edit') return [{ label: 'Repositories', to: '/repositories' }, { label: 'Edit Repository' }]
  if (name?.startsWith('repositor')) return [{ label: 'Repositories', to: '/repositories' }]

  if (name === 'script-new') return [{ label: 'Scripts', to: '/scripts' }, { label: 'New Script' }]
  if (name === 'script-edit') return [{ label: 'Scripts', to: '/scripts' }, { label: 'Edit Script' }]
  if (name?.startsWith('script')) return [{ label: 'Scripts', to: '/scripts' }]

  if (name === 'snapshots') return [{ label: 'Snapshots' }]
  if (name === 'settings') return [{ label: 'Settings' }]
  if (name === 'agents') return [{ label: 'Agents' }]
  if (name === 'jobs') return [{ label: 'Jobs' }]

  return []
})
</script>

<template>
  <header class="sticky top-0 z-10 flex h-12 items-center border-b border-surface-700 bg-surface-900/90 px-6 backdrop-blur">
    <nav v-if="breadcrumbs.length > 0" class="flex min-w-0 items-center gap-1 text-sm">
      <template v-for="(crumb, i) in breadcrumbs" :key="i">
        <span v-if="i > 0" class="select-none text-surface-600">/</span>
        <router-link
          v-if="crumb.to"
          :to="crumb.to"
          :class="[
            'flex items-center gap-1 truncate transition-colors',
            i === 0 ? 'text-accent hover:text-accent-dim' : 'text-slate-500 hover:text-slate-300',
          ]"
        >
          <svg v-if="i === 0" class="h-3.5 w-3.5 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M15.75 19.5L8.25 12l7.5-7.5" />
          </svg>
          {{ crumb.label }}
        </router-link>
        <span v-else class="truncate font-medium text-slate-300">{{ crumb.label }}</span>
      </template>
    </nav>
    <div v-else class="text-sm font-semibold tracking-widest text-slate-700">BACKUP ORCHESTRATOR</div>
  </header>
</template>
