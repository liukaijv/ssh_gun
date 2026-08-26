<script lang="ts" setup>
import { computed, h, onMounted, reactive, ref } from 'vue'
import {
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
  useMessage,
  useDialog,
  type DataTableColumns,
  type FormInst,
  type FormRules,
} from 'naive-ui'
import { useI18n } from 'vue-i18n'
import {
  ListServers,
  ListPortForwards,
  UpsertPortForward,
  DeletePortForward,
  StartPortForward,
  StopPortForward,
  PortForwardStatus,
} from '../../wailsjs/go/main/App'
import { canSaveForwardForm, type ForwardType } from './forwardForm'

const message = useMessage()
const dialog = useDialog()
const { t } = useI18n()
const rows = ref<any[]>([])
const servers = ref<{ label: string; value: string }[]>([])
const statusMap = ref<Record<string, any>>({})
const show = ref(false)
const refreshing = ref(false)
const formRef = ref<FormInst | null>(null)
const form = reactive<{
  id: string
  serverId: string
  name: string
  type: ForwardType
  localAddr: string
  localPort: number | null
  remoteHost: string
  remotePort: number | null
  autoStart: boolean
}>({
  id: '',
  serverId: '',
  name: '',
  type: 'local',
  localAddr: '127.0.0.1',
  localPort: 3316,
  remoteHost: '127.0.0.1',
  remotePort: 3306,
  autoStart: false,
})

const typeOptions = computed(() => [
  { label: t('forwards.localForward'), value: 'local' },
  { label: t('forwards.remoteForward'), value: 'remote' },
  { label: t('forwards.dynamicForward'), value: 'dynamic' },
])

const isDynamic = computed(() => form.type === 'dynamic')
const localLabel = computed(() => t(form.type === 'remote' ? 'forwards.localTargetAddress' : 'forwards.localListenAddress'))
const localPortLabel = computed(() => t(form.type === 'remote' ? 'forwards.localTargetPort' : 'forwards.localListenPort'))
const remoteLabel = computed(() => t(form.type === 'remote' ? 'forwards.remoteListenAddress' : 'forwards.remoteHost'))
const remotePortLabel = computed(() => t(form.type === 'remote' ? 'forwards.remoteListenPort' : 'forwards.remotePort'))

const rules = computed<FormRules>(() => ({
  name: {
    required: true,
    trigger: ['input', 'blur'],
    validator: () => {
      if (!form.name.trim()) return new Error(t('forwards.nameRequired'))
      return true
    },
  },
  serverId: {
    required: true,
    trigger: ['change', 'blur'],
    validator: () => {
      if (!String(form.serverId || '').trim()) return new Error(t('forwards.serverRequired'))
      return true
    },
  },
  localAddr: {
    trigger: ['input', 'blur'],
    validator: () => {
      if (form.type === 'remote' && !form.localAddr.trim()) {
        return new Error(t('forwards.localAddrRequired'))
      }
      return true
    },
  },
  localPort: {
    required: true,
    type: 'number',
    trigger: ['change', 'blur'],
    validator: () => {
      if (form.localPort == null || form.localPort < 1 || form.localPort > 65535) {
        return new Error(t('forwards.localPortRequired'))
      }
      return true
    },
  },
  remoteHost: {
    trigger: ['input', 'blur'],
    validator: () => {
      if (form.type === 'local' && !form.remoteHost.trim()) {
        return new Error(t('forwards.remoteHostRequired'))
      }
      return true
    },
  },
  remotePort: {
    type: 'number',
    trigger: ['change', 'blur'],
    validator: () => {
      if (form.type === 'dynamic') return true
      if (form.remotePort == null || form.remotePort < 1 || form.remotePort > 65535) {
        return new Error(t('forwards.remotePortRequired'))
      }
      return true
    },
  },
}))

function typeLabel(type: string) {
  switch (type) {
    case 'remote':
      return t('forwards.remote')
    case 'dynamic':
      return t('forwards.dynamic')
    default:
      return t('forwards.local')
  }
}

const columns = computed<DataTableColumns<any>>(() => [
  { title: t('common.name'), key: 'Name' },
  {
    title: t('forwards.type'),
    key: 'Type',
    width: 72,
    render: (r) => typeLabel(r.Type || 'local'),
  },
  {
    title: t('forwards.localSide'),
    key: 'local',
    render: (r) => `${r.LocalAddr || '127.0.0.1'}:${r.LocalPort}`,
  },
  {
    title: t('forwards.remoteSide'),
    key: 'remote',
    render: (r) => {
      if ((r.Type || 'local') === 'dynamic') return 'SOCKS5'
      return `${r.RemoteHost || '127.0.0.1'}:${r.RemotePort}`
    },
  },
  {
    title: t('common.status'),
    key: 'status',
    render: (r) => {
      const st = statusMap.value[r.ID]
      const running = st?.Running
      return h(NTag, { size: 'small', type: running ? 'success' : 'default' }, { default: () => (running ? t('forwards.running', { count: st.ActiveConns || 0 }) : t('common.stopped')) })
    },
  },
  {
    title: t('common.actions'),
    key: 'actions',
    width: 260,
    render: (r) =>
      h(NSpace, { size: 6 }, {
        default: () => [
          h(NButton, { size: 'tiny', secondary: true, onClick: () => onStart(r.ID) }, { default: () => t('forwards.start') }),
          h(NButton, { size: 'tiny', secondary: true, onClick: () => onStop(r.ID) }, { default: () => t('forwards.stop') }),
          h(NButton, { size: 'tiny', secondary: true, onClick: () => onEdit(r) }, { default: () => t('common.edit') }),
          h(NButton, { size: 'tiny', secondary: true, type: 'error', onClick: () => onDelete(r) }, { default: () => t('common.delete') }),
        ],
      }),
  },
])

async function refresh(showFeedback = false) {
  if (showFeedback) {
    refreshing.value = true
  }
  try {
    rows.value = await ListPortForwards()
    const list = await ListServers()
    servers.value = list.map((s: any) => ({ label: s.name || s.Name, value: s.id || s.ID }))
    const map: Record<string, any> = {}
    for (const r of rows.value) {
      map[r.ID] = await PortForwardStatus(r.ID)
    }
    statusMap.value = map
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
    type: 'local',
    localAddr: '127.0.0.1',
    localPort: 3316,
    remoteHost: '127.0.0.1',
    remotePort: 3306,
    autoStart: false,
  })
  show.value = true
}

function onEdit(r: any) {
  Object.assign(form, {
    id: r.ID,
    serverId: r.ServerID,
    name: r.Name,
    type: (r.Type || 'local') as ForwardType,
    localAddr: r.LocalAddr || '127.0.0.1',
    localPort: r.LocalPort,
    remoteHost: r.RemoteHost || '127.0.0.1',
    remotePort: r.RemotePort || 3306,
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
  if (!canSaveForwardForm({
    name: form.name,
    serverId: form.serverId,
    type: form.type,
    localAddr: form.localAddr,
    localPort: form.localPort,
    remoteHost: form.remoteHost,
    remotePort: form.remotePort,
  })) {
    return
  }
  try {
    await UpsertPortForward({
      ID: form.id,
      ServerID: String(form.serverId).trim(),
      Name: form.name.trim(),
      Type: form.type,
      LocalAddr: form.localAddr.trim(),
      LocalPort: form.localPort as number,
      RemoteHost: form.remoteHost.trim(),
      RemotePort: isDynamic.value ? 0 : (form.remotePort as number),
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
    content: t('forwards.deleteConfirm', { name }),
    positiveText: t('common.deleteConfirmPositive'),
    negativeText: t('common.cancel'),
    onPositiveClick: async () => {
      try {
        await DeletePortForward(row.ID)
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
    await StartPortForward(id)
    message.success(t('forwards.started'))
    await refresh()
  } catch (e: any) {
    message.error(String(e))
  }
}

async function onStop(id: string) {
  try {
    await StopPortForward(id)
    await refresh()
  } catch (e: any) {
    message.error(String(e))
  }
}

onMounted(refresh)
</script>

<template>
  <n-card :title="t('forwards.title')">
    <template #header-extra>
      <n-space>
        <n-button :loading="refreshing" @click="refresh(true)">{{ t('common.refresh') }}</n-button>
        <n-button type="primary" @click="onCreate">{{ t('common.add') }}</n-button>
      </n-space>
    </template>
    <n-data-table :columns="columns" :data="rows" :bordered="false" />
  </n-card>

  <n-modal v-model:show="show" preset="card" :title="t('forwards.title')" style="width: 520px">
    <n-form ref="formRef" :model="form" :rules="rules" label-placement="left" label-width="110">
      <n-form-item :label="t('common.name')" path="name"><n-input v-model:value="form.name" /></n-form-item>
      <n-form-item :label="t('common.server')" path="serverId">
        <n-select v-model:value="form.serverId" :options="servers" clearable />
      </n-form-item>
      <n-form-item :label="t('forwards.type')"><n-select v-model:value="form.type" :options="typeOptions" /></n-form-item>
      <n-form-item :label="localLabel" path="localAddr"><n-input v-model:value="form.localAddr" /></n-form-item>
      <n-form-item :label="localPortLabel" path="localPort">
        <n-input-number v-model:value="form.localPort" :min="1" :max="65535" clearable />
      </n-form-item>
      <template v-if="!isDynamic">
        <n-form-item :label="remoteLabel" path="remoteHost"><n-input v-model:value="form.remoteHost" /></n-form-item>
        <n-form-item :label="remotePortLabel" path="remotePort">
          <n-input-number v-model:value="form.remotePort" :min="1" :max="65535" clearable />
        </n-form-item>
      </template>
      <n-form-item :label="t('forwards.autoStart')"><n-switch v-model:value="form.autoStart" /></n-form-item>
    </n-form>
    <template #footer>
      <n-space justify="end">
        <n-button @click="show = false">{{ t('common.cancel') }}</n-button>
        <n-button type="primary" @click="onSave">{{ t('common.save') }}</n-button>
      </n-space>
    </template>
  </n-modal>
</template>
