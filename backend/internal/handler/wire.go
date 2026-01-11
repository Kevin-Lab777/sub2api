package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/google/wire"
)

// ProvideAdminHandlers creates the AdminHandlers struct
// [LITE] 移除了: userHandler, redeemHandler, promoHandler, userAttributeHandler
func ProvideAdminHandlers(
	dashboardHandler *admin.DashboardHandler,
	groupHandler *admin.GroupHandler,
	accountHandler *admin.AccountHandler,
	oauthHandler *admin.OAuthHandler,
	openaiOAuthHandler *admin.OpenAIOAuthHandler,
	geminiOAuthHandler *admin.GeminiOAuthHandler,
	antigravityOAuthHandler *admin.AntigravityOAuthHandler,
	proxyHandler *admin.ProxyHandler,
	settingHandler *admin.SettingHandler,
	systemHandler *admin.SystemHandler,
	subscriptionHandler *admin.SubscriptionHandler,
	usageHandler *admin.UsageHandler,
) *AdminHandlers {
	return &AdminHandlers{
		Dashboard:        dashboardHandler,
		Group:            groupHandler,
		Account:          accountHandler,
		OAuth:            oauthHandler,
		OpenAIOAuth:      openaiOAuthHandler,
		GeminiOAuth:      geminiOAuthHandler,
		AntigravityOAuth: antigravityOAuthHandler,
		Proxy:            proxyHandler,
		Setting:          settingHandler,
		System:           systemHandler,
		Subscription:     subscriptionHandler,
		Usage:            usageHandler,
	}
}

// ProvideSystemHandler creates admin.SystemHandler with UpdateService
func ProvideSystemHandler(updateService *service.UpdateService) *admin.SystemHandler {
	return admin.NewSystemHandler(updateService)
}

// ProvideSettingHandler creates SettingHandler with version from BuildInfo
func ProvideSettingHandler(settingService *service.SettingService, buildInfo BuildInfo) *SettingHandler {
	return NewSettingHandler(settingService, buildInfo.Version)
}

// [LITE] ProvidePublicHandler creates PublicHandler with version from BuildInfo
func ProvidePublicHandler(
	settingService *service.SettingService,
	userService *service.UserService,
	buildInfo BuildInfo,
) *PublicHandler {
	return NewPublicHandler(settingService, userService, buildInfo.Version)
}

// ProvideHandlers creates the Handlers struct
// [LITE] 保留 User, APIKey, Usage 给 Admin 管理 API Key
func ProvideHandlers(
	adminHandlers *AdminHandlers,
	gatewayHandler *GatewayHandler,
	openaiGatewayHandler *OpenAIGatewayHandler,
	settingHandler *SettingHandler,
	publicHandler *PublicHandler,
	authHandler *AuthHandler,
	userHandler *UserHandler,
	apiKeyHandler *APIKeyHandler,
	usageHandler *UsageHandler,
) *Handlers {
	return &Handlers{
		Admin:         adminHandlers,
		Gateway:       gatewayHandler,
		OpenAIGateway: openaiGatewayHandler,
		Setting:       settingHandler,
		Public:        publicHandler,
		Auth:          authHandler,
		User:          userHandler,
		APIKey:        apiKeyHandler,
		Usage:         usageHandler,
	}
}

// ProviderSet is the Wire provider set for all handlers
var ProviderSet = wire.NewSet(
	// Top-level handlers
	NewGatewayHandler,
	NewOpenAIGatewayHandler,
	ProvideSettingHandler,
	ProvidePublicHandler,
	NewAuthHandler,
	// [LITE] User handlers for API Key management
	NewUserHandler,
	NewAPIKeyHandler,
	NewUsageHandler,

	// Admin handlers
	admin.NewDashboardHandler,
	admin.NewGroupHandler,
	admin.NewAccountHandler,
	admin.NewOAuthHandler,
	admin.NewOpenAIOAuthHandler,
	admin.NewGeminiOAuthHandler,
	admin.NewAntigravityOAuthHandler,
	admin.NewProxyHandler,
	admin.NewSettingHandler,
	ProvideSystemHandler,
	admin.NewSubscriptionHandler,
	admin.NewUsageHandler,

	// AdminHandlers and Handlers constructors
	ProvideAdminHandlers,
	ProvideHandlers,
)
