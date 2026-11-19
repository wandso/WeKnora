<template>
  <div
    class="theme-mode-switch"
    :class="{ 'theme-mode-switch--fluid': fluid }"
    role="group"
    :aria-label="t('theme.appearance')"
  >
    <button
      v-for="option in options"
      :key="option.value"
      type="button"
      class="theme-mode-switch__option"
      :class="{ 'is-active': activeMode === option.value }"
      :aria-pressed="activeMode === option.value"
      :title="t(option.tipKey)"
      @click="select(option.value)"
    >
      <t-icon :name="option.icon" size="16px" class="theme-mode-switch__icon" />
      <span v-if="showLabel" class="theme-mode-switch__label">{{ t(option.labelKey) }}</span>
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useTheme, type ThemeMode } from '@/composables/useTheme'

/**
 * 白天 / 夜间 / 护眼 三态切换。
 * 跟随系统（system）在设置页里仍然可选，这里只负责把"当前实际观感"
 * 映射到三选一的选中态：system 解析成 light / dark 后对应到前两个按钮。
 */
withDefaults(defineProps<{
  /** 占满父容器宽度（设置页、下拉菜单里都需要） */
  fluid?: boolean
  /** 只显示图标，用于空间紧张的工具栏 */
  showLabel?: boolean
}>(), {
  fluid: true,
  showLabel: true,
})

const { t } = useI18n()
const { currentTheme, effectiveTheme, setTheme } = useTheme()

const options: Array<{ value: Exclude<ThemeMode, 'system'>; icon: string; labelKey: string; tipKey: string }> = [
  { value: 'light', icon: 'sunny', labelKey: 'theme.dayMode', tipKey: 'theme.dayModeTip' },
  { value: 'dark', icon: 'moon', labelKey: 'theme.nightMode', tipKey: 'theme.nightModeTip' },
  { value: 'eye', icon: 'browse', labelKey: 'theme.eyeModeShort', tipKey: 'theme.eyeModeTip' },
]

// effectiveTheme 已经把 system 解析成 light/dark；护眼模式保持 eye。
const activeMode = computed(() => effectiveTheme.value)

const select = (mode: Exclude<ThemeMode, 'system'>) => {
  // 比较 currentTheme 而不是 activeMode：跟随系统时高亮的是解析后的那一档，
  // 此时点它应当"钉住"该模式，而不是因为看起来已经选中而变成空操作。
  if (currentTheme.value === mode) return
  setTheme(mode)
}
</script>

<style lang="less" scoped>
.theme-mode-switch {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  padding: 2px;
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-md);
  background: var(--td-bg-color-secondarycontainer);

  &--fluid {
    display: flex;
    width: 100%;
  }
}

.theme-mode-switch__option {
  display: inline-flex;
  flex: 1 1 0;
  align-items: center;
  justify-content: center;
  gap: 4px;
  min-width: 0;
  padding: 5px 8px;
  border: 0;
  border-radius: var(--app-radius-sm);
  background: transparent;
  color: var(--td-text-color-secondary);
  font-family: inherit;
  font-size: var(--app-text-sm);
  line-height: 1.2;
  white-space: nowrap;
  cursor: pointer;
  transition: background-color var(--app-motion-fast) ease, color var(--app-motion-fast) ease;

  &:hover {
    background: var(--td-bg-color-container-hover);
    color: var(--td-text-color-primary);
  }

  &.is-active {
    background: var(--td-bg-color-container);
    color: var(--td-text-color-primary);
    font-weight: 600;
    box-shadow: 0 1px 2px rgba(0, 0, 0, 0.08);
  }

  &.is-active .theme-mode-switch__icon {
    color: var(--td-brand-color);
  }

  &:focus-visible {
    outline: 2px solid var(--td-brand-color);
    outline-offset: -1px;
  }
}

.theme-mode-switch__icon {
  flex-shrink: 0;
}

.theme-mode-switch__label {
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
