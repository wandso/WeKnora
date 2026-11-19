import { ref } from 'vue'
import {
  loadPreference,
  savePreference,
  safeRemoveItem,
  userKey,
  migratePreferencesIntoUser,
} from './preferenceStorage'
import {
  ACCENT_COLOR_KEY,
  applyAccentPalette,
  normalizeHex,
  type AccentAppearance,
} from './accentColor'
import {
  BACKDROP_THEME_KEY,
  applyBackdrop,
  backdropWindowColor,
  buildBackdropTokens,
  isBuiltinBackdropId,
  isSafeDataImage,
  parseBackdropState,
  type BackdropId,
  type BackdropState,
} from './backdropTheme'

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
// null means "use TDesign's built-in green ramp".
const accentColor = ref<string | null>(loadAccentColor())
const backdrop = ref<BackdropState>(loadBackdrop())

let lastPaintKey: string | null = null

function loadAccentColor(): string | null {
  return normalizeHex(loadPreference(ACCENT_COLOR_KEY))
}

function loadBackdrop(): BackdropState {
  const raw = loadPreference(BACKDROP_THEME_KEY)
  if (!raw) return { id: 'none' }
  try {
    return parseBackdropState(JSON.parse(raw))
  } catch {
    return { id: 'none' }
  }
}

/** Eye care uses the light ramp; only true dark gets the inverted ramp. */
function accentAppearance(effective: EffectiveTheme): AccentAppearance {
  return effective === 'dark' ? 'dark' : 'light'
}

function getSystemTheme(): 'light' | 'dark' {
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

function resolveTheme(mode: ThemeMode): EffectiveTheme {
  if (mode === 'system') return getSystemTheme()
  return mode
}

/**
 * Wails / 首屏兜底底色。背景主题激活时 #app 自身有渐变铺底，这里只保留
 * html/body 的实色作为边缘兜底，不再给 #app 写死纯色把渐变盖掉。
 */
function syncWailsNativeChrome(effective: EffectiveTheme, pageBackground: string | null) {
  const fallback = effective === 'dark' ? '#181818' : effective === 'eye' ? '#eae4d0' : '#eeeeee'
  document.documentElement.style.background = pageBackground ?? fallback
  document.documentElement.style.minHeight = '100%'
  document.documentElement.style.colorScheme = effective === 'dark' ? 'dark' : 'light'
  if (document.body) {
    document.body.style.background = pageBackground ?? fallback
    document.body.style.minHeight = '100%'
  }
  // #app 的 background 由 App.vue 的 var(--td-bg-color-page) 负责；清掉旧内联值
  // 让背景主题的渐变/图片能显示出来，无背景主题时再写回实色作为兜底。
  const appEl = document.getElementById('app')
  if (appEl) {
    if (pageBackground) {
      appEl.style.removeProperty('background')
    } else {
      appEl.style.background = fallback
    }
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
    if (effective === 'dark') w.WindowSetDarkTheme?.()
    else w.WindowSetLightTheme?.()
    const rgb = backdropWindowColor(backdrop.value, effective)
    const [r, g, b] = rgb
      ? [1, 3, 5].map((i) => parseInt(rgb.slice(i, i + 2), 16))
      : effective === 'dark'
        ? [24, 24, 24]
        : effective === 'eye'
          ? [234, 228, 208]
          : [238, 238, 238]
    w.WindowSetBackgroundColour(r, g, b, 255)
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
  const root = document.documentElement
  root.setAttribute('theme-mode', effective === 'dark' ? 'dark' : 'light')
  if (effective === 'eye') root.setAttribute('data-vision', 'eye')
  else root.removeAttribute('data-vision')

  // Rebuild on every apply: light/dark/eye flip the ramp order and the
  // backdrop token set, even when the chosen values did not change.
  applyAccentPalette(accentColor.value, accentAppearance(effective))
  applyBackdrop(backdrop.value, effective)

  const paintKey = [effective, backdrop.value.id, backdrop.value.image ?? ''].join('|')
  if (lastPaintKey !== paintKey) {
    lastPaintKey = paintKey
    const pageTokens = buildBackdropTokens(backdrop.value, effective)
    syncWailsNativeChrome(effective, pageTokens['--td-bg-color-page'] ?? null)
  }
}

export function useTheme() {
  function setTheme(mode: ThemeMode): boolean {
    if (!isThemeMode(mode)) return false
    currentTheme.value = mode
    savePreference(THEME_KEY, mode)
    applyTheme(mode)
    return true
  }

  /** null clears the override and restores TDesign's default ramp. */
  function setAccentColor(value: string | null): boolean {
    const hex = value === null ? null : normalizeHex(value)
    if (value !== null && !hex) return false
    accentColor.value = hex
    if (hex) {
      savePreference(ACCENT_COLOR_KEY, hex)
    } else {
      safeRemoveItem(userKey(ACCENT_COLOR_KEY))
    }
    applyAccentPalette(hex, accentAppearance(effectiveTheme.value))
    return true
  }

  /**
   * Switch the background theme. 'custom' requires a validated image data URL;
   * every other id is a built-in preset (use 'none' to clear).
   */
  function setBackdrop(id: BackdropId, image?: string): boolean {
    let next: BackdropState
    if (id === 'custom') {
      if (!image || !isSafeDataImage(image)) return false
      next = { id: 'custom', image }
    } else if (isBuiltinBackdropId(id)) {
      next = { id }
    } else {
      return false
    }
    backdrop.value = next
    if (next.id === 'none') {
      safeRemoveItem(userKey(BACKDROP_THEME_KEY))
    } else {
      savePreference(BACKDROP_THEME_KEY, JSON.stringify(next))
    }
    applyTheme(currentTheme.value)
    return true
  }

  return { currentTheme, effectiveTheme, accentColor, backdrop, setTheme, setAccentColor, setBackdrop }
}

/** Call once in main.ts to initialise theme and listen for OS changes. */
export function initTheme() {
  currentTheme.value = loadTheme()
  accentColor.value = loadAccentColor()
  backdrop.value = loadBackdrop()
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
  accentColor.value = loadAccentColor()
  backdrop.value = loadBackdrop()
  applyTheme(currentTheme.value)
}
