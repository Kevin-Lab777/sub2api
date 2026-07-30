package handler

import (
	"context"
	"log/slog"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// AuthHandler exposes only administrator authentication for Next API.
type AuthHandler struct {
	cfg         *config.Config
	authService *service.AdminAuthService
	userService *service.UserService
	settingSvc  *service.SettingService
	totpService *service.TotpService
}

func NewAuthHandler(
	cfg *config.Config,
	authService *service.AdminAuthService,
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

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type AuthResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresIn    int       `json:"expires_in"`
	TokenType    string    `json:"token_type"`
	User         *dto.User `json:"user"`
}

func ensureLoginUserActive(user *service.User) error {
	if user == nil {
		return infraerrors.Unauthorized("INVALID_USER", "user not found")
	}
	if !user.IsActive() {
		return service.ErrUserNotActive
	}
	return nil
}

func ensureNextAPIAdminLogin(user *service.User) error {
	if err := ensureLoginUserActive(user); err != nil {
		return err
	}
	if !user.IsAdmin() {
		return infraerrors.Forbidden("NEXT_API_ADMIN_ONLY", "Only administrator login is allowed.")
	}
	return nil
}

func (h *AuthHandler) ensureBackendModeAllowsUser(_ context.Context, user *service.User) error {
	return ensureNextAPIAdminLogin(user)
}

func (h *AuthHandler) respondWithTokenPair(c *gin.Context, user *service.User) {
	respondWithTokenPair(c, h.authService, user)
}

func respondWithTokenPair(c *gin.Context, authService *service.AdminAuthService, user *service.User) {
	if err := ensureNextAPIAdminLogin(user); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if authService == nil {
		response.InternalError(c, "Authentication service is not configured")
		return
	}

	tokenPair, err := authService.GenerateTokenPair(c.Request.Context(), user, "")
	if err != nil {
		slog.Error("failed to generate administrator token pair", "error", err, "user_id", user.ID)
		response.InternalError(c, "Failed to generate authentication session")
		return
	}
	response.Success(c, AuthResponse{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresIn:    tokenPair.ExpiresIn,
		TokenType:    "Bearer",
		User:         dto.UserFromService(user),
	})
}

// Login authenticates an administrator with a local password.
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if h.authService == nil {
		response.InternalError(c, "Authentication service is not configured")
		return
	}

	user, err := h.authService.Authenticate(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err := ensureNextAPIAdminLogin(user); err != nil {
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

	h.authService.RecordSuccessfulLogin(c.Request.Context(), user.ID)
	h.respondWithTokenPair(c, user)
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

func (h *AuthHandler) Login2FA(c *gin.Context) {
	if h.totpService == nil || h.userService == nil || h.authService == nil {
		response.InternalError(c, "Two-factor authentication is not configured")
		return
	}
	var req Login2FARequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
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
	if err := ensureNextAPIAdminLogin(user); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err := h.totpService.DeleteLoginSession(c.Request.Context(), req.TempToken); err != nil {
		response.InternalError(c, "Failed to consume 2FA session")
		return
	}

	h.authService.RecordSuccessfulLogin(c.Request.Context(), user.ID)
	h.respondWithTokenPair(c, user)
}

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
	if err := ensureNextAPIAdminLogin(user); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	identities, err := h.userService.GetProfileIdentitySummaries(c.Request.Context(), subject.UserID, user)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	type userResponse struct {
		userProfileResponse
		RunMode string `json:"run_mode"`
	}
	runMode := config.RunModeStandard
	if h.cfg != nil {
		runMode = h.cfg.RunMode
	}
	response.Success(c, userResponse{
		userProfileResponse: userProfileResponseFromService(user, identities),
		RunMode:             runMode,
	})
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type RefreshTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result, err := h.authService.RefreshTokenPair(c.Request.Context(), req.RefreshToken)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if result.UserRole != service.RoleAdmin {
		response.Forbidden(c, "Only administrator sessions can be refreshed")
		return
	}
	response.Success(c, RefreshTokenResponse{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresIn:    result.ExpiresIn,
		TokenType:    "Bearer",
	})
}

type LogoutRequest struct {
	RefreshToken string `json:"refresh_token,omitempty"`
}

type LogoutResponse struct {
	Message string `json:"message"`
}

func (h *AuthHandler) Logout(c *gin.Context) {
	var req LogoutRequest
	_ = c.ShouldBindJSON(&req)
	if req.RefreshToken != "" {
		if err := h.authService.RevokeRefreshToken(c.Request.Context(), req.RefreshToken); err != nil {
			response.ErrorFrom(c, err)
			return
		}
	}
	response.Success(c, LogoutResponse{Message: "Logged out successfully"})
}

type RevokeAllSessionsResponse struct {
	Message string `json:"message"`
}

func (h *AuthHandler) RevokeAllSessions(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	if err := h.authService.RevokeAllUserTokens(c.Request.Context(), subject.UserID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, RevokeAllSessionsResponse{Message: "All sessions have been revoked. Please log in again."})
}
