package handler

import (
	"github.com/Kevin-Lab777/sub2api/internal/handler/admin"
)

// AdminHandlers contains all admin-related HTTP handlers
// [LITE] 移除了: User, Redeem, Promo, UserAttribute
type AdminHandlers struct {
	Dashboard             *admin.DashboardHandler
	Group                 *admin.GroupHandler
	Account               *admin.AccountHandler
	Announcement          *admin.AnnouncementHandler
	DataManagement        *admin.DataManagementHandler
	Backup                *admin.BackupHandler
	OAuth                 *admin.OAuthHandler
	OpenAIOAuth           *admin.OpenAIOAuthHandler
	GeminiOAuth           *admin.GeminiOAuthHandler
	AntigravityOAuth      *admin.AntigravityOAuthHandler
	Proxy                 *admin.ProxyHandler
	Setting               *admin.SettingHandler
	Ops                   *admin.OpsHandler
	System                *admin.SystemHandler
	Subscription          *admin.SubscriptionHandler
	Usage                 *admin.UsageHandler
	ErrorPassthrough      *admin.ErrorPassthroughHandler
	TLSFingerprintProfile *admin.TLSFingerprintProfileHandler
	APIKey                *admin.AdminAPIKeyHandler
	ScheduledTest         *admin.ScheduledTestHandler
	Channel               *admin.ChannelHandler
	Payment               *admin.PaymentHandler
}

// Handlers contains all HTTP handlers
// [LITE] 保留 User, APIKey, Usage 给 Admin 使用
type Handlers struct {
	Auth           *AuthHandler
	User           *UserHandler
	APIKey         *APIKeyHandler
	Usage          *UsageHandler
	// [LITE:DELETED] Redeem, Subscription
	Announcement   *AnnouncementHandler
	Admin          *AdminHandlers
	Gateway        *GatewayHandler
	OpenAIGateway  *OpenAIGatewayHandler
	Setting        *SettingHandler
	Public         *PublicHandler
	Totp           *TotpHandler
	Payment        *PaymentHandler
	PaymentWebhook *PaymentWebhookHandler
}

// BuildInfo contains build-time information
type BuildInfo struct {
	Version   string
	BuildType string // "source" for manual builds, "release" for CI builds
}
