<script lang="ts" setup>
import {computed, h, onMounted, reactive, ref} from 'vue'
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
  NSelect,
  NSpace,
  NSwitch,
  NTag,
  useDialog,
  useMessage,
} from 'naive-ui'
import {useI18n} from 'vue-i18n'
import {
  DeleteSyncMapping,
  ListServers,
  ListSyncMappings,
  RunFullSync,
  SelectLocalDirectory,
  UpsertSyncMapping,
} from '../../wailsjs/go/main/App'
import {canSaveMappingForm} from './mappingForm'

const message = useMessage()
const dialog = useDialog()
const {t} = useI18n()
const rows = ref<any[]>([])
const servers = ref<{ label: string; value: string }[]>([])
const show = ref(false)
const refreshing = ref(false)
const formRef = ref<FormInst | null>(null)
const form = reactive<any>({
  id: '',
  serverId: '',
  name: '',
  localPath: '',
  remotePath: '',
  excludesText: '*~\n*.swp\n*.tmp\n.git\n.idea\nnode_modules\n*.log',
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

const columns = computed<DataTableColumns<any>>(() => [
  {title: t('common.name'), key: 'Name'},
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
    width: 220,
    render: (r) =>
        h(NSpace, {size: 6}, {
          default: () => [
            h(NButton, {
              size: 'tiny',
              secondary: true,
              disabled: !r.Enabled,
              onClick: () => onSync(r.ID),
            }, {default: () => t('mappings.sync')}),
            h(NButton, {size: 'tiny', secondary: true, onClick: () => onEdit(r)}, {default: () => t('common.edit')}),
            h(NButton, {
              size: 'tiny',
              secondary: true,
              type: 'error',
              onClick: () => onDelete(r)
            }, {default: () => t('common.delete')}),
          ],
        }),
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
    excludesText: '*.tmp\n.git\n.idea\nnode_modules\n*.log',
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
    excludesText: (r.Excludes || []).join('\n'),
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
      Excludes: String(form.excludesText)
          .split(/\r?\n/)
          .map((s: string) => s.trim())
          .filter(Boolean),
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

async function onSync(id: string) {
  const row = rows.value.find((r) => r.ID === id)
  if (row && !row.Enabled) {
    message.error(t('mappings.syncDisabled'))
    return
  }
  try {
    const res = await RunFullSync(id)
    if ((res as any).result === 'ok') message.success(t('mappings.syncDone', {count: (res as any).files ?? 0}))
    else message.error((res as any).error || t('mappings.syncFailed'))
    await refresh()
  } catch (e: any) {
    message.error(String(e))
  }
}

async function onBrowseLocal() {
  try {
    const path = await SelectLocalDirectory(form.localPath || '')
    if (path) {
      form.localPath = path
    }
  } catch (e: any) {
    message.error(String(e))
  }
}

onMounted(refresh)
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
      <n-form-item :label="t('mappings.excludes')">
        <n-space vertical :size="4" style="width: 100%">
          <n-input v-model:value="form.excludesText" type="textarea" :rows="4"/>
          <span style="font-size: 12px; opacity: 0.7">{{ t('mappings.excludesHint') }}</span>
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
