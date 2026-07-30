import { apiClient } from '../client'

export interface AdminApiKeyStatus {
  exists: boolean
  masked_key: string
}

export interface OverloadCooldownSettings {
  enabled: boolean
  cooldown_minutes: number
}

export interface RateLimit429CooldownSettings {
  enabled: boolean
  cooldown_seconds: number
}

export interface PanelRateLimitSettings {
  enabled: boolean
  user_rpm: number
  heavy_rpm: number
  exempt_admin: boolean
  public_ip_rpm: number
}

export interface StreamTimeoutSettings {
  enabled: boolean
  action: 'temp_unsched' | 'error' | 'none'
  temp_unsched_minutes: number
  threshold_count: number
  threshold_window_minutes: number
}

export interface RectifierSettings {
  enabled: boolean
  thinking_signature_enabled: boolean
  thinking_budget_enabled: boolean
  apikey_signature_enabled: boolean
  apikey_signature_patterns: string[]
}

export interface BetaPolicyRule {
  beta_token: string
  action: 'pass' | 'filter' | 'block'
  scope: 'all' | 'oauth' | 'apikey' | 'bedrock'
  error_message?: string
  model_whitelist?: string[]
  fallback_action?: 'pass' | 'filter' | 'block'
  fallback_error_message?: string
}

export interface BetaPolicySettings {
  rules: BetaPolicyRule[]
}

export interface WebSearchProviderConfig {
  type: 'brave' | 'tavily'
  api_key: string
  api_key_configured: boolean
  quota_limit: number | null
  subscribed_at: number | null
  quota_used?: number
  proxy_id: number | null
  expires_at: number | null
}

export interface WebSearchEmulationConfig {
  enabled: boolean
  providers: WebSearchProviderConfig[]
}

export async function getAdminApiKey(): Promise<AdminApiKeyStatus> {
  const { data } = await apiClient.get<AdminApiKeyStatus>('/admin/settings/admin-api-key')
  return data
}

export async function regenerateAdminApiKey(): Promise<{ key: string }> {
  const { data } = await apiClient.post<{ key: string }>('/admin/settings/admin-api-key/regenerate')
  return data
}

export async function deleteAdminApiKey(): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>('/admin/settings/admin-api-key')
  return data
}

export async function getOverloadCooldownSettings(): Promise<OverloadCooldownSettings> {
  const { data } = await apiClient.get<OverloadCooldownSettings>('/admin/settings/overload-cooldown')
  return data
}

export async function updateOverloadCooldownSettings(settings: OverloadCooldownSettings): Promise<OverloadCooldownSettings> {
  const { data } = await apiClient.put<OverloadCooldownSettings>('/admin/settings/overload-cooldown', settings)
  return data
}

export async function getRateLimit429CooldownSettings(): Promise<RateLimit429CooldownSettings> {
  const { data } = await apiClient.get<RateLimit429CooldownSettings>('/admin/settings/rate-limit-429-cooldown')
  return data
}

export async function updateRateLimit429CooldownSettings(settings: RateLimit429CooldownSettings): Promise<RateLimit429CooldownSettings> {
  const { data } = await apiClient.put<RateLimit429CooldownSettings>('/admin/settings/rate-limit-429-cooldown', settings)
  return data
}

export async function getPanelRateLimitSettings(): Promise<PanelRateLimitSettings> {
  const { data } = await apiClient.get<PanelRateLimitSettings>('/admin/settings/panel-rate-limit')
  return data
}

export async function updatePanelRateLimitSettings(settings: PanelRateLimitSettings): Promise<PanelRateLimitSettings> {
  const { data } = await apiClient.put<PanelRateLimitSettings>('/admin/settings/panel-rate-limit', settings)
  return data
}

export async function getStreamTimeoutSettings(): Promise<StreamTimeoutSettings> {
  const { data } = await apiClient.get<StreamTimeoutSettings>('/admin/settings/stream-timeout')
  return data
}

export async function updateStreamTimeoutSettings(settings: StreamTimeoutSettings): Promise<StreamTimeoutSettings> {
  const { data } = await apiClient.put<StreamTimeoutSettings>('/admin/settings/stream-timeout', settings)
  return data
}

export async function getRectifierSettings(): Promise<RectifierSettings> {
  const { data } = await apiClient.get<RectifierSettings>('/admin/settings/rectifier')
  return data
}

export async function updateRectifierSettings(settings: RectifierSettings): Promise<RectifierSettings> {
  const { data } = await apiClient.put<RectifierSettings>('/admin/settings/rectifier', settings)
  return data
}

export async function getBetaPolicySettings(): Promise<BetaPolicySettings> {
  const { data } = await apiClient.get<BetaPolicySettings>('/admin/settings/beta-policy')
  return data
}

export async function updateBetaPolicySettings(settings: BetaPolicySettings): Promise<BetaPolicySettings> {
  const { data } = await apiClient.put<BetaPolicySettings>('/admin/settings/beta-policy', settings)
  return data
}

export async function getWebSearchEmulationConfig(): Promise<WebSearchEmulationConfig> {
  const { data } = await apiClient.get<WebSearchEmulationConfig>('/admin/settings/web-search-emulation')
  return data
}

export async function updateWebSearchEmulationConfig(config: WebSearchEmulationConfig): Promise<WebSearchEmulationConfig> {
  const { data } = await apiClient.put<WebSearchEmulationConfig>('/admin/settings/web-search-emulation', config)
  return data
}

export const settingsAPI = {
  getAdminApiKey,
  regenerateAdminApiKey,
  deleteAdminApiKey,
  getOverloadCooldownSettings,
  updateOverloadCooldownSettings,
  getRateLimit429CooldownSettings,
  updateRateLimit429CooldownSettings,
  getPanelRateLimitSettings,
  updatePanelRateLimitSettings,
  getStreamTimeoutSettings,
  updateStreamTimeoutSettings,
  getRectifierSettings,
  updateRectifierSettings,
  getBetaPolicySettings,
  updateBetaPolicySettings,
  getWebSearchEmulationConfig,
  updateWebSearchEmulationConfig,
}

export default settingsAPI
