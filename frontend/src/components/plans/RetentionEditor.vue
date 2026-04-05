<script setup lang="ts">
import type { RetentionPolicy } from '../../types/api'

const props = defineProps<{
  modelValue: RetentionPolicy
  disabled?: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [value: RetentionPolicy]
}>()

function update(field: keyof RetentionPolicy, value: number) {
  emit('update:modelValue', { ...props.modelValue, [field]: value })
}

const fields: { key: keyof RetentionPolicy; label: string }[] = [
  { key: 'keep_last', label: 'Keep Last' },
  { key: 'keep_hourly', label: 'Keep Hourly' },
  { key: 'keep_daily', label: 'Keep Daily' },
  { key: 'keep_weekly', label: 'Keep Weekly' },
  { key: 'keep_monthly', label: 'Keep Monthly' },
  { key: 'keep_yearly', label: 'Keep Yearly' },
]
</script>

<template>
  <div class="grid grid-cols-2 gap-4 sm:grid-cols-3">
    <div v-for="field in fields" :key="field.key">
      <label class="block text-sm font-medium text-slate-400">{{ field.label }}</label>
      <input
        type="number"
        min="0"
        :value="modelValue[field.key]"
        :disabled="disabled"
        class="mt-1 block w-full rounded border border-surface-600 bg-surface-950 px-3 py-2 text-sm text-slate-100 placeholder:text-slate-600 focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent/30 disabled:bg-surface-800 disabled:text-slate-600"
        @input="update(field.key, Number(($event.target as HTMLInputElement).value) || 0)"
      />
    </div>
  </div>
</template>
