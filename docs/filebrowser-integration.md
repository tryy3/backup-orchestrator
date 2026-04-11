# FileBrowser Integration Guide

The `FileBrowser` component provides a user-friendly way to browse and select filesystem paths on connected agents. This guide shows how to integrate it into existing forms.

## Component Props & Events

```typescript
// Props
modelValue: string           // The currently selected path
agentId: string              // ID of the agent to browse

// Events
@update:modelValue            // Emitted when user selects a path
```

## Basic Example

```vue
<script setup lang="ts">
import { ref } from 'vue'
import FileBrowser from '@/components/common/FileBrowser.vue'

const form = ref({
  path: '/home/user/data',
  agentId: 'agent-123',
})
</script>

<template>
  <div class="form-group">
    <label>Source Path</label>
    <FileBrowser v-model="form.path" :agent-id="form.agentId" />
  </div>
</template>
```

## Integration into Repository Creation Form

When creating a local repository, allow users to browse the agent's filesystem:

```vue
<script setup lang="ts">
import { ref, computed } from 'vue'
import FileBrowser from '@/components/common/FileBrowser.vue'
import { api } from '@/api/client'

const form = ref({
  name: '',
  scope: 'local',
  agent_id: '',
  type: 'local',
  path: '',
  password: '',
})

const selectedAgent = ref<string | null>(null)

// Only show FileBrowser if this is a local repo with an agent selected
const showBrowser = computed(() => {
  return form.value.type === 'local' && form.value.agent_id
})

async function createRepository() {
  if (!form.value.path) {
    alert('Please specify a repository path')
    return
  }
  
  await api.repositories.create({
    name: form.value.name,
    scope: form.value.scope,
    agent_id: form.value.agent_id || undefined,
    type: form.value.type,
    path: form.value.path,
    password: form.value.password,
  })
}
</script>

<template>
  <form @submit.prevent="createRepository">
    <!-- Name -->
    <div class="form-group mb-4">
      <label class="block font-semibold mb-2">Repository Name</label>
      <input v-model="form.name" type="text" class="border rounded px-3 py-2 w-full" />
    </div>

    <!-- Agent selection -->
    <div class="form-group mb-4">
      <label class="block font-semibold mb-2">Agent</label>
      <select v-model="form.agent_id" class="border rounded px-3 py-2 w-full">
        <option value="">Select an agent...</option>
        <!-- Agent options here -->
      </select>
    </div>

    <!-- Repository type -->
    <div class="form-group mb-4">
      <label class="block font-semibold mb-2">Type</label>
      <select v-model="form.type" class="border rounded px-3 py-2 w-full">
        <option value="local">Local</option>
        <option value="s3">S3</option>
        <option value="rclone">Rclone</option>
      </select>
    </div>

    <!-- Path - with FileBrowser for local repos -->
    <div class="form-group mb-4">
      <label class="block font-semibold mb-2">Repository Path</label>
      <template v-if="showBrowser">
        <FileBrowser v-model="form.path" :agent-id="form.agent_id" />
      </template>
      <template v-else>
        <input v-model="form.path" type="text" placeholder="e.g. /backups/repo" 
               class="border rounded px-3 py-2 w-full" />
      </template>
    </div>

    <!-- Password -->
    <div class="form-group mb-4">
      <label class="block font-semibold mb-2">Password</label>
      <input v-model="form.password" type="password" class="border rounded px-3 py-2 w-full" />
    </div>

    <button type="submit" class="bg-blue-600 text-white px-4 py-2 rounded hover:bg-blue-700">
      Create Repository
    </button>
  </form>
</template>
```

## Integration into Backup Plan Source Paths

For backup plans, allow users to browse and select multiple source paths:

```vue
<script setup lang="ts">
import { ref } from 'vue'
import FileBrowser from '@/components/common/FileBrowser.vue'

const form = ref({
  name: '',
  agent_id: '',
  paths: ['/home/user/documents', '/home/user/photos'],
  // ... other fields
})

// Temporary path for the browser (not yet added to list)
const tempPath = ref('')

function addPath() {
  if (tempPath.value && !form.value.paths.includes(tempPath.value)) {
    form.value.paths.push(tempPath.value)
    tempPath.value = ''
  }
}

function removePath(path: string) {
  const idx = form.value.paths.indexOf(path)
  if (idx >= 0) {
    form.value.paths.splice(idx, 1)
  }
}

function handlePathSelect(path: string) {
  tempPath.value = path
}
</script>

<template>
  <div class="form-group">
    <label class="block font-semibold mb-2">Source Paths</label>
    
    <!-- FileBrowser to pick paths -->
    <div class="mb-3">
      <label class="block text-sm text-gray-600 mb-2">Browse and add paths:</label>
      <FileBrowser v-model="tempPath" :agent-id="form.agent_id" />
      <button 
        @click="addPath" 
        type="button"
        class="mt-2 bg-green-600 text-white px-3 py-1 rounded hover:bg-green-700 text-sm"
        :disabled="!tempPath || form.paths.includes(tempPath)"
      >
        Add Path
      </button>
    </div>

    <!-- List of selected paths -->
    <div class="border rounded p-3 bg-gray-50">
      <div v-if="form.paths.length === 0" class="text-gray-500 text-sm">
        No paths selected yet
      </div>
      <div v-else class="space-y-2">
        <div 
          v-for="(path, idx) in form.paths" 
          :key="idx"
          class="flex items-center justify-between bg-white p-2 rounded border"
        >
          <code class="text-sm">{{ path }}</code>
          <button 
            @click="removePath(path)"
            type="button"
            class="text-red-600 hover:text-red-800 text-sm font-semibold"
          >
            Remove
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
```

## Features & Behavior

### User Experience

- **Browse**: Users click the "Browse" button to open the directory browser
- **Navigate**: Use mouse to click folder names to expand/collapse and select, or keyboard (↑↓ arrows, Tab/→ to expand, ← to collapse, Enter to select)
- **Type**: Users can also type or paste paths directly in the input field
- **Close**: Click outside, press Esc, or click Browse again to close without changing the selected path

### Performance

- **On-demand loading**: Directories are fetched only when expanded
- **Session caching**: Once a directory is fetched, it's cached for the duration of the browser session
- **No server caching**: Each browse session queries the live filesystem

### Security

- **Agent-side validation**: The agent validates all paths:
  - Must be absolute (starting with `/`)
  - Blocked paths: `/proc`, `/sys`, `/dev`, `/run/credentials`, `/selinux`, `/cgroup`
  - Path traversal attempts are blocked with `filepath.Clean` validation
  - Permission errors are returned clearly to the user

### Styling

The component uses Tailwind CSS classes. Customize by modifying:
- Input styling: `.border`, `.rounded`, `.px-2`, `.py-2`
- Button styling: `.bg-blue-600`, `.text-white`, `.hover:bg-blue-700`
- Panel styling: `.shadow-lg`, `.max-h-80`, `.overflow-y-auto`

## Error Handling

The component gracefully handles errors:
- **Agent offline**: Shows "agent not connected" with 502 status
- **Path not found**: Shows "failed to read directory" with specific error
- **Permission denied**: Shows the error message from the agent
- **Per-node errors**: Errors on individual nodes are displayed inline as the user navigates

Errors are displayed inline next to the affected directory, allowing the user to continue browsing other paths.

## Accessibility

- Full keyboard navigation support (arrow keys, Tab, Enter, Esc)
- Semantic HTML with proper labels and roles
- Loading states and error messages for assistive technologies
- ARIA labels on interactive elements
