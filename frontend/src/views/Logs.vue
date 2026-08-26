<script lang="ts" setup>
import { onMounted, onUnmounted, ref } from 'vue'
import { NCard, NButton, NSpace, useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { OpenLogDirectory, RecentLogs } from '../../wailsjs/go/main/App'

const { t } = useI18n()
const message = useMessage()
const lines = ref<string[]>([])
const refreshing = ref(false)
let timer: number | undefined

async function refresh(showFeedback = false) {
  if (showFeedback) {
    refreshing.value = true
  }
  try {
    lines.value = (await RecentLogs()) || []
    if (showFeedback) {
      message.success(t('common.refreshed'))
    }
  } catch (error: any) {
    if (showFeedback) {
      message.error(String(error))
    }
  } finally {
    if (showFeedback) {
      refreshing.value = false
    }
  }
}

async function openLogDirectory() {
  try {
    await OpenLogDirectory()
  } catch (error: any) {
    message.error(`${t('logs.openFailed')}: ${String(error)}`)
  }
}

onMounted(() => {
  refresh()
  timer = window.setInterval(() => refresh(), 1000)
})
onUnmounted(() => {
  if (timer) clearInterval(timer)
})
</script>

<template>
  <n-card :title="t('logs.title')">
    <template #header-extra>
      <n-space>
        <n-button size="small" @click="openLogDirectory">{{ t('logs.openDirectory') }}</n-button>
        <n-button size="small" :loading="refreshing" @click="refresh(true)">{{ t('logs.refresh') }}</n-button>
      </n-space>
    </template>
    <pre class="log">{{ lines.join('\n') || t('logs.empty') }}</pre>
  </n-card>
</template>

<style scoped>
.log {
  margin: 0;
  max-height: calc(100vh - 160px);
  overflow: auto;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px;
  line-height: 1.45;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
