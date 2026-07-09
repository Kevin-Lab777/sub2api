/**
 * Authentication Store
 * Manages user authentication state, login/logout, token refresh, and token persistence
 * [LITE] Simplified - Admin API Key auth only
 */

import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { authAPI, isTotp2FARequired } from '@/api'
import type { User, LoginRequest, AuthResponse, TotpLoginResponse } from '@/types'

const AUTH_TOKEN_KEY = 'auth_token'
const AUTH_USER_KEY = 'auth_user'
const REFRESH_TOKEN_KEY = 'refresh_token'
const TOKEN_EXPIRES_AT_KEY = 'token_expires_at' // 存储过期时间戳而非有效期
const AUTO_REFRESH_INTERVAL = 60 * 1000 // 60 seconds for user data refresh
const TOKEN_REFRESH_BUFFER = 120 * 1000 // 120 seconds before expiry to refresh token

type LoginResponse = AuthResponse | TotpLoginResponse

export const useAuthStore = defineStore('auth', () => {
  // ==================== State ====================

  const user = ref<User | null>(null)
  const token = ref<string | null>(null)
  const refreshTokenValue = ref<string | null>(null)
  const tokenExpiresAt = ref<number | null>(null) // 过期时间戳（毫秒）
  // [LITE] runMode removed - always Lite mode
  let refreshIntervalId: ReturnType<typeof setInterval> | null = null
  let tokenRefreshTimeoutId: ReturnType<typeof setTimeout> | null = null

  // ==================== Computed ====================

  const isAuthenticated = computed(() => {
    return !!token.value && !!user.value
  })

  const isAdmin = computed(() => {
    return user.value?.role === 'admin'
  })

  // [LITE] Always Lite mode - hide user management, redeem, promo
  const isLiteMode = computed(() => true)
  const isSimpleMode = computed(() => true)

  // ==================== Actions ====================

  /**
   * Initialize auth state from localStorage
   * Call this on app startup to restore session
   * Also starts auto-refresh and immediately fetches latest user data
   */
  function checkAuth(): void {
    const savedToken = localStorage.getItem(AUTH_TOKEN_KEY)
    const savedUser = localStorage.getItem(AUTH_USER_KEY)
    const savedRefreshToken = localStorage.getItem(REFRESH_TOKEN_KEY)
    const savedExpiresAt = localStorage.getItem(TOKEN_EXPIRES_AT_KEY)

    if (savedToken && savedUser) {
      try {
        token.value = savedToken
        user.value = JSON.parse(savedUser)
        refreshTokenValue.value = savedRefreshToken
        tokenExpiresAt.value = savedExpiresAt ? parseInt(savedExpiresAt, 10) : null

        // Immediately refresh user data from backend (async, don't block)
        refreshUser().catch((error) => {
          console.error('Failed to refresh user on init:', error)
        })

        // Start auto-refresh interval for user data
        startAutoRefresh()

        // Start proactive token refresh if we have refresh token and expiry info
        // Note: use !== null to handle case when tokenExpiresAt.value is 0 (expired)
        if (savedRefreshToken && tokenExpiresAt.value !== null) {
          scheduleTokenRefreshAt(tokenExpiresAt.value)
        }
      } catch (error) {
        console.error('Failed to parse saved user data:', error)
        clearAuth()
      }
    }
  }

  /**
   * Start auto-refresh interval for user data
   * Refreshes user data every 60 seconds
   */
  function startAutoRefresh(): void {
    // Clear existing interval if any
    stopAutoRefresh()

    refreshIntervalId = setInterval(() => {
      if (token.value) {
        refreshUser().catch((error) => {
          console.error('Auto-refresh user failed:', error)
        })
      }
    }, AUTO_REFRESH_INTERVAL)
  }

  /**
   * Stop auto-refresh interval
   */
  function stopAutoRefresh(): void {
    if (refreshIntervalId) {
      clearInterval(refreshIntervalId)
      refreshIntervalId = null
    }
  }

  /**
   * Schedule proactive token refresh before expiry (based on expiry timestamp)
   * @param expiresAtMs - Token expiry timestamp in milliseconds
   */
  function scheduleTokenRefreshAt(expiresAtMs: number): void {
    // Clear any existing timeout
    if (tokenRefreshTimeoutId) {
      clearTimeout(tokenRefreshTimeoutId)
      tokenRefreshTimeoutId = null
    }

    // Calculate remaining time until refresh (buffer time before expiry)
    const now = Date.now()
    const refreshInMs = Math.max(0, expiresAtMs - now - TOKEN_REFRESH_BUFFER)

    if (refreshInMs <= 0) {
      // Token is about to expire or already expired, refresh immediately
      performTokenRefresh()
      return
    }

    tokenRefreshTimeoutId = setTimeout(() => {
      performTokenRefresh()
    }, refreshInMs)
  }

  /**
   * Schedule proactive token refresh before expiry (based on expires_in seconds)
   * @param expiresInSeconds - Token expiry time in seconds from now
   */
  function scheduleTokenRefresh(expiresInSeconds: number): void {
    const expiresAtMs = Date.now() + expiresInSeconds * 1000
    tokenExpiresAt.value = expiresAtMs
    localStorage.setItem(TOKEN_EXPIRES_AT_KEY, String(expiresAtMs))
    scheduleTokenRefreshAt(expiresAtMs)
  }

  /**
   * Perform the actual token refresh
   */
  async function performTokenRefresh(): Promise<void> {
    if (!refreshTokenValue.value) {
      return
    }

    try {
      const response = await authAPI.refreshToken()

      // Update state
      token.value = response.access_token
      refreshTokenValue.value = response.refresh_token ?? null

      // Schedule next refresh (this also updates tokenExpiresAt and localStorage)
      if (response.expires_in) {
        scheduleTokenRefresh(response.expires_in)
      }
    } catch (error) {
      console.error('Token refresh failed:', error)
      // Don't clear auth here - the interceptor will handle 401 errors
    }
  }

  /**
   * Stop token refresh timeout
   */
  function stopTokenRefresh(): void {
    if (tokenRefreshTimeoutId) {
      clearTimeout(tokenRefreshTimeoutId)
      tokenRefreshTimeoutId = null
    }
  }

  /**
   * [LITE] Admin login
   * @param credentials - Login credentials (email and password)
   * @returns Promise resolving to the login response
   * @throws Error if login fails
   */
  async function login(credentials: LoginRequest): Promise<LoginResponse> {
    try {
      const response = await authAPI.login(credentials)

      if (isTotp2FARequired(response)) {
        return response
      }

      // Set auth state from the response
      setAuthFromResponse(response)

      return response
    } catch (error) {
      // Clear any partial state on error
      clearAuth()
      throw error
    }
  }

  /**
   * Set auth state from an AuthResponse
   * Internal helper function
   */
  function setAuthFromResponse(response: AuthResponse): void {
    // Store token and user
    token.value = response.access_token

    // Store refresh token if present
    if (response.refresh_token) {
      refreshTokenValue.value = response.refresh_token
      localStorage.setItem(REFRESH_TOKEN_KEY, response.refresh_token)
    }

    user.value = response.user

    // Persist to localStorage
    localStorage.setItem(AUTH_TOKEN_KEY, response.access_token)
    localStorage.setItem(AUTH_USER_KEY, JSON.stringify(response.user))

    // Start auto-refresh interval for user data
    startAutoRefresh()

    // Start proactive token refresh if we have refresh token and expiry info
    if (response.refresh_token && response.expires_in) {
      scheduleTokenRefresh(response.expires_in)
    }
  }

  /**
   * User logout
   * Clears all authentication state and persisted data
   */
  async function logout(): Promise<void> {
    // Call API logout (revokes refresh token on server)
    await authAPI.logout()

    // Clear state
    clearAuth()
  }

  /**
   * Refresh current user data
   * Fetches latest user info from the server
   * @returns Promise resolving to the updated user
   * @throws Error if not authenticated or request fails
   */
  async function refreshUser(): Promise<User> {
    if (!token.value) {
      throw new Error('Not authenticated')
    }

    try {
      const response = await authAPI.getCurrentUser()
      user.value = response.data

      // Update localStorage
      localStorage.setItem(AUTH_USER_KEY, JSON.stringify(response.data))

      return response.data
    } catch (error) {
      // If refresh fails with 401, clear auth state
      if ((error as { status?: number }).status === 401) {
        clearAuth()
      }
      throw error
    }
  }

  /**
   * Clear all authentication state
   * Internal helper function
   */
  function clearAuth(): void {
    // Stop auto-refresh
    stopAutoRefresh()
    // Stop token refresh
    stopTokenRefresh()

    token.value = null
    refreshTokenValue.value = null
    tokenExpiresAt.value = null
    user.value = null
    localStorage.removeItem(AUTH_TOKEN_KEY)
    localStorage.removeItem(AUTH_USER_KEY)
    localStorage.removeItem(REFRESH_TOKEN_KEY)
    localStorage.removeItem(TOKEN_EXPIRES_AT_KEY)
  }

  /**
   * [LITE] 2FA Login - verify TOTP code
   * @param tempToken - Temporary token from initial login
   * @param code - TOTP verification code
   * @returns Promise resolving to the authenticated user
   */
  async function login2FA(tempToken: string, code: string): Promise<User> {
    try {
      const response = await authAPI.login2FA(tempToken, code)
      setAuthFromResponse(response)
      return user.value!
    } catch (error) {
      clearAuth()
      throw error
    }
  }

  // ==================== Return Store API ====================

  return {
    // State
    user,
    token,

    // Computed
    isAuthenticated,
    isAdmin,
    isLiteMode,
    isSimpleMode,

    // Actions
    login,
    login2FA,
    logout,
    checkAuth,
    refreshUser
  }
})
