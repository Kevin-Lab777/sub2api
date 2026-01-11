package routes

import (
	"github.com/Kevin-Lab777/sub2api/internal/handler"
	servermiddleware "github.com/Kevin-Lab777/sub2api/internal/server/middleware"

	"github.com/gin-gonic/gin"
)

// [LITE] 简化版认证路由 - 只保留 Admin 登录

// RegisterAuthRoutes 注册认证相关路由 [LITE]
func RegisterAuthRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	jwtAuth servermiddleware.JWTAuthMiddleware,
) {
	// [LITE] 公开接口 - 只保留登录
	auth := v1.Group("/auth")
	{
		auth.POST("/login", h.Auth.Login)
		// [LITE:DELETED] auth.POST("/register", ...)
		// [LITE:DELETED] auth.POST("/send-verify-code", ...)
		// [LITE:DELETED] auth.POST("/validate-promo-code", ...)
		// [LITE:DELETED] auth.GET("/oauth/linuxdo/*", ...)
	}

	// 公开设置（无需认证）
	settings := v1.Group("/settings")
	{
		settings.GET("/public", h.Setting.GetPublicSettings)
	}

	// 需要认证的当前用户信息
	authenticated := v1.Group("")
	authenticated.Use(gin.HandlerFunc(jwtAuth))
	{
		authenticated.GET("/auth/me", h.Auth.GetCurrentUser)
	}
}
