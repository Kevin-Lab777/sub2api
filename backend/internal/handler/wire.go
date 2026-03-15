package handler

import (
	"github.com/Kevin-Lab777/sub2api/internal/handler/admin"
	"github.com/Kevin-Lab777/sub2api/internal/service"

	"github.com/google/wire"
)

// ProvideAdminHandlers creates the AdminHandlers struct
// [LITE] 移除了: userHandler, redeemHandler, promoHandler, userAttributeHandler
func ProvideAdminHandlers(
	dashboardHandler *admin.DashboardHandler,
	groupHandler *admin.GroupHandler,
	accountHandler *admin.AccountHandler,
	announcementHandler *admin.AnnouncementHandler,
	dataManagementHandler *admin.DataManagementHandler,
	oauthHandler *admin.OAuthHandler,
	openaiOAuthHandler *admin.OpenAIOAuthHandler,
	geminiOAuthHandler *admin.GeminiOAuthHandler,
	antigravityOAuthHandler *admin.AntigravityOAuthHandler,
	proxyHandler *admin.ProxyHandler,
	settingHandler *admin.SettingHandler,
	opsHandler *admin.OpsHandler,
	systemHandler *admin.SystemHandler,
	subscriptionHandler *admin.SubscriptionHandler,
	usageHandler *admin.UsageHandler,
	errorPassthroughHandler *admin.ErrorPassthroughHandler,
	apiKeyHandler *admin.AdminAPIKeyHandler,
	scheduledTestHandler *admin.ScheduledTestHandler,
) *AdminHandlers {
	return &AdminHandlers{
		Dashboard:        dashboardHandler,
		Group:            groupHandler,
		Account:          accountHandler,
		Announcement:     announcementHandler,
		DataManagement:   dataManagementHandler,
		OAuth:            oauthHandler,
		OpenAIOAuth:      openaiOAuthHandler,
		GeminiOAuth:      geminiOAuthHandler,
		AntigravityOAuth: antigravityOAuthHandler,
		Proxy:            proxyHandler,
		Setting:          settingHandler,
		Ops:              opsHandler,
		System:           systemHandler,
		Subscription:     subscriptionHandler,
		Usage:            usageHandler,
		ErrorPassthrough: errorPassthroughHandler,
		APIKey:           apiKeyHandler,
		ScheduledTest:    scheduledTestHandler,
	}
}

// ProvideSystemHandler creates admin.SystemHandler with UpdateService
func ProvideSystemHandler(updateService *service.UpdateService, lockService *service.SystemOperationLockService) *admin.SystemHandler {
	return admin.NewSystemHandler(updateService, lockService)
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
	authHandler *AuthHandler,
	userHandler *UserHandler,
	apiKeyHandler *APIKeyHandler,
	usageHandler *UsageHandler,
	announcementHandler *AnnouncementHandler,
	adminHandlers *AdminHandlers,
	gatewayHandler *GatewayHandler,
	openaiGatewayHandler *OpenAIGatewayHandler,
	soraGatewayHandler *SoraGatewayHandler,
	soraClientHandler *SoraClientHandler,
	settingHandler *SettingHandler,
	publicHandler *PublicHandler,
	totpHandler *TotpHandler,
	_ *service.IdempotencyCoordinator,
	_ *service.IdempotencyCleanupService,
) *Handlers {
	return &Handlers{
		Auth:          authHandler,
		User:          userHandler,
		APIKey:        apiKeyHandler,
		Usage:         usageHandler,
		Announcement:  announcementHandler,
		Admin:         adminHandlers,
		Gateway:       gatewayHandler,
		OpenAIGateway: openaiGatewayHandler,
		SoraGateway:   soraGatewayHandler,
		SoraClient:    soraClientHandler,
		Setting:       settingHandler,
		Public:        publicHandler,
		Totp:          totpHandler,
	}
}

// ProviderSet is the Wire provider set for all handlers
var ProviderSet = wire.NewSet(
	// Top-level handlers
	NewAuthHandler,
	NewUserHandler,
	NewAPIKeyHandler,
	NewUsageHandler,
	NewAnnouncementHandler,
	NewGatewayHandler,
	NewOpenAIGatewayHandler,
	NewSoraGatewayHandler,
	NewSoraClientHandler,
	NewTotpHandler,
	ProvideSettingHandler,
	ProvidePublicHandler,

	// Admin handlers
	admin.NewDashboardHandler,
	admin.NewGroupHandler,
	admin.NewAccountHandler,
	admin.NewAnnouncementHandler,
	admin.NewDataManagementHandler,
	admin.NewOAuthHandler,
	admin.NewOpenAIOAuthHandler,
	admin.NewGeminiOAuthHandler,
	admin.NewAntigravityOAuthHandler,
	admin.NewProxyHandler,
	admin.NewSettingHandler,
	admin.NewOpsHandler,
	ProvideSystemHandler,
	admin.NewSubscriptionHandler,
	admin.NewUsageHandler,
	admin.NewErrorPassthroughHandler,
	admin.NewAdminAPIKeyHandler,
	admin.NewScheduledTestHandler,

	// AdminHandlers and Handlers constructors
	ProvideAdminHandlers,
	ProvideHandlers,
)
