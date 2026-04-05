<script setup lang="ts">
import { ref } from 'vue'
import Sidebar from './Sidebar.vue'
import Header from './Header.vue'

const sidebarCollapsed = ref(false)
</script>

<template>
  <div class="flex min-h-screen bg-surface-950">
    <Sidebar :collapsed="sidebarCollapsed" @toggle="sidebarCollapsed = !sidebarCollapsed" />

    <!-- Mobile overlay -->
    <div
      v-if="!sidebarCollapsed"
      class="fixed inset-0 z-20 bg-black/50 lg:hidden"
      @click="sidebarCollapsed = true"
    />

    <!-- Main content -->
    <div
      :class="['flex flex-1 flex-col transition-all duration-200', sidebarCollapsed ? 'ml-16' : 'ml-64']"
    >
      <Header />
      <main class="flex-1 p-6">
        <slot />
      </main>
    </div>
  </div>
</template>
