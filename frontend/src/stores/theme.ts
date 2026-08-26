import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { GetTheme, SetTheme } from '../../wailsjs/go/main/App'
import {
  WindowSetDarkTheme,
  WindowSetLightTheme,
  WindowSetSystemDefaultTheme,
} from '../../wailsjs/runtime/runtime'

export type ThemeMode = 'system' | 'light' | 'dark'
export type EffectiveTheme = 'light' | 'dark'

export function resolveEffectiveTheme(mode: ThemeMode, systemDark: boolean): EffectiveTheme {
  if (mode === 'system') {
    return systemDark ? 'dark' : 'light'
  }
  return mode
}

function isThemeMode(value: string): value is ThemeMode {
  return value === 'system' || value === 'light' || value === 'dark'
}

function syncWindowTheme(mode: ThemeMode) {
  if (!(window as any).runtime) return
  if (mode === 'dark') {
    WindowSetDarkTheme()
  } else if (mode === 'light') {
    WindowSetLightTheme()
  } else {
    WindowSetSystemDefaultTheme()
  }
}

export const useThemeStore = defineStore('theme', () => {
  const mode = ref<ThemeMode>('system')
  const media = window.matchMedia('(prefers-color-scheme: dark)')
  const systemDark = ref(media.matches)
  const effective = computed(() => resolveEffectiveTheme(mode.value, systemDark.value))
  let listening = false

  function startSystemListener() {
    if (listening) return
    listening = true
    media.addEventListener('change', (event) => {
      systemDark.value = event.matches
    })
  }

  async function initialize() {
    startSystemListener()
    try {
      const saved = await GetTheme()
      mode.value = isThemeMode(saved) ? saved : 'system'
    } catch {
      mode.value = 'system'
    }
    syncWindowTheme(mode.value)
  }

  async function setMode(next: ThemeMode) {
    mode.value = next
    syncWindowTheme(next)
    await SetTheme(next)
  }

  return { mode, effective, initialize, setMode }
})
