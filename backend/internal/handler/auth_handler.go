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
}

// NewAuthHandler creates a new AuthHandler [LITE] 简化版
func NewAuthHandler(
	cfg *config.Config,
	authService *service.AuthService,
	userService *service.UserService,
	settingService *service.SettingService,
) *AuthHandler {
	return &AuthHandler{
		cfg:         cfg,
		authService: authService,
		userService: userService,
		settingSvc:  settingService,
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

	type UserResponse struct {
		*dto.User
		RunMode string `json:"run_mode"`
	}

	runMode := config.RunModeStandard
	if h.cfg != nil {
		runMode = h.cfg.RunMode
	}

	response.Success(c, UserResponse{User: dto.UserFromService(user), RunMode: runMode})
}
