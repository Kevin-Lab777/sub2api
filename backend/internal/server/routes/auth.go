package routes

import (
	"time"

	"github.com/Kevin-Lab777/sub2api/internal/handler"
	ratelimit "github.com/Kevin-Lab777/sub2api/internal/middleware"
	servermiddleware "github.com/Kevin-Lab777/sub2api/internal/server/middleware"
	"github.com/Kevin-Lab777/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// [LITE] 简化版认证路由 - 只保留 Admin 登录

// RegisterAuthRoutes 注册认证相关路由 [LITE]
func RegisterAuthRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	jwtAuth servermiddleware.JWTAuthMiddleware,
	auditLog servermiddleware.AuditLogMiddleware,
	redisClient *redis.Client,
	settingService *service.SettingService,
) {
	rateLimiter := ratelimit.NewRateLimiter(redisClient)

	// [LITE] 公开接口 - 只保留登录
	auth := v1.Group("/auth")
	auth.Use(servermiddleware.BackendModeAuthGuard(settingService))
	auth.Use(gin.HandlerFunc(auditLog))
	{
		auth.POST("/login", rateLimiter.LimitWithOptions("auth-login", 20, time.Minute, ratelimit.RateLimitOptions{
			FailureMode: ratelimit.RateLimitFailClose,
		}), h.Auth.Login)
		auth.POST("/login/2fa", rateLimiter.LimitWithOptions("auth-login-2fa", 20, time.Minute, ratelimit.RateLimitOptions{
			FailureMode: ratelimit.RateLimitFailClose,
		}), h.Auth.Login2FA)
		// [LITE:DELETED] auth.POST("/register", ...)
		// [LITE:DELETED] auth.POST("/send-verify-code", ...)
		// [LITE:DELETED] auth.POST("/validate-promo-code", ...)
		// [LITE:DELETED] auth.GET("/oauth/linuxdo/*", ...)
		// [LITE:DELETED] OIDC OAuth routes — handler removed (depends on UserService registration flow)
	}

	// 公开设置（无需认证）
	settings := v1.Group("/settings")
	{
		settings.GET("/public", h.Setting.GetPublicSettings)
	}

	// 需要认证的当前用户信息
	authenticated := v1.Group("")
	authenticated.Use(gin.HandlerFunc(jwtAuth))
	authenticated.Use(servermiddleware.BackendModeUserGuard(settingService))
	authenticated.Use(gin.HandlerFunc(auditLog))
	{
		authenticated.GET("/auth/me", h.Auth.GetCurrentUser)

		// [LITE] 用户端公告路由（上游新增）
		announcements := authenticated.Group("/announcements")
		{
			announcements.GET("", h.Announcement.List)
			announcements.POST("/:id/read", h.Announcement.MarkRead)
		}
	}
}
