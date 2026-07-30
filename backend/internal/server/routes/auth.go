package routes

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	ratelimit "github.com/Wei-Shaw/sub2api/internal/middleware"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RegisterAuthRoutes registers the administrator-only authentication surface.
// Customer registration and social-login flows belong to New API.
func RegisterAuthRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	adminAuth servermiddleware.AdminAuthMiddleware,
	auditLog servermiddleware.AuditLogMiddleware,
	redisClient *redis.Client,
	settingService *service.SettingService,
	panelRateLimiter *servermiddleware.PanelRateLimiter,
) {
	rateLimiter := ratelimit.NewRateLimiter(redisClient)

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
		auth.POST("/refresh", rateLimiter.LimitWithOptions("refresh-token", 30, time.Minute, ratelimit.RateLimitOptions{
			FailureMode: ratelimit.RateLimitFailClose,
		}), h.Auth.RefreshToken)
		auth.POST("/logout", h.Auth.Logout)
	}

	settings := v1.Group("/settings")
	settings.Use(panelRateLimiter.PublicIP())
	{
		settings.GET("/public", h.Setting.GetPublicSettings)
	}

	authenticated := v1.Group("")
	authenticated.Use(gin.HandlerFunc(adminAuth))
	authenticated.Use(panelRateLimiter.Global())
	{
		authenticated.GET("/auth/me", h.Auth.GetCurrentUser)
		authenticated.POST("/auth/revoke-all-sessions", h.Auth.RevokeAllSessions)
	}
}
