<script lang="ts" setup>
import {computed, h, onMounted, onUnmounted, reactive, ref} from 'vue'
import {
  type DataTableColumns,
  type FormInst,
  type FormRules,
  NButton,
  NCard,
  NDataTable,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NModal,
  NRadioButton,
  NRadioGroup,
  NSelect,
  NSpace,
  NSwitch,
  NTag,
  useDialog,
  useMessage,
} from 'naive-ui'
import {useI18n} from 'vue-i18n'
import {useRouter} from 'vue-router'
import {
  DeleteSyncMapping,
  ListServers,
  ListSyncMappings,
  RunFullSync,
  SelectLocalDirectory,
  UpsertSyncMapping,
} from '../../wailsjs/go/main/App'
import {EventsOn} from '../../wailsjs/runtime/runtime'
import {canSaveMappingForm} from './mappingForm'

const message = useMessage()
const dialog = useDialog()
const router = useRouter()
const {t} = useI18n()
const rows = ref<any[]>([])
const servers = ref<{ label: string; value: string }[]>([])
const show = ref(false)
const refreshing = ref(false)
const syncingIds = ref<Set<string>>(new Set())
const formRef = ref<FormInst | null>(null)
let offSyncStatus: (() => void) | undefined
const form = reactive<any>({
  id: '',
  serverId: '',
  name: '',
  localPath: '',
  remotePath: '',
  filterMode: 'exclude',
  excludesText: '*~\n*.swp\n*.tmp\n.git\n.idea\nnode_modules\n*.log',
  includesText: '',
  useGitignore: true,
  autoSync: false,
  enabled: true,
  deleteExtra: false,
  debounceMs: 500,
  backend: 'sftp',
})

const rules = computed<FormRules>(() => ({
  name: {
    required: true,
    trigger: ['input', 'blur'],
    validator: () => {
      if (!form.name.trim()) return new Error(t('mappings.nameRequired'))
      return true
    },
  },
  serverId: {
    required: true,
    trigger: ['change', 'blur'],
    validator: () => {
      if (!String(form.serverId || '').trim()) return new Error(t('mappings.serverRequired'))
      return true
    },
  },
  localPath: {
    required: true,
    trigger: ['input', 'blur', 'change'],
    validator: () => {
      if (!form.localPath.trim()) return new Error(t('mappings.localRequired'))
      return true
    },
  },
  remotePath: {
    required: true,
    trigger: ['input', 'blur'],
    validator: () => {
      if (!form.remotePath.trim()) return new Error(t('mappings.remoteRequired'))
      return true
    },
  },
}))

function splitRules(text: string): string[] {
  return String(text || '')
      .split(/\r?\n/)
      .map((s: string) => s.trim())
      .filter(Boolean)
}

const columns = computed<DataTableColumns<any>>(() => [
  {title: t('common.name'), key: 'Name'},
  {
    title: t('mappings.filterMode'),
    key: 'FilterMode',
    width: 80,
    render: (r) => {
      const include = (r.FilterMode || r.filterMode) === 'include'
      return h(NTag, {
        size: 'small',
        type: include ? 'info' : 'default',
      }, {default: () => t(include ? 'mappings.filterInclude' : 'mappings.filterExclude')})
    },
  },
  {title: t('mappings.local'), key: 'LocalPath', ellipsis: {tooltip: true}},
  {title: t('mappings.remote'), key: 'RemotePath', ellipsis: {tooltip: true}},
  {
    title: t('mappings.automatic'),
    key: 'AutoSync',
    width: 70,
    render: (r) => h(NTag, {
      size: 'small',
      type: r.AutoSync ? 'success' : 'default'
    }, {default: () => t(r.AutoSync ? 'common.on' : 'common.off')}),
  },
  {
    title: t('common.status'),
    key: 'Enabled',
    width: 70,
    render: (r) => h(NTag, {
      size: 'small',
      type: r.Enabled ? 'success' : 'default'
    }, {default: () => t(r.Enabled ? 'common.on' : 'common.off')}),
  },
  {
    title: t('mappings.lastSync'),
    key: 'LastSyncResult',
    render: (r) => r.LastSyncResult || '-',
  },
  {
    title: t('common.actions'),
    key: 'actions',
    width: 280,
    render: (r) => {
      const syncing = syncingIds.value.has(r.ID)
      return h(NSpace, {size: 6}, {
        default: () => [
          h(NButton, {
            size: 'tiny',
            secondary: true,
            onClick: () => router.push({name: 'mapping-files', params: {id: r.ID}}),
          }, {default: () => t('mappings.browse')}),
          h(NButton, {
            size: 'tiny',
            secondary: true,
            loading: syncing,
            disabled: !r.Enabled || syncing,
            onClick: () => onSync(r),
          }, {default: () => t('mappings.sync')}),
            h(NButton, {size: 'tiny', secondary: true, onClick: () => onEdit(r)}, {default: () => t('common.edit')}),
            h(NButton, {
              size: 'tiny',
              secondary: true,
              type: 'error',
              onClick: () => onDelete(r)
            }, {default: () => t('common.delete')}),
          ], 
        })
    },
  },
])

async function refresh(showFeedback = false) {
  if (showFeedback) {
    refreshing.value = true
  }
  try {
    rows.value = await ListSyncMappings()
    const list = await ListServers()
    servers.value = list.map((s: any) => ({label: s.name || s.Name, value: s.id || s.ID}))
    if (showFeedback) {
      message.success(t('common.refreshed'))
    }
  } catch (e: any) {
    if (showFeedback) {
      message.error(String(e))
    } else {
      throw e
    }
  } finally {
    if (showFeedback) {
      refreshing.value = false
    }
  }
}

function onCreate() {
  Object.assign(form, {
    id: crypto.randomUUID(),
    serverId: servers.value[0]?.value || '',
    name: '',
    localPath: '',
    remotePath: '',
    filterMode: 'exclude',
    excludesText: '*.tmp\n.git\n.idea\nnode_modules\n*.log',
    includesText: '',
    useGitignore: true,
    autoSync: false,
    enabled: true,
    deleteExtra: false,
    debounceMs: 500,
    backend: 'sftp',
  })
  show.value = true
}

function onEdit(r: any) {
  Object.assign(form, {
    id: r.ID,
    serverId: r.ServerID,
    name: r.Name,
    localPath: r.LocalPath,
    remotePath: r.RemotePath,
    filterMode: (r.FilterMode || r.filterMode) === 'include' ? 'include' : 'exclude',
    excludesText: (r.Excludes || []).join('\n'),
    includesText: (r.Includes || r.includes || []).join('\n'),
    useGitignore: r.UseGitignore === true,
    autoSync: r.AutoSync,
    enabled: r.Enabled,
    deleteExtra: r.DeleteExtra,
    debounceMs: r.DebounceMs || 500,
    backend: r.Backend || 'sftp',
  })
  show.value = true
}

async function onSave() {
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  if (!canSaveMappingForm({
    name: form.name,
    serverId: form.serverId,
    localPath: form.localPath,
    remotePath: form.remotePath,
  })) {
    return
  }
  try {
    await UpsertSyncMapping({
      ID: form.id,
      ServerID: String(form.serverId).trim(),
      Name: form.name.trim(),
      LocalPath: form.localPath.trim(),
      RemotePath: form.remotePath.trim(),
      FilterMode: form.filterMode === 'include' ? 'include' : 'exclude',
      Excludes: splitRules(form.excludesText),
      Includes: splitRules(form.includesText),
      UseGitignore: !!form.useGitignore,
      AutoSync: form.autoSync,
      Enabled: form.enabled,
      DeleteExtra: form.deleteExtra,
      DebounceMs: form.debounceMs,
      Backend: form.backend,
    } as any)
    show.value = false
    await refresh()
    message.success(t('common.saved'))
  } catch (e: any) {
    message.error(String(e))
  }
}

function onDelete(row: any) {
  const name = row.Name || row.name || row.ID
  dialog.warning({
    title: t('common.deleteConfirmTitle'),
    content: t('mappings.deleteConfirm', { name }),
    positiveText: t('common.deleteConfirmPositive'),
    negativeText: t('common.cancel'),
    onPositiveClick: async () => {
      try {
        await DeleteSyncMapping(row.ID)
        await refresh()
        message.success(t('common.deleted'))
      } catch (e: any) {
        message.error(String(e))
      }
    },
  })
}

function onSync(row: any) {
  const id = row.ID
  if (!row.Enabled) {
    message.error(t('mappings.syncDisabled'))
    return
  }
  if (syncingIds.value.has(id)) {
    message.warning(t('mappings.syncInProgress'))
    return
  }
  const name = row.Name || row.name || id
  dialog.warning({
    title: t('mappings.syncConfirmTitle'),
    content: t('mappings.syncConfirm', { name }),
    positiveText: t('mappings.sync'),
    negativeText: t('common.cancel'),
    onPositiveClick: () => {
      void runFullSync(id)
    },
  })
}

async function runFullSync(id: string) {
  if (syncingIds.value.has(id)) {
    message.warning(t('mappings.syncInProgress'))
    return
  }
  syncingIds.value = new Set(syncingIds.value).add(id)
  // Defer "started" toast so an immediate busy response only shows syncInProgress.
  const startedToast = setTimeout(() => {
    message.info(t('mappings.syncStarted'))
  }, 400)
  try {
    const res = await RunFullSync(id)
    clearTimeout(startedToast)
    if ((res as any).result === 'ok') {
      message.success(t('mappings.syncDone', {count: (res as any).files ?? 0}))
    } else if ((res as any).result === 'busy') {
      message.warning(t('mappings.syncInProgress'))
    } else {
      message.error((res as any).error || t('mappings.syncFailed'))
    }
    await refresh()
  } catch (e: any) {
    clearTimeout(startedToast)
    message.error(String(e))
  } finally {
    clearTimeout(startedToast)
    const next = new Set(syncingIds.value)
    next.delete(id)
    syncingIds.value = next
  }
}

async function onBrowseLocal() {
  try {
    const path = await SelectLocalDirectory(form.localPath || '')
    if (path) {
      form.localPath = path
      await formRef.value?.validate(undefined, ['localPath']).catch(() => {})
    }
  } catch (e: any) {
    message.error(String(e))
  }
}

onMounted(() => {
  void refresh()
  offSyncStatus = EventsOn('sync:status', (payload: any) => {
    const id = payload?.id
    if (!id) return
    const idx = rows.value.findIndex((r: any) => r.ID === id)
    if (idx < 0) return
    const next = rows.value.slice()
    next[idx] = {
      ...next[idx],
      LastSyncResult: payload.result ?? next[idx].LastSyncResult,
      LastSyncError: payload.error ?? '',
      LastSyncAt: payload.at ?? next[idx].LastSyncAt,
    }
    rows.value = next
  })
})

onUnmounted(() => {
  offSyncStatus?.()
  offSyncStatus = undefined
})
</script>

<template>
  <n-card :title="t('mappings.title')">
    <template #header-extra>
      <n-space>
        <n-button :loading="refreshing" @click="refresh(true)">{{ t('common.refresh') }}</n-button>
        <n-button type="primary" @click="onCreate">{{ t('common.add') }}</n-button>
      </n-space>
    </template>
    <n-data-table :columns="columns" :data="rows" :bordered="false"/>
  </n-card>

  <n-modal v-model:show="show" preset="card" :title="t('mappings.title')" style="width: 560px">
    <n-form ref="formRef" :model="form" :rules="rules" label-placement="left" label-width="100">
      <n-form-item :label="t('common.name')" path="name">
        <n-input v-model:value="form.name"/>
      </n-form-item>
      <n-form-item :label="t('common.server')" path="serverId">
        <n-select v-model:value="form.serverId" :options="servers" clearable/>
      </n-form-item>
      <n-form-item :label="t('mappings.localDirectory')" path="localPath">
        <n-space :wrap="false" style="width: 100%">
          <n-input v-model:value="form.localPath" placeholder="D:\proj" style="flex: 1"/>
          <n-button @click="onBrowseLocal">{{ t('common.browse') }}</n-button>
        </n-space>
      </n-form-item>
      <n-form-item :label="t('mappings.remoteDirectory')" path="remotePath">
        <n-input v-model:value="form.remotePath" placeholder="/opt/proj"/>
      </n-form-item>
      <n-form-item :label="t('mappings.filterMode')">
        <n-radio-group v-model:value="form.filterMode" name="filterMode">
          <n-radio-button value="exclude">{{ t('mappings.filterExclude') }}</n-radio-button>
          <n-radio-button value="include">{{ t('mappings.filterInclude') }}</n-radio-button>
        </n-radio-group>
      </n-form-item>
      <n-form-item v-if="form.filterMode !== 'include'" :label="t('mappings.excludes')">
        <n-space vertical :size="4" style="width: 100%">
          <n-input v-model:value="form.excludesText" type="textarea" :rows="4"/>
          <span style="font-size: 12px; opacity: 0.7">{{ t('mappings.excludesHint') }}</span>
        </n-space>
      </n-form-item>
      <n-form-item v-else :label="t('mappings.includes')">
        <n-space vertical :size="4" style="width: 100%">
          <n-input v-model:value="form.includesText" type="textarea" :rows="4"/>
          <span style="font-size: 12px; opacity: 0.7">{{ t('mappings.includesHint') }}</span>
        </n-space>
      </n-form-item>
      <n-form-item :label="t('mappings.useGitignore')">
        <n-space vertical :size="4">
          <n-switch v-model:value="form.useGitignore"/>
          <span style="font-size: 12px; opacity: 0.7">{{ t('mappings.useGitignoreHint') }}</span>
        </n-space>
      </n-form-item>
      <n-form-item :label="t('mappings.debounce')">
        <n-input-number v-model:value="form.debounceMs" :min="100"/>
      </n-form-item>
      <n-form-item :label="t('mappings.autoSync')">
        <n-switch v-model:value="form.autoSync"/>
      </n-form-item>
      <n-form-item :label="t('mappings.deleteExtra')">
        <n-switch v-model:value="form.deleteExtra"/>
      </n-form-item>
      <n-form-item :label="t('mappings.enabled')">
        <n-switch v-model:value="form.enabled"/>
      </n-form-item>
    </n-form>
    <template #footer>
      <n-space justify="end">
        <n-button @click="show = false">{{ t('common.cancel') }}</n-button>
        <n-button type="primary" @click="onSave">{{ t('common.save') }}</n-button>
      </n-space>
    </template>
  </n-modal>
</template>
