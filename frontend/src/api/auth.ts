/**
 * Auth API endpoints
 * [LITE] Simplified - Admin login only, no registration/OAuth
 */

import { apiClient } from './client'
import type {
  LoginRequest,
  AuthResponse,
  PublicSettings,
  TotpLoginResponse
} from '@/types'

export type LoginResponse = AuthResponse | TotpLoginResponse

/**
 * Login with email and password
 */
export async function login(credentials: LoginRequest): Promise<LoginResponse> {
  const { data } = await apiClient.post<LoginResponse>('/auth/login', credentials)
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
  const { data } = await apiClient.post<AuthResponse>('/auth/login/2fa', {
    temp_token: tempToken,
    totp_code: code
  })
  return data
}

/**
 * [LITE] Check if TOTP 2FA is required from login response
 */
export function isTotp2FARequired(response: LoginResponse): response is TotpLoginResponse {
  return 'requires_2fa' in response && response.requires_2fa === true
}

export const authAPI = {
  login,
  getCurrentUser,
  getPublicSettings,
  logout,
  refreshToken,
  login2FA,
  isTotp2FARequired
}

export default authAPI
