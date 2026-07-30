package gatewayruntime

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/gatewaycore"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/gatewaytransport"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type OpenAIImagesDispatcherConfig struct {
	MaxAccountSwitches int
	MaxBodyBytes       int64
}

func OpenAIImagesDispatcherConfigFromApplication(cfg *config.Config) (OpenAIImagesDispatcherConfig, error) {
	if cfg == nil {
		return OpenAIImagesDispatcherConfig{}, errors.New("gateway application config is required")
	}
	return OpenAIImagesDispatcherConfig{
		MaxAccountSwitches: cfg.Gateway.MaxAccountSwitches,
		MaxBodyBytes:       cfg.Gateway.MaxBodySize,
	}, nil
}

type openAIImagesGateway interface {
	ValidateTechnicalRuntime() error
	SelectTechnicalImagesAccountWithLoadAwareness(context.Context, *int64, string, string, map[int64]struct{}) (*service.AccountSelectionResult, error)
	SelectTechnicalImagesDirectAccountWithLoadAwareness(context.Context, *int64, string, string, map[int64]struct{}) (*service.AccountSelectionResult, error)
	AcquireSelection(context.Context, *service.AccountSelectionResult) (func(), error)
	ForwardImagesDirectExchange(context.Context, gatewaytransport.Exchange, *service.Account, []byte) (*service.OpenAIForwardResult, error)
	ForwardImagesSubscriptionExchange(context.Context, gatewaytransport.Exchange, *service.Account, []byte) (*service.OpenAIForwardResult, error)
}

// OpenAIImagesDispatcher owns direct API-key Images forwarding and the strict
// subscription-account Responses adapter.
type OpenAIImagesDispatcher struct {
	gateway            openAIImagesGateway
	maxAccountSwitches int
	maxBodyBytes       int64
}

func NewOpenAIImagesDispatcher(gateway openAIImagesGateway, cfg OpenAIImagesDispatcherConfig) (*OpenAIImagesDispatcher, error) {
	if gateway == nil {
		return nil, errors.New("OpenAI Images gateway is required")
	}
	if err := gateway.ValidateTechnicalRuntime(); err != nil {
		return nil, fmt.Errorf("validate OpenAI Images gateway: %w", err)
	}
	if cfg.MaxAccountSwitches <= 0 {
		return nil, errors.New("OpenAI max account switches must be positive")
	}
	if cfg.MaxBodyBytes <= 0 {
		return nil, errors.New("OpenAI max body bytes must be positive")
	}
	return &OpenAIImagesDispatcher{
		gateway:            gateway,
		maxAccountSwitches: cfg.MaxAccountSwitches,
		maxBodyBytes:       cfg.MaxBodyBytes,
	}, nil
}

func (d *OpenAIImagesDispatcher) Forward(
	ctx context.Context,
	w http.ResponseWriter,
	req *http.Request,
	dispatch gatewaycore.DispatchRequest,
) (*gatewaycore.Measurement, error) {
	if d == nil || d.gateway == nil {
		return nil, errors.New("OpenAI Images dispatcher is not initialized")
	}
	if dispatch.Pool.Platform != service.PlatformOpenAI {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedPoolPlatform, dispatch.Pool.Platform)
	}
	if req == nil || req.URL == nil || req.Method != http.MethodPost ||
		(req.URL.Path != "/v1/images/generations" && req.URL.Path != "/v1/images/edits") {
		return nil, fmt.Errorf("invalid OpenAI Images endpoint: %s %s", requestMethod(req), requestPath(req))
	}
	body, err := pkghttputil.ReadRequestBodyWithPreallocLimit(req, d.maxBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("read OpenAI Images request body: %w", err)
	}
	parsed, err := service.ParseOpenAIImagesRuntimeRequest(req, body)
	if err != nil {
		return nil, err
	}
	if parsed.Model != dispatch.Invocation.Model {
		return nil, fmt.Errorf("%w: invocation=%q body=%q", ErrInvocationModelMismatch, dispatch.Invocation.Model, parsed.Model)
	}
	subscriptionValidationErr := service.ValidateOpenAIImagesSubscriptionRequest(req, body)

	req = req.Clone(ctx)
	exchange := gatewaytransport.NewHTTPExchange(w, req)
	poolID := dispatch.Pool.ID
	failedAccountIDs := make(map[int64]struct{})
	sameAccountRetries := make(map[int64]int)
	switchCount := 0
	var lastFailover *service.UpstreamFailoverError

	for {
		var selection *service.AccountSelectionResult
		var selectErr error
		if subscriptionValidationErr != nil {
			selection, selectErr = d.gateway.SelectTechnicalImagesDirectAccountWithLoadAwareness(
				ctx, &poolID, "", dispatch.Invocation.Model, failedAccountIDs,
			)
		} else {
			selection, selectErr = d.gateway.SelectTechnicalImagesAccountWithLoadAwareness(
				ctx, &poolID, "", dispatch.Invocation.Model, failedAccountIDs,
			)
		}
		if selectErr != nil {
			if subscriptionValidationErr != nil {
				selectErr = errors.Join(subscriptionValidationErr, selectErr)
			}
			if lastFailover != nil {
				return nil, errors.Join(lastFailover, selectErr)
			}
			return nil, selectErr
		}
		if selection == nil || selection.Account == nil {
			return nil, service.ErrInvalidAccountSelection
		}
		account := selection.Account
		release, acquireErr := d.gateway.AcquireSelection(ctx, selection)
		if acquireErr != nil {
			return nil, acquireErr
		}
		release = releaseOnContextDone(ctx, release)
		if account.Platform != service.PlatformOpenAI ||
			(account.Type != service.AccountTypeAPIKey && account.Type != service.AccountTypeOAuth) ||
			(subscriptionValidationErr != nil && account.Type != service.AccountTypeAPIKey) {
			release()
			return nil, fmt.Errorf("scheduler selected unsupported account %d for OpenAI Images", account.ID)
		}

		writtenBefore := exchange.Response().Written()
		sizeBefore := exchange.Response().Size()
		startedAt := time.Now()
		var result *service.OpenAIForwardResult
		var forwardErr error
		if account.Type == service.AccountTypeOAuth {
			result, forwardErr = d.gateway.ForwardImagesSubscriptionExchange(ctx, exchange, account, body)
		} else {
			result, forwardErr = d.gateway.ForwardImagesDirectExchange(ctx, exchange, account, body)
		}
		release()
		if result != nil {
			measurement, measureErr := openAIImagesMeasurement(exchange, account, result, startedAt)
			if measureErr != nil {
				return nil, errors.Join(forwardErr, measureErr)
			}
			return measurement, forwardErr
		}
		if forwardErr == nil {
			return nil, errors.New("OpenAI Images forward returned neither result nor error")
		}
		if exchange.Response().Size() != sizeBefore || (!writtenBefore && exchange.Response().Written()) {
			return nil, forwardErr
		}
		var failoverErr *service.UpstreamFailoverError
		if !errors.As(forwardErr, &failoverErr) || !failoverErr.ShouldRetryNextAccount() {
			return nil, forwardErr
		}
		lastFailover = failoverErr
		retryLimit := account.GetPoolModeRetryCount()
		if failoverErr.RetryableOnSameAccount && sameAccountRetries[account.ID] < retryLimit {
			sameAccountRetries[account.ID]++
			if err := waitForContext(ctx, sameAccountRetryDelay); err != nil {
				return nil, err
			}
			continue
		}
		failedAccountIDs[account.ID] = struct{}{}
		if switchCount >= d.maxAccountSwitches {
			return nil, lastFailover
		}
		switchCount++
	}
}

func (d *OpenAIImagesDispatcher) Close(context.Context) error { return nil }

func openAIImagesMeasurement(exchange gatewaytransport.Exchange, account *service.Account, result *service.OpenAIForwardResult, startedAt time.Time) (*gatewaycore.Measurement, error) {
	if result == nil {
		return nil, errors.New("OpenAI Images forward returned no result")
	}
	status := exchange.Response().Status()
	if status < 100 || status > 599 {
		return nil, fmt.Errorf("OpenAI Images forward returned invalid downstream status %d", status)
	}
	upstreamModel := strings.TrimSpace(result.UpstreamModel)
	if upstreamModel == "" {
		return nil, errors.New("OpenAI Images forward omitted upstream model")
	}
	firstResponse := time.Duration(0)
	if result.FirstTokenMs != nil {
		firstResponse = time.Duration(*result.FirstTokenMs) * time.Millisecond
	}
	return &gatewaycore.Measurement{
		AccountID:               account.ID,
		Endpoint:                exchange.Request().URL.Path,
		UpstreamModel:           upstreamModel,
		InputTokens:             int64(result.Usage.InputTokens),
		ImageInputTokens:        int64(result.Usage.ImageInputTokens),
		OutputTokens:            int64(result.Usage.OutputTokens),
		ImageOutputTokens:       int64(result.Usage.ImageOutputTokens),
		CacheReadInputTokens:    int64(result.Usage.CacheReadInputTokens),
		CacheWriteInputTokens:   int64(result.Usage.CacheCreationInputTokens),
		ImageCount:              result.ImageCount,
		ImageOutputSizes:        append([]string(nil), result.ImageOutputSizes...),
		UpstreamStatusCode:      status,
		StartedAt:               startedAt,
		Duration:                result.Duration,
		TimeToFirstResponseByte: firstResponse,
	}, nil
}

var _ gatewaycore.Dispatcher = (*OpenAIImagesDispatcher)(nil)
