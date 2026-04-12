<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'

defineProps<{
  collapsed: boolean
}>()

defineEmits<{
  toggle: []
}>()

const route = useRoute()

const appVersion = computed(() => import.meta.env.VITE_APP_VERSION || 'dev')

const navSections = [
  {
    label: 'MONITOR',
    items: [
      { name: 'Fleet Overview', path: '/', icon: 'grid' },
      { name: 'Agents', path: '/agents', icon: 'server' },
    ],
  },
  {
    label: 'CONFIGURE',
    items: [
      { name: 'Plans', path: '/plans', icon: 'calendar' },
      { name: 'Repositories', path: '/repositories', icon: 'database' },
      { name: 'Scripts', path: '/scripts', icon: 'code' },
    ],
  },
  {
    label: 'SYSTEM',
    items: [
      { name: 'Snapshots', path: '/snapshots', icon: 'clock' },
      { name: 'Settings', path: '/settings', icon: 'settings' },
    ],
  },
]

const isActive = computed(() => (path: string) => {
  if (path === '/') return route.path === '/'
  return route.path.startsWith(path)
})
</script>

<template>
  <aside
    :class="[
      'fixed inset-y-0 left-0 z-30 flex flex-col border-r border-surface-700 bg-surface-900 text-slate-100 transition-all duration-200',
      collapsed ? 'w-16' : 'w-64',
    ]"
  >
    <!-- Header -->
    <div class="flex h-14 items-center justify-between border-b border-surface-700 px-3">
      <div v-if="!collapsed" class="flex min-w-0 items-center gap-2">
        <div class="flex h-6 w-6 shrink-0 items-center justify-center rounded bg-accent/20">
          <svg class="h-3.5 w-3.5 text-accent" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M9 12.75L11.25 15 15 9.75m-3-7.036A11.959 11.959 0 013.598 6 11.99 11.99 0 003 9.749c0 5.592 3.824 10.29 9 11.623 5.176-1.332 9-6.03 9-11.622 0-1.31-.21-2.571-.598-3.751h-.152c-3.196 0-6.1-1.248-8.25-3.285z" />
          </svg>
        </div>
        <span class="truncate text-sm font-semibold tracking-tight text-slate-100">Backup Orchestrator</span>
      </div>
      <button
        :class="['rounded p-1.5 text-slate-500 transition-colors hover:bg-surface-800 hover:text-slate-300', collapsed ? 'mx-auto' : '']"
        @click="$emit('toggle')"
      >
        <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path v-if="!collapsed" stroke-linecap="round" stroke-linejoin="round" d="M11 19l-7-7 7-7M19 19l-7-7 7-7" />
          <path v-else stroke-linecap="round" stroke-linejoin="round" d="M13 5l7 7-7 7M5 5l7 7-7 7" />
        </svg>
      </button>
    </div>

    <!-- Navigation -->
    <nav class="flex-1 overflow-y-auto py-3">
      <div v-for="section in navSections" :key="section.label" class="mb-1">
        <!-- Section label -->
        <div v-if="!collapsed" class="mb-1 px-4 pb-1 pt-3">
          <span class="text-[10px] font-semibold uppercase tracking-widest text-slate-600">{{ section.label }}</span>
        </div>
        <div v-else class="mx-3 my-2 border-t border-surface-700" />

        <ul class="space-y-0.5 px-2">
          <li v-for="item in section.items" :key="item.path">
            <router-link
              :to="item.path"
              :class="[
                'flex items-center gap-3 rounded-md px-2.5 py-2 text-sm font-medium transition-colors',
                isActive(item.path)
                  ? 'bg-accent/10 text-accent'
                  : 'text-slate-400 hover:bg-surface-800 hover:text-slate-200',
              ]"
              :title="collapsed ? item.name : undefined"
            >
              <!-- Fleet Overview (grid) -->
              <svg v-if="item.icon === 'grid'" class="h-4 w-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M3.75 6A2.25 2.25 0 016 3.75h2.25A2.25 2.25 0 0110.5 6v2.25a2.25 2.25 0 01-2.25 2.25H6a2.25 2.25 0 01-2.25-2.25V6zM3.75 15.75A2.25 2.25 0 016 13.5h2.25a2.25 2.25 0 012.25 2.25V18a2.25 2.25 0 01-2.25 2.25H6A2.25 2.25 0 013.75 18v-2.25zM13.5 6a2.25 2.25 0 012.25-2.25H18A2.25 2.25 0 0120.25 6v2.25A2.25 2.25 0 0118 10.5h-2.25a2.25 2.25 0 01-2.25-2.25V6zM13.5 15.75a2.25 2.25 0 012.25-2.25H18a2.25 2.25 0 012.25 2.25V18A2.25 2.25 0 0118 20.25h-2.25A2.25 2.25 0 0113.5 18v-2.25z" />
              </svg>
              <!-- Agents (server) -->
              <svg v-else-if="item.icon === 'server'" class="h-4 w-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M21.75 17.25v.75a2.25 2.25 0 01-2.25 2.25h-15a2.25 2.25 0 01-2.25-2.25v-.75M4.5 12.75h15m-15 0V8.25A2.25 2.25 0 016.75 6h10.5a2.25 2.25 0 012.25 2.25v4.5m-15 0v3.75m15-3.75v3.75M8.25 9.75h.008v.008H8.25V9.75zm3.75 0h.008v.008H12V9.75zm3.75 0h.008v.008h-.008V9.75z" />
              </svg>
              <!-- Plans (calendar) -->
              <svg v-else-if="item.icon === 'calendar'" class="h-4 w-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M6.75 3v2.25M17.25 3v2.25M3 18.75V7.5a2.25 2.25 0 012.25-2.25h13.5A2.25 2.25 0 0121 7.5v11.25m-18 0A2.25 2.25 0 005.25 21h13.5A2.25 2.25 0 0021 18.75m-18 0v-7.5A2.25 2.25 0 015.25 9h13.5A2.25 2.25 0 0121 11.25v7.5" />
              </svg>
              <!-- Repositories (database) -->
              <svg v-else-if="item.icon === 'database'" class="h-4 w-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M20.25 6.375c0 2.278-3.694 4.125-8.25 4.125S3.75 8.653 3.75 6.375m16.5 0c0-2.278-3.694-4.125-8.25-4.125S3.75 4.097 3.75 6.375m16.5 0v11.25c0 2.278-3.694 4.125-8.25 4.125s-8.25-1.847-8.25-4.125V6.375m16.5 0v3.75m-16.5-3.75v3.75m16.5 0v3.75C20.25 16.153 16.556 18 12 18s-8.25-1.847-8.25-4.125v-3.75m16.5 0c0 2.278-3.694 4.125-8.25 4.125s-8.25-1.847-8.25-4.125" />
              </svg>
              <!-- Scripts (code) -->
              <svg v-else-if="item.icon === 'code'" class="h-4 w-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M17.25 6.75L22.5 12l-5.25 5.25m-10.5 0L1.5 12l5.25-5.25m7.5-3l-4.5 16.5" />
              </svg>
              <!-- Snapshots (clock) -->
              <svg v-else-if="item.icon === 'clock'" class="h-4 w-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M12 6v6h4.5m4.5 0a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
              <!-- Settings (gear) -->
              <svg v-else class="h-4 w-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M9.594 3.94c.09-.542.56-.94 1.11-.94h2.593c.55 0 1.02.398 1.11.94l.213 1.281c.063.374.313.686.645.87.074.04.147.083.22.127.324.196.72.257 1.075.124l1.217-.456a1.125 1.125 0 011.37.49l1.296 2.247a1.125 1.125 0 01-.26 1.431l-1.003.827c-.293.24-.438.613-.431.992a6.759 6.759 0 010 .255c-.007.378.138.75.43.99l1.005.828c.424.35.534.954.26 1.43l-1.298 2.247a1.125 1.125 0 01-1.369.491l-1.217-.456c-.355-.133-.75-.072-1.076.124a6.57 6.57 0 01-.22.128c-.331.183-.581.495-.644.869l-.213 1.28c-.09.543-.56.941-1.11.941h-2.594c-.55 0-1.02-.398-1.11-.94l-.213-1.281c-.062-.374-.312-.686-.644-.87a6.52 6.52 0 01-.22-.127c-.325-.196-.72-.257-1.076-.124l-1.217.456a1.125 1.125 0 01-1.369-.49l-1.297-2.247a1.125 1.125 0 01.26-1.431l1.004-.827c.292-.24.437-.613.43-.992a6.932 6.932 0 010-.255c.007-.378-.138-.75-.43-.99l-1.004-.828a1.125 1.125 0 01-.26-1.43l1.297-2.247a1.125 1.125 0 011.37-.491l1.216.456c.356.133.751.072 1.076-.124.072-.044.146-.087.22-.128.332-.183.582-.495.644-.869l.214-1.281z" />
                <path stroke-linecap="round" stroke-linejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
              </svg>
              <span v-if="!collapsed" class="truncate">{{ item.name }}</span>
            </router-link>
          </li>
        </ul>
      </div>
    </nav>

    <!-- Footer -->
    <div v-if="!collapsed" class="border-t border-surface-700 px-4 py-3">
      <span class="font-mono text-[10px] uppercase tracking-wider text-slate-700">{{ appVersion }}</span>
    </div>
  </aside>
</template>
