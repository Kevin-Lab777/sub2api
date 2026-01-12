package service

import (
	"time"

	"github.com/Kevin-Lab777/sub2api/internal/config"
	"github.com/google/wire"
)

// BuildInfo contains build information
type BuildInfo struct {
	Version   string
	BuildType string
}

// ProvidePricingService creates and initializes PricingService
func ProvidePricingService(cfg *config.Config, remoteClient PricingRemoteClient) (*PricingService, error) {
	svc := NewPricingService(cfg, remoteClient)
	if err := svc.Initialize(); err != nil {
		// Pricing service initialization failure should not block startup, use fallback prices
		println("[Service] Warning: Pricing service initialization failed:", err.Error())
	}
	return svc, nil
}

// ProvideUpdateService creates UpdateService with BuildInfo
func ProvideUpdateService(cache UpdateCache, githubClient GitHubReleaseClient, buildInfo BuildInfo) *UpdateService {
	return NewUpdateService(cache, githubClient, buildInfo.Version, buildInfo.BuildType)
}

// [LITE:DELETED] ProvideEmailQueueService

// ProvideTokenRefreshService creates and starts TokenRefreshService
func ProvideTokenRefreshService(
	accountRepo AccountRepository,
	oauthService *OAuthService,
	openaiOAuthService *OpenAIOAuthService,
	geminiOAuthService *GeminiOAuthService,
	antigravityOAuthService *AntigravityOAuthService,
	cfg *config.Config,
) *TokenRefreshService {
	svc := NewTokenRefreshService(accountRepo, oauthService, openaiOAuthService, geminiOAuthService, antigravityOAuthService, cfg)
	svc.Start()
	return svc
}

// ProvideDashboardAggregationService 创建并启动仪表盘聚合服务
func ProvideDashboardAggregationService(repo DashboardAggregationRepository, timingWheel *TimingWheelService, cfg *config.Config) *DashboardAggregationService {
	svc := NewDashboardAggregationService(repo, timingWheel, cfg)
	svc.Start()
	return svc
}

// ProvideAccountExpiryService creates and starts AccountExpiryService.
func ProvideAccountExpiryService(accountRepo AccountRepository) *AccountExpiryService {
	svc := NewAccountExpiryService(accountRepo, time.Minute)
	svc.Start()
	return svc
}

// ProvideTimingWheelService creates and starts TimingWheelService
func ProvideTimingWheelService() *TimingWheelService {
	svc := NewTimingWheelService()
	svc.Start()
	return svc
}

// ProvideDeferredService creates and starts DeferredService
func ProvideDeferredService(accountRepo AccountRepository, timingWheel *TimingWheelService) *DeferredService {
	svc := NewDeferredService(accountRepo, timingWheel, 10*time.Second)
	svc.Start()
	return svc
}

// ProvideConcurrencyService creates ConcurrencyService and starts slot cleanup worker.
func ProvideConcurrencyService(cache ConcurrencyCache, accountRepo AccountRepository, cfg *config.Config) *ConcurrencyService {
	svc := NewConcurrencyService(cache)
	if cfg != nil {
		svc.StartSlotCleanupWorker(accountRepo, cfg.Gateway.Scheduling.SlotCleanupInterval)
	}
	return svc
}

// ProvideAPIKeyAuthCacheInvalidator 提供 API Key 认证缓存失效能力
func ProvideAPIKeyAuthCacheInvalidator(apiKeyService *APIKeyService) APIKeyAuthCacheInvalidator {
	return apiKeyService
}

// ProviderSet is the Wire provider set for all services
var ProviderSet = wire.NewSet(
	// Core services
	NewAuthService,
	NewUserService,
	NewAPIKeyService,
	NewAPIKeyUsageService,              // [LITE] API Key 用量管理服务
	ProvideAPIKeyAuthCacheInvalidator,  // 上游新增：API Key 认证缓存失效
	NewGroupService,
	NewAccountService,
	NewProxyService,
	NewUsageService,
	NewDashboardService,
	ProvidePricingService,
	NewBillingService,
	NewBillingCacheService,
	NewAdminService,
	NewGatewayService,
	NewOpenAIGatewayService,
	NewOAuthService,
	NewOpenAIOAuthService,
	NewGeminiOAuthService,
	NewGeminiQuotaService,
	NewAntigravityOAuthService,
	NewGeminiTokenProvider,
	NewGeminiMessagesCompatService,
	NewAntigravityTokenProvider,
	NewAntigravityGatewayService,
	NewRateLimitService,
	NewAccountUsageService,
	NewAccountTestService,
	NewSettingService,
	NewSubscriptionService,
	ProvideConcurrencyService,
	NewCRSSyncService,
	ProvideUpdateService,
	ProvideTokenRefreshService,
	ProvideAccountExpiryService,
	ProvideTimingWheelService,
	ProvideDashboardAggregationService,
	ProvideDeferredService,
	NewAntigravityQuotaFetcher,
	NewUsageCache,
)
