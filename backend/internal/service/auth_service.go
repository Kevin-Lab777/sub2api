package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	dbent "github.com/Kevin-Lab777/sub2api/ent"
	"github.com/Kevin-Lab777/sub2api/internal/config"
	infraerrors "github.com/Kevin-Lab777/sub2api/internal/pkg/errors"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// [LITE] 简化版 AuthService - 只支持 Admin 登录，移除注册和多用户功能

var (
	ErrInvalidCredentials  = infraerrors.Unauthorized("INVALID_CREDENTIALS", "invalid email or password")
	ErrInvalidToken        = infraerrors.Unauthorized("INVALID_TOKEN", "invalid token")
	ErrTokenExpired        = infraerrors.Unauthorized("TOKEN_EXPIRED", "token has expired")
	ErrTokenTooLarge       = infraerrors.BadRequest("TOKEN_TOO_LARGE", "token too large")
	ErrTokenRevoked        = infraerrors.Unauthorized("TOKEN_REVOKED", "token has been revoked")
	ErrRegDisabled         = infraerrors.Forbidden("REGISTRATION_DISABLED", "registration is disabled in Lite mode")
	ErrServiceUnavailable  = infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "service temporarily unavailable")
	ErrAdminOnly           = infraerrors.Forbidden("ADMIN_ONLY", "only admin users can login in Lite mode")
	ErrUserNotActive       = infraerrors.Forbidden("USER_NOT_ACTIVE", "user is not active")
	ErrEmailExists         = infraerrors.Conflict("EMAIL_EXISTS", "email already exists")
	ErrEmailReserved       = infraerrors.BadRequest("EMAIL_RESERVED", "email is reserved")
	ErrEmailVerifyRequired = infraerrors.BadRequest(
		"EMAIL_VERIFY_REQUIRED",
		"email verification is required",
	)
	ErrInvalidVerifyCode     = infraerrors.BadRequest("INVALID_VERIFY_CODE", "invalid verification code")
	ErrVerifyCodeTooFrequent = infraerrors.TooManyRequests(
		"VERIFY_CODE_TOO_FREQUENT",
		"verification code requested too frequently",
	)
	ErrVerifyCodeMaxAttempts = infraerrors.TooManyRequests(
		"VERIFY_CODE_MAX_ATTEMPTS",
		"verification code max attempts exceeded",
	)
	ErrEmailSuffixNotAllowed = infraerrors.Forbidden(
		"EMAIL_SUFFIX_NOT_ALLOWED",
		"email suffix is not allowed",
	)
	ErrInvitationCodeRequired  = infraerrors.BadRequest("INVITATION_CODE_REQUIRED", "invitation code is required")
	ErrOAuthInvitationRequired = infraerrors.Forbidden(
		"OAUTH_INVITATION_REQUIRED",
		"invitation code is required for oauth signup",
	)
	ErrInvitationCodeInvalid = infraerrors.BadRequest("INVITATION_CODE_INVALID", "invitation code is invalid")
)

// maxTokenLength 限制 token 大小，避免超长 header 触发解析时的异常内存分配
const maxTokenLength = 8192

// JWTClaims JWT载荷数据
type JWTClaims struct {
	UserID       int64  `json:"user_id"`
	Email        string `json:"email"`
	Role         string `json:"role"`
	TokenVersion int64  `json:"token_version"`
	SessionID    string `json:"sid,omitempty"`
	BindingHash  string `json:"bnd,omitempty"`
	jwt.RegisteredClaims
}

// AuthService 认证服务 [LITE] 简化版
type AuthService struct {
	userRepo                    UserRepository
	cfg                         *config.Config
	settingService              *SettingService
	entClient                   *dbent.Client
	refreshTokenCache           RefreshTokenCache
	emailService                *EmailService
	defaultSubscriptionAssigner DefaultSubscriptionAssigner
	affiliateService            *AffiliateService
	userPlatformQuotaRepo       UserPlatformQuotaRepository
}

// NewAuthService 创建认证服务实例 [LITE] 简化版
func NewAuthService(
	userRepo UserRepository,
	cfg *config.Config,
	settingService *SettingService,
) *AuthService {
	return &AuthService{
		userRepo:       userRepo,
		cfg:            cfg,
		settingService: settingService,
	}
}

// Login 用户登录 [LITE] 只允许 admin 用户登录
func (s *AuthService) Login(ctx context.Context, email, password string) (string, *User, error) {
	// 查找用户
	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return "", nil, ErrInvalidCredentials
		}
		log.Printf("[Auth] Database error during login: %v", err)
		return "", nil, ErrServiceUnavailable
	}

	// 验证密码
	if !s.CheckPassword(password, user.PasswordHash) {
		return "", nil, ErrInvalidCredentials
	}

	// 检查用户状态
	if !user.IsActive() {
		return "", nil, ErrUserNotActive
	}

	// [LITE] 只允许 admin 用户登录
	if user.Role != RoleAdmin {
		return "", nil, ErrAdminOnly
	}

	// 生成JWT token
	token, err := s.GenerateToken(user)
	if err != nil {
		return "", nil, fmt.Errorf("generate token: %w", err)
	}

	return token, user, nil
}

// VerifyTurnstile [LITE] 简化版 - 直接返回 nil，不做验证
func (s *AuthService) VerifyTurnstile(ctx context.Context, token string, remoteIP string) error {
	return nil
}

// ValidateToken 验证JWT token并返回用户声明
func (s *AuthService) ValidateToken(tokenString string) (*JWTClaims, error) {
	if len(tokenString) > maxTokenLength {
		return nil, ErrTokenTooLarge
	}

	parser := jwt.NewParser(jwt.WithValidMethods([]string{
		jwt.SigningMethodHS256.Name,
		jwt.SigningMethodHS384.Name,
		jwt.SigningMethodHS512.Name,
	}))

	token, err := parser.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.cfg.JWT.Secret), nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			if claims, ok := token.Claims.(*JWTClaims); ok {
				return claims, ErrTokenExpired
			}
			return nil, ErrTokenExpired
		}
		return nil, ErrInvalidToken
	}

	if claims, ok := token.Claims.(*JWTClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, ErrInvalidToken
}

func randomHexString(byteLength int) (string, error) {
	if byteLength <= 0 {
		byteLength = 16
	}
	buf := make([]byte, byteLength)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func isReservedEmail(email string) bool {
	normalized := strings.ToLower(strings.TrimSpace(email))
	return strings.HasSuffix(normalized, LinuxDoConnectSyntheticEmailDomain) ||
		strings.HasSuffix(normalized, OIDCConnectSyntheticEmailDomain) ||
		strings.HasSuffix(normalized, WeChatConnectSyntheticEmailDomain) ||
		strings.HasSuffix(normalized, DingTalkConnectSyntheticEmailDomain)
}

// GenerateToken 生成JWT access token
// 使用新的access_token_expire_minutes配置项（如果配置了），否则回退到expire_hour
func (s *AuthService) GenerateToken(user *User) (string, error) {
	now := time.Now()
	expiresAt := now.Add(time.Duration(s.GetAccessTokenExpiresIn()) * time.Second)

	claims := &JWTClaims{
		UserID:       user.ID,
		Email:        user.Email,
		Role:         user.Role,
		TokenVersion: user.TokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(s.cfg.JWT.Secret))
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}

	return tokenString, nil
}

// HashPassword 使用bcrypt加密密码
func (s *AuthService) HashPassword(password string) (string, error) {
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashedBytes), nil
}

// CheckPassword 验证密码是否匹配
func (s *AuthService) CheckPassword(password, hashedPassword string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	return err == nil
}

// RefreshToken 刷新token
func (s *AuthService) RefreshToken(ctx context.Context, oldTokenString string) (string, error) {
	claims, err := s.ValidateToken(oldTokenString)
	if err != nil && !errors.Is(err, ErrTokenExpired) {
		return "", err
	}

	user, err := s.userRepo.GetByID(ctx, claims.UserID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return "", ErrInvalidToken
		}
		log.Printf("[Auth] Database error refreshing token: %v", err)
		return "", ErrServiceUnavailable
	}

	if !user.IsActive() {
		return "", ErrUserNotActive
	}

	// [LITE] 只允许 admin 刷新 token
	if user.Role != RoleAdmin {
		return "", ErrAdminOnly
	}

	if claims.TokenVersion != user.TokenVersion {
		return "", ErrTokenRevoked
	}

	return s.GenerateToken(user)
}
