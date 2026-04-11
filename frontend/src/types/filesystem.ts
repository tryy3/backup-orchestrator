import type { FilesystemEntry } from './api'

export interface TreeNode {
  entry: FilesystemEntry
  children: TreeNode[]
  loading: boolean
  error?: string
  expanded: boolean
}
