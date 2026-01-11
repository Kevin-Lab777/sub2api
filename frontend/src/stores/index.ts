/**
 * Pinia Stores Export
 * [LITE] Simplified - Admin only
 */

export { useAuthStore } from './auth'
export { useAppStore } from './app'
export { useOnboardingStore } from './onboarding'

// Re-export types for convenience
export type { User, LoginRequest, AuthResponse } from '@/types'
export type { Toast, ToastType, AppState } from '@/types'
