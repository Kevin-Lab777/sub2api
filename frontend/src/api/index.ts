/**
 * API Client for Sub2API Backend
 * [LITE] Simplified - Admin only
 */

// Re-export the HTTP client
export { apiClient } from './client'

// Auth API (Admin login only)
export { authAPI } from './auth'

// Admin APIs
export { adminAPI } from './admin'

// [LITE] API Keys management for Admin
export { keysAPI } from './keys'
export { usageAPI } from './usage'
export { userGroupsAPI } from './groups'

// Default export
export { default } from './client'
