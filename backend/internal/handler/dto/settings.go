package dto

import (
	"encoding/json"
	"strings"
)

type CustomMenuItem struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	IconSVG    string `json:"icon_svg"`
	URL        string `json:"url"`
	PageSlug   string `json:"page_slug,omitempty"`
	Visibility string `json:"visibility"`
	SortOrder  int    `json:"sort_order"`
}

type CustomEndpoint struct {
	Name        string `json:"name"`
	Endpoint    string `json:"endpoint"`
	Description string `json:"description"`
}

type PublicSettings struct {
	RegistrationEnabled                  bool                     `json:"registration_enabled"`
	EmailVerifyEnabled                   bool                     `json:"email_verify_enabled"`
	ForceEmailOnThirdPartySignup         bool                     `json:"force_email_on_third_party_signup"`
	RegistrationEmailSuffixWhitelist     []string                 `json:"registration_email_suffix_whitelist"`
	PromoCodeEnabled                     bool                     `json:"promo_code_enabled"`
	PasswordResetEnabled                 bool                     `json:"password_reset_enabled"`
	InvitationCodeEnabled                bool                     `json:"invitation_code_enabled"`
	TotpEnabled                          bool                     `json:"totp_enabled"`
	PasskeyEnabled                       bool                     `json:"passkey_enabled"`
	LoginAgreementEnabled                bool                     `json:"login_agreement_enabled"`
	LoginAgreementMode                   string                   `json:"login_agreement_mode"`
	LoginAgreementUpdatedAt              string                   `json:"login_agreement_updated_at"`
	LoginAgreementRevision               string                   `json:"login_agreement_revision"`
	LoginAgreementDocuments              []LoginAgreementDocument `json:"login_agreement_documents"`
	TurnstileEnabled                     bool                     `json:"turnstile_enabled"`
	TurnstileSiteKey                     string                   `json:"turnstile_site_key"`
	SiteName                             string                   `json:"site_name"`
	SiteLogo                             string                   `json:"site_logo"`
	SiteSubtitle                         string                   `json:"site_subtitle"`
	APIBaseURL                           string                   `json:"api_base_url"`
	ContactInfo                          string                   `json:"contact_info"`
	DocURL                               string                   `json:"doc_url"`
	HomeContent                          string                   `json:"home_content"`
	HideCcsImportButton                  bool                     `json:"hide_ccs_import_button"`
	PurchaseSubscriptionEnabled          bool                     `json:"purchase_subscription_enabled"`
	PurchaseSubscriptionURL              string                   `json:"purchase_subscription_url"`
	TableDefaultPageSize                 int                      `json:"table_default_page_size"`
	TablePageSizeOptions                 []int                    `json:"table_page_size_options"`
	CustomMenuItems                      []CustomMenuItem         `json:"custom_menu_items"`
	CustomEndpoints                      []CustomEndpoint         `json:"custom_endpoints"`
	DingTalkOAuthEnabled                 bool                     `json:"dingtalk_oauth_enabled"`
	LinuxDoOAuthEnabled                  bool                     `json:"linuxdo_oauth_enabled"`
	WeChatOAuthEnabled                   bool                     `json:"wechat_oauth_enabled"`
	WeChatOAuthOpenEnabled               bool                     `json:"wechat_oauth_open_enabled"`
	WeChatOAuthMPEnabled                 bool                     `json:"wechat_oauth_mp_enabled"`
	WeChatOAuthMobileEnabled             bool                     `json:"wechat_oauth_mobile_enabled"`
	OIDCOAuthEnabled                     bool                     `json:"oidc_oauth_enabled"`
	OIDCOAuthProviderName                string                   `json:"oidc_oauth_provider_name"`
	GitHubOAuthEnabled                   bool                     `json:"github_oauth_enabled"`
	GoogleOAuthEnabled                   bool                     `json:"google_oauth_enabled"`
	SoraClientEnabled                    bool                     `json:"sora_client_enabled"`
	BackendModeEnabled                   bool                     `json:"backend_mode_enabled"`
	PaymentEnabled                       bool                     `json:"payment_enabled"`
	Version                              string                   `json:"version"`
	ServerTimezone                       string                   `json:"server_timezone"`
	ServerUTCOffset                      string                   `json:"server_utc_offset"`
	BalanceLowNotifyEnabled              bool                     `json:"balance_low_notify_enabled"`
	AccountQuotaNotifyEnabled            bool                     `json:"account_quota_notify_enabled"`
	BalanceLowNotifyThreshold            float64                  `json:"balance_low_notify_threshold"`
	BalanceLowNotifyRechargeURL          string                   `json:"balance_low_notify_recharge_url"`
	ChannelMonitorEnabled                bool                     `json:"channel_monitor_enabled"`
	ChannelMonitorDefaultIntervalSeconds int                      `json:"channel_monitor_default_interval_seconds"`
	AvailableChannelsEnabled             bool                     `json:"available_channels_enabled"`
	ModelPlazaEnabled                    bool                     `json:"model_plaza_enabled"`
	ModelPlazaRequireAuth                bool                     `json:"model_plaza_require_auth"`
	AffiliateEnabled                     bool                     `json:"affiliate_enabled"`
	RiskControlEnabled                   bool                     `json:"risk_control_enabled"`
	AllowUserViewErrorRequests           bool                     `json:"allow_user_view_error_requests"`
}

type LoginAgreementDocument struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	ContentMD string `json:"content_md"`
}

type OverloadCooldownSettings struct {
	Enabled         bool `json:"enabled"`
	CooldownMinutes int  `json:"cooldown_minutes"`
}

type RateLimit429CooldownSettings struct {
	Enabled         bool `json:"enabled"`
	CooldownSeconds int  `json:"cooldown_seconds"`
}

type PanelRateLimitSettings struct {
	Enabled     bool `json:"enabled"`
	UserRPM     int  `json:"user_rpm"`
	HeavyRPM    int  `json:"heavy_rpm"`
	ExemptAdmin bool `json:"exempt_admin"`
	PublicIPRPM int  `json:"public_ip_rpm"`
}

type StreamTimeoutSettings struct {
	Enabled                bool   `json:"enabled"`
	Action                 string `json:"action"`
	TempUnschedMinutes     int    `json:"temp_unsched_minutes"`
	ThresholdCount         int    `json:"threshold_count"`
	ThresholdWindowMinutes int    `json:"threshold_window_minutes"`
}

type RectifierSettings struct {
	Enabled                  bool     `json:"enabled"`
	ThinkingSignatureEnabled bool     `json:"thinking_signature_enabled"`
	ThinkingBudgetEnabled    bool     `json:"thinking_budget_enabled"`
	APIKeySignatureEnabled   bool     `json:"apikey_signature_enabled"`
	APIKeySignaturePatterns  []string `json:"apikey_signature_patterns"`
}

type BetaPolicyRule struct {
	BetaToken            string   `json:"beta_token"`
	Action               string   `json:"action"`
	Scope                string   `json:"scope"`
	ErrorMessage         string   `json:"error_message,omitempty"`
	ModelWhitelist       []string `json:"model_whitelist,omitempty"`
	FallbackAction       string   `json:"fallback_action,omitempty"`
	FallbackErrorMessage string   `json:"fallback_error_message,omitempty"`
}

type BetaPolicySettings struct {
	Rules []BetaPolicyRule `json:"rules"`
}

func ParseCustomMenuItems(raw string) []CustomMenuItem {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return []CustomMenuItem{}
	}
	var items []CustomMenuItem
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return []CustomMenuItem{}
	}
	return items
}

func ParseUserVisibleMenuItems(raw string) []CustomMenuItem {
	items := ParseCustomMenuItems(raw)
	filtered := make([]CustomMenuItem, 0, len(items))
	for _, item := range items {
		if item.Visibility != "admin" {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func ParseCustomEndpoints(raw string) []CustomEndpoint {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return []CustomEndpoint{}
	}
	var items []CustomEndpoint
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return []CustomEndpoint{}
	}
	return items
}
