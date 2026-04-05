<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()

const routeTitles: Record<string, string> = {
  dashboard: 'Dashboard',
  agents: 'Agents',
  'agent-detail': 'Agent Details',
  repositories: 'Repositories',
  'repository-new': 'New Repository',
  'repository-edit': 'Edit Repository',
  plans: 'Backup Plans',
  'plan-new': 'New Backup Plan',
  'plan-detail': 'Plan Details',
  'plan-edit': 'Edit Backup Plan',
  scripts: 'Scripts',
  'script-new': 'New Script',
  'script-edit': 'Edit Script',
  jobs: 'Jobs',
  'job-detail': 'Job Details',
  snapshots: 'Snapshots',
  settings: 'Settings',
}

const pageTitle = computed(() => {
  const name = route.name as string
  return routeTitles[name] ?? 'Backup Orchestrator'
})

interface Breadcrumb {
  label: string
  to?: string
}

const breadcrumbs = computed<Breadcrumb[]>(() => {
  const crumbs: Breadcrumb[] = [{ label: 'Home', to: '/' }]
  const name = route.name as string

  if (name === 'dashboard') return crumbs

  if (name === 'agents' || name === 'agent-detail') {
    crumbs.push({ label: 'Agents', to: '/agents' })
    if (name === 'agent-detail') crumbs.push({ label: 'Details' })
  } else if (name?.startsWith('repositor')) {
    crumbs.push({ label: 'Repositories', to: '/repositories' })
    if (name === 'repository-new') crumbs.push({ label: 'New' })
    if (name === 'repository-edit') crumbs.push({ label: 'Edit' })
  } else if (name?.startsWith('plan')) {
    crumbs.push({ label: 'Plans', to: '/plans' })
    if (name === 'plan-new') crumbs.push({ label: 'New' })
    if (name === 'plan-detail') crumbs.push({ label: 'Details' })
    if (name === 'plan-edit') crumbs.push({ label: 'Edit' })
  } else if (name?.startsWith('script')) {
    crumbs.push({ label: 'Scripts', to: '/scripts' })
    if (name === 'script-new') crumbs.push({ label: 'New' })
    if (name === 'script-edit') crumbs.push({ label: 'Edit' })
  } else if (name === 'jobs' || name === 'job-detail') {
    crumbs.push({ label: 'Jobs', to: '/jobs' })
    if (name === 'job-detail') crumbs.push({ label: 'Details' })
  } else if (name === 'snapshots') {
    crumbs.push({ label: 'Snapshots' })
  } else if (name === 'settings') {
    crumbs.push({ label: 'Settings' })
  }

  return crumbs
})
</script>

<template>
  <header class="border-b border-gray-200 bg-white px-6 py-4">
    <nav class="mb-1 flex items-center gap-1 text-sm text-gray-500">
      <template v-for="(crumb, index) in breadcrumbs" :key="index">
        <span v-if="index > 0" class="mx-1">/</span>
        <router-link
          v-if="crumb.to"
          :to="crumb.to"
          class="hover:text-blue-600"
        >
          {{ crumb.label }}
        </router-link>
        <span v-else class="text-gray-700">{{ crumb.label }}</span>
      </template>
    </nav>
    <h1 class="text-2xl font-bold text-gray-900">{{ pageTitle }}</h1>
  </header>
</template>
