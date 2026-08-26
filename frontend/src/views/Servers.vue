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
  NTag,
  useMessage,
  type DataTableColumns,
  type FormInst,
  type FormRules,
} from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { ListServers, UpsertServer, DeleteServer, TestServerConnection } from '../../wailsjs/go/main/App'
import { canSaveServerForm } from './serverForm'

interface ServerUI {
  id: string
  name: string
  host: string
  port: number
  user: string
  authType: string
  keyPath?: string
  hostKeyPolicy?: string
  hasPassword?: boolean
  password?: string
}

const message = useMessage()
const { t } = useI18n()
const rows = ref<ServerUI[]>([])
const show = ref(false)
const formRef = ref<FormInst | null>(null)
const form = reactive({
  id: '',
  name: '',
  host: '',
  port: 22 as number | null,
  user: '',
  authType: 'password' as 'password' | 'key',
  password: '',
  keyPath: '',
  keyPassphrase: '',
  hostKeyPolicy: 'accept-new',
})

const rules = computed<FormRules>(() => ({
  name: {
    required: true,
    trigger: ['input', 'blur'],
    validator: () => {
      if (!form.name.trim()) return new Error(t('servers.nameRequired'))
      return true
    },
  },
  host: {
    required: true,
    trigger: ['input', 'blur'],
    validator: () => {
      if (!form.host.trim()) return new Error(t('servers.hostRequired'))
      return true
    },
  },
  port: {
    required: true,
    type: 'number',
    trigger: ['change', 'blur'],
    validator: () => {
      if (form.port == null || form.port < 1 || form.port > 65535) {
        return new Error(t('servers.portRequired'))
      }
      return true
    },
  },
  user: {
    required: true,
    trigger: ['input', 'blur'],
    validator: () => {
      if (!form.user.trim()) return new Error(t('servers.userRequired'))
      return true
    },
  },
}))

const authOptions = computed(() => [
  { label: t('servers.password'), value: 'password' },
  { label: t('servers.key'), value: 'key' },
])

const columns = computed<DataTableColumns<ServerUI>>(() => [
  { title: t('common.name'), key: 'name' },
  { title: t('servers.host'), key: 'host' },
  { title: t('servers.port'), key: 'port', width: 80 },
  { title: t('servers.user'), key: 'user' },
  {
    title: t('servers.auth'),
    key: 'authType',
    render: (r) => h(NTag, { size: 'small', bordered: false }, { default: () => t(r.authType === 'key' ? 'servers.key' : 'servers.password') }),
  },
  {
    title: t('common.actions'),
    key: 'actions',
    render: (r) =>
      h(NSpace, { size: 8 }, {
        default: () => [
          h(NButton, { size: 'tiny', secondary: true, onClick: () => onTest(r.id) }, { default: () => t('servers.test') }),
          h(NButton, { size: 'tiny', secondary: true, onClick: () => onEdit(r) }, { default: () => t('common.edit') }),
          h(NButton, { size: 'tiny', secondary: true, type: 'error', onClick: () => onDelete(r.id) }, { default: () => t('common.delete') }),
        ],
      }),
  },
])

async function refresh() {
  rows.value = (await ListServers()) as ServerUI[]
}

function onCreate() {
  Object.assign(form, {
    id: crypto.randomUUID(),
    name: '',
    host: '',
    port: 22,
    user: '',
    authType: 'password',
    password: '',
    keyPath: '',
    keyPassphrase: '',
    hostKeyPolicy: 'accept-new',
  })
  show.value = true
}

function onEdit(r: ServerUI) {
  Object.assign(form, {
    id: r.id,
    name: r.name,
    host: r.host,
    port: r.port,
    user: r.user,
    authType: r.authType,
    password: r.hasPassword ? '********' : '',
    keyPath: r.keyPath || '',
    keyPassphrase: '',
    hostKeyPolicy: r.hostKeyPolicy || 'accept-new',
  })
  show.value = true
}

async function onSave() {
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  if (!canSaveServerForm({
    name: form.name,
    host: form.host,
    port: form.port,
    user: form.user,
  })) {
    return
  }
  try {
    await UpsertServer({
      ID: form.id,
      Name: form.name.trim(),
      Host: form.host.trim(),
      Port: form.port as number,
      User: form.user.trim(),
      AuthType: form.authType,
      Password: form.password,
      KeyPath: form.keyPath,
      KeyPassphrase: form.keyPassphrase,
      HostKeyPolicy: form.hostKeyPolicy,
    } as any)
    show.value = false
    await refresh()
    message.success(t('common.saved'))
  } catch (e: any) {
    message.error(String(e))
  }
}

async function onDelete(id: string) {
  try {
    await DeleteServer(id)
    await refresh()
    message.success(t('common.deleted'))
  } catch (e: any) {
    message.error(String(e))
  }
}

async function onTest(id: string) {
  try {
    const res = (await TestServerConnection(id)) as { ok: boolean; message: string }
    if (res.ok) message.success(t('servers.connected'))
    else message.error(res.message || t('servers.connectionFailed'))
  } catch (e: any) {
    message.error(String(e))
  }
}

onMounted(refresh)
</script>

<template>
  <n-card :title="t('servers.title')">
    <template #header-extra>
      <n-button type="primary" @click="onCreate">{{ t('common.add') }}</n-button>
    </template>
    <n-data-table :columns="columns" :data="rows" :bordered="false" />
  </n-card>

  <n-modal v-model:show="show" preset="card" :title="t('servers.dialogTitle')" style="width: 520px">
    <n-form ref="formRef" :model="form" :rules="rules" label-placement="left" label-width="90">
      <n-form-item :label="t('common.name')" path="name"><n-input v-model:value="form.name" /></n-form-item>
      <n-form-item :label="t('servers.host')" path="host"><n-input v-model:value="form.host" /></n-form-item>
      <n-form-item :label="t('servers.port')" path="port">
        <n-input-number v-model:value="form.port" :min="1" :max="65535" clearable />
      </n-form-item>
      <n-form-item :label="t('servers.user')" path="user"><n-input v-model:value="form.user" /></n-form-item>
      <n-form-item :label="t('servers.auth')">
        <n-select
          v-model:value="form.authType"
          :options="authOptions"
        />
      </n-form-item>
      <n-form-item v-if="form.authType === 'password'" :label="t('servers.password')">
        <n-input v-model:value="form.password" type="password" show-password-on="click" />
      </n-form-item>
      <template v-else>
        <n-form-item :label="t('servers.keyPath')"><n-input v-model:value="form.keyPath" /></n-form-item>
        <n-form-item :label="t('servers.keyPassphrase')">
          <n-input v-model:value="form.keyPassphrase" type="password" show-password-on="click" />
        </n-form-item>
      </template>
    </n-form>
    <template #footer>
      <n-space justify="end">
        <n-button @click="show = false">{{ t('common.cancel') }}</n-button>
        <n-button type="primary" @click="onSave">{{ t('common.save') }}</n-button>
      </n-space>
    </template>
  </n-modal>
</template>
