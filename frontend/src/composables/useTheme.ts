import { ref } from 'vue'
import {
  loadPreference,
  savePreference,
  migratePreferencesIntoUser,
} from './preferenceStorage'

/**
 * light = 白天模式（TDesign light）
 * dark  = 夜间模式（TDesign dark）
 * eye   = 护眼模式（TDesign light + 暖色纸感调色板，见 theme.css 的 [data-vision="eye"]）
 * system= 跟随系统（解析为 light / dark）
 */
export type ThemeMode = 'light' | 'dark' | 'eye' | 'system'
export type EffectiveTheme = 'light' | 'dark' | 'eye'

const THEME_KEY = 'theme'

const THEME_MODES: readonly ThemeMode[] = ['light', 'dark', 'eye', 'system']

export function isThemeMode(value: unknown): value is ThemeMode {
  return typeof value === 'string' && (THEME_MODES as readonly string[]).includes(value)
}

function loadTheme(): ThemeMode {
  const v = loadPreference(THEME_KEY)
  return isThemeMode(v) ? v : 'light'
}

// Shared reactive state across all consumers
const currentTheme = ref<ThemeMode>(loadTheme())
// What is actually painted right now: 'system' is resolved, 'eye' stays 'eye'.
const effectiveTheme = ref<EffectiveTheme>('light')

let lastEffective: EffectiveTheme | null = null

function getSystemTheme(): 'light' | 'dark' {
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

function resolveTheme(mode: ThemeMode): EffectiveTheme {
  if (mode === 'system') return getSystemTheme()
  return mode
}

/** Wails：与原生窗口底色 / 系统深浅色一致，减轻 Ctrl+R 整窗白闪（浅色与 --td-bg-color-page 对齐） */
function syncWailsNativeChrome(effective: EffectiveTheme) {
  const bg = effective === 'dark' ? '#181818' : effective === 'eye' ? '#eae4d0' : '#eeeeee'
  document.documentElement.style.background = bg
  document.documentElement.style.minHeight = '100%'
  document.documentElement.style.colorScheme = effective === 'dark' ? 'dark' : 'light'
  if (document.body) {
    document.body.style.background = bg
    document.body.style.minHeight = '100%'
  }
  const appEl = document.getElementById('app')
  if (appEl) {
    appEl.style.background = bg
    appEl.style.minHeight = '100%'
  }
  const w = (window as unknown as {
    runtime?: {
      WindowSetDarkTheme?: () => void
      WindowSetLightTheme?: () => void
      WindowSetBackgroundColour?: (r: number, g: number, b: number, a: number) => void
    }
  }).runtime
  if (!w?.WindowSetBackgroundColour) return
  try {
    if (effective === 'dark') {
      w.WindowSetDarkTheme?.()
      w.WindowSetBackgroundColour(24, 24, 24, 255)
    } else if (effective === 'eye') {
      w.WindowSetLightTheme?.()
      w.WindowSetBackgroundColour(234, 228, 208, 255)
    } else {
      w.WindowSetLightTheme?.()
      w.WindowSetBackgroundColour(238, 238, 238, 255)
    }
  } catch {
    /* 非桌面壳或未注入 runtime */
  }
}

/**
 * 两个维度共同决定观感：
 *  - theme-mode 属性：TDesign 只认 light / dark，护眼模式沿用 light 作为基底；
 *  - data-vision 属性：应用级调色板开关，只在护眼模式下存在，theme.css 用它覆写暖色令牌。
 */
function applyTheme(mode: ThemeMode) {
  const effective = resolveTheme(mode)
  effectiveTheme.value = effective
  if (lastEffective === effective) return
  lastEffective = effective
  const root = document.documentElement
  root.setAttribute('theme-mode', effective === 'dark' ? 'dark' : 'light')
  if (effective === 'eye') {
    root.setAttribute('data-vision', 'eye')
  } else {
    root.removeAttribute('data-vision')
  }
  syncWailsNativeChrome(effective)
}

export function useTheme() {
  function setTheme(mode: ThemeMode): boolean {
    if (!isThemeMode(mode)) return false
    currentTheme.value = mode
    savePreference(THEME_KEY, mode)
    applyTheme(mode)
    return true
  }

  return { currentTheme, effectiveTheme, setTheme }
}

/** Call once in main.ts to initialise theme and listen for OS changes. */
export function initTheme() {
  currentTheme.value = loadTheme()
  applyTheme(currentTheme.value)

  // React to OS theme changes when user chose "system"
  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
    if (currentTheme.value === 'system') {
      applyTheme('system')
    }
  })
}

/** Re-read preferences from storage (call after login / logout). */
export function reloadThemeFromStorage() {
  migratePreferencesIntoUser()
  currentTheme.value = loadTheme()
  applyTheme(currentTheme.value)
}
