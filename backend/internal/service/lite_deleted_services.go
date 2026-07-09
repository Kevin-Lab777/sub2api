package service

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/Kevin-Lab777/sub2api/internal/pkg/pagination"
)

var errLiteDeletedService = errors.New("service is not available in Lite")

const (
	verifyCodeTTL         = 10 * time.Minute
	verifyCodeCooldown    = 60 * time.Second
	maxVerifyCodeAttempts = 5
)

// Fingerprint is retained as a data shape for OAuth gateway call sites. The
// producer service is removed in Lite, so no fingerprints are created.
type Fingerprint struct {
	ClientID  string
	UserAgent string
}

// IdentityCache is kept for services that can optionally read cached
// fingerprints. Lite wires it as nil.
type IdentityCache interface {
	GetFingerprint(ctx context.Context, accountID int64) (*Fingerprint, error)
}

// IdentityService is a no-op compatibility shim. [LITE:DELETED] The real
// identity service is intentionally not wired.
type IdentityService struct{}

func (*IdentityService) GetFingerprint(ctx context.Context, accountID int64) (*Fingerprint, error) {
	return nil, errLiteDeletedService
}

func (*IdentityService) GetOrCreateFingerprint(ctx context.Context, accountID int64, headers http.Header) (*Fingerprint, error) {
	return nil, errLiteDeletedService
}

func (*IdentityService) RewriteUserIDWithMasking(ctx context.Context, body []byte, account *Account, accountUUID, clientID, userAgent string) ([]byte, error) {
	return body, nil
}

func (*IdentityService) ApplyFingerprint(req *http.Request, fingerprint *Fingerprint) {}

// VerificationCodeData and EmailCache are retained for admin-only Lite code
// paths and tests that still pass nil email services.
type VerificationCodeData struct {
	Code      string
	Attempts  int
	CreatedAt time.Time
	ExpiresAt time.Time
}

type PasswordResetTokenData struct {
	Token     string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type EmailCache interface {
	GetVerificationCode(ctx context.Context, email string) (*VerificationCodeData, error)
	SetVerificationCode(ctx context.Context, email string, data *VerificationCodeData, ttl time.Duration) error
	DeleteVerificationCode(ctx context.Context, email string) error
	GetNotifyVerifyCode(ctx context.Context, email string) (*VerificationCodeData, error)
	SetNotifyVerifyCode(ctx context.Context, email string, data *VerificationCodeData, ttl time.Duration) error
	DeleteNotifyVerifyCode(ctx context.Context, email string) error
	GetPasswordResetToken(ctx context.Context, email string) (*PasswordResetTokenData, error)
	SetPasswordResetToken(ctx context.Context, email string, data *PasswordResetTokenData, ttl time.Duration) error
	DeletePasswordResetToken(ctx context.Context, email string) error
	IsPasswordResetEmailInCooldown(ctx context.Context, email string) bool
	SetPasswordResetEmailCooldown(ctx context.Context, email string, ttl time.Duration) error
	GetNotifyCodeUserRate(ctx context.Context, userID int64) (int64, error)
	IncrNotifyCodeUserRate(ctx context.Context, userID int64, window time.Duration) (int64, error)
}

type SendVerifyCodeResult struct {
	Countdown int `json:"countdown"`
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
}

type EmailService struct {
	notificationEmailService *NotificationEmailService
}

func NewEmailService(settingRepo SettingRepository, cache EmailCache) *EmailService {
	return &EmailService{}
}

func (s *EmailService) SetNotificationEmailService(notificationEmailService *NotificationEmailService) {
	if s != nil {
		s.notificationEmailService = notificationEmailService
	}
}

func (*EmailService) GenerateVerifyCode() (string, error) {
	return "", errLiteDeletedService
}

func (*EmailService) SendVerifyCode(ctx context.Context, email, siteName string, locale ...string) error {
	return errLiteDeletedService
}

func (*EmailService) VerifyCode(ctx context.Context, email, code string) error {
	return errLiteDeletedService
}

func (*EmailService) SendEmail(ctx context.Context, to, subject, body string) error {
	return errLiteDeletedService
}

type EmailTask struct {
	Email    string
	SiteName string
	Locale   string
}

type EmailQueueService struct {
	taskChan chan EmailTask
}

func (q *EmailQueueService) EnqueueVerifyCode(email, siteName string, locale ...string) error {
	if q != nil && q.taskChan != nil {
		select {
		case q.taskChan <- EmailTask{Email: email, SiteName: siteName, Locale: firstEmailLocale(locale)}:
		default:
		}
	}
	return errLiteDeletedService
}

var (
	ErrRedeemCodeNotFound = errors.New("redeem code not found")
	ErrRedeemCodeUsed     = errors.New("redeem code used")
	ErrRedeemCodeExpired  = errors.New("redeem code expired")
)

type RedeemCode struct {
	ID           int64
	Code         string
	Type         string
	Value        float64
	Status       string
	UserID       *int64
	UsedBy       *int64
	UsedAt       *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
	ExpiresAt    *time.Time
	GroupID      *int64
	ValidityDays int
	Notes        string
	User         *User
	Group        *Group
}

func (r RedeemCode) CanUse() bool {
	if r.Status != StatusUnused {
		return false
	}
	return r.ExpiresAt == nil || r.ExpiresAt.After(time.Now())
}

func (r RedeemCode) IsExpired() bool {
	return r.ExpiresAt != nil && time.Now().After(*r.ExpiresAt)
}

type RedeemCodeBatchUpdateFields struct {
	Status    *string
	ExpiresAt *time.Time
	Notes     *string
}

type RedeemCodeRepository interface {
	Create(ctx context.Context, code *RedeemCode) error
	CreateBatch(ctx context.Context, codes []RedeemCode) error
	GetByID(ctx context.Context, id int64) (*RedeemCode, error)
	GetByCode(ctx context.Context, code string) (*RedeemCode, error)
	Update(ctx context.Context, code *RedeemCode) error
	BatchUpdate(ctx context.Context, ids []int64, fields RedeemCodeBatchUpdateFields) (int64, error)
	Delete(ctx context.Context, id int64) error
	Use(ctx context.Context, id, userID int64) error
	List(ctx context.Context, params pagination.PaginationParams) ([]RedeemCode, *pagination.PaginationResult, error)
	ListWithFilters(ctx context.Context, params pagination.PaginationParams, codeType, status, search string) ([]RedeemCode, *pagination.PaginationResult, error)
	ListByUser(ctx context.Context, userID int64, limit int) ([]RedeemCode, error)
	ListByUserPaginated(ctx context.Context, userID int64, params pagination.PaginationParams, codeType string) ([]RedeemCode, *pagination.PaginationResult, error)
	SumPositiveBalanceByUser(ctx context.Context, userID int64) (float64, error)
}

func GenerateRedeemCode() (string, error) {
	return "", errLiteDeletedService
}

type PromoCode struct {
	ID          int64
	Code        string
	BonusAmount float64
	MaxUses     int
	UsedCount   int
	Status      string
	ExpiresAt   *time.Time
	Notes       string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type PromoCodeUsage struct {
	ID          int64
	PromoCodeID int64
	UserID      int64
	BonusAmount float64
	UsedAt      time.Time
	User        *User
}
