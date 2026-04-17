/**
 * Auth API endpoints
 * [LITE] Simplified - Admin login only, no registration/OAuth
 */

import { apiClient } from './client'
import type { LoginRequest, AuthResponse, PublicSettings } from '@/types'

/**
 * Login with email and password
 */
export async function login(credentials: LoginRequest): Promise<AuthResponse> {
  const { data } = await apiClient.post<AuthResponse>('/auth/login', credentials)
  return data
}

/**
 * Get current authenticated user
 */
export async function getCurrentUser() {
  return apiClient.get<import('@/types').User>('/auth/me')
}

/**
 * Get public settings (no auth required)
 */
export async function getPublicSettings(): Promise<PublicSettings> {
  const { data } = await apiClient.get<PublicSettings>('/settings/public')
  return data
}

/**
 * [LITE] Logout - clear local state only (no server endpoint)
 */
export async function logout(): Promise<void> {
  // Lite mode: no server-side logout endpoint
}

/**
 * [LITE] Refresh token - stub (no server endpoint)
 */
export async function refreshToken(): Promise<AuthResponse> {
  throw new Error('Token refresh not supported in Lite mode')
}

/**
 * [LITE] 2FA Login - verify TOTP code
 */
export async function login2FA(tempToken: string, code: string): Promise<AuthResponse> {
  const { data } = await apiClient.post<AuthResponse>('/auth/2fa/verify', {
    temp_token: tempToken,
    code
  })
  return data
}

/**
 * [LITE] Check if TOTP 2FA is required from login response
 */
export function isTotp2FARequired(response: any): boolean {
  return response?.requires_2fa === true
}

/**
 * Complete OIDC OAuth registration by supplying an invitation code
 */
export async function completeOIDCOAuthRegistration(
  pendingOAuthToken: string,
  invitationCode: string
): Promise<{ access_token: string; refresh_token: string; expires_in: number; token_type: string }> {
  const { data } = await apiClient.post<{
    access_token: string
    refresh_token: string
    expires_in: number
    token_type: string
  }>('/auth/oauth/oidc/complete-registration', {
    pending_oauth_token: pendingOAuthToken,
    invitation_code: invitationCode
  })
  return data
}

export const authAPI = {
  login,
  getCurrentUser,
  getPublicSettings,
  logout,
  refreshToken,
  login2FA,
  isTotp2FARequired,
  completeOIDCOAuthRegistration
}

export default authAPI
