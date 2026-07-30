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

type OpenAIChatCompletionsDispatcherConfig struct {
	MaxAccountSwitches int
	MaxBodyBytes       int64
}

func OpenAIChatCompletionsDispatcherConfigFromApplication(cfg *config.Config) (OpenAIChatCompletionsDispatcherConfig, error) {
	if cfg == nil {
		return OpenAIChatCompletionsDispatcherConfig{}, errors.New("gateway application config is required")
	}
	return OpenAIChatCompletionsDispatcherConfig{
		MaxAccountSwitches: cfg.Gateway.MaxAccountSwitches,
		MaxBodyBytes:       cfg.Gateway.MaxBodySize,
	}, nil
}

type openAIChatCompletionsGateway interface {
	ValidateTechnicalRuntime() error
	SelectTechnicalChatCompletionsAccountWithLoadAwareness(context.Context, *int64, string, string, map[int64]struct{}) (*service.AccountSelectionResult, error)
	SelectTechnicalChatCompletionsDirectAccountWithLoadAwareness(context.Context, *int64, string, string, map[int64]struct{}) (*service.AccountSelectionResult, error)
	SelectTechnicalCompletionsDirectAccountWithLoadAwareness(context.Context, *int64, string, string, map[int64]struct{}) (*service.AccountSelectionResult, error)
	AcquireSelection(context.Context, *service.AccountSelectionResult) (func(), error)
	ForwardChatCompletionsExchange(context.Context, gatewaytransport.Exchange, *service.Account, []byte) (*service.OpenAIForwardResult, error)
	ForwardCompletionsExchange(context.Context, gatewaytransport.Exchange, *service.Account, []byte) (*service.OpenAIForwardResult, error)
	BindStickySession(context.Context, *int64, string, int64) error
}

// OpenAIChatCompletionsDispatcher is the native Chat Completions component for
// direct API-key and subscription accounts. Legacy Completions remains a
// direct API-key protocol.
type OpenAIChatCompletionsDispatcher struct {
	gateway            openAIChatCompletionsGateway
	maxAccountSwitches int
	maxBodyBytes       int64
}

func NewOpenAIChatCompletionsDispatcher(gateway openAIChatCompletionsGateway, cfg OpenAIChatCompletionsDispatcherConfig) (*OpenAIChatCompletionsDispatcher, error) {
	if gateway == nil {
		return nil, errors.New("OpenAI Chat Completions gateway is required")
	}
	if err := gateway.ValidateTechnicalRuntime(); err != nil {
		return nil, fmt.Errorf("validate OpenAI Chat Completions gateway: %w", err)
	}
	if cfg.MaxAccountSwitches <= 0 {
		return nil, errors.New("OpenAI max account switches must be positive")
	}
	if cfg.MaxBodyBytes <= 0 {
		return nil, errors.New("OpenAI max body bytes must be positive")
	}
	return &OpenAIChatCompletionsDispatcher{
		gateway:            gateway,
		maxAccountSwitches: cfg.MaxAccountSwitches,
		maxBodyBytes:       cfg.MaxBodyBytes,
	}, nil
}

func (d *OpenAIChatCompletionsDispatcher) Forward(
	ctx context.Context,
	w http.ResponseWriter,
	req *http.Request,
	dispatch gatewaycore.DispatchRequest,
) (*gatewaycore.Measurement, error) {
	if d == nil || d.gateway == nil {
		return nil, errors.New("OpenAI Chat Completions dispatcher is not initialized")
	}
	if dispatch.Pool.Platform != service.PlatformOpenAI {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedPoolPlatform, dispatch.Pool.Platform)
	}
	if req == nil || req.URL == nil || req.Method != http.MethodPost || (req.URL.Path != "/v1/chat/completions" && req.URL.Path != "/v1/completions") {
		return nil, fmt.Errorf("invalid OpenAI Chat Completions endpoint: %s %s", requestMethod(req), requestPath(req))
	}
	body, err := pkghttputil.ReadRequestBodyWithPreallocLimit(req, d.maxBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("read OpenAI Chat Completions request body: %w", err)
	}
	model, _, err := service.ParseOpenAIChatCompletionsRequest(body)
	legacyCompletions := req.URL.Path == "/v1/completions"
	if legacyCompletions {
		model, _, err = service.ParseOpenAICompletionsRequest(body)
	}
	if err != nil {
		return nil, err
	}
	if model != dispatch.Invocation.Model {
		return nil, fmt.Errorf("%w: invocation=%q body=%q", ErrInvocationModelMismatch, dispatch.Invocation.Model, model)
	}
	var subscriptionValidationErr error
	if !legacyCompletions {
		subscriptionValidationErr = service.ValidateOpenAIChatCompletionsSubscriptionRequest(body)
	}

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
		if legacyCompletions {
			selection, selectErr = d.gateway.SelectTechnicalCompletionsDirectAccountWithLoadAwareness(ctx, &poolID, dispatch.Invocation.SessionID, dispatch.Invocation.Model, failedAccountIDs)
		} else if subscriptionValidationErr != nil {
			selection, selectErr = d.gateway.SelectTechnicalChatCompletionsDirectAccountWithLoadAwareness(ctx, &poolID, dispatch.Invocation.SessionID, dispatch.Invocation.Model, failedAccountIDs)
		} else {
			selection, selectErr = d.gateway.SelectTechnicalChatCompletionsAccountWithLoadAwareness(ctx, &poolID, dispatch.Invocation.SessionID, dispatch.Invocation.Model, failedAccountIDs)
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
			(legacyCompletions && account.Type != service.AccountTypeAPIKey) ||
			(!legacyCompletions && account.Type != service.AccountTypeAPIKey && account.Type != service.AccountTypeOAuth) {
			release()
			return nil, fmt.Errorf("scheduler selected unsupported account %d for OpenAI Chat Completions", account.ID)
		}

		writtenBefore := exchange.Response().Written()
		sizeBefore := exchange.Response().Size()
		startedAt := time.Now()
		var result *service.OpenAIForwardResult
		var forwardErr error
		if legacyCompletions {
			result, forwardErr = d.gateway.ForwardCompletionsExchange(ctx, exchange, account, body)
		} else {
			result, forwardErr = d.gateway.ForwardChatCompletionsExchange(ctx, exchange, account, body)
		}
		release()
		if result != nil {
			measurement, measureErr := openAIChatCompletionsMeasurement(exchange, account, result, req.URL.Path, startedAt)
			if measureErr != nil {
				return nil, errors.Join(forwardErr, measureErr)
			}
			if forwardErr == nil {
				forwardErr = d.gateway.BindStickySession(ctx, &poolID, dispatch.Invocation.SessionID, account.ID)
			}
			return measurement, forwardErr
		}
		if forwardErr == nil {
			return nil, errors.New("OpenAI Chat Completions forward returned neither result nor error")
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

func (d *OpenAIChatCompletionsDispatcher) Close(context.Context) error { return nil }

func openAIChatCompletionsMeasurement(exchange gatewaytransport.Exchange, account *service.Account, result *service.OpenAIForwardResult, endpoint string, startedAt time.Time) (*gatewaycore.Measurement, error) {
	if result == nil {
		return nil, errors.New("OpenAI Chat Completions forward returned no result")
	}
	status := exchange.Response().Status()
	if status < 100 || status > 599 {
		return nil, fmt.Errorf("OpenAI Chat Completions forward returned invalid downstream status %d", status)
	}
	upstreamModel := strings.TrimSpace(result.UpstreamModel)
	if upstreamModel == "" {
		return nil, errors.New("OpenAI Chat Completions forward omitted upstream model")
	}
	firstResponse := time.Duration(0)
	if result.FirstTokenMs != nil {
		firstResponse = time.Duration(*result.FirstTokenMs) * time.Millisecond
	}
	return &gatewaycore.Measurement{
		AccountID:               account.ID,
		Endpoint:                endpoint,
		UpstreamModel:           upstreamModel,
		InputTokens:             int64(result.Usage.InputTokens),
		ImageInputTokens:        int64(result.Usage.ImageInputTokens),
		OutputTokens:            int64(result.Usage.OutputTokens),
		ImageOutputTokens:       int64(result.Usage.ImageOutputTokens),
		CacheReadInputTokens:    int64(result.Usage.CacheReadInputTokens),
		CacheWriteInputTokens:   int64(result.Usage.CacheCreationInputTokens),
		UpstreamStatusCode:      status,
		StartedAt:               startedAt,
		Duration:                result.Duration,
		TimeToFirstResponseByte: firstResponse,
	}, nil
}

var _ gatewaycore.Dispatcher = (*OpenAIChatCompletionsDispatcher)(nil)
