<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { agents } from '../../api/client'
import type { TreeNode } from '../../types/filesystem'
import FileBrowserNode from './FileBrowserNode.vue'

const props = defineProps<{
  modelValue: string
  agentId: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const isOpen = ref(false)
const inputValue = ref(props.modelValue)
const rootNodes = ref<TreeNode[]>([])
const rootLoading = ref(false)
const rootError = ref<string | null>(null)
const nodeCache = new Map<string, TreeNode[]>()
const inflightRequests = new Map<string, Promise<void>>()
const focusedPath = ref<string | null>(null)
const selectedPath = ref(props.modelValue || null)
const wrapperRef = ref<HTMLDivElement>()
const panelRef = ref<HTMLDivElement>()

// Aborts all in-flight browseFs requests when the agent changes or the component unmounts.
let fetchController = new AbortController()

// Track whether the input change came from the tree (to avoid re-syncing)
let syncingFromTree = false

watch(() => props.modelValue, (v) => {
  inputValue.value = v
  selectedPath.value = v || null
})

// Flatten visible tree into a navigable list for keyboard nav
const flattenedNodes = computed(() => {
  const result: { node: TreeNode; depth: number }[] = []
  const traverse = (nodes: TreeNode[], depth: number) => {
    for (const node of nodes) {
      result.push({ node, depth })
      if (node.expanded && node.children.length > 0) {
        traverse(node.children, depth + 1)
      }
    }
  }
  traverse(rootNodes.value, 0)
  return result
})

const focusedIndex = computed(() => {
  if (!focusedPath.value) return -1
  return flattenedNodes.value.findIndex(item => item.node.entry.path === focusedPath.value)
})

function findNodeByPath(nodes: TreeNode[], path: string): TreeNode | null {
  for (const node of nodes) {
    if (node.entry.path === path) return node
    if (node.children.length > 0) {
      const found = findNodeByPath(node.children, path)
      if (found) return found
    }
  }
  return null
}

// --- Input-to-tree sync ---
function pathSegments(p: string): string[] {
  const clean = p.replace(/\/+$/, '') || '/'
  if (clean === '/') return []
  const parts = clean.split('/').filter(Boolean)
  return parts.map((_, i) => '/' + parts.slice(0, i + 1).join('/'))
}

let syncTimer: ReturnType<typeof setTimeout> | null = null
let syncGeneration = 0

watch(inputValue, (val) => {
  if (syncingFromTree) {
    syncingFromTree = false
    return
  }
  selectedPath.value = val || null
  if (!isOpen.value || !val || !val.startsWith('/')) return
  if (syncTimer) clearTimeout(syncTimer)
  syncTimer = setTimeout(() => expandToPath(val), 300)
})

async function expandToPath(targetPath: string) {
  const gen = ++syncGeneration
  const segments = pathSegments(targetPath)

  if (rootNodes.value.length === 0) {
    await fetchChildren(null)
    if (gen !== syncGeneration) return
  }

  let deepestMatch: string | null = null
  for (const seg of segments) {
    const node = findNodeByPath(rootNodes.value, seg)
    if (!node) break
    deepestMatch = seg
    if (!node.expanded) {
      await fetchChildren(seg)
      if (gen !== syncGeneration) return
    }
  }

  if (deepestMatch) {
    focusedPath.value = deepestMatch
    if (findNodeByPath(rootNodes.value, targetPath)) {
      selectedPath.value = targetPath
    }
    await nextTick()
    scrollFocusedIntoView()
  }
}

function scrollFocusedIntoView() {
  const el = panelRef.value?.querySelector('[data-focused="true"]')
  el?.scrollIntoView({ block: 'nearest' })
}

// --- Panel open/close ---

async function openPanel() {
  if (isOpen.value) {
    isOpen.value = false
    return
  }
  isOpen.value = true
  if (rootNodes.value.length === 0) {
    await fetchChildren(null)
  }
  if (inputValue.value && inputValue.value.startsWith('/')) {
    await expandToPath(inputValue.value)
  }
  await nextTick()
  panelRef.value?.focus()
  if (!focusedPath.value && flattenedNodes.value.length > 0) {
    focusedPath.value = flattenedNodes.value[0].node.entry.path
  }
}

async function handleInputFocus() {
  if (!isOpen.value && props.agentId) {
    isOpen.value = true
    if (rootNodes.value.length === 0) {
      await fetchChildren(null)
    }
    if (inputValue.value && inputValue.value.startsWith('/')) {
      await expandToPath(inputValue.value)
    }
    if (!focusedPath.value && flattenedNodes.value.length > 0) {
      focusedPath.value = flattenedNodes.value[0].node.entry.path
    }
  }
}

// --- Data fetching with dedup ---

async function fetchChildren(nodePath: string | null) {
  const path = nodePath ?? '/'

  if (nodeCache.has(path)) {
    if (nodePath) {
      const target = findNodeByPath(rootNodes.value, nodePath)
      if (target) {
        target.children = nodeCache.get(path)!
        target.expanded = true
      }
    }
    return
  }

  // Deduplicate in-flight requests for the same path
  if (inflightRequests.has(path)) {
    return inflightRequests.get(path)
  }

  const promise = doFetchChildren(nodePath, path)
  inflightRequests.set(path, promise)
  try {
    await promise
  } finally {
    inflightRequests.delete(path)
  }
}

async function doFetchChildren(nodePath: string | null, path: string) {
  const target = nodePath ? findNodeByPath(rootNodes.value, nodePath) : null
  if (target) {
    target.loading = true
    target.error = undefined
  } else if (!nodePath) {
    rootLoading.value = true
    rootError.value = null
  }

  try {
    const entries = await agents.browseFs(props.agentId, path, fetchController.signal)
    const nodes: TreeNode[] = entries.map((e) => ({
      entry: e,
      children: [],
      loading: false,
      expanded: false,
    }))
    nodeCache.set(path, nodes)

    if (target) {
      target.children = nodes
      target.expanded = true
      target.loading = false
    } else {
      rootNodes.value = nodes
      rootLoading.value = false
    }
  } catch (e) {
    // Aborted requests are expected on agent change / unmount — silently ignore.
    if (e instanceof DOMException && e.name === 'AbortError') return

    const msg = e instanceof Error ? e.message : String(e)
    if (target) {
      target.error = msg
      target.loading = false
    } else {
      rootError.value = msg
      rootLoading.value = false
    }
  }
}

async function toggleNode(nodePath: string) {
  const node = findNodeByPath(rootNodes.value, nodePath)
  if (!node) return
  if (node.expanded) {
    node.expanded = false
  } else {
    await fetchChildren(nodePath)
  }
}

function selectPath(path: string) {
  syncingFromTree = true
  inputValue.value = path
  selectedPath.value = path
  focusedPath.value = path
  emit('update:modelValue', path)
  isOpen.value = false
}

function handleInputBlur() {
  if (inputValue.value !== props.modelValue) {
    emit('update:modelValue', inputValue.value)
  }
}

function handleInputKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter') {
    emit('update:modelValue', inputValue.value)
  }
}

async function handlePanelKeydown(e: KeyboardEvent) {
  if (!isOpen.value) return
  const items = flattenedNodes.value
  const current = focusedIndex.value

  switch (e.key) {
    case 'ArrowUp':
      e.preventDefault()
      if (current > 0) {
        focusedPath.value = items[current - 1].node.entry.path
        await nextTick()
        scrollFocusedIntoView()
      }
      break
    case 'ArrowDown':
      e.preventDefault()
      if (current < items.length - 1) {
        focusedPath.value = items[current + 1].node.entry.path
        await nextTick()
        scrollFocusedIntoView()
      }
      break
    case 'ArrowRight':
      e.preventDefault()
      if (focusedPath.value) {
        const node = findNodeByPath(rootNodes.value, focusedPath.value)
        if (node && !node.expanded) toggleNode(focusedPath.value)
      }
      break
    case 'ArrowLeft':
      e.preventDefault()
      if (focusedPath.value) {
        const node = findNodeByPath(rootNodes.value, focusedPath.value)
        if (node?.expanded) node.expanded = false
      }
      break
    case 'Enter':
      e.preventDefault()
      if (focusedPath.value) selectPath(focusedPath.value)
      break
    case 'Escape':
      e.preventDefault()
      isOpen.value = false
      break
  }
}

function handleClickOutside(e: MouseEvent) {
  if (isOpen.value && wrapperRef.value && !wrapperRef.value.contains(e.target as Node)) {
    isOpen.value = false
  }
}

// Reset cache when agent changes; abort in-flight requests
watch(() => props.agentId, () => {
  fetchController.abort()
  fetchController = new AbortController()
  nodeCache.clear()
  inflightRequests.clear()
  rootNodes.value = []
  rootError.value = null
})

onMounted(() => document.addEventListener('mousedown', handleClickOutside))
onUnmounted(() => {
  fetchController.abort()
  document.removeEventListener('mousedown', handleClickOutside)
  if (syncTimer) clearTimeout(syncTimer)
})
</script>

<template>
  <div ref="wrapperRef" class="relative">
    <!-- Trigger: input + browse button -->
    <div class="flex gap-2">
      <div class="flex flex-1 items-center rounded border border-surface-600 bg-surface-950 focus-within:border-accent focus-within:ring-1 focus-within:ring-accent/30">
        <span class="pl-3 text-slate-500">
          <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" viewBox="0 0 20 20" fill="currentColor">
            <path d="M2 6a2 2 0 012-2h5l2 2h5a2 2 0 012 2v6a2 2 0 01-2 2H4a2 2 0 01-2-2V6z" />
          </svg>
        </span>
        <input
          v-model="inputValue"
          type="text"
          class="flex-1 bg-transparent px-2 py-2 font-mono text-sm text-slate-100 placeholder:text-slate-600 outline-none"
          placeholder="Enter path or browse..."
          @focus="handleInputFocus"
          @blur="handleInputBlur"
          @keydown="handleInputKeydown"
        />
      </div>
      <button
        type="button"
        class="rounded border border-surface-600 bg-surface-700 px-3 py-2 text-sm font-medium text-slate-300 transition-colors hover:bg-surface-600"
        :disabled="!agentId"
        @click="openPanel"
      >
        Browse
      </button>
    </div>

    <!-- Dropdown panel -->
    <div
      v-if="isOpen"
      ref="panelRef"
      class="absolute left-0 right-0 top-full z-50 mt-1 rounded border border-surface-600 bg-surface-800 shadow-xl"
      tabindex="-1"
      @keydown="handlePanelKeydown"
    >
      <div class="border-b border-surface-700 px-3 py-2 text-xs font-semibold uppercase tracking-wider text-slate-500">
        Select Destination
      </div>
      <div class="max-h-80 overflow-y-auto p-1">
        <!-- Loading root -->
        <div v-if="rootLoading" class="flex items-center justify-center py-6 text-sm text-slate-500">
          <svg class="mr-2 h-4 w-4 animate-spin" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
          </svg>
          Loading...
        </div>

        <!-- Root error -->
        <div v-else-if="rootError" class="rounded bg-red-500/10 px-3 py-2 text-xs text-red-400">
          {{ rootError }}
        </div>

        <!-- Empty -->
        <div v-else-if="rootNodes.length === 0 && !rootLoading" class="py-6 text-center text-sm text-slate-500">
          No directories found
        </div>

        <!-- Tree -->
        <template v-else>
          <FileBrowserNode
            v-for="node in rootNodes"
            :key="node.entry.path"
            :node="node"
            :depth="0"
            :focused-path="focusedPath"
            :selected-path="selectedPath"
            @toggle="toggleNode"
            @select="selectPath"
            @focus="focusedPath = $event"
          />
        </template>
      </div>
    </div>
  </div>
</template>
