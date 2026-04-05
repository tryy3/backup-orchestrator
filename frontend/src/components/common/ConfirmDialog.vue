<script setup lang="ts">
defineProps<{
  open: boolean
  title: string
  message: string
  confirmText?: string
  confirmVariant?: 'danger' | 'primary'
}>()

defineEmits<{
  confirm: []
  cancel: []
}>()
</script>

<template>
  <Teleport to="body">
    <Transition
      enter-active-class="transition-opacity duration-200"
      leave-active-class="transition-opacity duration-150"
      enter-from-class="opacity-0"
      leave-to-class="opacity-0"
    >
      <div v-if="open" class="fixed inset-0 z-50 flex items-center justify-center">
        <!-- Backdrop -->
        <div class="absolute inset-0 bg-black/50" @click="$emit('cancel')" />

        <!-- Dialog -->
        <div class="relative z-10 w-full max-w-md rounded-lg border border-surface-600 bg-surface-800 p-6 shadow-xl">
          <h3 class="text-lg font-semibold text-slate-100">{{ title }}</h3>
          <p class="mt-2 text-sm text-slate-400">{{ message }}</p>

          <div class="mt-6 flex justify-end gap-3">
            <button
              class="rounded-md border border-surface-600 bg-surface-700 px-4 py-2 text-sm font-medium text-slate-300 transition-colors hover:bg-surface-600"
              @click="$emit('cancel')"
            >
              Cancel
            </button>
            <button
              :class="[
                'rounded-md px-4 py-2 text-sm font-medium text-white transition-colors',
                confirmVariant === 'danger'
                  ? 'bg-red-600 hover:bg-red-700'
                  : 'bg-cyan-600 hover:bg-cyan-700',
              ]"
              @click="$emit('confirm')"
            >
              {{ confirmText ?? 'Confirm' }}
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
