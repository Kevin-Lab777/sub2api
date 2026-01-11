/**
 * Authentication API endpoints
 * [LITE] Simplified - Admin login only
 */

import { apiClient } from './client'
import type {
  LoginRequest,
  AuthResponse,
  CurrentUserResponse,
  PublicSettings
} from '@/types'

/**
 * Store authentication token in localStorage
 */
export function setAuthToken(token: string): void {
  localStorage.setItem('auth_token', token)
}

/**
 * Get authentication token from localStorage
 */
export function getAuthToken(): string | null {
  return localStorage.getItem('auth_token')
}

/**
 * Clear authentication token from localStorage
 */
export function clearAuthToken(): void {
  localStorage.removeItem('auth_token')
  localStorage.removeItem('auth_user')
}

/**
 * Admin login
 * @param credentials - Email and password
 * @returns Authentication response with token and user data
 */
export async function login(credentials: LoginRequest): Promise<AuthResponse> {
  const { data } = await apiClient.post<AuthResponse>('/auth/login', credentials)

  // Store token and user data
  setAuthToken(data.access_token)
  localStorage.setItem('auth_user', JSON.stringify(data.user))

  return data
}

/**
 * Get current authenticated user
 * @returns User profile data
 */
export async function getCurrentUser() {
  return apiClient.get<CurrentUserResponse>('/auth/me')
}

/**
 * User logout
 * Clears authentication token and user data from localStorage
 */
export function logout(): void {
  clearAuthToken()
}

/**
 * Check if user is authenticated
 * @returns True if user has valid token
 */
export function isAuthenticated(): boolean {
  return getAuthToken() !== null
}

/**
 * Get public settings (no auth required)
 * @returns Public settings
 */
export async function getPublicSettings(): Promise<PublicSettings> {
  const { data } = await apiClient.get<PublicSettings>('/settings/public')
  return data
}

export const authAPI = {
  login,
  getCurrentUser,
  logout,
  isAuthenticated,
  setAuthToken,
  getAuthToken,
  clearAuthToken,
  getPublicSettings
}

export default authAPI
