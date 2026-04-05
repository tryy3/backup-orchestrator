<script setup lang="ts">
import { computed, watch } from 'vue'
import { useRepositoriesStore } from '../../stores/repositories'

const props = defineProps<{
  modelValue: string[]
  agentId: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string[]]
}>()

const repoStore = useRepositoriesStore()

watch(
  () => props.agentId,
  (id) => {
    if (id) {
      repoStore.fetchAll()
    }
  },
  { immediate: true },
)

const globalRepos = computed(() => repoStore.list.filter((r) => r.scope === 'global'))
const localRepos = computed(() =>
  repoStore.list.filter((r) => r.scope === 'local' && r.agent_id === props.agentId),
)

function toggle(id: string) {
  const current = [...props.modelValue]
  const idx = current.indexOf(id)
  if (idx >= 0) {
    current.splice(idx, 1)
  } else {
    current.push(id)
  }
  emit('update:modelValue', current)
}
</script>

<template>
  <div v-if="!agentId" class="text-sm text-slate-500">
    Select an agent first to see available repositories.
  </div>
  <div v-else class="space-y-4">
    <!-- Global repos -->
    <div v-if="globalRepos.length > 0">
      <h4 class="text-sm font-medium text-slate-400">Global Repositories</h4>
      <div class="mt-2 space-y-2">
        <label
          v-for="repo in globalRepos"
          :key="repo.id"
          class="flex cursor-pointer items-center gap-2 rounded border border-surface-700 p-2 transition-colors hover:bg-surface-800"
        >
          <input
            type="checkbox"
            :checked="modelValue.includes(repo.id)"
            class="rounded text-accent"
            @change="toggle(repo.id)"
          />
          <span class="text-sm text-slate-200">{{ repo.name }}</span>
          <span class="text-xs text-slate-500">({{ repo.type }})</span>
        </label>
      </div>
    </div>

    <!-- Local repos -->
    <div v-if="localRepos.length > 0">
      <h4 class="text-sm font-medium text-slate-400">Local Repositories (this agent)</h4>
      <div class="mt-2 space-y-2">
        <label
          v-for="repo in localRepos"
          :key="repo.id"
          class="flex cursor-pointer items-center gap-2 rounded border border-surface-700 p-2 transition-colors hover:bg-surface-800"
        >
          <input
            type="checkbox"
            :checked="modelValue.includes(repo.id)"
            class="rounded text-accent"
            @change="toggle(repo.id)"
          />
          <span class="text-sm text-slate-200">{{ repo.name }}</span>
          <span class="text-xs text-slate-500">({{ repo.type }})</span>
        </label>
      </div>
    </div>

    <div v-if="globalRepos.length === 0 && localRepos.length === 0" class="text-sm text-slate-500">
      No repositories available. Create one first.
    </div>
  </div>
</template>
