<script setup lang="ts">
import { onUnmounted } from 'vue'
import AppLayout from './components/layout/AppLayout.vue'
import { connectWebSocket, disconnectWebSocket } from './api/websocket'

connectWebSocket()

onUnmounted(() => {
  disconnectWebSocket()
})
</script>

<template>
  <AppLayout>
    <router-view />
  </AppLayout>
</template>
