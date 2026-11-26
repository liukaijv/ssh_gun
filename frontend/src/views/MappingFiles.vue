<script lang="ts" setup>
import { computed, nextTick, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NButton, NDropdown, NSpace, NSpin, NTabPane, NTabs, useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import {
  ListMappingChanges,
  ListMappingDir,
  ListSyncMappings,
  SyncMappingChanges,
  SyncMappingFiles,
  SyncMappingPath,
} from '../../wailsjs/go/main/App'
import {
  changeDirectoryPaths,
  directoryHasSyncableChange,
  menuSync,
  nestChanges,
  nextFileSelection,
  selectionWithout,
  visibleFiles,
  type BrowseRow,
} from './mappingBrowse'
import MappingFileTree from './MappingFileTree.vue'

type TreeNode = BrowseRow & { children?: TreeNode[]; loaded?: boolean; error?: string; open?: boolean }

const route = useRoute()
const router = useRouter()
const message = useMessage()
const { t } = useI18n()
const mapping = ref<any>(null)
const tab = ref<'files' | 'changes'>('files')
const filesRoot = ref<TreeNode[]>([])
const filesError = ref('')
const changesRoot = ref<TreeNode[]>([])
// Full flat list from ListMappingChanges. directoryHasSyncableChange must see
// every changed file, including those under collapsed directories.
const changesFlat = ref<BrowseRow[]>([])
const changesError = ref('')
const selected = ref<string[]>([])
const anchor = ref<string | null>(null)
const expanded = ref<Set<string>>(new Set())
const syncing = ref(false)
const loading = ref(false)
let loadSeq = 0

const menuShow = ref(false)
const menuX = ref(0)
const menuY = ref(0)
const menuRow = ref<BrowseRow | null>(null)

const id = computed(() => String(route.params.id || ''))
const visible = computed(() =>
  visibleFiles(flatOpen(tab.value === 'files' ? filesRoot.value : changesRoot.value), expanded.value),
)
const menuItems = computed(() => {
  const row = menuRow.value
  if (!row) return []
  const action = menuSync(row, selected.value, directoryEnabled(row))
  return [
    {
      key: 'sync',
      label: t('mappings.syncToRemote'),
      disabled: action.kind === 'disabled' || !mapping.value?.Enabled || syncing.value,
    },
  ]
})

function rowFrom(entry: any): BrowseRow {
  const status = entry.status ?? entry.Status
  return {
    name: entry.name ?? entry.Name,
    relPath: entry.relPath ?? entry.RelPath,
    isDir: Boolean(entry.isDir ?? entry.IsDir),
    skipped: Boolean(entry.skipped ?? entry.Skipped),
    status: typeof status === 'string' && status ? status : undefined,
  }
}

function flatOpen(nodes: TreeNode[]): BrowseRow[] {
  const out: BrowseRow[] = []
  for (const node of nodes) {
    out.push(node)
    if (node.open && node.children) out.push(...flatOpen(node.children))
  }
  return out
}

function setExpanded(node: TreeNode) {
  const next = new Set(expanded.value)
  if (node.open) next.add(node.relPath)
  else next.delete(node.relPath)
  expanded.value = next
}

async function loadMapping() {
  const rows = await ListSyncMappings()
  mapping.value = rows.find((row: any) => (row.ID ?? row.id) === id.value) || null
}

async function loadFiles() {
  const seq = ++loadSeq
  filesError.value = ''
  loading.value = true
  try {
    const entries = await ListMappingDir(id.value, '')
    if (seq !== loadSeq) return
    filesRoot.value = entries.map((entry: any) => ({ ...rowFrom(entry) }))
  } catch (error: any) {
    if (seq !== loadSeq) return
    filesError.value = String(error)
    filesRoot.value = []
  } finally {
    if (seq === loadSeq) loading.value = false
  }
}

async function loadChanges() {
  const seq = ++loadSeq
  changesError.value = ''
  loading.value = true
  try {
    const flat = (await ListMappingChanges(id.value)).map((entry: any) => rowFrom(entry))
    if (seq !== loadSeq) return
    changesFlat.value = flat
    changesRoot.value = nestChanges(flat)
    expanded.value = new Set(changeDirectoryPaths(flat))
  } catch (error: any) {
    if (seq !== loadSeq) return
    changesFlat.value = []
    changesRoot.value = []
    expanded.value = new Set()
    changesError.value = String(error).includes('not a git repository') ? t('mappings.notGitRepo') : String(error)
  } finally {
    if (seq === loadSeq) loading.value = false
  }
}

function closeMenu() {
  menuShow.value = false
  menuRow.value = null
}

async function refresh() {
  closeMenu()
  selected.value = []
  anchor.value = null
  expanded.value = new Set()
  if (tab.value === 'files') await loadFiles()
  else await loadChanges()
}

async function onTabChange(value: 'files' | 'changes') {
  tab.value = value
  await refresh()
}

async function toggleFileDir(node: TreeNode) {
  node.open = !node.open
  setExpanded(node)
  if (!node.open) return
  // Expanding always re-lists the directory.
  node.error = ''
  try {
    const entries = await ListMappingDir(id.value, node.relPath)
    node.children = entries.map((entry: any) => ({ ...rowFrom(entry) }))
    node.loaded = true
  } catch (error: any) {
    node.error = String(error)
  }
}

function toggleChangeDir(node: TreeNode) {
  node.open = !node.open
  setExpanded(node)
}

function onToggle(node: TreeNode) {
  if (tab.value === 'changes') toggleChangeDir(node)
  else void toggleFileDir(node)
  if (!node.open) {
    const next = selectionWithout(selected.value, anchor.value, node.relPath)
    selected.value = next.selected
    anchor.value = next.anchor
  }
}

function onFileClick(row: BrowseRow, event: MouseEvent) {
  const next = nextFileSelection(
    selected.value,
    anchor.value,
    row,
    visible.value,
    event.ctrlKey || event.metaKey,
    event.shiftKey,
  )
  selected.value = next.selected
  anchor.value = next.anchor
}

function directoryEnabled(row: BrowseRow) {
  if (tab.value === 'changes') return directoryHasSyncableChange(changesFlat.value, row.relPath)
  return true
}

async function runSync(start: () => Promise<any>) {
  if (syncing.value) {
    message.warning(t('mappings.syncInProgress'))
    return
  }
  if (!mapping.value?.Enabled) {
    message.error(t('mappings.syncDisabled'))
    return
  }
  syncing.value = true
  const started = setTimeout(() => message.info(t('mappings.syncStarted')), 400)
  try {
    const res = await start()
    clearTimeout(started)
    if (res?.result === 'ok') message.success(t('mappings.syncDone', { count: res.files ?? 0 }))
    else if (res?.result === 'busy') message.warning(t('mappings.syncInProgress'))
    else message.error(res?.error || t('mappings.syncFailed'))
  } catch (error: any) {
    clearTimeout(started)
    message.error(String(error))
  } finally {
    clearTimeout(started)
    syncing.value = false
  }
}

function choose(row: BrowseRow) {
  const action = menuSync(row, selected.value, directoryEnabled(row))
  if (action.kind === 'disabled' || !mapping.value?.Enabled || syncing.value) return
  const mappingId = id.value
  if (action.kind === 'files') {
    const rels = action.rels
    void runSync(() => SyncMappingFiles(mappingId, rels))
    return
  }
  if (action.clear) {
    selected.value = []
    anchor.value = null
  }
  const rel = action.rel
  void runSync(() => (tab.value === 'changes' ? SyncMappingChanges(mappingId, rel) : SyncMappingPath(mappingId, rel)))
}

async function openMenu(row: BrowseRow, event: MouseEvent) {
  closeMenu()
  await nextTick()
  menuRow.value = row
  menuX.value = event.clientX
  menuY.value = event.clientY
  menuShow.value = true
}

function onMenuSelect() {
  const row = menuRow.value
  closeMenu()
  if (row) choose(row)
}

onMounted(async () => {
  try {
    await loadMapping()
  } catch (error: any) {
    message.error(String(error))
  }
  await loadFiles()
})
</script>

<template>
  <n-spin :show="loading">
    <n-space align="center" justify="space-between">
      <n-button text @click="router.push({ name: 'mappings' })">{{ t('mappings.backToMappings') }}</n-button>
      <n-button @click="refresh">{{ t('common.refresh') }}</n-button>
    </n-space>
    <h2>{{ mapping?.Name }}</h2>
    <!-- <p>{{ mapping?.LocalPath }} → {{ mapping?.RemotePath }}</p> -->
    <n-tabs :value="tab" @update:value="onTabChange">
      <n-tab-pane name="files" :tab="t('mappings.browseFiles')">
        <p v-if="filesError">{{ filesError }}</p>
        <MappingFileTree
          :nodes="filesRoot"
          :selected="selected"
          @toggle="onToggle"
          @file="onFileClick"
          @menu="openMenu"
        />
      </n-tab-pane>
      <n-tab-pane name="changes" :tab="t('mappings.browseChanges')">
        <p v-if="changesError">{{ changesError }}</p>
        <p v-else-if="changesRoot.length === 0 && !loading">{{ t('mappings.noChanges') }}</p>
        <MappingFileTree
          v-else
          :nodes="changesRoot"
          :selected="selected"
          :dim-skipped-dirs="false"
          @toggle="onToggle"
          @file="onFileClick"
          @menu="openMenu"
        />
      </n-tab-pane>
    </n-tabs>
  </n-spin>
  <n-dropdown
    trigger="manual"
    placement="bottom-start"
    :show="menuShow"
    :x="menuX"
    :y="menuY"
    :options="menuItems"
    @select="onMenuSelect"
    @clickoutside="closeMenu"
  />
</template>
