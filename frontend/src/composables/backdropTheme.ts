/**
 * Background theme ("整体背景主题外观").
 *
 * Orthogonal to the light/dark/eye switch: that one picks TDesign's base
 * palette, this one paints a decorative layer over page/sidebar/card plus
 * matching text/border tokens. Built-in presets ship both a light and a dark
 * token set so switching modes still visibly changes the UI; a user-uploaded
 * image is tinted per mode with a translucent scrim to keep text readable.
 *
 * Everything is written as inline custom properties on <html> (same approach
 * as accentColor.ts) so it beats theme.css and reaches teleported layers.
 * Only the properties listed in BACKDROP_PROPERTIES are ever touched, which
 * makes clearing safe.
 */

export const BACKDROP_THEME_KEY = 'backdrop_theme'

export type BackdropMode = 'light' | 'dark' | 'eye'
export type BuiltinBackdropId = 'none' | 'xianxia' | 'ink' | 'starry' | 'cyber'
export type BackdropId = BuiltinBackdropId | 'custom'

export interface BackdropState {
  /** Built-in preset, or 'custom' when a user image is active. */
  id: BackdropId
  /** Data URL of the uploaded image, only meaningful when id === 'custom'. */
  image?: string
}

/** Every property this module may write; clearing removes exactly these. */
const BACKDROP_PROPERTIES = [
  '--td-bg-color-page',
  '--td-bg-color-container',
  '--td-bg-color-container-hover',
  '--td-bg-color-container-active',
  '--td-bg-color-container-select',
  '--td-bg-color-secondarycontainer',
  '--td-bg-color-secondarycontainer-hover',
  '--td-bg-color-secondarycontainer-active',
  '--td-bg-color-component',
  '--td-bg-color-component-hover',
  '--td-bg-color-component-active',
  '--td-bg-color-component-disabled',
  '--td-bg-color-sidebar',
  '--td-bg-color-settings-modal',
  '--td-component-stroke',
  '--td-component-border',
  '--td-font-gray-1',
  '--td-font-gray-2',
  '--td-font-gray-3',
  '--td-font-gray-4',
  '--td-text-color-primary',
  '--td-text-color-secondary',
  '--td-text-color-placeholder',
  '--td-text-color-disabled',
  '--app-backdrop-image',
] as const

interface BackdropTokens {
  page: string
  container: string
  containerHover: string
  containerActive: string
  secondary: string
  secondaryHover: string
  secondaryActive: string
  component: string
  componentHover: string
  componentActive: string
  componentDisabled: string
  sidebar: string
  stroke: string
  border: string
  text1: string
  text2: string
  text3: string
  text4: string
}

export interface BackdropPreset {
  id: BuiltinBackdropId
  labelKey: string
  swatch: [string, string]
  light: BackdropTokens
  dark: BackdropTokens
}

function tokens(t: BackdropTokens): Record<string, string> {
  return {
    '--td-bg-color-page': t.page,
    '--td-bg-color-container': t.container,
    '--td-bg-color-container-hover': t.containerHover,
    '--td-bg-color-container-active': t.containerActive,
    '--td-bg-color-container-select': t.container,
    '--td-bg-color-secondarycontainer': t.secondary,
    '--td-bg-color-secondarycontainer-hover': t.secondaryHover,
    '--td-bg-color-secondarycontainer-active': t.secondaryActive,
    '--td-bg-color-component': t.component,
    '--td-bg-color-component-hover': t.componentHover,
    '--td-bg-color-component-active': t.componentActive,
    '--td-bg-color-component-disabled': t.componentDisabled,
    '--td-bg-color-sidebar': t.sidebar,
    '--td-bg-color-settings-modal': t.sidebar,
    '--td-component-stroke': t.stroke,
    '--td-component-border': t.border,
    '--td-font-gray-1': t.text1,
    '--td-font-gray-2': t.text2,
    '--td-font-gray-3': t.text3,
    '--td-font-gray-4': t.text4,
    '--td-text-color-primary': t.text1,
    '--td-text-color-secondary': t.text2,
    '--td-text-color-placeholder': t.text3,
    '--td-text-color-disabled': t.text4,
  }
}

/**
 * Presets use CSS gradients for the page layer so no binary assets ship with
 * the frontend. Text is kept at >= 4.5:1 against both container surfaces.
 */
export const BACKDROP_PRESETS: readonly BackdropPreset[] = [
  {
    id: 'xianxia',
    labelKey: 'theme.backdropXianxia',
    swatch: ['#dcebd6', '#173a30'],
    light: {
      page: 'radial-gradient(120% 85% at 0% 0%, rgba(43,94,72,.16), transparent 55%), radial-gradient(90% 70% at 100% 0%, rgba(184,150,74,.14), transparent 60%), #eef2e9',
      container: '#fbfcf7', containerHover: '#f2f6ea', containerActive: '#e6edda',
      secondary: '#f4f7ee', secondaryHover: '#eaf0e2', secondaryActive: '#dfe9d2',
      component: '#dbe5d0', componentHover: '#ccd9bd', componentActive: '#b6c9a2', componentDisabled: '#e9eee2',
      sidebar: '#e5ecdf', stroke: '#d4e0c8', border: '#c3d2b2',
      text1: 'rgba(26,44,32,.92)', text2: 'rgba(26,44,32,.7)', text3: 'rgba(26,44,32,.48)', text4: 'rgba(26,44,32,.28)',
    },
    dark: {
      page: 'radial-gradient(120% 85% at 0% 0%, rgba(60,120,90,.24), transparent 55%), #101b16',
      container: '#18261e', containerHover: '#1f3026', containerActive: '#29402f',
      secondary: '#1b2a21', secondaryHover: '#243729', secondaryActive: '#2f4635',
      component: '#2b3f30', componentHover: '#38513d', componentActive: '#49644f', componentDisabled: '#1c2a21',
      sidebar: '#0d1712', stroke: '#2c4233', border: '#3b5442',
      text1: 'rgba(228,240,226,.92)', text2: 'rgba(228,240,226,.7)', text3: 'rgba(228,240,226,.48)', text4: 'rgba(228,240,226,.28)',
    },
  },
  {
    id: 'ink',
    labelKey: 'theme.backdropInk',
    swatch: ['#f0efe9', '#2b2b2b'],
    light: {
      page: 'radial-gradient(120% 80% at 85% 0%, rgba(60,60,60,.09), transparent 60%), #f1f0eb',
      container: '#fdfcf9', containerHover: '#f6f5f0', containerActive: '#eae9e2',
      secondary: '#f6f5f0', secondaryHover: '#eeede7', secondaryActive: '#e2e1d9',
      component: '#e3e2da', componentHover: '#d6d5cc', componentActive: '#c2c1b7', componentDisabled: '#efeee9',
      sidebar: '#e8e6df', stroke: '#dedcd4', border: '#cfcdc4',
      text1: 'rgba(28,28,26,.92)', text2: 'rgba(28,28,26,.7)', text3: 'rgba(28,28,26,.48)', text4: 'rgba(28,28,26,.28)',
    },
    dark: {
      page: 'radial-gradient(120% 80% at 85% 0%, rgba(180,180,180,.07), transparent 60%), #161719',
      container: '#1e1f21', containerHover: '#28292c', containerActive: '#33353a',
      secondary: '#232427', secondaryHover: '#2c2e31', secondaryActive: '#383a3f',
      component: '#34363a', componentHover: '#41434a', componentActive: '#51545d', componentDisabled: '#25262a',
      sidebar: '#111214', stroke: '#34363b', border: '#454850',
      text1: 'rgba(238,238,234,.92)', text2: 'rgba(238,238,234,.7)', text3: 'rgba(238,238,234,.48)', text4: 'rgba(238,238,234,.28)',
    },
  },
  {
    id: 'starry',
    labelKey: 'theme.backdropStarry',
    swatch: ['#e8ebfb', '#111538'],
    light: {
      page: 'radial-gradient(90% 70% at 100% 0%, rgba(91,104,190,.2), transparent 60%), radial-gradient(70% 60% at 0% 100%, rgba(138,104,190,.12), transparent 60%), #eef0fb',
      container: '#fcfcff', containerHover: '#f2f3fd', containerActive: '#e5e8f8',
      secondary: '#f3f4fc', secondaryHover: '#e9ebf8', secondaryActive: '#dce0f2',
      component: '#dfe2f3', componentHover: '#cfd4ec', componentActive: '#b8c0e2', componentDisabled: '#eff0f9',
      sidebar: '#e6e9f7', stroke: '#d9dcef', border: '#c9cee8',
      text1: 'rgba(26,30,66,.92)', text2: 'rgba(26,30,66,.7)', text3: 'rgba(26,30,66,.48)', text4: 'rgba(26,30,66,.28)',
    },
    dark: {
      page: 'radial-gradient(90% 70% at 100% 0%, rgba(99,114,220,.22), transparent 60%), #0f1224',
      container: '#191d34', containerHover: '#222742', containerActive: '#2d3352',
      secondary: '#1d2140', secondaryHover: '#262b4b', secondaryActive: '#32385c',
      component: '#2c3255', componentHover: '#394066', componentActive: '#4a527d', componentDisabled: '#20243e',
      sidebar: '#0c0f1f', stroke: '#2b3050', border: '#3b4168',
      text1: 'rgba(232,234,255,.92)', text2: 'rgba(232,234,255,.7)', text3: 'rgba(232,234,255,.48)', text4: 'rgba(232,234,255,.28)',
    },
  },
  {
    id: 'cyber',
    labelKey: 'theme.backdropCyber',
    swatch: ['#f2eefb', '#160e24'],
    light: {
      page: 'radial-gradient(90% 70% at 0% 0%, rgba(124,58,237,.16), transparent 60%), radial-gradient(80% 60% at 100% 100%, rgba(6,182,212,.12), transparent 60%), #f3effb',
      container: '#fdfbff', containerHover: '#f5f0fd', containerActive: '#e9dffa',
      secondary: '#f6f1fd', secondaryHover: '#ede4fa', secondaryActive: '#e0d2f4',
      component: '#e6daf5', componentHover: '#d8c8ee', componentActive: '#c3abe4', componentDisabled: '#f1ecf8',
      sidebar: '#ece5f7', stroke: '#e0d4f0', border: '#d1c1e8',
      text1: 'rgba(40,24,66,.92)', text2: 'rgba(40,24,66,.7)', text3: 'rgba(40,24,66,.48)', text4: 'rgba(40,24,66,.28)',
    },
    dark: {
      page: 'radial-gradient(90% 70% at 0% 0%, rgba(124,58,237,.24), transparent 60%), radial-gradient(80% 60% at 100% 100%, rgba(6,182,212,.14), transparent 60%), #100b1a',
      container: '#1d152b', containerHover: '#281d3a', containerActive: '#372750',
      secondary: '#221934', secondaryHover: '#2d2142', secondaryActive: '#3a2b56',
      component: '#35284d', componentHover: '#43335f', componentActive: '#564376', componentDisabled: '#251c38',
      sidebar: '#0d0916', stroke: '#332748', border: '#46345f',
      text1: 'rgba(240,234,255,.92)', text2: 'rgba(240,234,255,.7)', text3: 'rgba(240,234,255,.48)', text4: 'rgba(240,234,255,.28)',
    },
  },
]

export function isBuiltinBackdropId(value: unknown): value is BuiltinBackdropId {
  return value === 'none' || BACKDROP_PRESETS.some((preset) => preset.id === value)
}

export function parseBackdropState(raw: unknown): BackdropState {
  if (!raw || typeof raw !== 'object') return { id: 'none' }
  const candidate = raw as { id?: unknown; image?: unknown }
  if (candidate.id === 'custom' && typeof candidate.image === 'string' && isSafeDataImage(candidate.image)) {
    return { id: 'custom', image: candidate.image }
  }
  if (isBuiltinBackdropId(candidate.id)) return { id: candidate.id }
  return { id: 'none' }
}

/** Only image data URLs are accepted — never arbitrary CSS or remote URLs. */
export function isSafeDataImage(value: string): boolean {
  return /^data:image\/(?:png|jpe?g|webp|gif);base64,[a-z0-9+/=]+$/i.test(value)
}

/** Solid fallback for the native window chrome, which cannot paint a gradient. */
export function backdropWindowColor(state: BackdropState, mode: BackdropMode): string | null {
  if (state.id === 'custom') return mode === 'dark' ? '#0a0c14' : '#f3f4f8'
  const preset = BACKDROP_PRESETS.find((item) => item.id === state.id)
  if (!preset) return null
  return mode === 'dark' ? preset.swatch[1] : preset.swatch[0]
}

/** Scrim keeps uploaded photos from washing out text; dark modes need a darker one. */
function scrim(mode: BackdropMode): string {
  if (mode === 'dark') {
    return 'linear-gradient(rgba(10, 12, 20, .72), rgba(10, 12, 20, .72))'
  }
  if (mode === 'eye') {
    return 'linear-gradient(rgba(234, 228, 208, .8), rgba(234, 228, 208, .8))'
  }
  return 'linear-gradient(rgba(243, 244, 248, .82), rgba(243, 244, 248, .82))'
}

/** Build the inline token map for a state, or an empty map for "none". */
export function buildBackdropTokens(state: BackdropState, mode: BackdropMode): Record<string, string> {
  if (state.id === 'custom') {
    if (!state.image || !isSafeDataImage(state.image)) return {}
    return {
      '--td-bg-color-page': `${scrim(mode)}, url("${state.image}")`,
      '--app-backdrop-image': `url("${state.image}")`,
    }
  }
  const preset = BACKDROP_PRESETS.find((item) => item.id === state.id)
  if (!preset) return {}
  return tokens(mode === 'dark' ? preset.dark : preset.light)
}

export function applyBackdrop(state: BackdropState, mode: BackdropMode): void {
  const style = document.documentElement.style
  BACKDROP_PROPERTIES.forEach((name) => style.removeProperty(name))
  const applied = buildBackdropTokens(state, mode)
  Object.entries(applied).forEach(([name, value]) => style.setProperty(name, value))
  if (state.id === 'custom') {
    document.documentElement.setAttribute('data-backdrop', 'custom')
  } else if (state.id !== 'none') {
    document.documentElement.setAttribute('data-backdrop', state.id)
  } else {
    document.documentElement.removeAttribute('data-backdrop')
  }
}

const MAX_IMAGE_BYTES = 8 * 1024 * 1024
const MAX_IMAGE_EDGE = 1600
const JPEG_QUALITY = 0.82

/** Downscale + re-encode uploads so localStorage stays comfortably within quota. */
export async function readBackdropImage(file: File): Promise<string> {
  if (!/^image\/(png|jpe?g|webp|gif)$/i.test(file.type)) {
    throw new Error('unsupported-image-type')
  }
  if (file.size > MAX_IMAGE_BYTES) throw new Error('image-too-large')

  const objectUrl = URL.createObjectURL(file)
  try {
    const image = await new Promise<HTMLImageElement>((resolve, reject) => {
      const img = new Image()
      img.onload = () => resolve(img)
      img.onerror = () => reject(new Error('image-decode-failed'))
      img.src = objectUrl
    })
    const scale = Math.min(1, MAX_IMAGE_EDGE / Math.max(image.naturalWidth, image.naturalHeight))
    const width = Math.max(1, Math.round(image.naturalWidth * scale))
    const height = Math.max(1, Math.round(image.naturalHeight * scale))
    const canvas = document.createElement('canvas')
    canvas.width = width
    canvas.height = height
    const context = canvas.getContext('2d')
    if (!context) throw new Error('canvas-unavailable')
    context.drawImage(image, 0, 0, width, height)
    return canvas.toDataURL('image/jpeg', JPEG_QUALITY)
  } finally {
    URL.revokeObjectURL(objectUrl)
  }
}
