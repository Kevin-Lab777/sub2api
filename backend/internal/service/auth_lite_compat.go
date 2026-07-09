package service

import (
	"context"
	"time"
)

// [LITE] User registration and user-side OAuth remain disabled. The methods in
// this file keep upstream call sites build-compatible without wiring deleted
// registration, email, invitation, promo, or affiliate flows back into Lite.

func (s *AuthService) Register(ctx context.Context, email, password string) (string, *User, error) {
	return "", nil, ErrRegDisabled
}

func (s *AuthService) RegisterWithVerification(
	ctx context.Context,
	email string,
	password string,
	verifyCode string,
	invitationCode string,
	affiliateCode string,
	promoCode string,
) (*TokenPair, *User, error) {
	return nil, nil, ErrRegDisabled
}

func (s *AuthService) SendVerifyCode(ctx context.Context, email string, locale ...string) error {
	return ErrRegDisabled
}

func (s *AuthService) LoginOrRegisterOAuthWithTokenPair(
	ctx context.Context,
	email string,
	username string,
	invitationCode string,
	affiliateCode string,
	signupSource string,
) (*TokenPair, *User, error) {
	return nil, nil, ErrRegDisabled
}

func (s *AuthService) LoginOrRegisterOAuthWithTokenPairAndPromoCode(
	ctx context.Context,
	email string,
	username string,
	invitationCode string,
	affiliateCode string,
	promoCode string,
	signupSource string,
) (*TokenPair, *User, error) {
	return nil, nil, ErrRegDisabled
}

func (s *AuthService) GenerateTokenPair(ctx context.Context, user *User, refreshToken string) (*TokenPair, error) {
	if user == nil {
		return nil, ErrInvalidCredentials
	}
	accessToken, err := s.GenerateToken(user)
	if err != nil {
		return nil, err
	}
	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    s.GetAccessTokenExpiresIn(),
	}, nil
}

func (s *AuthService) GetAccessTokenExpiresIn() int {
	if s == nil || s.cfg == nil {
		return 0
	}
	if s.cfg.JWT.AccessTokenExpireMinutes > 0 {
		return s.cfg.JWT.AccessTokenExpireMinutes * int(time.Minute/time.Second)
	}
	return s.cfg.JWT.ExpireHour * int(time.Hour/time.Second)
}

func (s *AuthService) RevokeAllUserSessions(ctx context.Context, userID int64) error {
	if s == nil || s.refreshTokenCache == nil || userID <= 0 {
		return nil
	}
	return s.refreshTokenCache.DeleteUserRefreshTokens(ctx, userID)
}

func (s *AuthService) validateRegistrationEmailPolicy(ctx context.Context, email string) error {
	return ErrRegDisabled
}

func (s *AuthService) canBypassRegistrationDisabledForOAuth(ctx context.Context, signupSource string) bool {
	return false
}

func (s *AuthService) ApplyProviderDefaultSettingsOnFirstBind(ctx context.Context, userID int64, providerType string) error {
	return nil
}

func (s *AuthService) resolveSignupGrantPlan(ctx context.Context, signupSource string) ProviderDefaultGrantSettings {
	return ProviderDefaultGrantSettings{}
}

func (s *AuthService) assignSubscriptions(ctx context.Context, userID int64, subscriptions []DefaultSubscriptionSetting, note string) {
}

func (s *AuthService) snapshotPlatformQuotaDefaults(ctx context.Context, userID int64, grantPlan *ProviderDefaultGrantSettings) error {
	return nil
}

func (s *AuthService) postAuthUserBootstrap(ctx context.Context, user *User, signupSource string, firstBind bool) {
}

func (s *AuthService) applyOAuthSignupPromoCode(ctx context.Context, user *User, promoCode string) *User {
	return user
}

func (s *AuthService) bindOAuthAffiliate(ctx context.Context, userID int64, affiliateCode string) {
}

func (s *AuthService) touchUserLogin(ctx context.Context, userID int64) {
}

func (s *AuthService) backfillEmailIdentityOnSuccessfulLogin(ctx context.Context, user *User) {
}
