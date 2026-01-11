// Package handler provides HTTP handlers for public endpoints
// [LITE] Public endpoints for Lite mode
package handler

import (
	"crypto/subtle"

	"github.com/Kevin-Lab777/sub2api/internal/handler/dto"
	"github.com/Kevin-Lab777/sub2api/internal/pkg/response"
	"github.com/Kevin-Lab777/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// PublicHandler handles public endpoints that don't require authentication
// [LITE] Added for Lite mode admin API key login
type PublicHandler struct {
	settingService *service.SettingService
	userService    *service.UserService
	version        string
}

// NewPublicHandler creates a new PublicHandler
func NewPublicHandler(
	settingService *service.SettingService,
	userService *service.UserService,
	version string,
) *PublicHandler {
	return &PublicHandler{
		settingService: settingService,
		userService:    userService,
		version:        version,
	}
}

// AdminLoginRequest represents the admin login request body
type AdminLoginRequest struct {
	APIKey string `json:"api_key" binding:"required"`
}

// AdminLoginResponse represents the admin login response
type AdminLoginResponse struct {
	AccessToken string   `json:"access_token"`
	User        dto.User `json:"user"`
}

// AdminLogin validates admin API key and returns access token
// POST /api/v1/admin/auth/login
// [LITE] This endpoint allows login via admin API key
func (h *PublicHandler) AdminLogin(c *gin.Context) {
	var req AdminLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: api_key is required")
		return
	}

	// Get stored admin API key
	storedKey, err := h.settingService.GetAdminAPIKey(c.Request.Context())
	if err != nil {
		response.InternalError(c, "Internal server error")
		return
	}

	// Validate API key
	if storedKey == "" || subtle.ConstantTimeCompare([]byte(req.APIKey), []byte(storedKey)) != 1 {
		response.Unauthorized(c, "Invalid admin API key")
		return
	}

	// Get admin user
	admin, err := h.userService.GetFirstAdmin(c.Request.Context())
	if err != nil {
		response.InternalError(c, "No admin user found")
		return
	}

	// [LITE] In Lite mode, the admin API key itself serves as the access token
	// The frontend will send it as Authorization: Bearer <api_key>
	response.Success(c, AdminLoginResponse{
		AccessToken: req.APIKey, // Return the API key as the token
		User: dto.User{
			ID:          admin.ID,
			Email:       admin.Email,
			Role:        admin.Role,
			Status:      string(admin.Status),
			Balance:     admin.Balance,
			Concurrency: admin.Concurrency,
			CreatedAt:   admin.CreatedAt,
			UpdatedAt:   admin.UpdatedAt,
		},
	})
}

// GetCurrentAdmin returns the current admin user info
// GET /api/v1/admin/me
func (h *PublicHandler) GetCurrentAdmin(c *gin.Context) {
	// Get admin user (already authenticated via middleware)
	admin, err := h.userService.GetFirstAdmin(c.Request.Context())
	if err != nil {
		response.InternalError(c, "Failed to get admin user")
		return
	}

	response.Success(c, dto.User{
		ID:          admin.ID,
		Email:       admin.Email,
		Role:        admin.Role,
		Status:      string(admin.Status),
		Balance:     admin.Balance,
		Concurrency: admin.Concurrency,
		CreatedAt:   admin.CreatedAt,
		UpdatedAt:   admin.UpdatedAt,
	})
}
