package repository

import (
	"database/sql"

	"github.com/Kevin-Lab777/sub2api/ent"
	"github.com/Kevin-Lab777/sub2api/internal/config"
	"github.com/Kevin-Lab777/sub2api/internal/service"
	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
)

// ProvideConcurrencyCache 创建并发控制缓存，从配置读取 TTL 参数
// 性能优化：TTL 可配置，支持长时间运行的 LLM 请求场景
func ProvideConcurrencyCache(rdb *redis.Client, cfg *config.Config) service.ConcurrencyCache {
	waitTTLSeconds := int(cfg.Gateway.Scheduling.StickySessionWaitTimeout.Seconds())
	if cfg.Gateway.Scheduling.FallbackWaitTimeout > cfg.Gateway.Scheduling.StickySessionWaitTimeout {
		waitTTLSeconds = int(cfg.Gateway.Scheduling.FallbackWaitTimeout.Seconds())
	}
	if waitTTLSeconds <= 0 {
		waitTTLSeconds = cfg.Gateway.ConcurrencySlotTTLMinutes * 60
	}
	return NewConcurrencyCache(rdb, cfg.Gateway.ConcurrencySlotTTLMinutes, waitTTLSeconds)
}

// ProvideGitHubReleaseClient 创建 GitHub Release 客户端
// 从配置中读取代理设置，支持国内服务器通过代理访问 GitHub
func ProvideGitHubReleaseClient(cfg *config.Config) service.GitHubReleaseClient {
	return NewGitHubReleaseClient(cfg.Update.ProxyURL)
}

// ProvidePricingRemoteClient 创建定价数据远程客户端
// 从配置中读取代理设置，支持国内服务器通过代理访问 GitHub 上的定价数据
func ProvidePricingRemoteClient(cfg *config.Config) service.PricingRemoteClient {
	return NewPricingRemoteClient(cfg.Update.ProxyURL)
}

// EntAndDB 包含 Ent 客户端和底层 SQL DB
// [LITE] 重构：合并 ProvideEnt 和 ProvideSQLDB 为单一结构体
type EntAndDB struct {
	Client *ent.Client
	DB     *sql.DB
}

// ProvideEntAndDB 为依赖注入同时提供 Ent 客户端和 SQL DB
// [LITE] 修复了原来 ProvideSQLDB 无法获取 driver 的问题
func ProvideEntAndDB(cfg *config.Config) (EntAndDB, error) {
	client, db, err := InitEnt(cfg)
	if err != nil {
		return EntAndDB{}, err
	}
	return EntAndDB{Client: client, DB: db}, nil
}

// ProvideEntClient 从 EntAndDB 提取 *ent.Client
func ProvideEntClient(entAndDB EntAndDB) *ent.Client {
	return entAndDB.Client
}

// ProvideSQLDB 从 EntAndDB 提取 *sql.DB
func ProvideSQLDB(entAndDB EntAndDB) *sql.DB {
	return entAndDB.DB
}

// ProviderSet is the Wire provider set for all repositories
// [LITE] 移除了: NewRedeemCodeRepository, NewPromoCodeRepository,
//
//	NewUserAttributeDefinitionRepository, NewUserAttributeValueRepository,
//	NewEmailCache, NewIdentityCache, NewRedeemCache, NewTurnstileVerifier
var ProviderSet = wire.NewSet(
	NewUserRepository,
	NewAPIKeyRepository,
	// [LITE] Provide APIKeyUsageRepository from the same implementation
	ProvideAPIKeyUsageRepository,
	NewGroupRepository,
	NewAccountRepository,
	NewProxyRepository,
	// [LITE:DELETED] NewRedeemCodeRepository
	// [LITE:DELETED] NewPromoCodeRepository
	NewUsageLogRepository,
	NewDashboardAggregationRepository,
	NewSettingRepository,
	NewOpsRepository,
	NewUserSubscriptionRepository,
	// [LITE:DELETED] NewUserAttributeDefinitionRepository
	// [LITE:DELETED] NewUserAttributeValueRepository

	// Cache implementations
	NewGatewayCache,
	NewBillingCache,
	NewAPIKeyCache,
	NewTempUnschedCache,
	NewTimeoutCounterCache,
	ProvideConcurrencyCache,
	NewDashboardCache,
	// [LITE:DELETED] NewEmailCache
	// [LITE:DELETED] NewIdentityCache
	// [LITE:DELETED] NewRedeemCache
	NewUpdateCache,
	NewGeminiTokenCache,

	// HTTP service ports (DI Strategy A: return interface directly)
	// [LITE:DELETED] NewTurnstileVerifier
	ProvidePricingRemoteClient,
	ProvideGitHubReleaseClient,
	NewProxyExitInfoProber,
	NewClaudeUsageFetcher,
	NewClaudeOAuthClient,
	NewHTTPUpstream,
	NewOpenAIOAuthClient,
	NewGeminiOAuthClient,
	NewGeminiCliCodeAssistClient,

	ProvideEntAndDB,
	ProvideEntClient,
	ProvideSQLDB,
	ProvideRedis,
)

// ProvideRedis 为依赖注入提供 Redis 客户端。
//
// Redis 用于：
//   - 分布式锁（如并发控制）
//   - 缓存（如用户会话、API 响应缓存）
//   - 速率限制
//   - 实时统计数据
//
// 依赖：config.Config
// 提供：*redis.Client
func ProvideRedis(cfg *config.Config) *redis.Client {
	return InitRedis(cfg)
}

