<script lang="ts" setup>
import { NButton } from 'naive-ui'
import { changeStatusClass, type BrowseRow } from './mappingBrowse'

type TreeNode = BrowseRow & { children?: TreeNode[]; error?: string; open?: boolean }

withDefaults(
  defineProps<{
    nodes: TreeNode[]
    selected: string[]
    // Changes tab keeps directory names fully opaque; skipped still disables directory sync.
    dimSkippedDirs?: boolean
  }>(),
  { dimSkippedDirs: true },
)

// "menu" is emitted by both the three-dot button and the context menu; the
// parent owns the single dropdown and decides what the menu item syncs.
const emit = defineEmits<{
  toggle: [node: TreeNode, event: MouseEvent]
  file: [row: BrowseRow, event: MouseEvent]
  menu: [row: BrowseRow, event: MouseEvent]
}>()

function rowOpacity(node: TreeNode, dimSkippedDirs: boolean) {
  if (!node.skipped) return 1
  if (node.isDir && !dimSkippedDirs) return 1
  return 0.45
}
</script>

<template>
  <ul style="list-style: none; padding-left: 16px; margin: 0">
    <li v-for="node in nodes" :key="node.relPath">
      <div
        :style="{
          opacity: rowOpacity(node, dimSkippedDirs),
          background: selected.includes(node.relPath) ? 'rgba(128, 128, 128, 0.25)' : '',
          cursor: 'pointer',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '2px 4px',
          userSelect: 'none',
        }"
        @click="node.isDir ? emit('toggle', node, $event) : emit('file', node, $event)"
        @contextmenu.prevent.stop="emit('menu', node, $event)"
      >
        <span :class="changeStatusClass(node)">{{ node.isDir ? (node.open ? '▾ ' : '▸ ') : '' }}{{ node.name }}</span>
        <n-button data-menu size="tiny" text @click.stop="emit('menu', node, $event)">···</n-button>
      </div>
      <p v-if="node.error" style="margin: 2px 0 2px 16px; color: #d03050">{{ node.error }}</p>
      <MappingFileTree
        v-if="node.open && node.children"
        :nodes="node.children"
        :selected="selected"
        :dim-skipped-dirs="dimSkippedDirs"
        @toggle="(child, event) => emit('toggle', child, event)"
        @file="(child, event) => emit('file', child, event)"
        @menu="(child, event) => emit('menu', child, event)"
      />
    </li>
  </ul>
</template>

<style>
html[data-theme='light'] .change-status-added {
  color: #1a7f37;
}
html[data-theme='light'] .change-status-modified {
  color: #9a6700;
}
html[data-theme='light'] .change-status-renamed {
  color: #0969da;
}
html[data-theme='dark'] .change-status-added {
  color: #3fb950;
}
html[data-theme='dark'] .change-status-modified {
  color: #d4a72c;
}
html[data-theme='dark'] .change-status-renamed {
  color: #58a6ff;
}
</style>
