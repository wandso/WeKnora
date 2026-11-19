import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  BACKDROP_PRESETS,
  buildBackdropTokens,
  isBuiltinBackdropId,
  isSafeDataImage,
  parseBackdropState,
} from './backdropTheme'

test('only built-in ids and none are accepted', () => {
  assert.equal(isBuiltinBackdropId('none'), true)
  assert.equal(isBuiltinBackdropId('xianxia'), true)
  assert.equal(isBuiltinBackdropId('custom'), false)
  assert.equal(isBuiltinBackdropId('evil'), false)
  assert.equal(isBuiltinBackdropId(42), false)
})

test('parseBackdropState rejects arbitrary CSS and remote URLs', () => {
  assert.deepEqual(parseBackdropState({ id: 'custom', image: 'url(http://evil)' }), { id: 'none' })
  assert.deepEqual(parseBackdropState({ id: 'custom', image: 'javascript:alert(1)' }), { id: 'none' })
  assert.deepEqual(parseBackdropState({ id: 'xianxia' }), { id: 'xianxia' })
  assert.deepEqual(parseBackdropState(null), { id: 'none' })
})

test('only image data URLs survive validation', () => {
  assert.equal(isSafeDataImage('data:image/png;base64,AAAA'), true)
  assert.equal(isSafeDataImage('data:image/jpeg;base64,AAAA'), true)
  assert.equal(isSafeDataImage('data:image/svg+xml;base64,AAAA'), false)
  assert.equal(isSafeDataImage('http://example.com/a.png'), false)
})

test('parsed custom image is applied as a scrim over the image', () => {
  const state = parseBackdropState({ id: 'custom', image: 'data:image/png;base64,QUJD' })
  const light = buildBackdropTokens(state, 'light')
  assert.match(light['--td-bg-color-page'], /^linear-gradient\(.*\), url\("data:image\/png;base64,QUJD"\)$/)
  const dark = buildBackdropTokens(state, 'dark')
  assert.match(dark['--td-bg-color-page'], /rgba\(10, 12, 20, \.72\)/)
  assert.notEqual(light['--td-bg-color-page'], dark['--td-bg-color-page'])
})

test('every preset ships a distinct light and dark token set', () => {
  for (const preset of BACKDROP_PRESETS) {
    const light = buildBackdropTokens({ id: preset.id }, 'light')
    const dark = buildBackdropTokens({ id: preset.id }, 'dark')
    assert.notEqual(light['--td-bg-color-page'], dark['--td-bg-color-page'], preset.id)
    assert.ok(light['--td-text-color-primary'], preset.id)
    assert.ok(dark['--td-bg-color-container'], preset.id)
    assert.match(String(light['--td-bg-color-page']), /gradient|#/, preset.id)
  }
})

test('none and invalid states produce no tokens', () => {
  assert.deepEqual(buildBackdropTokens({ id: 'none' }, 'light'), {})
  assert.deepEqual(buildBackdropTokens({ id: 'custom' }, 'light'), {})
  assert.deepEqual(buildBackdropTokens({ id: 'evil' as never }, 'light'), {})
})
