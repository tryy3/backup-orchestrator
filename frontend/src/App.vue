<script setup lang="ts">
import AppLayout from './components/layout/AppLayout.vue'
import { connectWebSocket } from './api/websocket'

connectWebSocket()
</script>

<template>
  <AppLayout>
    <router-view />
  </AppLayout>
</template>
