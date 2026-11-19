/**
 * Custom accent colour support.
 *
 * TDesign only ships fixed green palettes, so a custom colour has to provide
 * the whole ramp: components refer to --td-brand-color-N as well as the
 * shorthand tokens, and light/dark palettes run in opposite directions
 * (light: 1 is palest, dark: 10 is brightest).
 *
 * The functions here are pure except `applyAccentPalette`, which writes the
 * resolved ramp onto <html> as inline custom properties. Inline wins over
 * theme.css, and because the variables are set on the root they reach
 * teleported popovers/dialogs too.
 */

export type AccentAppearance = 'light' | 'dark'

export const ACCENT_COLOR_KEY = 'accent_color'
export const DEFAULT_ACCENT_COLOR = '#07c05f'

/** #abc / #aabbcc (with or without #), case-insensitive. Rejects anything else. */
export function normalizeHex(value: unknown): string | null {
  if (typeof value !== 'string') return null
  const raw = value.trim().replace(/^#/, '')
  if (/^[0-9a-f]{3}$/i.test(raw)) {
    return `#${raw.split('').map((c) => c + c).join('').toLowerCase()}`
  }
  if (/^[0-9a-f]{6}$/i.test(raw)) return `#${raw.toLowerCase()}`
  return null
}

function toRgb(hex: string): [number, number, number] {
  const h = hex.slice(1)
  return [
    parseInt(h.slice(0, 2), 16),
    parseInt(h.slice(2, 4), 16),
    parseInt(h.slice(4, 6), 16),
  ]
}

function mix(hex: string, target: string, amount: number): string {
  const from = toRgb(hex)
  const to = toRgb(target)
  const parts = from.map((v, i) =>
    Math.round(v * (1 - amount) + to[i] * amount)
      .toString(16)
      .padStart(2, '0'),
  )
  return `#${parts.join('')}`
}

/**
 * Ten steps indexed 1..10. Light runs pale → saturated → near-black; dark
 * runs near-black → saturated → pale, mirroring TDesign's own ramps so the
 * existing hover (-3 / -5) and active (-5 / -7) references stay sensible.
 */
const LIGHT_STEPS: ReadonlyArray<{ to: string; amount: number }> = [
  { to: '#ffffff', amount: 0.9 },
  { to: '#ffffff', amount: 0.74 },
  { to: '#ffffff', amount: 0.5 },
  { to: '#ffffff', amount: 0 },
  { to: '#000000', amount: 0.1 },
  { to: '#000000', amount: 0.2 },
  { to: '#000000', amount: 0.3 },
  { to: '#000000', amount: 0.4 },
  { to: '#000000', amount: 0.5 },
  { to: '#000000', amount: 0.6 },
]

const DARK_STEPS: ReadonlyArray<{ to: string; amount: number }> = [
  { to: '#000000', amount: 0.62 },
  { to: '#000000', amount: 0.52 },
  { to: '#000000', amount: 0.42 },
  { to: '#000000', amount: 0.28 },
  { to: '#000000', amount: 0.14 },
  { to: '#ffffff', amount: 0 },
  { to: '#ffffff', amount: 0.12 },
  { to: '#ffffff', amount: 0.24 },
  { to: '#ffffff', amount: 0.38 },
  { to: '#ffffff', amount: 0.52 },
]

export function buildAccentPalette(hex: string, appearance: AccentAppearance): Record<string, string> {
  const steps = (appearance === 'dark' ? DARK_STEPS : LIGHT_STEPS).map(({ to, amount }) => mix(hex, to, amount))
  const step = (n: number) => steps[n - 1]
  const dark = appearance === 'dark'
  return {
    '--td-brand-color-1': step(1),
    '--td-brand-color-2': step(2),
    '--td-brand-color-3': step(3),
    '--td-brand-color-4': step(4),
    '--td-brand-color-5': step(5),
    '--td-brand-color-6': step(6),
    '--td-brand-color-7': step(7),
    '--td-brand-color-8': step(8),
    '--td-brand-color-9': step(9),
    '--td-brand-color-10': step(10),
    '--td-brand-color': hex,
    '--td-brand-color-hover': step(dark ? 5 : 3),
    '--td-brand-color-active': step(dark ? 7 : 5),
    '--td-brand-color-disabled': mix(hex, dark ? '#000000' : '#ffffff', 0.45),
    // Dark theme's palest token is a translucent wash, not an opaque tint.
    '--td-brand-color-light': dark ? `color-mix(in srgb, ${hex} 12%, transparent)` : step(1),
    '--brand-main': hex,
    '--app-accent-color': hex,
  }
}

/** Only these properties are ever written, so clearing cannot clobber theme.css. */
const ACCENT_PROPERTIES = [
  ...Array.from({ length: 10 }, (_, i) => `--td-brand-color-${i + 1}`),
  '--td-brand-color',
  '--td-brand-color-hover',
  '--td-brand-color-active',
  '--td-brand-color-disabled',
  '--td-brand-color-light',
  '--brand-main',
  '--app-accent-color',
]

export function applyAccentPalette(hex: string | null, appearance: AccentAppearance): void {
  const style = document.documentElement.style
  if (!hex) {
    ACCENT_PROPERTIES.forEach((name) => style.removeProperty(name))
    return
  }
  const palette = buildAccentPalette(hex, appearance)
  ACCENT_PROPERTIES.forEach((name) => style.setProperty(name, palette[name]))
}

function luminance(hex: string): number {
  const channels = toRgb(hex).map((v) => {
    const s = v / 255
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4
  })
  return 0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2]
}

function contrast(a: string, b: string): number {
  const l1 = luminance(a)
  const l2 = luminance(b)
  return (Math.max(l1, l2) + 0.05) / (Math.min(l1, l2) + 0.05)
}

/** Warn only about extremes; mid-tone colours are usable on both surfaces. */
export function isAccentReadable(hex: string): boolean {
  return contrast(hex, '#ffffff') >= 2 && contrast(hex, '#181818') >= 2
}
