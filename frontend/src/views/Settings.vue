<script lang="ts" setup>
import { onMounted, ref } from 'vue'
import {
  NButton,
  NCard,
  NForm,
  NFormItem,
  NInput,
  NRadio,
  NRadioGroup,
  NSpace,
  useMessage,
} from 'naive-ui'
import { useI18n } from 'vue-i18n'
import {
  ExportConfig,
  GetSyncBackend,
  ImportConfig,
  OpenConfigDirectory,
  SetSyncBackend,
} from '../../wailsjs/go/main/App'
import { useLocaleStore } from '@/stores/locale'
import type { SupportedLocale } from '@/i18n'
import { useThemeStore, type ThemeMode } from '@/stores/theme'

const message = useMessage()
const { t } = useI18n()
const localeStore = useLocaleStore()
const themeStore = useThemeStore()

const exportPass = ref('')
const importPass = ref('')
const importMode = ref<'merge' | 'replace'>('merge')
const exportBusy = ref(false)
const importBusy = ref(false)
const syncBackend = ref<'sftp' | 'rsync'>('sftp')
const syncBackendBusy = ref(false)

async function loadSyncBackend() {
  const backend = await GetSyncBackend()
  syncBackend.value = backend === 'rsync' ? 'rsync' : 'sftp'
}

async function onSaveSyncBackend() {
  syncBackendBusy.value = true
  try {
    await SetSyncBackend(syncBackend.value)
    message.success(t('settings.syncBackendSaved'))
  } catch (e: any) {
    message.error(String(e))
  } finally {
    syncBackendBusy.value = false
  }
}

async function onThemeChange(value: ThemeMode) {
  try {
    await themeStore.setMode(value)
  } catch (e: any) {
    message.error(String(e))
  }
}

async function onLocaleChange(value: SupportedLocale) {
  try {
    await localeStore.setLocale(value)
    message.success(t('settings.languageSaved'))
  } catch (e: any) {
    message.error(String(e))
  }
}

async function onExport() {
  exportBusy.value = true
  try {
    const path = await ExportConfig(exportPass.value)
    if (!path) return
    message.success(t(exportPass.value ? 'settings.exportedEncrypted' : 'settings.exportedOmitted'))
  } catch (e: any) {
    message.error(String(e))
  } finally {
    exportBusy.value = false
  }
}

async function openConfigDirectory() {
  try {
    await OpenConfigDirectory()
  } catch (error: any) {
    message.error(`${t('settings.openConfigDirectoryFailed')}: ${String(error)}`)
  }
}

async function onImport() {
  if (importMode.value === 'replace') {
    const ok = window.confirm(t('settings.replaceConfirm'))
    if (!ok) return
  }
  importBusy.value = true
  try {
    const res: any = await ImportConfig(importPass.value, importMode.value)
    if (res?.cancelled) return

    const parts = [
      t('settings.summaryServers', { added: res.serversAdded || 0, updated: res.serversUpdated || 0 }),
      t('settings.summaryMappings', { added: res.mappingsAdded || 0, updated: res.mappingsUpdated || 0 }),
      t('settings.summaryForwards', { added: res.forwardsAdded || 0, updated: res.forwardsUpdated || 0 }),
    ]
    if (res.secretsMode === 'omitted') {
      parts.push(t('settings.secretsOmitted'))
    }
    message.success(t('settings.importSummary', { summary: parts.join(', ') }))
    if (Array.isArray(res.missingServerRefs) && res.missingServerRefs.length) {
      message.warning(res.missingServerRefs.slice(0, 3).join('；'))
    }
    if (importMode.value === 'replace') {
      await themeStore.initialize()
      await localeStore.initialize()
      await loadSyncBackend()
    }
  } catch (e: any) {
    message.error(String(e))
  } finally {
    importBusy.value = false
  }
}

onMounted(async () => {
  try {
    await loadSyncBackend()
  } catch (e: any) {
    message.error(String(e))
  }
})
</script>

<template>
  <n-space vertical :size="16">
    <n-card :title="t('settings.appearance')">
      <n-form label-placement="left" label-width="100" style="max-width: 560px">
        <n-form-item :label="t('settings.theme')">
          <n-radio-group
            :value="themeStore.mode"
            @update:value="onThemeChange"
          >
            <n-space>
              <n-radio value="system">{{ t('settings.system') }}</n-radio>
              <n-radio value="light">{{ t('settings.light') }}</n-radio>
              <n-radio value="dark">{{ t('settings.dark') }}</n-radio>
            </n-space>
          </n-radio-group>
        </n-form-item>
        <n-form-item :label="t('settings.language')">
          <n-radio-group
            :value="localeStore.locale"
            @update:value="onLocaleChange"
          >
            <n-space>
              <n-radio value="zh-CN">{{ t('settings.chinese') }}</n-radio>
              <n-radio value="en-US">{{ t('settings.english') }}</n-radio>
            </n-space>
          </n-radio-group>
        </n-form-item>
        <n-form-item label=" ">
          <span class="setting-hint">{{ t('settings.trayHint') }}</span>
        </n-form-item>
      </n-form>
    </n-card>

    <n-card :title="t('settings.syncSettings')">
      <n-form label-placement="left" label-width="100" style="max-width: 560px">
        <n-form-item :label="t('settings.syncBackend')">
          <n-radio-group v-model:value="syncBackend">
            <n-space>
              <n-radio value="sftp">SFTP</n-radio>
              <n-radio value="rsync">rsync</n-radio>
            </n-space>
          </n-radio-group>
        </n-form-item>
        <n-form-item label=" ">
          <n-space vertical>
            <n-button type="primary" :loading="syncBackendBusy" @click="onSaveSyncBackend">
              {{ t('common.save') }}
            </n-button>
            <span class="setting-hint">{{ t('settings.syncBackendRestartHint') }}</span>
          </n-space>
        </n-form-item>
      </n-form>
    </n-card>

    <n-card :title="t('settings.configTransfer')">
      <template #header-extra>
        <n-button size="small" @click="openConfigDirectory">
          {{ t('settings.openConfigDirectory') }}
        </n-button>
      </template>
      <n-form label-placement="left" label-width="100" style="max-width: 560px">
        <n-form-item :label="t('settings.exportPassphrase')">
          <n-input
            v-model:value="exportPass"
            type="password"
            show-password-on="click"
            :placeholder="t('settings.exportPlaceholder')"
          />
        </n-form-item>
        <n-form-item label=" ">
          <n-button type="primary" :loading="exportBusy" @click="onExport">{{ t('settings.export') }}</n-button>
        </n-form-item>

        <n-form-item :label="t('settings.importMode')">
          <n-radio-group v-model:value="importMode">
            <n-space>
              <n-radio value="merge">{{ t('settings.mergeById') }}</n-radio>
              <n-radio value="replace">{{ t('settings.replaceAll') }}</n-radio>
            </n-space>
          </n-radio-group>
        </n-form-item>
        <n-form-item :label="t('settings.importPassphrase')">
          <n-input
            v-model:value="importPass"
            type="password"
            show-password-on="click"
            :placeholder="t('settings.importPlaceholder')"
          />
        </n-form-item>
        <n-form-item label=" ">
          <n-button type="primary" secondary :loading="importBusy" @click="onImport">{{ t('settings.import') }}</n-button>
        </n-form-item>
      </n-form>
    </n-card>
  </n-space>
</template>

<style scoped>
.setting-hint {
  color: var(--n-text-color-3);
  font-size: 12px;
}
</style>
