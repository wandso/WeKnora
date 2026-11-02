<template>
  <div class="reservoir-tool">
    <div class="reservoir-tool__bar">
      <span class="reservoir-tool__hint">
        <t-icon name="info-circle" size="14px" />
        {{ t('reservoirTool.hint') }}
      </span>
      <t-button variant="text" size="small" class="reservoir-tool__open" @click="openInNewTab">
        {{ t('reservoirTool.openInNewTab') }}
      </t-button>
    </div>

    <div class="reservoir-tool__frame-wrap">
      <iframe
        ref="frame"
        class="reservoir-tool__frame"
        :src="src"
        :title="t('reservoirTool.frameTitle')"
        @load="syncTheme"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

const base = (import.meta.env.BASE_URL || '/').replace(/\/+$/, '')
const frame = ref<HTMLIFrameElement | null>(null)
const src = `${base}/reservoir-curve-tool.html?theme=${readTheme()}`

function readTheme(): 'light' | 'dark' {
  return document.documentElement.getAttribute('theme-mode') === 'dark' ? 'dark' : 'light'
}

/** 宿主主题变化时通过 postMessage 通知内嵌页，避免整页重载。 */
function syncTheme() {
  frame.value?.contentWindow?.postMessage({ type: 'weknora-theme', theme: readTheme() }, '*')
}

function openInNewTab() {
  window.open(src, '_blank', 'noopener')
}

let observer: MutationObserver | null = null

onMounted(() => {
  observer = new MutationObserver(syncTheme)
  observer.observe(document.documentElement, { attributes: true, attributeFilter: ['theme-mode'] })
})

onBeforeUnmount(() => {
  observer?.disconnect()
  observer = null
})
</script>

<style scoped lang="less">
.reservoir-tool {
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-height: 0;
}

.reservoir-tool__bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.reservoir-tool__hint {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--td-text-color-secondary);
  font-size: var(--app-text-sm);
}

.reservoir-tool__open.t-button {
  flex-shrink: 0;
  color: var(--td-brand-color);
}

.reservoir-tool__frame-wrap {
  height: clamp(620px, calc(100dvh - 248px), 1600px);
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-lg, 12px);
  overflow: hidden;
  background: var(--td-bg-color-container);
}

.reservoir-tool__frame {
  display: block;
  width: 100%;
  height: 100%;
  border: 0;
}
</style>
