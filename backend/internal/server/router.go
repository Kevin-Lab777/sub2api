package server

import (
	"log"

	"github.com/Kevin-Lab777/sub2api/internal/config"
	"github.com/Kevin-Lab777/sub2api/internal/handler"
	middleware2 "github.com/Kevin-Lab777/sub2api/internal/server/middleware"
	"github.com/Kevin-Lab777/sub2api/internal/server/routes"
	"github.com/Kevin-Lab777/sub2api/internal/service"
	"github.com/Kevin-Lab777/sub2api/internal/web"
	"github.com/redis/go-redis/v9"

	"github.com/gin-gonic/gin"
)

// SetupRouter 配置路由器中间件和路由
func SetupRouter(
	r *gin.Engine,
	handlers *handler.Handlers,
	jwtAuth middleware2.JWTAuthMiddleware,
	adminAuth middleware2.AdminAuthMiddleware,
	apiKeyAuth middleware2.APIKeyAuthMiddleware,
	apiKeyService *service.APIKeyService,
	subscriptionService *service.SubscriptionService,
	opsService *service.OpsService,
	settingService *service.SettingService,
	cfg *config.Config,
	redisClient *redis.Client,
) *gin.Engine {
	// 应用中间件
	r.Use(middleware2.Logger())
	r.Use(middleware2.CORS(cfg.CORS))
	r.Use(middleware2.SecurityHeaders(cfg.Security.CSP))

	// Serve embedded frontend with settings injection if available
	if web.HasEmbeddedFrontend() {
		frontendServer, err := web.NewFrontendServer(settingService)
		if err != nil {
			log.Printf("Warning: Failed to create frontend server with settings injection: %v, using legacy mode", err)
			r.Use(web.ServeEmbeddedFrontend())
		} else {
			// Register cache invalidation callback
			settingService.SetOnUpdateCallback(frontendServer.InvalidateCache)
			r.Use(frontendServer.Middleware())
		}
	}

	// 注册路由
	registerRoutes(r, handlers, jwtAuth, adminAuth, apiKeyAuth, apiKeyService, subscriptionService, opsService, cfg, redisClient)

	return r
}

// registerRoutes 注册所有 HTTP 路由
func registerRoutes(
	r *gin.Engine,
	h *handler.Handlers,
	jwtAuth middleware2.JWTAuthMiddleware,
	adminAuth middleware2.AdminAuthMiddleware,
	apiKeyAuth middleware2.APIKeyAuthMiddleware,
	apiKeyService *service.APIKeyService,
	subscriptionService *service.SubscriptionService,
	opsService *service.OpsService,
	cfg *config.Config,
	redisClient *redis.Client,
) {
	// 通用路由（健康检查、状态等）
	routes.RegisterCommonRoutes(r)

	// API v1
	v1 := r.Group("/api/v1")

	// [LITE] 注册认证路由 (登录、/auth/me) - 不传 redisClient，Lite 版本不需要速率限制
	routes.RegisterAuthRoutes(v1, h, jwtAuth)

	// [LITE] 注册用户路由 (API Key 管理等，Admin 使用)
	routes.RegisterUserRoutes(v1, h, jwtAuth)

	// 注册管理员路由
	routes.RegisterAdminRoutes(v1, h, adminAuth)

	// 注册网关路由
	routes.RegisterGatewayRoutes(r, h, apiKeyAuth, apiKeyService, subscriptionService, opsService, cfg)
}
