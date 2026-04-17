/**
 * API Client for Sub2API Backend
 * [LITE] Simplified - Admin only
 */

// Re-export the HTTP client
export { apiClient } from './client'

// Auth API
export { authAPI, isTotp2FARequired } from './auth'

// Payment API (kept in Lite)
export { paymentAPI } from './payment'

// Admin APIs
export { adminAPI } from './admin'

// [LITE] User profile API (for password change)
export { userAPI } from './user'

// [LITE] API Keys management for Admin
export { keysAPI } from './keys'
export { usageAPI } from './usage'
export { userGroupsAPI } from './groups'

// Upstream APIs (compatible with Lite)
export { totpAPI } from './totp'
export { default as announcementsAPI } from './announcements'

// Default export
export { default } from './client'
