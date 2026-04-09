<script setup lang="ts">
import { useRouter } from 'vue-router'

const router = useRouter()
</script>

<template>
  <div class="flex flex-col items-center justify-center py-32 text-center">
    <h1 class="text-6xl font-bold text-slate-600">404</h1>
    <p class="mt-4 text-lg text-slate-400">Page not found</p>
    <button
      class="mt-6 rounded-lg bg-accent/10 px-4 py-2 text-sm font-medium text-accent ring-1 ring-accent/30 hover:bg-accent/20 transition-colors"
      @click="router.push('/')"
    >
      Back to Dashboard
    </button>
  </div>
</template>
