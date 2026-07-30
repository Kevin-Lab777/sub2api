import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

import SettingsView from '../SettingsView.vue'

const api = vi.hoisted(() => ({
  getOverloadCooldownSettings: vi.fn(),
  updateOverloadCooldownSettings: vi.fn(),
  getRateLimit429CooldownSettings: vi.fn(),
  updateRateLimit429CooldownSettings: vi.fn(),
  getPanelRateLimitSettings: vi.fn(),
  updatePanelRateLimitSettings: vi.fn(),
  getStreamTimeoutSettings: vi.fn(),
  updateStreamTimeoutSettings: vi.fn(),
  getRectifierSettings: vi.fn(),
  updateRectifierSettings: vi.fn(),
  getBetaPolicySettings: vi.fn(),
  updateBetaPolicySettings: vi.fn(),
  getWebSearchEmulationConfig: vi.fn(),
  updateWebSearchEmulationConfig: vi.fn(),
  getAdminApiKey: vi.fn(),
  regenerateAdminApiKey: vi.fn(),
  deleteAdminApiKey: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  copyToClipboard: vi.fn(),
}))

vi.mock('@/api', () => ({
  adminAPI: {
    settings: {
      getOverloadCooldownSettings: api.getOverloadCooldownSettings,
      updateOverloadCooldownSettings: api.updateOverloadCooldownSettings,
      getRateLimit429CooldownSettings: api.getRateLimit429CooldownSettings,
      updateRateLimit429CooldownSettings: api.updateRateLimit429CooldownSettings,
      getPanelRateLimitSettings: api.getPanelRateLimitSettings,
      updatePanelRateLimitSettings: api.updatePanelRateLimitSettings,
      getStreamTimeoutSettings: api.getStreamTimeoutSettings,
      updateStreamTimeoutSettings: api.updateStreamTimeoutSettings,
      getRectifierSettings: api.getRectifierSettings,
      updateRectifierSettings: api.updateRectifierSettings,
      getBetaPolicySettings: api.getBetaPolicySettings,
      updateBetaPolicySettings: api.updateBetaPolicySettings,
      getWebSearchEmulationConfig: api.getWebSearchEmulationConfig,
      updateWebSearchEmulationConfig: api.updateWebSearchEmulationConfig,
      getAdminApiKey: api.getAdminApiKey,
      regenerateAdminApiKey: api.regenerateAdminApiKey,
      deleteAdminApiKey: api.deleteAdminApiKey,
    },
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError: api.showError,
    showSuccess: api.showSuccess,
  }),
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: api.copyToClipboard }),
}))

vi.mock('@/utils/apiError', () => ({
  extractApiErrorMessage: () => 'request failed',
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const ToggleStub = defineComponent({
  props: { modelValue: { type: Boolean, required: true } },
  emits: ['update:modelValue'],
  setup(props, { emit }) {
    return () => h('input', {
      class: 'toggle-stub',
      type: 'checkbox',
      checked: props.modelValue,
      onChange: (event: Event) => emit('update:modelValue', (event.target as HTMLInputElement).checked),
    })
  },
})

const ConfirmDialogStub = defineComponent({
  props: { show: { type: Boolean, required: true } },
  emits: ['confirm', 'cancel'],
  setup(props, { emit }) {
    return () => props.show
      ? h('button', { 'data-testid': 'confirm-delete-admin-key', onClick: () => emit('confirm') }, 'confirm')
      : null
  },
})

function mountView() {
  return mount(SettingsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        BackupView: { template: '<div data-testid="backup-view" />' },
        ConfirmDialog: ConfirmDialogStub,
        Toggle: ToggleStub,
        Icon: true,
      },
    },
  })
}

describe('admin technical settings', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.getOverloadCooldownSettings.mockResolvedValue({ enabled: true, cooldown_minutes: 10 })
    api.updateOverloadCooldownSettings.mockImplementation(async (value) => value)
    api.getRateLimit429CooldownSettings.mockResolvedValue({ enabled: true, cooldown_seconds: 5 })
    api.updateRateLimit429CooldownSettings.mockImplementation(async (value) => value)
    api.getPanelRateLimitSettings.mockResolvedValue({ enabled: true, user_rpm: 240, heavy_rpm: 60, exempt_admin: true, public_ip_rpm: 300 })
    api.updatePanelRateLimitSettings.mockImplementation(async (value) => value)
    api.getStreamTimeoutSettings.mockResolvedValue({ enabled: true, action: 'temp_unsched', temp_unsched_minutes: 5, threshold_count: 3, threshold_window_minutes: 10 })
    api.updateStreamTimeoutSettings.mockImplementation(async (value) => value)
    api.getRectifierSettings.mockResolvedValue({ enabled: true, thinking_signature_enabled: true, thinking_budget_enabled: true, apikey_signature_enabled: true, apikey_signature_patterns: [] })
    api.updateRectifierSettings.mockImplementation(async (value) => value)
    api.getBetaPolicySettings.mockResolvedValue({ rules: [] })
    api.updateBetaPolicySettings.mockImplementation(async (value) => value)
    api.getWebSearchEmulationConfig.mockResolvedValue({ enabled: false, providers: [] })
    api.updateWebSearchEmulationConfig.mockImplementation(async (value) => value)
    api.getAdminApiKey.mockResolvedValue({ exists: true, masked_key: 'sk-admin-****' })
    api.regenerateAdminApiKey.mockResolvedValue({ key: 'sk-admin-new' })
    api.deleteAdminApiKey.mockResolvedValue({ message: 'deleted' })
  })

  it('loads only the eight retained technical setting resources', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(api.getOverloadCooldownSettings).toHaveBeenCalledOnce()
    expect(api.getRateLimit429CooldownSettings).toHaveBeenCalledOnce()
    expect(api.getPanelRateLimitSettings).toHaveBeenCalledOnce()
    expect(api.getStreamTimeoutSettings).toHaveBeenCalledOnce()
    expect(api.getRectifierSettings).toHaveBeenCalledOnce()
    expect(api.getBetaPolicySettings).toHaveBeenCalledOnce()
    expect(api.getWebSearchEmulationConfig).toHaveBeenCalledOnce()
    expect(api.getAdminApiKey).toHaveBeenCalledOnce()
    expect(wrapper.text()).not.toContain('admin.settings.tabs.payment')
    expect(wrapper.text()).not.toContain('admin.settings.tabs.users')
    expect(wrapper.text()).not.toContain('admin.settings.tabs.email')
  })

  it('exposes only gateway, security, and backup tabs', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-testid="settings-tab-gateway"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="settings-tab-security"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="settings-tab-backup"]').exists()).toBe(true)
    expect(wrapper.findAll('[role="tab"]')).toHaveLength(3)

    await wrapper.get('[data-testid="settings-tab-backup"]').trigger('click')
    expect(wrapper.get('[data-testid="backup-view"]').exists()).toBe(true)
  })

  it('updates panel rate limits through the dedicated endpoint', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-testid="settings-tab-security"]').trigger('click')
    await wrapper.get('[data-testid="panel-rate-user-rpm"]').setValue('120')
    await wrapper.get('[data-testid="save-panel-rate"]').trigger('click')
    await flushPromises()

    expect(api.updatePanelRateLimitSettings).toHaveBeenCalledWith({
      enabled: true,
      user_rpm: 120,
      heavy_rpm: 60,
      exempt_admin: true,
      public_ip_rpm: 300,
    })
    expect(api.showSuccess).toHaveBeenCalledWith('common.saved')
  })

  it('normalizes rectifier patterns before saving', async () => {
    const wrapper = mountView()
    await flushPromises()
    const textarea = wrapper.get('textarea')
    await textarea.setValue('signature_error\n\nsignature_error\nthinking_error  ')
    await wrapper.get('[data-testid="save-rectifier"]').trigger('click')
    await flushPromises()

    expect(api.updateRectifierSettings).toHaveBeenCalledWith(expect.objectContaining({
      apikey_signature_patterns: ['signature_error', 'thinking_error'],
    }))
  })

  it('regenerates and deletes the administrator API key', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-testid="settings-tab-security"]').trigger('click')
    await wrapper.get('[data-testid="regenerate-admin-key"]').trigger('click')
    await flushPromises()

    expect(api.regenerateAdminApiKey).toHaveBeenCalledOnce()
    expect(wrapper.text()).toContain('sk-admin-new')

    const deleteButton = wrapper.findAll('button').find((button) => button.attributes('title') === 'common.delete')
    expect(deleteButton).toBeDefined()
    await deleteButton?.trigger('click')
    await wrapper.get('[data-testid="confirm-delete-admin-key"]').trigger('click')
    await flushPromises()

    expect(api.deleteAdminApiKey).toHaveBeenCalledOnce()
    expect(wrapper.text()).toContain('admin.settings.adminApiKey.notConfigured')
  })
})
