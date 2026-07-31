package gatewayapp

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/wire"
)

// technicalProviderSet is the dependency graph for the in-process account
// gateway. It intentionally does not include service.ProviderSet or
// repository.ProviderSet: those sets also construct customer authentication,
// billing, subscription, payment, and notification services that do not belong
// to Next API's runtime boundary.
var technicalProviderSet = wire.NewSet(
	// Infrastructure retained by the technical account gateway.
	repository.ProvideEnt,
	repository.ProvideSQLDB,
	repository.ProvideRedis,
	repository.ProvideSchedulerCache,
	repository.ProvideConcurrencyCache,
	repository.ProvideSessionLimitCache,
	repository.NewAccountRepository,
	repository.NewGroupRepository,
	repository.NewUsageLogRepository,
	repository.NewGatewayCache,
	repository.NewTempUnschedCache,
	repository.NewTimeoutCounterCache,
	repository.NewOpenAI403CounterCache,
	repository.NewInternal500CounterCache,
	repository.NewRPMCache,
	repository.NewGeminiTokenCache,
	repository.NewIdentityCache,
	repository.NewSettingRepository,
	repository.NewProxyRepository,
	repository.NewTLSFingerprintProfileRepository,
	repository.NewTLSFingerprintProfileCache,
	repository.NewSchedulerOutboxRepository,
	repository.NewHTTPUpstream,
	repository.NewClaudeOAuthClient,
	repository.NewOpenAIOAuthClient,
	repository.NewGeminiOAuthClient,
	repository.NewGeminiCliCodeAssistClient,
	repository.NewGeminiDriveClient,

	// Scheduler, account concurrency, provider authentication, and gateway
	// policy services. These are all account-pool concerns; none owns a
	// customer principal or commercial quota.
	service.NewIdentityService,
	service.ProvideTimingWheelService,
	service.ProvideDeferredService,
	service.ProvideConcurrencyService,
	service.ProvideSchedulerSnapshotService,
	service.ProvideSettingService,
	service.NewTLSFingerprintProfileService,
	service.NewGeminiQuotaService,
	service.NewOAuthService,
	service.ProvideOpenAIOAuthService,
	service.NewGeminiOAuthService,
	service.NewAntigravityOAuthService,
	service.ProvideOAuthRefreshAPI,
	service.ProvideClaudeTokenProvider,
	service.ProvideOpenAITokenProvider,
	service.ProvideGeminiTokenProvider,
	service.ProvideAntigravityTokenProvider,
	service.NewCompositeTokenCacheInvalidator,
	wire.Bind(new(service.TokenCacheInvalidator), new(*service.CompositeTokenCacheInvalidator)),
	service.ProvideRateLimitService,

	// The legacy provider services still contain transitional compatibility
	// fields for the old HTTP handlers. The technical constructors below pass
	// only the account-pool dependencies and leave customer-only ports absent.
	provideTechnicalGatewayService,
	provideTechnicalOpenAIGatewayService,
	service.NewAntigravityGatewayService,
	service.NewGeminiMessagesCompatService,
	service.NewDigestSessionStore,

	providePrivacyClientFactory,
)

func provideTechnicalGatewayService(
	accountRepo service.AccountRepository,
	groupRepo service.GroupRepository,
	usageLogRepo service.UsageLogRepository,
	cache service.GatewayCache,
	cfg *config.Config,
	schedulerSnapshot *service.SchedulerSnapshotService,
	concurrencyService *service.ConcurrencyService,
	rateLimitService *service.RateLimitService,
	identityService *service.IdentityService,
	httpUpstream service.HTTPUpstream,
	deferredService *service.DeferredService,
	claudeTokenProvider *service.ClaudeTokenProvider,
	sessionLimitCache service.SessionLimitCache,
	rpmCache service.RPMCache,
	settingService *service.SettingService,
	tlsFingerprintProfileService *service.TLSFingerprintProfileService,
	digestStore *service.DigestSessionStore,
) *service.GatewayService {
	return service.NewGatewayService(
		accountRepo,
		groupRepo,
		usageLogRepo,
		nil, // customer usage billing repository
		nil, // customer user repository
		nil, // customer subscription repository
		nil, // customer group-rate repository
		cache,
		cfg,
		schedulerSnapshot,
		concurrencyService,
		nil, // customer billing service
		rateLimitService,
		nil, // customer billing cache service
		identityService,
		httpUpstream,
		deferredService,
		claudeTokenProvider,
		sessionLimitCache,
		rpmCache,
		digestStore,
		settingService,
		tlsFingerprintProfileService,
		nil, // customer channel service
		nil, // customer model pricing resolver
		nil, // customer composite route resolver
		nil, // customer balance notification service
		nil, // customer platform quota repository
	)
}

func provideTechnicalOpenAIGatewayService(
	accountRepo service.AccountRepository,
	usageLogRepo service.UsageLogRepository,
	cache service.GatewayCache,
	cfg *config.Config,
	schedulerSnapshot *service.SchedulerSnapshotService,
	concurrencyService *service.ConcurrencyService,
	rateLimitService *service.RateLimitService,
	httpUpstream service.HTTPUpstream,
	deferredService *service.DeferredService,
	openAITokenProvider *service.OpenAITokenProvider,
	settingService *service.SettingService,
) *service.OpenAIGatewayService {
	return service.NewOpenAIGatewayService(
		accountRepo,
		usageLogRepo,
		nil, // customer usage billing repository
		nil, // customer user repository
		nil, // customer subscription repository
		nil, // customer group-rate repository
		cache,
		cfg,
		schedulerSnapshot,
		concurrencyService,
		nil, // customer billing service
		rateLimitService,
		nil, // customer billing cache service
		httpUpstream,
		deferredService,
		openAITokenProvider,
		nil, // Grok is not part of the OpenAI technical pool
		nil, // customer model pricing resolver
		nil, // customer channel service
		nil, // customer balance notification service
		settingService,
		nil, // customer platform quota repository
	)
}
