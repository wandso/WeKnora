<template>
  <div class="login-layout">
    <!-- 顶部右上角：语言切换（保留 i18n 功能） -->
    <div class="header-links">
      <div class="language-switch">
        <button @click="toggleLanguageMenu" class="header-link" :title="currentLangOption?.label">
          <span class="lang-flag-icon">{{ currentLangOption?.flag }}</span>
          <span class="link-text">{{ currentLangOption?.shortLabel }}</span>
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"
            stroke-linecap="round">
            <polyline points="6 9 12 15 18 9" />
          </svg>
        </button>
        <div v-if="showLanguageMenu" class="language-dropdown">
          <div v-for="lang in languageOptions" :key="lang.value" @click="selectLanguage(lang.value)"
            class="language-option" :class="{ active: currentLanguage === lang.value }">
            <span class="lang-flag">{{ lang.flag }}</span>
            <span class="lang-label">{{ lang.label }}</span>
            <span v-if="currentLanguage === lang.value" class="check-icon">✓</span>
          </div>
        </div>
      </div>
    </div>

    <!-- 右侧表单区 -->
    <div class="form-section">
      <div class="form-panel">
        <!-- 品牌标识：原 WeKnora 图片 logo 改为「设计院知识库」文字 -->
        <div class="brand-title">设计院知识库</div>

        <!-- 登录卡片 -->
        <div class="form-card" v-if="!isRegisterMode">
          <div v-if="inviteLookup" class="invite-banner">
            <t-icon name="link" class="invite-banner__icon" />
            <div class="invite-banner__text">
              <div class="invite-banner__title">
                {{ $t('inviteRegister.bannerTitle', { tenant: inviteLookup.tenant_name || '' }) }}
              </div>
              <div class="invite-banner__hint">
                {{ $t('inviteRegister.bannerHintLogin') }}
              </div>
            </div>
          </div>
          <div v-else-if="inviteLookupError" class="invite-banner invite-banner--error">
            {{ inviteLookupError }}
          </div>
          <div class="form-header">
            <h2 class="form-title">{{ $t('auth.login') }}</h2>
            <p class="form-welcome">{{ $t('auth.subtitle') }}</p>
            <p v-if="registrationEnabled" class="form-hint">{{ $t('auth.loginHint') }}</p>
          </div>

          <div class="form-content">
            <t-form ref="formRef" :data="formData" :rules="formRules" @submit="handleLogin" layout="vertical"
              label-align="top">
              <t-form-item :label="$t('auth.email')" name="email">
                <t-input v-model="formData.email" :placeholder="$t('auth.emailPlaceholder')" type="text"
                  autocomplete="email" size="large" :disabled="loading">
                  <template #prefix-icon><img src="@/assets/img/login-user.png" class="input-prefix-icon" alt="" /></template>
                </t-input>
              </t-form-item>

              <t-form-item :label="$t('auth.password')" name="password">
                <t-input v-model="formData.password" :placeholder="$t('auth.passwordPlaceholder')" type="password"
                  autocomplete="current-password" size="large" :disabled="loading" @enter="handleLogin">
                  <template #prefix-icon><img src="@/assets/img/login-password.png" class="input-prefix-icon" alt="" /></template>
                </t-input>
              </t-form-item>

              <t-button type="submit" theme="primary" size="large" block :loading="loading" class="submit-button">
                {{ loading ? $t('auth.loggingIn') : $t('auth.login') }}
              </t-button>

              <div class="register-cta" v-if="registrationEnabled">
                <div class="register-cta__divider">
                  <span>{{ $t('auth.firstTime') }}</span>
                </div>
                <t-button theme="default" variant="outline" size="large" block class="register-cta__button"
                  :disabled="loading" @click="toggleMode">
                  {{ $t('auth.createAccount') }}
                </t-button>
              </div>

              <div v-if="oidcEnabled" class="oidc-divider">
                <span>{{ $t('auth.orContinueWith') }}</span>
              </div>

              <t-button v-if="oidcEnabled" theme="default" size="large" block :loading="oidcLoading" :disabled="loading"
                class="oidc-button" @click="handleOIDCLogin">
                {{ oidcLoading ? $t('auth.redirectingToOIDC') : oidcLoginText }}
              </t-button>
            </t-form>
          </div>
        </div>

        <!-- 注册卡片 -->
        <div class="form-card" v-if="isRegisterMode && registrationEnabled">
          <div v-if="inviteLookup" class="invite-banner">
            <t-icon name="link" class="invite-banner__icon" />
            <div class="invite-banner__text">
              <div class="invite-banner__title">
                {{ $t('inviteRegister.bannerTitle', { tenant: inviteLookup.tenant_name || '' }) }}
              </div>
              <div class="invite-banner__hint">
                {{ $t('inviteRegister.bannerHint') }}
              </div>
            </div>
          </div>
          <div v-else-if="inviteLookupError" class="invite-banner invite-banner--error">
            {{ inviteLookupError }}
          </div>
          <div class="form-header">
            <h2 class="form-title">{{ $t('auth.createAccount') }}</h2>
            <p class="form-subtitle">{{ $t('auth.registerSubtitle') }}</p>
          </div>

          <div class="form-content">
            <t-form ref="registerFormRef" :data="registerData" :rules="registerRules" @submit="handleRegister"
              layout="vertical" label-align="top">
              <t-form-item :label="$t('auth.username')" name="username">
                <t-input v-model="registerData.username" :placeholder="$t('auth.usernamePlaceholder')" size="large"
                  :disabled="loading" />
              </t-form-item>
              <t-form-item :label="$t('auth.email')" name="email">
                <t-input v-model="registerData.email" :placeholder="$t('auth.emailPlaceholder')" type="text"
                  autocomplete="email" size="large" :disabled="loading" />
              </t-form-item>
              <t-form-item :label="$t('auth.password')" name="password">
                <t-input v-model="registerData.password" :placeholder="$t('auth.passwordPlaceholder')" type="password"
                  autocomplete="new-password" size="large" :disabled="loading" />
              </t-form-item>
              <t-form-item :label="$t('auth.confirmPassword')" name="confirmPassword">
                <t-input v-model="registerData.confirmPassword" :placeholder="$t('auth.confirmPasswordPlaceholder')"
                  type="password" autocomplete="new-password" size="large" :disabled="loading"
                  @enter="handleRegister" />
              </t-form-item>

              <t-button type="submit" theme="primary" size="large" block :loading="loading" class="submit-button">
                {{ loading ? $t('auth.registering') : $t('auth.register') }}
              </t-button>
            </t-form>

            <div class="form-footer">
              <span>{{ $t('auth.haveAccount') }}</span>
              <a href="#" @click.prevent="toggleMode" class="link-button">
                {{ $t('auth.backToLogin') }}
              </a>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 底部版权：参考页样式 -->
    <div class="el-login-footer">
      <span>Copyright © 2018-{{ new Date().getFullYear() }} 设计院知识库 All Rights Reserved.</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, nextTick, onMounted, onBeforeUnmount, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { MessagePlugin } from 'tdesign-vue-next'
import { useRoleLabel } from '@/composables/useRoleLabel'
import { notifyLoginSuccess } from '@/utils/loginNotify'
import { canRegister } from './registrationPolicy'
import { newPasswordRules } from '@/utils/passwordPolicy'
import {
  login,
  register,
  getOIDCAuthorizationURL,
  getOIDCConfig,
  autoSetup,
  getAuthConfig,
  userInfoFromApi,
  getInvitationByToken,
  registerByInvite,
  type InviteLookup,
} from '@/api/auth'
import { useAuthStore } from '@/stores/auth'
import { normalizeMainLocale, setI18nLanguage } from '@/i18n'
import { useI18n } from 'vue-i18n'
import { getApiBaseUrl } from '@/utils/api-base'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()
const { t, tm, locale } = useI18n()
const { formatRole, roleIcon } = useRoleLabel()

// Form references
const formRef = ref()
const registerFormRef = ref()

// State management
const loading = ref(false)
const oidcLoading = ref(false)
const isRegisterMode = ref(false)
const showLanguageMenu = ref(false)
const oidcEnabled = ref(false)
const oidcProviderName = ref('')
// Wait for configuration before showing a registration entry.
const registrationMode = ref('')
const registrationEnabled = computed(() => canRegister(registrationMode.value, !!inviteLookup.value))
const complexPasswordEnabled = ref(false)

// invite-link state. When the URL carries ?token=xxx we resolve it to
// the originating tenant + role and switch the form into a "register
// via invitation" mode when invitation registration is enabled. Submitting the register
// form with this set hits /auth/register-by-invite (auto-login on
// success) instead of /auth/register.
const inviteToken = ref('')
const inviteLookup = ref<InviteLookup | null>(null)
const inviteLookupError = ref('')
const inviteLookupLoading = ref(false)

// Language options
const languageOptions = [
  { value: 'zh-CN', label: '简体中文', shortLabel: '中文', flag: '🇨🇳' },
  { value: 'zh-TW', label: '繁體中文', shortLabel: '繁中', flag: '繁' },
  { value: 'en-US', label: 'English', shortLabel: 'EN', flag: '🇺🇸' },
  { value: 'ru-RU', label: 'Русский', shortLabel: 'RU', flag: '🇷🇺' },
  { value: 'ko-KR', label: '한국어', shortLabel: '한국어', flag: '🇰🇷' },
  { value: 'ja-JP', label: '日本語', shortLabel: '日本語', flag: '🇯🇵' }
]

const currentLanguage = computed(() => locale.value)
const oidcLoginText = computed(() => {
  if (oidcProviderName.value) {
    return t('auth.oidcLoginWithProvider', { provider: oidcProviderName.value })
  }
  return t('auth.oidcLogin')
})
const currentLangOption = computed(() => languageOptions.find(l => l.value === currentLanguage.value))

// Login form data
const formData = reactive<{ [key: string]: any }>({
  email: '',
  password: '',
})

// Register form data
const registerData = reactive<{ [key: string]: any }>({
  username: '',
  email: '',
  password: '',
  confirmPassword: ''
})

// Login form validation rules
const formRules = computed(() => ({
  email: [
    { required: true, message: t('auth.emailRequired'), type: 'error' },
    { email: true, message: t('auth.emailInvalid'), type: 'error' }
  ],
  password: [
    { required: true, message: t('auth.passwordRequired'), type: 'error' },
    { min: 8, message: t('auth.passwordMinLength'), type: 'error' },
    { max: 32, message: t('auth.passwordMaxLength'), type: 'error' }
  ],
}))

// Register form validation rules
const registerRules = computed(() => ({
  username: [
    { required: true, message: t('auth.usernameRequired'), type: 'error' },
    { min: 2, message: t('auth.usernameMinLength'), type: 'error' },
    { max: 20, message: t('auth.usernameMaxLength'), type: 'error' },
    {
      pattern: /^[a-zA-Z0-9_\u4e00-\u9fa5]+$/,
      message: t('auth.usernameInvalid'),
      type: 'error'
    }
  ],
  email: [
    { required: true, message: t('auth.emailRequired'), type: 'error' },
    { email: true, message: t('auth.emailInvalid'), type: 'error' }
  ],
  password: newPasswordRules(t, complexPasswordEnabled.value),
  confirmPassword: [
    { required: true, message: t('auth.confirmPasswordRequired'), type: 'error' },
    {
      validator: (val: string) => val === registerData.password,
      message: t('auth.passwordMismatch'),
      type: 'error'
    }
  ]
}))

// Toggle login/register mode
const toggleMode = () => {
  isRegisterMode.value = !isRegisterMode.value && registrationEnabled.value

  Object.keys(registerData).forEach(key => {
    (registerData as any)[key] = ''
  })
}

// Toggle language menu
const toggleLanguageMenu = () => {
  showLanguageMenu.value = !showLanguageMenu.value
}

// Select language
const selectLanguage = async (lang: string) => {
  showLanguageMenu.value = false
  const requested = normalizeMainLocale(lang)
  try {
    const applied = await setI18nLanguage(requested)
    if (applied === requested) {
      MessagePlugin.success(t('language.languageSaved'))
    }
  } catch (err) {
    console.error('[设计院知识库] Failed to switch language:', err)
    MessagePlugin.error(t('common.failed'))
  }
}

// Close language menu when clicking outside
const handleClickOutside = (event: MouseEvent) => {
  const target = event.target as HTMLElement
  if (!target.closest('.language-switch')) {
    showLanguageMenu.value = false
  }
}

// Add click outside listener
onMounted(() => {
  document.addEventListener('click', handleClickOutside)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleClickOutside)
})

const persistLoginResponse = async (response: any, skipRedirect = false) => {
  // Backend renamed `tenant` to `active_tenant` and added `memberships`
  // when tenant-level RBAC landed (issue #1303). The two are otherwise
  // identical — `active_tenant` is the tenant whose ID is encoded in the
  // JWT, defaulting to the user's home tenant on a fresh login.
  const activeTenant = response.active_tenant || response.tenant
  if (response.user && response.token) {
    // user.tenant_id must be the user's HOME tenant (the immutable row
    // on the users table); useHomeTenant() and the home-badge logic both
    // assume so. The ACTIVE tenant (which can differ from home when the
    // server honoured a remembered last-active-tenant preference) is
    // expressed separately via setSelectedTenant below.
    const homeTenantIdRaw = response.user.tenant_id ?? activeTenant?.id ?? ''
    authStore.setUser(userInfoFromApi(response.user, homeTenantIdRaw))
    authStore.setToken(response.token)
    if (response.refresh_token) {
      authStore.setRefreshToken(response.refresh_token)
    }
    if (activeTenant) {
      authStore.setTenant({
        id: String(activeTenant.id) || '',
        name: activeTenant.name || '',
        owner_id: response.user.id || '',
        created_at: activeTenant.created_at || new Date().toISOString(),
        updated_at: activeTenant.updated_at || new Date().toISOString()
      })
    } else {
      authStore.setTenant(null)
    }
    if (Array.isArray(response.memberships)) {
      authStore.setMemberships(response.memberships)
    }
    // If the backend dropped us into a non-home tenant (honoured a
    // remembered "last active tenant" preference), set the override so
    // subsequent requests carry X-Tenant-ID and the UI stays consistent.
    // Otherwise clear any stale override left in localStorage by a
    // previous session for a different account.
    const activeIdNum = Number(activeTenant?.id)
    const homeIdNum = Number(homeTenantIdRaw)
    if (Number.isFinite(activeIdNum) && Number.isFinite(homeIdNum) && activeIdNum !== homeIdNum) {
      authStore.setSelectedTenant(activeIdNum, activeTenant?.name || null)
    } else {
      authStore.setSelectedTenant(null, null)
    }
  }

  // Pull runtime capabilities (including whether ordinary users may create
  // workspaces) before entering the main UI so create actions never flash
  // briefly when the deployment is invitation-only.
  await authStore.refreshFromAuthMe()
  await nextTick()
  if (skipRedirect) return
  router.replace(authStore.hasValidTenant ? '/platform/knowledge-bases' : '/onboarding/workspace')
}

const getBackendOIDCRedirectURI = () => `${window.location.origin}${getApiBaseUrl()}/api/v1/auth/oidc/callback`

const loadOIDCConfig = async () => {
  try {
    const response = await getOIDCConfig()
    oidcEnabled.value = !!response.success && !!response.enabled
    oidcProviderName.value = response.provider_display_name || ''
  } catch {
    oidcEnabled.value = false
    oidcProviderName.value = ''
  }
}

// Registration stays hidden if configuration cannot be loaded.
const loadAuthConfig = async () => {
  try {
    const response = await getAuthConfig()
    registrationMode.value = response.success ? response.registration_mode : ''
    complexPasswordEnabled.value = response.complex_password_enabled
  } catch {
    registrationMode.value = ''
    complexPasswordEnabled.value = false
  }
}

const handleOIDCLogin = async () => {
  try {
    oidcLoading.value = true
    const response = await getOIDCAuthorizationURL(getBackendOIDCRedirectURI())
    const authorizationURL = response.authorization_url

    if (!response.success || !authorizationURL) {
      MessagePlugin.error(response.message || t('auth.oidcLoginFailed'))
      return
    }

    // 跳转 IdP 会丢失 URL 中的 token，暂存到 sessionStorage，回调后由 App.vue 兑换。
    if (inviteToken.value) {
      sessionStorage.setItem('weknora_pending_invite_token', inviteToken.value)
    }
    window.location.href = authorizationURL
  } catch (error: any) {
    console.error('OIDC 登录跳转失败:', error)
    MessagePlugin.error(error.message || t('auth.oidcLoginFailed'))
  } finally {
    oidcLoading.value = false
  }
}

// 用 token 加入空间并进入应用。会话此时已有效，故即便 token 失效也照常进入（避免困在登录页）。
const acceptAndEnter = async (token: string) => {
  loading.value = true
  try {
    const result = await authStore.acceptInvitationByTokenAndRefresh(token)
    if (result.ok) {
      MessagePlugin.success(t('inviteRegister.joined'))
    } else {
      MessagePlugin.warning(t('inviteRegister.invalidBody'))
    }
  } catch {
    MessagePlugin.warning(t('inviteRegister.invalidBody'))
  } finally {
    loading.value = false
    await nextTick()
    router.replace('/platform/knowledge-bases')
  }
}

// Handle login
const handleLogin = async () => {
  try {
    const valid = await formRef.value?.validate()
    if (valid !== true) return

    loading.value = true

    const response = await login({
      email: formData.email,
      password: formData.password,
    })

    if (response.success) {
      if (inviteToken.value) {
        // 从邀请链接登录：持久化会话后兑换 token 并进入对应空间。
        await persistLoginResponse(response, true)
        await acceptAndEnter(inviteToken.value)
        return
      }
      await persistLoginResponse(response)
      notifyLoginSuccess(response, t, tm, formatRole, roleIcon)
    } else {
      MessagePlugin.error(response.message || t('auth.loginError'))
    }
  } catch (error: any) {
    console.error('登录错误:', error)
    MessagePlugin.error(error.message || t('auth.loginErrorRetry'))
  } finally {
    loading.value = false
  }
}

// Handle registration. Dispatches based on whether the user arrived
// with a share-link token: with token -> register-by-invite (auto-
// login on success); without -> the normal self-service register
// (drops back to the login form for the user to sign in).
const handleRegister = async () => {
  if (!registrationEnabled.value) return
  try {
    const valid = await registerFormRef.value?.validate()
    if (valid !== true) return

    loading.value = true

    if (inviteToken.value) {
      const response = await registerByInvite({
        token: inviteToken.value,
        username: registerData.username,
        email: registerData.email,
        password: registerData.password,
      })
      if (!response.success) {
        MessagePlugin.error(response.message || t('auth.registerFailed'))
        return
      }
      MessagePlugin.success(t('auth.registerSuccess'))
      // register-by-invite returns the same shape as login (token +
      // active_tenant + memberships), so reuse the login persistence
      // path — same store writes, same redirect target.
      await persistLoginResponse(response)
      return
    }

    const response = await register({
      username: registerData.username,
      email: registerData.email,
      password: registerData.password
    })

    if (response.success) {
      MessagePlugin.success(t('auth.registerSuccess'))

      // Switch to login mode and fill in email
      isRegisterMode.value = false
      formData.email = registerData.email

      // Clear register form
      Object.keys(registerData).forEach(key => {
        (registerData as any)[key] = ''
      })
    } else {
      MessagePlugin.error(response.message || t('auth.registerFailed'))
    }
  } catch (error: any) {
    console.error('注册错误:', error)
    MessagePlugin.error(error.message || t('auth.registerError'))
  } finally {
    loading.value = false
  }
}

// Check if already logged in; for lite edition, attempt transparent auto-setup
onMounted(async () => {
  // Share-link landing: ?token=xxx switches the form into invite-
  // register mode before any other auto-flow (logged-in redirect /
  // auto-setup / OIDC) gets a chance to redirect. Resolution failure
  // surfaces inline; the user can still log in normally if they
  // already have an account. We check this BEFORE the isLoggedIn
  // redirect so an existing session doesn't bounce the user to
  // /platform (and possibly back to /login if the session is stale),
  // dropping the invite token along the way.
  const tokenFromQuery = String(route.query.token || '').trim()
  if (tokenFromQuery) {
    inviteToken.value = tokenFromQuery
    inviteLookupLoading.value = true
    // 1. 先校验 token：无效/过期则停在登录页报错，不进注册模式。
    try {
      const resp = await getInvitationByToken(tokenFromQuery)
      if (resp.success && resp.data) {
        inviteLookup.value = resp.data
      } else {
        inviteLookupError.value = resp.message || t('inviteRegister.invalidBody')
        loadOIDCConfig()
        loadAuthConfig()
        return
      }
    } catch {
      inviteLookupError.value = t('inviteRegister.invalidBody')
      loadOIDCConfig()
      loadAuthConfig()
      return
    } finally {
      inviteLookupLoading.value = false
    }

    // 2. 已登录则直接兑换 token 进入空间（三种模式通用）。
    if (authStore.isLoggedIn && (await authStore.refreshFromAuthMe())) {
      await acceptAndEnter(tokenFromQuery)
      return
    }

    // 3. Only enabled registration modes may create an invited account.
    await loadAuthConfig()
    isRegisterMode.value = registrationEnabled.value
    loadOIDCConfig()
    return
  }

  if (authStore.isLoggedIn) {
    router.replace('/platform/knowledge-bases')
    return
  }

  localStorage.removeItem('weknora_auto_setup_failed')
  try {
    const response = await autoSetup()
    if (response.success) {
      authStore.setLiteMode(true)
      await persistLoginResponse(response)
      return
    }
  } catch {
    // Auto-setup may be unavailable outside the native Lite shell.
  }

  loadOIDCConfig()
  loadAuthConfig()
})
</script>

<style lang="less" scoped>
.login-layout {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  width: 100%;
  min-height: 100vh;
  box-sizing: border-box;
  position: relative;
  overflow: hidden;
  padding: 80px 15% 60px 40px;
  background: #eaf1ff url('../../assets/img/login-background.png') center center / cover no-repeat;
}

/* ---------- 顶部语言切换 ---------- */
.header-links {
  position: fixed;
  top: 24px;
  right: 28px;
  display: flex;
  align-items: center;
  gap: 10px;
  z-index: 100;
}

.header-link {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 8px 14px;
  border-radius: 20px;
  background: rgba(255, 255, 255, 0.65);
  border: 1px solid rgba(54, 128, 255, 0.25);
  color: #3680ff;
  text-decoration: none;
  font-size: 14px;
  font-weight: 600;
  letter-spacing: 0.2px;
  cursor: pointer;

  svg { flex-shrink: 0; }
  .link-text { line-height: 1; }

  &:hover {
    background: rgba(255, 255, 255, 0.9);
    border-color: rgba(54, 128, 255, 0.45);
    color: #2d65f5;
  }
}

.language-switch { position: relative; }

.lang-flag-icon {
  font-size: 18px;
  line-height: 1;
  flex-shrink: 0;
}

.language-dropdown {
  position: absolute;
  top: calc(100% + 8px);
  right: 0;
  min-width: 160px;
  background: #fff;
  border: 1px solid #dbe6f8;
  border-radius: 8px;
  box-shadow: 0 4px 16px rgba(45, 101, 245, 0.15);
  overflow: hidden;
  z-index: 1000;
}

.language-option {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 14px;
  cursor: pointer;
  font-size: 14px;
  font-family: var(--app-font-family);
  color: #325994;

  .lang-flag { font-size: 18px; flex-shrink: 0; }
  .lang-label { flex: 1; }
  .check-icon { color: #3680ff; font-weight: 700; flex-shrink: 0; }

  &:hover { background: #f0f5ff; }
  &.active { background: #e8f0ff; color: #2d65f5; }
}

/* ---------- 表单区 ---------- */
.form-section {
  width: 456px;
  max-width: 100%;
  position: relative;
  z-index: 2;
}

.form-panel { width: 100%; }

.brand-title {
  text-align: center;
  font-size: 42px;
  font-weight: 800;
  letter-spacing: 6px;
  color: #2d65f5;
  white-space: nowrap;
  margin-bottom: 22px;
  text-shadow: 0 2px 12px rgba(255, 255, 255, 0.95), 0 0 26px rgba(54, 128, 255, 0.22);
}

.form-card {
  width: 100%;
  box-sizing: border-box;
  background: rgba(255, 255, 255, 0.5);
  border: 1px solid rgba(255, 255, 255, 0.7);
  border-radius: 10px;
  box-shadow: 0 10px 20px 0 rgba(54, 128, 255, 0.2);
  padding: 34px 44px 38px;
  position: relative;
  backdrop-filter: blur(2px);
}

.form-card::after {
  content: " ";
  pointer-events: none;
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  border-style: solid;
  border-width: 3px;
  border-image: linear-gradient(180deg, #ffffff 0%, rgba(255, 255, 255, 0) 100%) 1;
  border-radius: 10px;
}

/* 邀请横幅 */
.invite-banner {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 12px 14px;
  margin-bottom: 20px;
  border-radius: 8px;
  background: #eaf1ff;
  border: 1px solid #cfdefb;
  color: #325994;
}
.invite-banner__icon { margin-top: 2px; font-size: 20px; flex-shrink: 0; color: #6f8fc4; }
.invite-banner__text { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
.invite-banner__title { font-size: 14px; font-weight: 600; line-height: 1.4; }
.invite-banner__hint { font-size: 12px; color: #6b7fa8; line-height: 1.5; }
.invite-banner--error { background: #fdecec; border-color: #f7c7c7; color: #c93939; font-size: 14px; }

/* 表单头部 */
.form-header { text-align: center; margin-bottom: 28px; }
.form-title {
  margin: 0;
  font-size: 28px;
  line-height: 36px;
  font-weight: 700;
  letter-spacing: 0.1em;
  color: #3680ff;
  font-family: var(--app-font-family);
}
.form-welcome { margin: 6px 0 0; font-size: 13px; color: #6b7fa8; line-height: 1.5; }
.form-subtitle { margin: 0; font-size: 13px; color: #6b7fa8; }
.form-hint {
  margin: 10px 0 0;
  padding: 8px 12px;
  border-radius: 6px;
  background: rgba(54, 128, 255, 0.08);
  color: #2d65f5;
  font-size: 12.5px;
  line-height: 1.5;
}

/* 表单主体 / 输入框 */
.form-content {
  :deep(.t-form-item__label) { display: none; }
  :deep(.t-form-item) { margin-bottom: 18px; }
  :deep(.t-form-item__control) { width: 100%; }
  :deep(.t-input) {
    height: 50px;
    border-radius: 4px;
    background: #dee6f8;
    border: 1px solid #dee6f8;
    box-shadow: none;
    transition: all 0.2s ease;

    &:hover { border-color: #b9cdf4; }
    &:focus-within {
      border-color: #3680ff;
      box-shadow: 0 0 0 3px rgba(54, 128, 255, 0.12);
    }

    .t-input__inner {
      border: none !important;
      box-shadow: none !important;
      outline: none !important;
      background: transparent;
      font-size: 16px;
      color: #325994;
      font-family: var(--app-font-family);

      &::placeholder { color: #9cadc7; }
      &:focus { border: none !important; box-shadow: none !important; outline: none !important; }
    }
    .t-input__prefix { color: #7d9fd6; }
  }
}

.input-prefix-icon {
  width: 16px;
  height: 16px;
  display: block;
  object-fit: contain;
  pointer-events: none;
}

/* 登录 / 注册按钮 */
.submit-button {
  height: 50px;
  border-radius: 4px;
  font-size: 16px;
  font-weight: 600;
  letter-spacing: 0.1em;
  margin-top: 4px;
  background: #3680ff !important;
  border-color: #3680ff !important;
  box-shadow: 0 0 8px 0 rgba(54, 128, 255, 0.8);
  font-family: var(--app-font-family);

  &:hover { background: #2d65f5 !important; border-color: #2d65f5 !important; }
}

/* 注册入口 */
.register-cta { margin-top: 10px; }
.register-cta__divider {
  position: relative;
  text-align: center;
  margin: 4px 0 14px;
  color: #7d90b3;
  font-size: 13px;

  span { position: relative; z-index: 1; padding: 0 12px; background: rgba(255, 255, 255, 0.75); }
  &::before {
    content: "";
    position: absolute;
    left: 0; right: 0; top: 50%;
    border-top: 1px solid #d5e0f5;
  }
}
.register-cta__button { height: 44px; border-radius: 4px; font-size: 15px; border-color: #3680ff; color: #3680ff; }

.oidc-divider {
  position: relative;
  margin: 14px 0 8px;
  text-align: center;
  color: #8ea0bf;
  font-size: 12px;
  span { position: relative; z-index: 1; padding: 0 12px; background: rgba(255, 255, 255, 0.75); }
  &::before { content: ""; position: absolute; left: 0; right: 0; top: 50%; border-top: 1px solid #d5e0f5; }
}
.oidc-button { height: 44px; border-radius: 4px; font-size: 15px; }

.form-footer {
  text-align: center;
  font-size: 13px;
  color: #6b7fa8;
  margin-top: 18px;
  .link-button { color: #3680ff; text-decoration: none; margin-left: 4px; font-weight: 500; }
  .link-button:hover { text-decoration: underline; }
}

/* ---------- 底部版权 ---------- */
.el-login-footer {
  position: fixed;
  bottom: 0;
  left: 0;
  right: 0;
  text-align: center;
  color: #9cadc7;
  font-family: Arial, sans-serif;
  font-size: 14px;
  letter-spacing: 1px;
  padding: 10px 0;
}

/* ---------- 响应式 ---------- */
@media (max-width: 1024px) {
  .login-layout { padding-left: 24px; padding-right: 8%; }
  .brand-title { font-size: 36px; }
}

@media (max-width: 768px) {
  .login-layout {
    flex-direction: column;
    padding-top: 64px;
    padding-left: 24px;
    padding-right: 24px;
    padding-bottom: 60px;
    justify-content: center;
    overflow-y: auto;
  }
  .form-section { order: -1; width: 100%; max-width: 456px; }
  .brand-title { font-size: 30px; letter-spacing: 4px; }
  .form-card { padding: 28px 26px 30px; }
}

@media (max-width: 480px) {
  .header-links {
    top: 14px;
    right: 14px;
    gap: 6px;
    .link-text { display: none; }
    .header-link { padding: 7px 10px; }
  }
  .brand-title { font-size: 25px; }
  .form-card { padding: 24px 20px 26px; }
  .form-title { font-size: 24px; }
  .el-login-footer { font-size: 11px; letter-spacing: 0; }
}
</style>
