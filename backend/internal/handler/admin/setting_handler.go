package admin

import (
	"log"
	"strings"
	"time"

	"github.com/Kevin-Lab777/sub2api/internal/config"
	"github.com/Kevin-Lab777/sub2api/internal/handler/dto"
	"github.com/Kevin-Lab777/sub2api/internal/pkg/response"
	"github.com/Kevin-Lab777/sub2api/internal/server/middleware"
	"github.com/Kevin-Lab777/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// SettingHandler 系统设置处理器
// [LITE] 移除了 emailService 和 turnstileService
type SettingHandler struct {
	settingService *service.SettingService
}

// NewSettingHandler 创建系统设置处理器
// [LITE] 移除了 EmailService 和 TurnstileService 参数
func NewSettingHandler(settingService *service.SettingService) *SettingHandler {
	return &SettingHandler{
		settingService: settingService,
	}
}

// GetSettings 获取所有系统设置
// GET /api/v1/admin/settings
func (h *SettingHandler) GetSettings(c *gin.Context) {
	settings, err := h.settingService.GetAllSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.SystemSettings{
		RegistrationEnabled:                  settings.RegistrationEnabled,
		EmailVerifyEnabled:                   settings.EmailVerifyEnabled,
		SMTPHost:                             settings.SMTPHost,
		SMTPPort:                             settings.SMTPPort,
		SMTPUsername:                         settings.SMTPUsername,
		SMTPPasswordConfigured:               settings.SMTPPasswordConfigured,
		SMTPFrom:                             settings.SMTPFrom,
		SMTPFromName:                         settings.SMTPFromName,
		SMTPUseTLS:                           settings.SMTPUseTLS,
		TurnstileEnabled:                     settings.TurnstileEnabled,
		TurnstileSiteKey:                     settings.TurnstileSiteKey,
		TurnstileSecretKeyConfigured:         settings.TurnstileSecretKeyConfigured,
		LinuxDoConnectEnabled:                settings.LinuxDoConnectEnabled,
		LinuxDoConnectClientID:               settings.LinuxDoConnectClientID,
		LinuxDoConnectClientSecretConfigured: settings.LinuxDoConnectClientSecretConfigured,
		LinuxDoConnectRedirectURL:            settings.LinuxDoConnectRedirectURL,
		SiteName:                             settings.SiteName,
		SiteLogo:                             settings.SiteLogo,
		SiteSubtitle:                         settings.SiteSubtitle,
		APIBaseURL:                           settings.APIBaseURL,
		ContactInfo:                          settings.ContactInfo,
		DocURL:                               settings.DocURL,
		HomeContent:                          settings.HomeContent,
		DefaultConcurrency:                   settings.DefaultConcurrency,
		DefaultBalance:                       settings.DefaultBalance,
		EnableModelFallback:                  settings.EnableModelFallback,
		FallbackModelAnthropic:               settings.FallbackModelAnthropic,
		FallbackModelOpenAI:                  settings.FallbackModelOpenAI,
		FallbackModelGemini:                  settings.FallbackModelGemini,
		FallbackModelAntigravity:             settings.FallbackModelAntigravity,
		EnableIdentityPatch:                  settings.EnableIdentityPatch,
		IdentityPatchPrompt:                  settings.IdentityPatchPrompt,
	})
}

// UpdateSettingsRequest 更新设置请求
type UpdateSettingsRequest struct {
	// 注册设置
	RegistrationEnabled bool `json:"registration_enabled"`
	EmailVerifyEnabled  bool `json:"email_verify_enabled"`

	// 邮件服务设置
	SMTPHost     string `json:"smtp_host"`
	SMTPPort     int    `json:"smtp_port"`
	SMTPUsername string `json:"smtp_username"`
	SMTPPassword string `json:"smtp_password"`
	SMTPFrom     string `json:"smtp_from_email"`
	SMTPFromName string `json:"smtp_from_name"`
	SMTPUseTLS   bool   `json:"smtp_use_tls"`

	// Cloudflare Turnstile 设置
	TurnstileEnabled   bool   `json:"turnstile_enabled"`
	TurnstileSiteKey   string `json:"turnstile_site_key"`
	TurnstileSecretKey string `json:"turnstile_secret_key"`

	// LinuxDo Connect OAuth 登录（终端用户 SSO）
	LinuxDoConnectEnabled      bool   `json:"linuxdo_connect_enabled"`
	LinuxDoConnectClientID     string `json:"linuxdo_connect_client_id"`
	LinuxDoConnectClientSecret string `json:"linuxdo_connect_client_secret"`
	LinuxDoConnectRedirectURL  string `json:"linuxdo_connect_redirect_url"`

	// OEM设置
	SiteName     string `json:"site_name"`
	SiteLogo     string `json:"site_logo"`
	SiteSubtitle string `json:"site_subtitle"`
	APIBaseURL   string `json:"api_base_url"`
	ContactInfo  string `json:"contact_info"`
	DocURL       string `json:"doc_url"`
	HomeContent  string `json:"home_content"`

	// 默认配置
	DefaultConcurrency int     `json:"default_concurrency"`
	DefaultBalance     float64 `json:"default_balance"`

	// Model fallback configuration
	EnableModelFallback      bool   `json:"enable_model_fallback"`
	FallbackModelAnthropic   string `json:"fallback_model_anthropic"`
	FallbackModelOpenAI      string `json:"fallback_model_openai"`
	FallbackModelGemini      string `json:"fallback_model_gemini"`
	FallbackModelAntigravity string `json:"fallback_model_antigravity"`

	// Identity patch configuration (Claude -> Gemini)
	EnableIdentityPatch bool   `json:"enable_identity_patch"`
	IdentityPatchPrompt string `json:"identity_patch_prompt"`
}

// UpdateSettings 更新系统设置
// PUT /api/v1/admin/settings
func (h *SettingHandler) UpdateSettings(c *gin.Context) {
	var req UpdateSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	previousSettings, err := h.settingService.GetAllSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	// 验证参数
	if req.DefaultConcurrency < 1 {
		req.DefaultConcurrency = 1
	}
	if req.DefaultBalance < 0 {
		req.DefaultBalance = 0
	}
	if req.SMTPPort <= 0 {
		req.SMTPPort = 587
	}

	// Turnstile 参数验证
	// [LITE] 移除 Turnstile 验证，Lite 模式不需要验证码
	if req.TurnstileEnabled {
		// 检查必填字段
		if req.TurnstileSiteKey == "" {
			response.BadRequest(c, "Turnstile Site Key is required when enabled")
			return
		}
		// 如果未提供 secret key，使用已保存的值（留空保留当前值）
		if req.TurnstileSecretKey == "" {
			if previousSettings.TurnstileSecretKey == "" {
				response.BadRequest(c, "Turnstile Secret Key is required when enabled")
				return
			}
			req.TurnstileSecretKey = previousSettings.TurnstileSecretKey
		}
		// [LITE:DELETED] Turnstile 密钥验证已移除
	}

	// LinuxDo Connect 参数验证
	if req.LinuxDoConnectEnabled {
		req.LinuxDoConnectClientID = strings.TrimSpace(req.LinuxDoConnectClientID)
		req.LinuxDoConnectClientSecret = strings.TrimSpace(req.LinuxDoConnectClientSecret)
		req.LinuxDoConnectRedirectURL = strings.TrimSpace(req.LinuxDoConnectRedirectURL)

		if req.LinuxDoConnectClientID == "" {
			response.BadRequest(c, "LinuxDo Client ID is required when enabled")
			return
		}
		if req.LinuxDoConnectRedirectURL == "" {
			response.BadRequest(c, "LinuxDo Redirect URL is required when enabled")
			return
		}
		if err := config.ValidateAbsoluteHTTPURL(req.LinuxDoConnectRedirectURL); err != nil {
			response.BadRequest(c, "LinuxDo Redirect URL must be an absolute http(s) URL")
			return
		}

		// 如果未提供 client_secret，则保留现有值（如有）。
		if req.LinuxDoConnectClientSecret == "" {
			if previousSettings.LinuxDoConnectClientSecret == "" {
				response.BadRequest(c, "LinuxDo Client Secret is required when enabled")
				return
			}
			req.LinuxDoConnectClientSecret = previousSettings.LinuxDoConnectClientSecret
		}
	}

	settings := &service.SystemSettings{
		RegistrationEnabled:        req.RegistrationEnabled,
		EmailVerifyEnabled:         req.EmailVerifyEnabled,
		SMTPHost:                   req.SMTPHost,
		SMTPPort:                   req.SMTPPort,
		SMTPUsername:               req.SMTPUsername,
		SMTPPassword:               req.SMTPPassword,
		SMTPFrom:                   req.SMTPFrom,
		SMTPFromName:               req.SMTPFromName,
		SMTPUseTLS:                 req.SMTPUseTLS,
		TurnstileEnabled:           req.TurnstileEnabled,
		TurnstileSiteKey:           req.TurnstileSiteKey,
		TurnstileSecretKey:         req.TurnstileSecretKey,
		LinuxDoConnectEnabled:      req.LinuxDoConnectEnabled,
		LinuxDoConnectClientID:     req.LinuxDoConnectClientID,
		LinuxDoConnectClientSecret: req.LinuxDoConnectClientSecret,
		LinuxDoConnectRedirectURL:  req.LinuxDoConnectRedirectURL,
		SiteName:                   req.SiteName,
		SiteLogo:                   req.SiteLogo,
		SiteSubtitle:               req.SiteSubtitle,
		APIBaseURL:                 req.APIBaseURL,
		ContactInfo:                req.ContactInfo,
		DocURL:                     req.DocURL,
		HomeContent:                req.HomeContent,
		DefaultConcurrency:         req.DefaultConcurrency,
		DefaultBalance:             req.DefaultBalance,
		EnableModelFallback:        req.EnableModelFallback,
		FallbackModelAnthropic:     req.FallbackModelAnthropic,
		FallbackModelOpenAI:        req.FallbackModelOpenAI,
		FallbackModelGemini:        req.FallbackModelGemini,
		FallbackModelAntigravity:   req.FallbackModelAntigravity,
		EnableIdentityPatch:        req.EnableIdentityPatch,
		IdentityPatchPrompt:        req.IdentityPatchPrompt,
	}

	if err := h.settingService.UpdateSettings(c.Request.Context(), settings); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	h.auditSettingsUpdate(c, previousSettings, settings, req)

	// 重新获取设置返回
	updatedSettings, err := h.settingService.GetAllSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.SystemSettings{
		RegistrationEnabled:                  updatedSettings.RegistrationEnabled,
		EmailVerifyEnabled:                   updatedSettings.EmailVerifyEnabled,
		SMTPHost:                             updatedSettings.SMTPHost,
		SMTPPort:                             updatedSettings.SMTPPort,
		SMTPUsername:                         updatedSettings.SMTPUsername,
		SMTPPasswordConfigured:               updatedSettings.SMTPPasswordConfigured,
		SMTPFrom:                             updatedSettings.SMTPFrom,
		SMTPFromName:                         updatedSettings.SMTPFromName,
		SMTPUseTLS:                           updatedSettings.SMTPUseTLS,
		TurnstileEnabled:                     updatedSettings.TurnstileEnabled,
		TurnstileSiteKey:                     updatedSettings.TurnstileSiteKey,
		TurnstileSecretKeyConfigured:         updatedSettings.TurnstileSecretKeyConfigured,
		LinuxDoConnectEnabled:                updatedSettings.LinuxDoConnectEnabled,
		LinuxDoConnectClientID:               updatedSettings.LinuxDoConnectClientID,
		LinuxDoConnectClientSecretConfigured: updatedSettings.LinuxDoConnectClientSecretConfigured,
		LinuxDoConnectRedirectURL:            updatedSettings.LinuxDoConnectRedirectURL,
		SiteName:                             updatedSettings.SiteName,
		SiteLogo:                             updatedSettings.SiteLogo,
		SiteSubtitle:                         updatedSettings.SiteSubtitle,
		APIBaseURL:                           updatedSettings.APIBaseURL,
		ContactInfo:                          updatedSettings.ContactInfo,
		DocURL:                               updatedSettings.DocURL,
		HomeContent:                          updatedSettings.HomeContent,
		DefaultConcurrency:                   updatedSettings.DefaultConcurrency,
		DefaultBalance:                       updatedSettings.DefaultBalance,
		EnableModelFallback:                  updatedSettings.EnableModelFallback,
		FallbackModelAnthropic:               updatedSettings.FallbackModelAnthropic,
		FallbackModelOpenAI:                  updatedSettings.FallbackModelOpenAI,
		FallbackModelGemini:                  updatedSettings.FallbackModelGemini,
		FallbackModelAntigravity:             updatedSettings.FallbackModelAntigravity,
		EnableIdentityPatch:                  updatedSettings.EnableIdentityPatch,
		IdentityPatchPrompt:                  updatedSettings.IdentityPatchPrompt,
	})
}

func (h *SettingHandler) auditSettingsUpdate(c *gin.Context, before *service.SystemSettings, after *service.SystemSettings, req UpdateSettingsRequest) {
	if before == nil || after == nil {
		return
	}

	changed := diffSettings(before, after, req)
	if len(changed) == 0 {
		return
	}

	subject, _ := middleware.GetAuthSubjectFromContext(c)
	role, _ := middleware.GetUserRoleFromContext(c)
	log.Printf("AUDIT: settings updated at=%s user_id=%d role=%s changed=%v",
		time.Now().UTC().Format(time.RFC3339),
		subject.UserID,
		role,
		changed,
	)
}

func diffSettings(before *service.SystemSettings, after *service.SystemSettings, req UpdateSettingsRequest) []string {
	changed := make([]string, 0, 20)
	if before.RegistrationEnabled != after.RegistrationEnabled {
		changed = append(changed, "registration_enabled")
	}
	if before.EmailVerifyEnabled != after.EmailVerifyEnabled {
		changed = append(changed, "email_verify_enabled")
	}
	if before.SMTPHost != after.SMTPHost {
		changed = append(changed, "smtp_host")
	}
	if before.SMTPPort != after.SMTPPort {
		changed = append(changed, "smtp_port")
	}
	if before.SMTPUsername != after.SMTPUsername {
		changed = append(changed, "smtp_username")
	}
	if req.SMTPPassword != "" {
		changed = append(changed, "smtp_password")
	}
	if before.SMTPFrom != after.SMTPFrom {
		changed = append(changed, "smtp_from_email")
	}
	if before.SMTPFromName != after.SMTPFromName {
		changed = append(changed, "smtp_from_name")
	}
	if before.SMTPUseTLS != after.SMTPUseTLS {
		changed = append(changed, "smtp_use_tls")
	}
	if before.TurnstileEnabled != after.TurnstileEnabled {
		changed = append(changed, "turnstile_enabled")
	}
	if before.TurnstileSiteKey != after.TurnstileSiteKey {
		changed = append(changed, "turnstile_site_key")
	}
	if req.TurnstileSecretKey != "" {
		changed = append(changed, "turnstile_secret_key")
	}
	if before.LinuxDoConnectEnabled != after.LinuxDoConnectEnabled {
		changed = append(changed, "linuxdo_connect_enabled")
	}
	if before.LinuxDoConnectClientID != after.LinuxDoConnectClientID {
		changed = append(changed, "linuxdo_connect_client_id")
	}
	if req.LinuxDoConnectClientSecret != "" {
		changed = append(changed, "linuxdo_connect_client_secret")
	}
	if before.LinuxDoConnectRedirectURL != after.LinuxDoConnectRedirectURL {
		changed = append(changed, "linuxdo_connect_redirect_url")
	}
	if before.SiteName != after.SiteName {
		changed = append(changed, "site_name")
	}
	if before.SiteLogo != after.SiteLogo {
		changed = append(changed, "site_logo")
	}
	if before.SiteSubtitle != after.SiteSubtitle {
		changed = append(changed, "site_subtitle")
	}
	if before.APIBaseURL != after.APIBaseURL {
		changed = append(changed, "api_base_url")
	}
	if before.ContactInfo != after.ContactInfo {
		changed = append(changed, "contact_info")
	}
	if before.DocURL != after.DocURL {
		changed = append(changed, "doc_url")
	}
	if before.HomeContent != after.HomeContent {
		changed = append(changed, "home_content")
	}
	if before.DefaultConcurrency != after.DefaultConcurrency {
		changed = append(changed, "default_concurrency")
	}
	if before.DefaultBalance != after.DefaultBalance {
		changed = append(changed, "default_balance")
	}
	if before.EnableModelFallback != after.EnableModelFallback {
		changed = append(changed, "enable_model_fallback")
	}
	if before.FallbackModelAnthropic != after.FallbackModelAnthropic {
		changed = append(changed, "fallback_model_anthropic")
	}
	if before.FallbackModelOpenAI != after.FallbackModelOpenAI {
		changed = append(changed, "fallback_model_openai")
	}
	if before.FallbackModelGemini != after.FallbackModelGemini {
		changed = append(changed, "fallback_model_gemini")
	}
	if before.FallbackModelAntigravity != after.FallbackModelAntigravity {
		changed = append(changed, "fallback_model_antigravity")
	}
	if before.EnableIdentityPatch != after.EnableIdentityPatch {
		changed = append(changed, "enable_identity_patch")
	}
	if before.IdentityPatchPrompt != after.IdentityPatchPrompt {
		changed = append(changed, "identity_patch_prompt")
	}
	return changed
}

// [LITE:DELETED] TestSMTPRequest, TestSMTPConnection, SendTestEmailRequest, SendTestEmail
// 这些 SMTP 相关的测试功能在 Lite 模式中已移除，因为 Lite 不需要邮件服务

// GetAdminAPIKey 获取管理员 API Key 状态
// GET /api/v1/admin/settings/admin-api-key
func (h *SettingHandler) GetAdminAPIKey(c *gin.Context) {
	maskedKey, exists, err := h.settingService.GetAdminAPIKeyStatus(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{
		"exists":     exists,
		"masked_key": maskedKey,
	})
}

// RegenerateAdminAPIKey 生成/重新生成管理员 API Key
// POST /api/v1/admin/settings/admin-api-key/regenerate
func (h *SettingHandler) RegenerateAdminAPIKey(c *gin.Context) {
	key, err := h.settingService.GenerateAdminAPIKey(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{
		"key": key, // 完整 key 只在生成时返回一次
	})
}

// DeleteAdminAPIKey 删除管理员 API Key
// DELETE /api/v1/admin/settings/admin-api-key
func (h *SettingHandler) DeleteAdminAPIKey(c *gin.Context) {
	if err := h.settingService.DeleteAdminAPIKey(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{"message": "Admin API key deleted"})
}
