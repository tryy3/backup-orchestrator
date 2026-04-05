<script setup lang="ts">
import { ref, computed } from 'vue'
import LoadingSpinner from './LoadingSpinner.vue'
import EmptyState from './EmptyState.vue'

export interface Column {
  key: string
  label: string
  sortable?: boolean
}

const props = defineProps<{
  columns: Column[]
  rows: Record<string, unknown>[]
  loading?: boolean
  emptyTitle?: string
  emptyMessage?: string
}>()

defineEmits<{
  'row-click': [row: Record<string, unknown>]
}>()

const sortKey = ref<string | null>(null)
const sortAsc = ref(true)

function toggleSort(col: Column) {
  if (!col.sortable) return
  if (sortKey.value === col.key) {
    sortAsc.value = !sortAsc.value
  } else {
    sortKey.value = col.key
    sortAsc.value = true
  }
}

const sortedRows = computed(() => {
  if (!sortKey.value) return props.rows
  const key = sortKey.value
  const dir = sortAsc.value ? 1 : -1
  return [...props.rows].sort((a, b) => {
    const aVal = a[key]
    const bVal = b[key]
    if (aVal == null && bVal == null) return 0
    if (aVal == null) return 1
    if (bVal == null) return -1
    if (typeof aVal === 'string' && typeof bVal === 'string') {
      return aVal.localeCompare(bVal) * dir
    }
    if (typeof aVal === 'number' && typeof bVal === 'number') {
      return (aVal - bVal) * dir
    }
    return String(aVal).localeCompare(String(bVal)) * dir
  })
})
</script>

<template>
  <div class="overflow-hidden rounded-lg border border-gray-200 bg-white shadow-sm">
    <LoadingSpinner v-if="loading" />
    <EmptyState
      v-else-if="rows.length === 0"
      :title="emptyTitle ?? 'No data'"
      :message="emptyMessage ?? 'No records found.'"
    />
    <div v-else class="overflow-x-auto">
      <table class="min-w-full divide-y divide-gray-200">
        <thead class="bg-gray-50">
          <tr>
            <th
              v-for="col in columns"
              :key="col.key"
              :class="[
                'px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500',
                col.sortable ? 'cursor-pointer select-none hover:text-gray-700' : '',
              ]"
              @click="toggleSort(col)"
            >
              <span class="flex items-center gap-1">
                {{ col.label }}
                <template v-if="col.sortable && sortKey === col.key">
                  <span v-if="sortAsc">&#9650;</span>
                  <span v-else>&#9660;</span>
                </template>
              </span>
            </th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-200">
          <tr
            v-for="(row, idx) in sortedRows"
            :key="idx"
            :class="[
              idx % 2 === 0 ? 'bg-white' : 'bg-gray-50',
              'hover:bg-blue-50 transition-colors',
            ]"
            @click="$emit('row-click', row)"
          >
            <td v-for="col in columns" :key="col.key" class="whitespace-nowrap px-4 py-3 text-sm text-gray-700">
              <slot :name="`cell-${col.key}`" :row="row">
                {{ row[col.key] ?? '-' }}
              </slot>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
