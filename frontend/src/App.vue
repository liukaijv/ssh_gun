<script lang="ts" setup>
import { computed, h, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  NConfigProvider,
  NIcon,
  NLayout,
  NLayoutSider,
  NLayoutContent,
  NMenu,
  NMessageProvider,
  NDialogProvider,
  darkTheme,
  type MenuOption,
} from 'naive-ui'
import {
  DocumentTextOutline,
  FolderOpenOutline,
  GitNetworkOutline,
  ServerOutline,
  SettingsOutline,
} from '@vicons/ionicons5'
import { useI18n } from 'vue-i18n'
import { useLocaleStore } from '@/stores/locale'
import { useThemeStore } from '@/stores/theme'

const route = useRoute()
const router = useRouter()
const { t, locale } = useI18n()
const localeStore = useLocaleStore()
const themeStore = useThemeStore()

const activeKey = computed(() => String(route.name ?? 'servers'))
const naiveTheme = computed(() => (themeStore.effective === 'dark' ? darkTheme : null))

function renderIcon(icon: typeof ServerOutline) {
  return () => h(NIcon, null, { default: () => h(icon) })
}

const menuOptions = computed<MenuOption[]>(() => {
  void locale.value
  void localeStore.locale
  return [
    { label: t('nav.servers'), key: 'servers', icon: renderIcon(ServerOutline) },
    { label: t('nav.mappings'), key: 'mappings', icon: renderIcon(FolderOpenOutline) },
    { label: t('nav.forwards'), key: 'forwards', icon: renderIcon(GitNetworkOutline) },
    { label: t('nav.logs'), key: 'logs', icon: renderIcon(DocumentTextOutline) },
    { label: t('nav.settings'), key: 'settings', icon: renderIcon(SettingsOutline) },
  ]
})

function onUpdate(key: string) {
  router.push({ name: key })
}

watch(
  () => themeStore.effective,
  (value) => {
    document.documentElement.dataset.theme = value
  },
  { immediate: true },
)

watch(
  () => localeStore.locale,
  (value) => {
    if (locale.value !== value) {
      locale.value = value
    }
  },
)

onMounted(() => {
  themeStore.initialize()
  localeStore.initialize()
})
</script>

<template>
  <n-config-provider :theme="naiveTheme">
    <n-message-provider>
      <n-dialog-provider>
        <n-layout has-sider style="height: 100vh">
          <n-layout-sider bordered collapse-mode="width" :collapsed-width="64" :width="180" show-trigger>
            <n-menu :value="activeKey" :collapsed-width="64" :options="menuOptions" @update:value="onUpdate" />
          </n-layout-sider>
          <n-layout-content content-style="padding: 16px;">
            <router-view :key="localeStore.locale" />
          </n-layout-content>
        </n-layout>
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>

<style>
html,
body,
#app {
  margin: 0;
  height: 100%;
  font-family: Inter, system-ui, sans-serif;
}
html[data-theme='dark'],
html[data-theme='dark'] body,
html[data-theme='dark'] #app {
  background: #101014;
  color: #e5e5e5;
}
html[data-theme='light'],
html[data-theme='light'] body,
html[data-theme='light'] #app {
  background: #f5f5f7;
  color: #1f1f1f;
}
</style>
