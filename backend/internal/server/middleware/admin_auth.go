// Package middleware provides HTTP middleware for authentication, authorization, and request processing.
package middleware

import (
	"crypto/subtle"
	"errors"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// NewAdminAuthMiddleware 创建管理员认证中间件
func NewAdminAuthMiddleware(
	authService *service.AuthService,
	userService *service.UserService,
	settingService *service.SettingService,
) AdminAuthMiddleware {
	return AdminAuthMiddleware(adminAuth(authService, userService, settingService))
}

// adminAuth 管理员认证中间件实现
// 支持三种认证方式：
// 1. x-api-key: <admin-api-key>
// 2. Authorization: Bearer <admin-api-key>
// 3. Authorization: Bearer <jwt-token> (JWT token for admin user)
func adminAuth(
	authService *service.AuthService,
	userService *service.UserService,
	settingService *service.SettingService,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 检查 x-api-key header（Admin API Key 认证）
		apiKey := c.GetHeader("x-api-key")
		if apiKey != "" {
			if !validateAdminAPIKey(c, apiKey, settingService, userService) {
				return
			}
			c.Next()
			return
		}

		// 检查 Authorization: Bearer <token>
		authHeader := c.GetHeader("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token != "" {
				// 先尝试作为 Admin API Key 验证
				if validateAdminAPIKeyNoAbort(c, token, settingService, userService) {
					c.Next()
					return
				}

				// 再尝试作为 JWT token 验证
				if validateJWTForAdmin(c, token, authService, userService) {
					c.Next()
					return
				}

				// 都失败了
				AbortWithError(c, 401, "INVALID_TOKEN", "Invalid token")
				return
			}
		}

		// 无有效认证信息
		AbortWithError(c, 401, "UNAUTHORIZED", "Authorization required")
	}
}

// validateAdminAPIKey 验证管理员 API Key（失败时会 abort）
func validateAdminAPIKey(
	c *gin.Context,
	key string,
	settingService *service.SettingService,
	userService *service.UserService,
) bool {
	storedKey, err := settingService.GetAdminAPIKey(c.Request.Context())
	if err != nil {
		AbortWithError(c, 500, "INTERNAL_ERROR", "Internal server error")
		return false
	}

	if storedKey == "" || subtle.ConstantTimeCompare([]byte(key), []byte(storedKey)) != 1 {
		AbortWithError(c, 401, "INVALID_ADMIN_KEY", "Invalid admin API key")
		return false
	}

	admin, err := userService.GetFirstAdmin(c.Request.Context())
	if err != nil {
		AbortWithError(c, 500, "INTERNAL_ERROR", "No admin user found")
		return false
	}

	c.Set(string(ContextKeyUser), AuthSubject{
		UserID:      admin.ID,
		Concurrency: admin.Concurrency,
	})
	c.Set(string(ContextKeyUserRole), admin.Role)
	c.Set("auth_method", "admin_api_key")
	return true
}

// validateAdminAPIKeyNoAbort 验证管理员 API Key（失败时不 abort，返回 false）
func validateAdminAPIKeyNoAbort(
	c *gin.Context,
	key string,
	settingService *service.SettingService,
	userService *service.UserService,
) bool {
	storedKey, err := settingService.GetAdminAPIKey(c.Request.Context())
	if err != nil {
		return false
	}

	if storedKey == "" || subtle.ConstantTimeCompare([]byte(key), []byte(storedKey)) != 1 {
		return false
	}

	admin, err := userService.GetFirstAdmin(c.Request.Context())
	if err != nil {
		return false
	}

	c.Set(string(ContextKeyUser), AuthSubject{
		UserID:      admin.ID,
		Concurrency: admin.Concurrency,
	})
	c.Set(string(ContextKeyUserRole), admin.Role)
	c.Set("auth_method", "admin_api_key")
	return true
}

// validateJWTForAdmin 验证 JWT token 并检查是否为 admin
func validateJWTForAdmin(
	c *gin.Context,
	tokenString string,
	authService *service.AuthService,
	userService *service.UserService,
) bool {
	// 验证 token
	claims, err := authService.ValidateToken(tokenString)
	if err != nil {
		if errors.Is(err, service.ErrTokenExpired) {
			return false
		}
		return false
	}

	// 获取用户
	user, err := userService.GetByID(c.Request.Context(), claims.UserID)
	if err != nil {
		return false
	}

	// 检查用户状态
	if !user.IsActive() {
		return false
	}

	// 检查 token version
	if claims.TokenVersion != user.TokenVersion {
		return false
	}

	// 检查是否为 admin
	if user.Role != service.RoleAdmin {
		return false
	}

	c.Set(string(ContextKeyUser), AuthSubject{
		UserID:      user.ID,
		Concurrency: user.Concurrency,
	})
	c.Set(string(ContextKeyUserRole), user.Role)
	c.Set("auth_method", "jwt")
	return true
}
