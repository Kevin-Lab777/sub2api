package handler

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// UserHandler exposes only administrator self-service identity operations.
type UserHandler struct {
	userService *service.UserService
}

func NewUserHandler(userService *service.UserService) *UserHandler {
	return &UserHandler{userService: userService}
}

type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6"`
}

type UpdateProfileRequest struct {
	Username  *string `json:"username"`
	AvatarURL *string `json:"avatar_url"`
}

type userProfileResponse struct {
	dto.User
	AvatarURL         string                                 `json:"avatar_url,omitempty"`
	AvatarSource      *userProfileSourceContext              `json:"avatar_source,omitempty"`
	UsernameSource    *userProfileSourceContext              `json:"username_source,omitempty"`
	DisplayNameSource *userProfileSourceContext              `json:"display_name_source,omitempty"`
	NicknameSource    *userProfileSourceContext              `json:"nickname_source,omitempty"`
	ProfileSources    map[string]*userProfileSourceContext   `json:"profile_sources,omitempty"`
	Identities        service.UserIdentitySummarySet         `json:"identities"`
	AuthBindings      map[string]service.UserIdentitySummary `json:"auth_bindings"`
	IdentityBindings  map[string]service.UserIdentitySummary `json:"identity_bindings"`
	EmailBound        bool                                   `json:"email_bound"`
	LinuxDoBound      bool                                   `json:"linuxdo_bound"`
	OIDCBound         bool                                   `json:"oidc_bound"`
	WeChatBound       bool                                   `json:"wechat_bound"`
	DingTalkBound     bool                                   `json:"dingtalk_bound"`
}

type userProfileSourceContext struct {
	Provider string `json:"provider,omitempty"`
	Source   string `json:"source,omitempty"`
}

func (h *UserHandler) GetProfile(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	user, err := h.userService.GetProfile(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	profile, err := h.buildUserProfileResponse(c.Request.Context(), subject.UserID, user)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, profile)
}

func (h *UserHandler) ChangePassword(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := h.userService.ChangePassword(c.Request.Context(), subject.UserID, service.ChangePasswordRequest{
		CurrentPassword: req.OldPassword,
		NewPassword:     req.NewPassword,
	}); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "Password changed successfully"})
}

func (h *UserHandler) UpdateProfile(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	user, err := h.userService.UpdateProfile(c.Request.Context(), subject.UserID, service.UpdateProfileRequest{
		Username:  req.Username,
		AvatarURL: req.AvatarURL,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	profile, err := h.buildUserProfileResponse(c.Request.Context(), subject.UserID, user)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, profile)
}

func (h *UserHandler) buildUserProfileResponse(ctx context.Context, userID int64, user *service.User) (userProfileResponse, error) {
	identities, err := h.userService.GetProfileIdentitySummaries(ctx, userID, user)
	if err != nil {
		return userProfileResponse{}, err
	}
	return userProfileResponseFromService(user, identities), nil
}

func userProfileResponseFromService(user *service.User, identities service.UserIdentitySummarySet) userProfileResponse {
	base := dto.UserFromService(user)
	if base == nil {
		return userProfileResponse{}
	}
	bindings := userProfileBindingMap(identities)
	profileSources, avatarSource, usernameSource := inferUserProfileSources(user, identities)
	return userProfileResponse{
		User:              *base,
		AvatarURL:         user.AvatarURL,
		AvatarSource:      avatarSource,
		UsernameSource:    usernameSource,
		DisplayNameSource: usernameSource,
		NicknameSource:    usernameSource,
		ProfileSources:    profileSources,
		Identities:        identities,
		AuthBindings:      bindings,
		IdentityBindings:  bindings,
		EmailBound:        identities.Email.Bound,
		LinuxDoBound:      identities.LinuxDo.Bound,
		OIDCBound:         identities.OIDC.Bound,
		WeChatBound:       identities.WeChat.Bound,
		DingTalkBound:     identities.DingTalk.Bound,
	}
}

func userProfileBindingMap(identities service.UserIdentitySummarySet) map[string]service.UserIdentitySummary {
	return map[string]service.UserIdentitySummary{
		"email": identities.Email, "linuxdo": identities.LinuxDo, "oidc": identities.OIDC,
		"wechat": identities.WeChat, "dingtalk": identities.DingTalk,
	}
}

func inferUserProfileSources(user *service.User, identities service.UserIdentitySummarySet) (
	map[string]*userProfileSourceContext,
	*userProfileSourceContext,
	*userProfileSourceContext,
) {
	if user == nil {
		return nil, nil, nil
	}
	thirdParty := thirdPartyIdentityProviders(identities)
	var avatarSource *userProfileSourceContext
	for _, summary := range thirdParty {
		if avatar := strings.TrimSpace(user.AvatarURL); avatar != "" && avatar == strings.TrimSpace(summary.AvatarURL) {
			avatarSource = buildUserProfileSourceContext(summary.Provider)
			break
		}
	}
	var usernameSource *userProfileSourceContext
	for _, summary := range thirdParty {
		if username := strings.TrimSpace(user.Username); username != "" && username == strings.TrimSpace(summary.DisplayName) {
			usernameSource = buildUserProfileSourceContext(summary.Provider)
			break
		}
	}
	profileSources := map[string]*userProfileSourceContext{}
	if avatarSource != nil {
		profileSources["avatar"] = avatarSource
	}
	if usernameSource != nil {
		profileSources["username"] = usernameSource
		profileSources["display_name"] = usernameSource
		profileSources["nickname"] = usernameSource
	}
	if len(profileSources) == 0 {
		profileSources = nil
	}
	return profileSources, avatarSource, usernameSource
}

func thirdPartyIdentityProviders(identities service.UserIdentitySummarySet) []service.UserIdentitySummary {
	providers := []service.UserIdentitySummary{identities.LinuxDo, identities.OIDC, identities.WeChat, identities.DingTalk}
	out := make([]service.UserIdentitySummary, 0, len(providers))
	for _, summary := range providers {
		if summary.Bound {
			out = append(out, summary)
		}
	}
	return out
}

func buildUserProfileSourceContext(provider string) *userProfileSourceContext {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return nil
	}
	return &userProfileSourceContext{Provider: provider, Source: provider}
}
