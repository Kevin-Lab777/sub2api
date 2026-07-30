<template>
  <AppLayout>
    <div class="mx-auto max-w-6xl space-y-6">
      <nav class="settings-tabs" role="tablist" :aria-label="t('admin.settings.title')">
        <button
          v-for="tab in tabs"
          :key="tab.key"
          type="button"
          role="tab"
          class="settings-tab"
          :data-testid="`settings-tab-${tab.key}`"
          :class="activeTab === tab.key && 'settings-tab-active'"
          :aria-selected="activeTab === tab.key"
          @click="activeTab = tab.key"
        >
          <Icon :name="tab.icon" size="sm" />
          <span>{{ t(`admin.settings.tabs.${tab.key}`) }}</span>
        </button>
      </nav>

      <div v-if="loading" class="flex min-h-64 items-center justify-center">
        <div class="h-8 w-8 animate-spin rounded-full border-2 border-gray-200 border-b-primary-600"></div>
      </div>

      <div v-else-if="loadError" class="card flex min-h-56 flex-col items-center justify-center gap-4 p-8 text-center">
        <Icon name="exclamationCircle" size="lg" class="text-red-500" />
        <p class="text-sm text-gray-600 dark:text-gray-300">{{ loadError }}</p>
        <button type="button" class="btn btn-secondary btn-sm" @click="loadSettings">
          <Icon name="refresh" size="sm" />
          {{ t('common.refresh') }}
        </button>
      </div>

      <template v-else>
        <div v-show="activeTab === 'gateway'" class="grid grid-cols-1 gap-6 xl:grid-cols-2">
          <section class="card p-6">
            <SettingHeader
              :title="t('admin.settings.overloadCooldown.title')"
              :description="t('admin.settings.overloadCooldown.description')"
            />
            <ToggleRow
              v-model="overload.enabled"
              :label="t('admin.settings.overloadCooldown.enabled')"
              :hint="t('admin.settings.overloadCooldown.enabledHint')"
            />
            <NumberField
              v-if="overload.enabled"
              v-model="overload.cooldown_minutes"
              :label="t('admin.settings.overloadCooldown.cooldownMinutes')"
              :hint="t('admin.settings.overloadCooldown.cooldownMinutesHint')"
              :min="1"
              :max="120"
            />
            <SaveButton test-id="save-overload" :saving="saving.overload" @click="saveOverload" />
          </section>

          <section class="card p-6">
            <SettingHeader
              :title="t('admin.settings.rateLimit429Cooldown.title')"
              :description="t('admin.settings.rateLimit429Cooldown.description')"
            />
            <ToggleRow
              v-model="rateLimit.enabled"
              :label="t('admin.settings.rateLimit429Cooldown.enabled')"
              :hint="t('admin.settings.rateLimit429Cooldown.enabledHint')"
            />
            <NumberField
              v-if="rateLimit.enabled"
              v-model="rateLimit.cooldown_seconds"
              :label="t('admin.settings.rateLimit429Cooldown.cooldownSeconds')"
              :hint="t('admin.settings.rateLimit429Cooldown.cooldownSecondsHint')"
              :min="1"
              :max="7200"
            />
            <SaveButton test-id="save-rate-limit" :saving="saving.rateLimit" @click="saveRateLimit" />
          </section>

          <section class="card p-6 xl:col-span-2">
            <SettingHeader
              :title="t('admin.settings.streamTimeout.title')"
              :description="t('admin.settings.streamTimeout.description')"
            />
            <ToggleRow
              v-model="streamTimeout.enabled"
              :label="t('admin.settings.streamTimeout.enabled')"
              :hint="t('admin.settings.streamTimeout.enabledHint')"
            />
            <div v-if="streamTimeout.enabled" class="mt-5 grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
              <label class="field-label">
                <span>{{ t('admin.settings.streamTimeout.action') }}</span>
                <select v-model="streamTimeout.action" class="input w-full">
                  <option value="temp_unsched">{{ t('admin.settings.streamTimeout.actionTempUnsched') }}</option>
                  <option value="error">{{ t('admin.settings.streamTimeout.actionError') }}</option>
                  <option value="none">{{ t('admin.settings.streamTimeout.actionNone') }}</option>
                </select>
              </label>
              <NumberField
                v-model="streamTimeout.temp_unsched_minutes"
                :label="t('admin.settings.streamTimeout.tempUnschedMinutes')"
                :min="1"
                :max="60"
              />
              <NumberField
                v-model="streamTimeout.threshold_count"
                :label="t('admin.settings.streamTimeout.thresholdCount')"
                :min="1"
                :max="10"
              />
              <NumberField
                v-model="streamTimeout.threshold_window_minutes"
                :label="t('admin.settings.streamTimeout.thresholdWindowMinutes')"
                :min="1"
                :max="60"
              />
            </div>
            <SaveButton test-id="save-stream-timeout" :saving="saving.streamTimeout" @click="saveStreamTimeout" />
          </section>

          <section class="card p-6 xl:col-span-2">
            <SettingHeader
              :title="t('admin.settings.rectifier.title')"
              :description="t('admin.settings.rectifier.description')"
            />
            <div class="grid grid-cols-1 gap-x-8 md:grid-cols-2">
              <ToggleRow v-model="rectifier.enabled" :label="t('admin.settings.rectifier.enabled')" :hint="t('admin.settings.rectifier.enabledHint')" />
              <ToggleRow v-model="rectifier.thinking_signature_enabled" :label="t('admin.settings.rectifier.thinkingSignature')" :hint="t('admin.settings.rectifier.thinkingSignatureHint')" />
              <ToggleRow v-model="rectifier.thinking_budget_enabled" :label="t('admin.settings.rectifier.thinkingBudget')" :hint="t('admin.settings.rectifier.thinkingBudgetHint')" />
              <ToggleRow v-model="rectifier.apikey_signature_enabled" :label="t('admin.settings.rectifier.apikeySignature')" :hint="t('admin.settings.rectifier.apikeySignatureHint')" />
            </div>
            <label class="field-label mt-5">
              <span>{{ t('admin.settings.rectifier.apikeyPatterns') }}</span>
              <textarea v-model="rectifierPatterns" rows="4" class="input w-full font-mono text-sm" />
              <small>{{ t('admin.settings.rectifier.apikeyPatternsHint') }}</small>
            </label>
            <SaveButton test-id="save-rectifier" :saving="saving.rectifier" @click="saveRectifier" />
          </section>

          <section class="card p-6 xl:col-span-2">
            <div class="flex flex-wrap items-start justify-between gap-3">
              <SettingHeader :title="t('admin.settings.betaPolicy.title')" :description="t('admin.settings.betaPolicy.description')" />
              <button type="button" class="btn btn-secondary btn-sm" @click="addBetaRule">
                <Icon name="plus" size="sm" />
                {{ t('common.add') }}
              </button>
            </div>
            <div v-if="betaPolicy.rules.length" class="mt-5 space-y-3">
              <div v-for="(rule, index) in betaPolicy.rules" :key="index" class="rule-row">
                <input v-model="rule.beta_token" class="input min-w-0 flex-1" placeholder="context-1m-*" />
                <select v-model="rule.action" class="input w-full sm:w-40">
                  <option value="pass">{{ t('admin.settings.betaPolicy.actionPass') }}</option>
                  <option value="filter">{{ t('admin.settings.betaPolicy.actionFilter') }}</option>
                  <option value="block">{{ t('admin.settings.betaPolicy.actionBlock') }}</option>
                </select>
                <select v-model="rule.scope" class="input w-full sm:w-40">
                  <option value="all">{{ t('admin.settings.betaPolicy.scopeAll') }}</option>
                  <option value="oauth">{{ t('admin.settings.betaPolicy.scopeOAuth') }}</option>
                  <option value="apikey">{{ t('admin.settings.betaPolicy.scopeAPIKey') }}</option>
                  <option value="bedrock">{{ t('admin.settings.betaPolicy.scopeBedrock') }}</option>
                </select>
                <button type="button" class="icon-button text-red-500" :title="t('common.delete')" @click="betaPolicy.rules.splice(index, 1)">
                  <Icon name="trash" size="sm" />
                </button>
              </div>
            </div>
            <SaveButton test-id="save-beta-policy" :saving="saving.betaPolicy" @click="saveBetaPolicy" />
          </section>

          <section class="card p-6 xl:col-span-2">
            <div class="flex flex-wrap items-start justify-between gap-3">
              <SettingHeader
                :title="t('admin.settings.webSearchEmulation.title')"
                :description="t('admin.settings.webSearchEmulation.description')"
              />
              <button type="button" class="btn btn-secondary btn-sm" @click="addWebSearchProvider">
                <Icon name="plus" size="sm" />
                {{ t('admin.settings.webSearchEmulation.addProvider') }}
              </button>
            </div>
            <ToggleRow
              v-model="webSearch.enabled"
              :label="t('admin.settings.webSearchEmulation.enabled')"
              :hint="t('admin.settings.webSearchEmulation.enabledHint')"
            />
            <div class="mt-4 space-y-3">
              <div v-for="(provider, index) in webSearch.providers" :key="index" class="rule-row">
                <select v-model="provider.type" class="input w-full sm:w-36">
                  <option value="brave">Brave</option>
                  <option value="tavily">Tavily</option>
                </select>
                <input v-model="provider.api_key" type="password" class="input min-w-0 flex-1" :placeholder="provider.api_key_configured ? '********' : t('admin.settings.webSearchEmulation.apiKeyPlaceholder')" />
                <input v-model.number="provider.quota_limit" type="number" min="0" class="input w-full sm:w-36" :placeholder="t('admin.settings.webSearchEmulation.quotaLimit')" />
                <input v-model.number="provider.proxy_id" type="number" min="1" class="input w-full sm:w-28" :placeholder="t('admin.settings.webSearchEmulation.proxy')" />
                <button type="button" class="icon-button text-red-500" :title="t('common.delete')" @click="webSearch.providers.splice(index, 1)">
                  <Icon name="trash" size="sm" />
                </button>
              </div>
            </div>
            <SaveButton test-id="save-web-search" :saving="saving.webSearch" @click="saveWebSearch" />
          </section>
        </div>

        <div v-show="activeTab === 'security'" class="grid grid-cols-1 gap-6 xl:grid-cols-2">
          <section class="card p-6">
            <SettingHeader :title="t('admin.settings.adminApiKey.title')" :description="t('admin.settings.adminApiKey.description')" />
            <div class="mt-5 flex flex-wrap items-center justify-between gap-3">
              <code v-if="adminKey.exists" class="rounded bg-gray-100 px-3 py-2 text-sm dark:bg-dark-700">{{ adminKey.masked_key }}</code>
              <span v-else class="text-sm text-gray-500">{{ t('admin.settings.adminApiKey.notConfigured') }}</span>
              <div class="flex gap-2">
                <button type="button" class="btn btn-primary btn-sm" data-testid="regenerate-admin-key" :disabled="saving.adminKey" @click="regenerateAdminKey">
                  <Icon name="refresh" size="sm" />
                  {{ adminKey.exists ? t('admin.settings.adminApiKey.regenerate') : t('admin.settings.adminApiKey.create') }}
                </button>
                <button v-if="adminKey.exists" type="button" class="icon-button text-red-500" :title="t('common.delete')" @click="showDeleteKey = true">
                  <Icon name="trash" size="sm" />
                </button>
              </div>
            </div>
            <div v-if="newAdminKey" class="mt-5 rounded border border-green-300 bg-green-50 p-4 dark:border-green-800 dark:bg-green-900/20">
              <div class="flex items-center gap-2">
                <code class="min-w-0 flex-1 break-all text-sm">{{ newAdminKey }}</code>
                <button type="button" class="icon-button" :title="t('common.copy')" @click="copyAdminKey">
                  <Icon name="copy" size="sm" />
                </button>
              </div>
            </div>
          </section>

          <section class="card p-6">
            <SettingHeader :title="t('admin.settings.panelRateLimit.title')" :description="t('admin.settings.panelRateLimit.description')" />
            <ToggleRow v-model="panelRate.enabled" :label="t('admin.settings.panelRateLimit.enabled')" :hint="t('admin.settings.panelRateLimit.enabledHint')" />
            <p class="mt-4 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.panelRateLimit.proxySafeNote') }}</p>
            <div class="mt-5 grid grid-cols-2 gap-4">
              <NumberField v-model="panelRate.user_rpm" test-id="panel-rate-user-rpm" :label="t('admin.settings.panelRateLimit.userRpm')" :hint="t('admin.settings.panelRateLimit.userRpmHint')" :min="0" />
              <NumberField v-model="panelRate.heavy_rpm" test-id="panel-rate-heavy-rpm" :label="t('admin.settings.panelRateLimit.heavyRpm')" :hint="t('admin.settings.panelRateLimit.heavyRpmHint')" :min="0" />
              <NumberField v-model="panelRate.public_ip_rpm" test-id="panel-rate-public-ip-rpm" :label="t('admin.settings.panelRateLimit.publicIpRpm')" :hint="t('admin.settings.panelRateLimit.publicIpRpmHint')" :min="0" />
              <ToggleRow v-model="panelRate.exempt_admin" :label="t('admin.settings.panelRateLimit.exemptAdmin')" :hint="t('admin.settings.panelRateLimit.exemptAdminHint')" />
            </div>
            <SaveButton test-id="save-panel-rate" :saving="saving.panelRate" @click="savePanelRate" />
          </section>
        </div>

        <div v-show="activeTab === 'backup'">
          <BackupView />
        </div>
      </template>
    </div>

    <ConfirmDialog
      :show="showDeleteKey"
      :title="t('admin.settings.adminApiKey.delete')"
      :message="t('admin.settings.adminApiKey.securityWarning')"
      :confirm-text="t('common.delete')"
      danger
      @confirm="deleteAdminKey"
      @cancel="showDeleteKey = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { defineComponent, h, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Toggle from '@/components/common/Toggle.vue'
import { Icon } from '@/components/icons'
import BackupView from '@/views/admin/BackupView.vue'
import { adminAPI } from '@/api'
import type {
  AdminApiKeyStatus,
  BetaPolicyRule,
  BetaPolicySettings,
  OverloadCooldownSettings,
  PanelRateLimitSettings,
  RateLimit429CooldownSettings,
  RectifierSettings,
  StreamTimeoutSettings,
  WebSearchEmulationConfig,
  WebSearchProviderConfig,
} from '@/api/admin/settings'
import { useAppStore } from '@/stores'
import { useClipboard } from '@/composables/useClipboard'
import { extractApiErrorMessage } from '@/utils/apiError'

type SettingsTab = 'gateway' | 'security' | 'backup'
type SavingKey = 'overload' | 'rateLimit' | 'streamTimeout' | 'rectifier' | 'betaPolicy' | 'webSearch' | 'panelRate' | 'adminKey'

const SettingHeader = defineComponent({
  props: { title: { type: String, required: true }, description: { type: String, required: true } },
  setup: (props) => () => h('div', [
    h('h2', { class: 'text-base font-semibold text-gray-900 dark:text-white' }, props.title),
    h('p', { class: 'mt-1 text-sm text-gray-500 dark:text-gray-400' }, props.description),
  ]),
})
const ToggleRow = defineComponent({
  props: { modelValue: { type: Boolean, required: true }, label: { type: String, required: true }, hint: { type: String, required: true } },
  emits: ['update:modelValue'],
  setup: (props, { emit }) => () => h('div', { class: 'mt-5 flex items-center justify-between gap-4' }, [
    h('div', [h('p', { class: 'text-sm font-medium text-gray-900 dark:text-white' }, props.label), props.hint ? h('p', { class: 'mt-1 text-xs text-gray-500 dark:text-gray-400' }, props.hint) : null]),
    h(Toggle, { modelValue: props.modelValue, 'onUpdate:modelValue': (value: boolean) => emit('update:modelValue', value) }),
  ]),
})
const NumberField = defineComponent({
  props: { modelValue: { type: Number, required: true }, label: { type: String, required: true }, hint: { type: String, default: '' }, testId: { type: String, default: '' }, min: Number, max: Number },
  emits: ['update:modelValue'],
  setup: (props, { emit }) => () => h('label', { class: 'field-label' }, [
    h('span', props.label),
    h('input', { class: 'input w-full', 'data-testid': props.testId || undefined, type: 'number', min: props.min, max: props.max, value: props.modelValue, onInput: (event: Event) => emit('update:modelValue', Number((event.target as HTMLInputElement).value)) }),
    props.hint ? h('small', props.hint) : null,
  ]),
})
const SaveButton = defineComponent({
  props: { saving: { type: Boolean, required: true }, testId: { type: String, default: '' } },
  emits: ['click'],
  setup: (props, { emit }) => () => h('div', { class: 'mt-6 flex justify-end border-t border-gray-100 pt-4 dark:border-dark-700' }, [
    h('button', { type: 'button', class: 'btn btn-primary btn-sm', 'data-testid': props.testId || undefined, disabled: props.saving, onClick: () => emit('click') }, props.saving ? t('common.saving') : t('common.save')),
  ]),
})

const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()
const tabs = [
  { key: 'gateway' as SettingsTab, icon: 'server' as const },
  { key: 'security' as SettingsTab, icon: 'shield' as const },
  { key: 'backup' as SettingsTab, icon: 'database' as const },
]
const activeTab = ref<SettingsTab>('gateway')
const loading = ref(true)
const loadError = ref('')
const saving = reactive<Record<SavingKey, boolean>>({ overload: false, rateLimit: false, streamTimeout: false, rectifier: false, betaPolicy: false, webSearch: false, panelRate: false, adminKey: false })

const overload = reactive<OverloadCooldownSettings>({ enabled: true, cooldown_minutes: 10 })
const rateLimit = reactive<RateLimit429CooldownSettings>({ enabled: true, cooldown_seconds: 5 })
const streamTimeout = reactive<StreamTimeoutSettings>({ enabled: true, action: 'temp_unsched', temp_unsched_minutes: 5, threshold_count: 3, threshold_window_minutes: 10 })
const rectifier = reactive<RectifierSettings>({ enabled: true, thinking_signature_enabled: true, thinking_budget_enabled: true, apikey_signature_enabled: true, apikey_signature_patterns: [] })
const rectifierPatterns = ref('')
const betaPolicy = reactive<BetaPolicySettings>({ rules: [] })
const webSearch = reactive<WebSearchEmulationConfig>({ enabled: false, providers: [] })
const panelRate = reactive<PanelRateLimitSettings>({ enabled: true, user_rpm: 240, heavy_rpm: 60, exempt_admin: true, public_ip_rpm: 300 })
const adminKey = reactive<AdminApiKeyStatus>({ exists: false, masked_key: '' })
const newAdminKey = ref('')
const showDeleteKey = ref(false)

function assign<T extends object>(target: T, value: T): void { Object.assign(target, value) }
function reportError(error: unknown): void { appStore.showError(extractApiErrorMessage(error, t('errors.networkError'))) }
async function runSave(key: SavingKey, operation: () => Promise<void>): Promise<void> {
  saving[key] = true
  try { await operation(); appStore.showSuccess(t('common.saved')) } catch (error) { reportError(error) } finally { saving[key] = false }
}

async function loadSettings(): Promise<void> {
  loading.value = true
  loadError.value = ''
  try {
    const [overloadValue, rateValue, streamValue, rectifierValue, betaValue, webSearchValue, panelValue, adminKeyValue] = await Promise.all([
      adminAPI.settings.getOverloadCooldownSettings(), adminAPI.settings.getRateLimit429CooldownSettings(), adminAPI.settings.getStreamTimeoutSettings(),
      adminAPI.settings.getRectifierSettings(), adminAPI.settings.getBetaPolicySettings(), adminAPI.settings.getWebSearchEmulationConfig(),
      adminAPI.settings.getPanelRateLimitSettings(), adminAPI.settings.getAdminApiKey(),
    ])
    assign(overload, overloadValue); assign(rateLimit, rateValue); assign(streamTimeout, streamValue); assign(rectifier, rectifierValue)
    rectifierPatterns.value = rectifierValue.apikey_signature_patterns.join('\n')
    betaPolicy.rules = betaValue.rules.map((rule) => ({ ...rule }))
    webSearch.enabled = webSearchValue.enabled; webSearch.providers = webSearchValue.providers.map((provider) => ({ ...provider }))
    assign(panelRate, panelValue); assign(adminKey, adminKeyValue)
  } catch (error) {
    loadError.value = extractApiErrorMessage(error, t('errors.networkError'))
  } finally { loading.value = false }
}

const saveOverload = () => runSave('overload', async () => assign(overload, await adminAPI.settings.updateOverloadCooldownSettings({ ...overload })))
const saveRateLimit = () => runSave('rateLimit', async () => assign(rateLimit, await adminAPI.settings.updateRateLimit429CooldownSettings({ ...rateLimit })))
const saveStreamTimeout = () => runSave('streamTimeout', async () => assign(streamTimeout, await adminAPI.settings.updateStreamTimeoutSettings({ ...streamTimeout })))
const savePanelRate = () => runSave('panelRate', async () => assign(panelRate, await adminAPI.settings.updatePanelRateLimitSettings({ ...panelRate })))
const saveBetaPolicy = () => runSave('betaPolicy', async () => { betaPolicy.rules = (await adminAPI.settings.updateBetaPolicySettings({ rules: betaPolicy.rules.map((rule) => ({ ...rule })) })).rules })
const saveWebSearch = () => runSave('webSearch', async () => { const updated = await adminAPI.settings.updateWebSearchEmulationConfig({ enabled: webSearch.enabled, providers: webSearch.providers.map((provider) => ({ ...provider })) }); webSearch.enabled = updated.enabled; webSearch.providers = updated.providers })
const saveRectifier = () => runSave('rectifier', async () => {
  const patterns = Array.from(new Set(rectifierPatterns.value.split('\n').map((item) => item.trim()).filter(Boolean)))
  const updated = await adminAPI.settings.updateRectifierSettings({ ...rectifier, apikey_signature_patterns: patterns })
  assign(rectifier, updated); rectifierPatterns.value = updated.apikey_signature_patterns.join('\n')
})

function addBetaRule(): void { betaPolicy.rules.push({ beta_token: '', action: 'filter', scope: 'all' } as BetaPolicyRule) }
function addWebSearchProvider(): void {
  webSearch.providers.push({ type: 'brave', api_key: '', api_key_configured: false, quota_limit: null, subscribed_at: null, proxy_id: null, expires_at: null } as WebSearchProviderConfig)
}
async function regenerateAdminKey(): Promise<void> {
  await runSave('adminKey', async () => { const result = await adminAPI.settings.regenerateAdminApiKey(); newAdminKey.value = result.key; assign(adminKey, await adminAPI.settings.getAdminApiKey()) })
}
async function deleteAdminKey(): Promise<void> {
  showDeleteKey.value = false
  await runSave('adminKey', async () => { await adminAPI.settings.deleteAdminApiKey(); assign(adminKey, { exists: false, masked_key: '' }); newAdminKey.value = '' })
}
async function copyAdminKey(): Promise<void> { if (newAdminKey.value) await copyToClipboard(newAdminKey.value) }

onMounted(loadSettings)
</script>

<style scoped>
.settings-tabs { display: flex; gap: 0.25rem; overflow-x: auto; border-bottom: 1px solid rgb(229 231 235); }
.settings-tab { display: inline-flex; min-height: 2.75rem; align-items: center; gap: 0.5rem; border-bottom: 2px solid transparent; padding: 0 1rem; color: rgb(107 114 128); font-size: 0.875rem; font-weight: 500; white-space: nowrap; }
.settings-tab-active { border-color: rgb(var(--color-primary-600)); color: rgb(var(--color-primary-600)); }
.field-label { display: flex; flex-direction: column; gap: 0.375rem; color: rgb(55 65 81); font-size: 0.875rem; font-weight: 500; }
.field-label small { color: rgb(107 114 128); font-size: 0.75rem; font-weight: 400; }
.rule-row { display: flex; flex-wrap: wrap; align-items: center; gap: 0.75rem; border: 1px solid rgb(229 231 235); padding: 0.75rem; border-radius: 0.375rem; }
.icon-button { display: inline-flex; height: 2.25rem; width: 2.25rem; flex: 0 0 auto; align-items: center; justify-content: center; border-radius: 0.375rem; }
.icon-button:hover { background: rgb(243 244 246); }
:global(.dark) .settings-tabs, :global(.dark) .rule-row { border-color: rgb(55 65 81); }
:global(.dark) .field-label { color: rgb(209 213 219); }
:global(.dark) .icon-button:hover { background: rgb(55 65 81); }
</style>
