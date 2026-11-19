import assert from 'node:assert/strict'
import { test } from 'node:test'
import {
  buildAccentPalette,
  isAccentReadable,
  normalizeHex,
} from './accentColor'

test('normalizeHex accepts #rgb / #rrggbb and rejects injection', () => {
  assert.equal(normalizeHex('ABC'), '#aabbcc')
  assert.equal(normalizeHex('#F0A9Bc'), '#f0a9bc')
  assert.equal(normalizeHex('  #07C05F  '), '#07c05f')
  for (const bad of ['red', 'rgb(1,2,3)', '#12345', '#12345678', '#12g4f5', 'url(x);color:red', null, 42]) {
    assert.equal(normalizeHex(bad), null)
  }
})

test('light ramp puts the chosen colour at step 4 and dark at step 6', () => {
  const base = '#3b82f6'
  const light = buildAccentPalette(base, 'light')
  const dark = buildAccentPalette(base, 'dark')
  assert.equal(light['--td-brand-color-4'], base)
  assert.equal(dark['--td-brand-color-6'], base)
  assert.equal(light['--td-brand-color'], base)
  assert.equal(dark['--brand-main'], base)
})

test('hover lightens in light mode and darkens in dark mode', () => {
  const base = '#3b82f6'
  const light = buildAccentPalette(base, 'light')
  const dark = buildAccentPalette(base, 'dark')
  assert.equal(light['--td-brand-color-hover'], light['--td-brand-color-3'])
  assert.equal(light['--td-brand-color-active'], light['--td-brand-color-5'])
  assert.equal(dark['--td-brand-color-hover'], dark['--td-brand-color-5'])
  assert.equal(dark['--td-brand-color-active'], dark['--td-brand-color-7'])
  assert.match(dark['--td-brand-color-light'], /^color-mix\(/)
})

test('extreme colours are flagged, mid-tone brand colours are not', () => {
  assert.equal(isAccentReadable('#07c05f'), true)
  assert.equal(isAccentReadable('#3b82f6'), true)
  assert.equal(isAccentReadable('#ffffff'), false)
  assert.equal(isAccentReadable('#000000'), false)
})
