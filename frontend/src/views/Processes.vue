<script lang="ts" setup>
import { computed, h, onMounted, onUnmounted, reactive, ref } from 'vue'
import {
  NButton,
  NCard,
  NDataTable,
  NForm,
  NFormItem,
  NInput,
  NModal,
  NSpace,
  NSwitch,
  NTag,
  useMessage,
  useDialog,
  type DataTableColumns,
  type FormInst,
  type FormRules,
} from 'naive-ui'
import { useI18n } from 'vue-i18n'
import {
  ListManagedProcesses,
  UpsertManagedProcess,
  DeleteManagedProcess,
  StartManagedProcess,
  StopManagedProcess,
  ManagedProcessStatus,
  SelectLocalDirectory,
} from '../../wailsjs/go/main/App'
import { canSaveProcessForm, formatProcessEnv, parseProcessEnvText } from './processForm'

function formatUptime(sec: number): string {
  const s = Math.max(0, Math.floor(sec || 0))
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const r = s % 60
  if (h > 0) return `${h}h ${m}m`
  if (m > 0) return `${m}m ${r}s`
  return `${r}s`
}

const message = useMessage()
const dialog = useDialog()
const { t } = useI18n()
const rows = ref<any[]>([])
const statusMap = ref<Record<string, any>>({})
const show = ref(false)
const refreshing = ref(false)
const formRef = ref<FormInst | null>(null)
const form = reactive({
  id: '',
  name: '',
  command: '',
  args: '',
  workDir: '',
  envText: '',
  enabled: true,
  autoStart: false,
})

const rules = computed<FormRules>(() => ({
  name: {
    required: true,
    trigger: ['input', 'blur'],
    validator: () => {
      if (!form.name.trim()) return new Error(t('processes.nameRequired'))
      return true
    },
  },
  command: {
    required: true,
    trigger: ['input', 'blur'],
    validator: () => {
      if (!form.command.trim()) return new Error(t('processes.commandRequired'))
      return true
    },
  },
}))

const columns = computed<DataTableColumns<any>>(() => [
  { title: t('common.name'), key: 'Name' },
  { title: t('processes.command'), key: 'Command', ellipsis: { tooltip: true } },
  {
    title: t('processes.args'),
    key: 'Args',
    ellipsis: { tooltip: true },
    render: (r) => r.Args || '-',
  },
  {
    title: t('processes.enabled'),
    key: 'Enabled',
    width: 70,
    render: (r) => h(NTag, {
      size: 'small',
      type: r.Enabled ? 'success' : 'default',
    }, { default: () => t(r.Enabled ? 'common.on' : 'common.off') }),
  },
  {
    title: t('common.status'),
    key: 'status',
    width: 180,
    render: (r) => {
      const st = statusMap.value[r.ID] || {}
      const running = !!st.Running
      return h(NTag, {
        size: 'small',
        type: running ? 'success' : 'default',
      }, {
        default: () => (running
          ? t('processes.running', { pid: st.PID || 0, uptime: formatUptime(st.UptimeSec || 0) })
          : t('common.stopped')),
      })
    },
  },
  {
    title: t('common.actions'),
    key: 'actions',
    width: 260,
    render: (r) =>
      h(NSpace, { size: 6 }, {
        default: () => [
          h(NButton, {
            size: 'tiny',
            secondary: true,
            disabled: !r.Enabled,
            onClick: () => onStart(r.ID),
          }, { default: () => t('processes.start') }),
          h(NButton, { size: 'tiny', secondary: true, onClick: () => onStop(r.ID) }, { default: () => t('processes.stop') }),
          h(NButton, { size: 'tiny', secondary: true, onClick: () => onEdit(r) }, { default: () => t('common.edit') }),
          h(NButton, {
            size: 'tiny',
            secondary: true,
            type: 'error',
            onClick: () => onDelete(r),
          }, { default: () => t('common.delete') }),
        ],
      }),
  },
])

async function refresh(showFeedback = false) {
  if (showFeedback) {
    refreshing.value = true
  }
  try {
    rows.value = await ListManagedProcesses()
    await refreshStatuses()
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

async function refreshStatuses() {
  const map: Record<string, any> = {}
  for (const r of rows.value) {
    map[r.ID] = await ManagedProcessStatus(r.ID)
  }
  statusMap.value = map
}

function onCreate() {
  Object.assign(form, {
    id: crypto.randomUUID(),
    name: '',
    command: '',
    args: '',
    workDir: '',
    envText: '',
    enabled: true,
    autoStart: false,
  })
  show.value = true
}

function onEdit(r: any) {
  Object.assign(form, {
    id: r.ID,
    name: r.Name,
    command: r.Command,
    args: r.Args || '',
    workDir: r.WorkDir || '',
    envText: formatProcessEnv(r.Env),
    enabled: r.Enabled,
    autoStart: r.AutoStart,
  })
  show.value = true
}

async function onSave() {
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  if (!canSaveProcessForm({ name: form.name, command: form.command })) {
    return
  }
  const parsed = parseProcessEnvText(form.envText)
  if (parsed.error) {
    message.error(t('processes.envInvalid', { detail: parsed.error }))
    return
  }
  try {
    await UpsertManagedProcess({
      ID: form.id,
      Name: form.name.trim(),
      Command: form.command.trim(),
      Args: form.args.trim(),
      WorkDir: form.workDir.trim(),
      Env: parsed.env,
      Enabled: form.enabled,
      AutoStart: form.autoStart,
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
    content: t('processes.deleteConfirm', { name }),
    positiveText: t('common.deleteConfirmPositive'),
    negativeText: t('common.cancel'),
    onPositiveClick: async () => {
      try {
        await DeleteManagedProcess(row.ID)
        await refresh()
        message.success(t('common.deleted'))
      } catch (e: any) {
        message.error(String(e))
      }
    },
  })
}

async function onStart(id: string) {
  try {
    await StartManagedProcess(id)
    message.success(t('processes.started'))
    await refresh()
  } catch (e: any) {
    message.error(String(e))
  }
}

async function onStop(id: string) {
  try {
    await StopManagedProcess(id)
    message.success(t('processes.stopped'))
    await refresh()
  } catch (e: any) {
    message.error(String(e))
  }
}

async function onBrowseWorkDir() {
  try {
    const path = await SelectLocalDirectory(form.workDir || '')
    if (path) {
      form.workDir = path
    }
  } catch (e: any) {
    message.error(String(e))
  }
}

let statusTimer: ReturnType<typeof setInterval> | undefined

onMounted(async () => {
  await refresh()
  statusTimer = setInterval(() => {
    void refreshStatuses()
  }, 1000)
})

onUnmounted(() => {
  if (statusTimer) {
    clearInterval(statusTimer)
    statusTimer = undefined
  }
})
</script>

<template>
  <n-card :title="t('processes.title')">
    <template #header-extra>
      <n-space>
        <n-button :loading="refreshing" @click="refresh(true)">{{ t('common.refresh') }}</n-button>
        <n-button type="primary" @click="onCreate">{{ t('common.add') }}</n-button>
      </n-space>
    </template>
    <n-data-table :columns="columns" :data="rows" :bordered="false" />
  </n-card>

  <n-modal v-model:show="show" preset="card" :title="t('processes.dialogTitle')" style="width: 560px">
    <n-form ref="formRef" :model="form" :rules="rules" label-placement="left" label-width="100">
      <n-form-item :label="t('common.name')" path="name"><n-input v-model:value="form.name" /></n-form-item>
      <n-form-item :label="t('processes.command')" path="command">
        <n-input v-model:value="form.command" :placeholder="t('processes.commandPlaceholder')" />
      </n-form-item>
      <n-form-item :label="t('processes.args')">
        <n-input v-model:value="form.args" :placeholder="t('processes.argsPlaceholder')" />
      </n-form-item>
      <n-form-item :label="t('processes.workDir')">
        <n-space style="width: 100%">
          <n-input v-model:value="form.workDir" style="flex: 1" :placeholder="t('processes.workDirPlaceholder')" />
          <n-button @click="onBrowseWorkDir">{{ t('common.browse') }}</n-button>
        </n-space>
      </n-form-item>
      <n-form-item :label="t('processes.env')">
        <n-input
          v-model:value="form.envText"
          type="textarea"
          :rows="4"
          :placeholder="t('processes.envPlaceholder')"
        />
      </n-form-item>
      <n-form-item :label="t('processes.enabled')"><n-switch v-model:value="form.enabled" /></n-form-item>
      <n-form-item :label="t('processes.autoStart')"><n-switch v-model:value="form.autoStart" /></n-form-item>
    </n-form>
    <template #footer>
      <n-space justify="end">
        <n-button @click="show = false">{{ t('common.cancel') }}</n-button>
        <n-button type="primary" @click="onSave">{{ t('common.save') }}</n-button>
      </n-space>
    </template>
  </n-modal>
</template>
