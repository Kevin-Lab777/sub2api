package handler

import (
	"github.com/Kevin-Lab777/sub2api/internal/config"
	"github.com/Kevin-Lab777/sub2api/internal/handler/dto"
	"github.com/Kevin-Lab777/sub2api/internal/pkg/response"
	middleware2 "github.com/Kevin-Lab777/sub2api/internal/server/middleware"
	"github.com/Kevin-Lab777/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// [LITE] 简化版 AuthHandler - 只支持 Admin 登录

// AuthHandler handles authentication-related requests
type AuthHandler struct {
	cfg         *config.Config
	authService *service.AuthService
	userService *service.UserService
	settingSvc  *service.SettingService
	totpService *service.TotpService
}

// NewAuthHandler creates a new AuthHandler [LITE] 简化版
func NewAuthHandler(
	cfg *config.Config,
	authService *service.AuthService,
	userService *service.UserService,
	settingService *service.SettingService,
	totpService *service.TotpService,
) *AuthHandler {
	return &AuthHandler{
		cfg:         cfg,
		authService: authService,
		userService: userService,
		settingSvc:  settingService,
		totpService: totpService,
	}
}

// LoginRequest represents the login request payload
type LoginRequest struct {
	Email          string `json:"email" binding:"required,email"`
	Password       string `json:"password" binding:"required"`
	TurnstileToken string `json:"turnstile_token"`
}

// AuthResponse 认证响应格式
type AuthResponse struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	User        *dto.User `json:"user"`
}

type TotpLoginResponse struct {
	Requires2FA     bool   `json:"requires_2fa"`
	TempToken       string `json:"temp_token,omitempty"`
	UserEmailMasked string `json:"user_email_masked,omitempty"`
}

type Login2FARequest struct {
	TempToken string `json:"temp_token" binding:"required"`
	TotpCode  string `json:"totp_code" binding:"required,len=6"`
}

// Login handles user login [LITE] 只允许 admin 登录
// POST /api/v1/auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// [LITE] Turnstile 验证已禁用
	// if err := h.authService.VerifyTurnstile(...); err != nil { ... }

	token, user, err := h.authService.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	if h.totpService != nil && h.settingSvc != nil && h.settingSvc.IsTotpEnabled(c.Request.Context()) && user.TotpEnabled {
		tempToken, err := h.totpService.CreateLoginSession(c.Request.Context(), user.ID, user.Email)
		if err != nil {
			response.InternalError(c, "Failed to create 2FA session")
			return
		}

		response.Success(c, TotpLoginResponse{
			Requires2FA:     true,
			TempToken:       tempToken,
			UserEmailMasked: service.MaskEmail(user.Email),
		})
		return
	}

	response.Success(c, AuthResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		User:        dto.UserFromService(user),
	})
}

// Login2FA completes admin login after TOTP verification.
// POST /api/v1/auth/login/2fa
func (h *AuthHandler) Login2FA(c *gin.Context) {
	var req Login2FARequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if h.totpService == nil {
		response.InternalError(c, "TOTP service unavailable")
		return
	}

	session, err := h.totpService.GetLoginSession(c.Request.Context(), req.TempToken)
	if err != nil || session == nil {
		response.BadRequest(c, "Invalid or expired 2FA session")
		return
	}

	if err := h.totpService.VerifyCode(c.Request.Context(), session.UserID, req.TotpCode); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	user, err := h.userService.GetByID(c.Request.Context(), session.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if !user.IsActive() {
		response.ErrorFrom(c, service.ErrUserNotActive)
		return
	}
	if user.Role != service.RoleAdmin {
		response.ErrorFrom(c, service.ErrAdminOnly)
		return
	}

	token, err := h.authService.GenerateToken(user)
	if err != nil {
		response.InternalError(c, "Failed to generate token")
		return
	}

	_ = h.totpService.DeleteLoginSession(c.Request.Context(), req.TempToken)

	response.Success(c, AuthResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		User:        dto.UserFromService(user),
	})
}

// GetCurrentUser handles getting current authenticated user
// GET /api/v1/auth/me
func (h *AuthHandler) GetCurrentUser(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	user, err := h.userService.GetByID(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	// [LITE] run_mode removed - always Lite mode
	response.Success(c, dto.UserFromService(user))
}
