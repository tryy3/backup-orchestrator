<script setup lang="ts">
import type { TreeNode } from '../../types/filesystem'

const props = defineProps<{
  node: TreeNode
  depth: number
  focusedPath: string | null
  selectedPath: string | null
}>()

defineEmits<{
  toggle: [path: string]
  select: [path: string]
  focus: [path: string]
}>()
</script>

<template>
  <div>
    <div
      :class="[
        'flex cursor-pointer items-center rounded px-2 py-1 text-sm transition-colors',
        focusedPath === node.entry.path
          ? 'bg-accent/10 text-slate-100'
          : 'text-slate-300 hover:bg-surface-700',
        selectedPath === node.entry.path && 'ring-1 ring-accent/30',
      ]"
      :style="{ paddingLeft: `${depth * 16 + 8}px` }"
      :data-focused="focusedPath === node.entry.path || undefined"
      @mouseover="$emit('focus', node.entry.path)"
      @click="$emit('select', node.entry.path)"
    >
      <!-- Chevron / spinner -->
      <button
        type="button"
        class="mr-1 flex h-5 w-5 flex-shrink-0 items-center justify-center rounded text-slate-500 hover:text-slate-300"
        :aria-label="node.expanded ? `Collapse ${node.entry.name}` : `Expand ${node.entry.name}`"
        :aria-expanded="node.expanded"
        @click.stop="$emit('toggle', node.entry.path)"
      >
        <svg v-if="node.loading" class="h-3.5 w-3.5 animate-spin text-accent" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
        </svg>
        <svg v-else-if="node.expanded" class="h-3 w-3" viewBox="0 0 12 12" fill="currentColor">
          <path d="M2 4l4 4 4-4H2z" />
        </svg>
        <svg v-else class="h-3 w-3" viewBox="0 0 12 12" fill="currentColor">
          <path d="M4 2l4 4-4 4V2z" />
        </svg>
      </button>

      <!-- Folder icon -->
      <svg class="mr-1.5 h-4 w-4 flex-shrink-0 text-amber-500/70" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor">
        <path d="M2 6a2 2 0 012-2h5l2 2h5a2 2 0 012 2v6a2 2 0 01-2 2H4a2 2 0 01-2-2V6z" />
      </svg>

      <!-- Name -->
      <span class="flex-1 truncate font-mono text-xs">{{ node.entry.name }}</span>

      <!-- Check if selected -->
      <svg v-if="selectedPath === node.entry.path" class="ml-1 h-4 w-4 flex-shrink-0 text-accent" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor">
        <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
      </svg>
    </div>

    <!-- Per-node error -->
    <div
      v-if="node.error"
      class="rounded bg-red-500/10 px-2 py-1 text-xs text-red-400"
      :style="{ marginLeft: `${depth * 16 + 32}px` }"
    >
      {{ node.error }}
    </div>

    <!-- Children -->
    <template v-if="node.expanded && node.children.length > 0">
      <FileBrowserNode
        v-for="child in node.children"
        :key="child.entry.path"
        :node="child"
        :depth="depth + 1"
        :focused-path="focusedPath"
        :selected-path="selectedPath"
        @toggle="$emit('toggle', $event)"
        @select="$emit('select', $event)"
        @focus="$emit('focus', $event)"
      />
    </template>
  </div>
</template>
