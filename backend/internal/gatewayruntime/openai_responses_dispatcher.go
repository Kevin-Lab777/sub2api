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

type OpenAIResponsesDispatcherConfig struct {
	MaxAccountSwitches int
	MaxBodyBytes       int64
}

func OpenAIResponsesDispatcherConfigFromApplication(cfg *config.Config) (OpenAIResponsesDispatcherConfig, error) {
	if cfg == nil {
		return OpenAIResponsesDispatcherConfig{}, errors.New("gateway application config is required")
	}
	return OpenAIResponsesDispatcherConfig{
		MaxAccountSwitches: cfg.Gateway.MaxAccountSwitches,
		MaxBodyBytes:       cfg.Gateway.MaxBodySize,
	}, nil
}

type openAIResponsesGateway interface {
	ValidateTechnicalRuntime() error
	SelectTechnicalResponsesAccountWithLoadAwareness(context.Context, *int64, string, string, map[int64]struct{}) (*service.AccountSelectionResult, error)
	SelectTechnicalResponsesCompactAccountWithLoadAwareness(context.Context, *int64, string, string, map[int64]struct{}) (*service.AccountSelectionResult, error)
	SelectTechnicalResponsesWebSocketAccountWithLoadAwareness(context.Context, *int64, string, string, map[int64]struct{}) (*service.AccountSelectionResult, error)
	AcquireSelection(context.Context, *service.AccountSelectionResult) (func(), error)
	ForwardResponsesExchange(context.Context, gatewaytransport.Exchange, *service.Account, []byte) (*service.OpenAIForwardResult, error)
	ForwardResponsesCompactExchange(context.Context, gatewaytransport.Exchange, *service.Account, []byte) (*service.OpenAIForwardResult, error)
	ForwardResponsesWebSocket(context.Context, http.ResponseWriter, *http.Request, *service.Account, string, string) (*service.OpenAIResponsesWebSocketResult, error)
	BindStickySession(context.Context, *int64, string, int64) error
}

// OpenAIResponsesDispatcher is deliberately limited to the Responses HTTP
// endpoints. It is not a complete OpenAI protocol dispatcher and must not be
// registered as one until the remaining endpoint families are native.
type OpenAIResponsesDispatcher struct {
	gateway            openAIResponsesGateway
	maxAccountSwitches int
	maxBodyBytes       int64
}

func NewOpenAIResponsesDispatcher(gateway openAIResponsesGateway, cfg OpenAIResponsesDispatcherConfig) (*OpenAIResponsesDispatcher, error) {
	if gateway == nil {
		return nil, errors.New("openai Responses gateway is required")
	}
	if err := gateway.ValidateTechnicalRuntime(); err != nil {
		return nil, fmt.Errorf("validate OpenAI Responses gateway: %w", err)
	}
	if cfg.MaxAccountSwitches <= 0 {
		return nil, errors.New("openai max account switches must be positive")
	}
	if cfg.MaxBodyBytes <= 0 {
		return nil, errors.New("openai max body bytes must be positive")
	}
	return &OpenAIResponsesDispatcher{
		gateway:            gateway,
		maxAccountSwitches: cfg.MaxAccountSwitches,
		maxBodyBytes:       cfg.MaxBodyBytes,
	}, nil
}

func (d *OpenAIResponsesDispatcher) Forward(
	ctx context.Context,
	w http.ResponseWriter,
	req *http.Request,
	dispatch gatewaycore.DispatchRequest,
) (*gatewaycore.Measurement, error) {
	if d == nil || d.gateway == nil {
		return nil, errors.New("openai Responses dispatcher is not initialized")
	}
	if dispatch.Pool.Platform != service.PlatformOpenAI {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedPoolPlatform, dispatch.Pool.Platform)
	}
	if req == nil || req.URL == nil {
		return nil, fmt.Errorf("invalid OpenAI Responses endpoint: %s %s", requestMethod(req), requestPath(req))
	}
	if isOpenAIResponsesWebSocketUpgrade(req) {
		if req.Method != http.MethodGet || req.URL.Path != "/v1/responses" {
			return nil, fmt.Errorf("invalid OpenAI Responses WebSocket endpoint: %s %s", req.Method, req.URL.Path)
		}
		return d.forwardWebSocket(ctx, w, req, dispatch)
	}
	if req.Method != http.MethodPost {
		return nil, fmt.Errorf("invalid OpenAI Responses endpoint: %s %s", req.Method, req.URL.Path)
	}
	compact := false
	switch req.URL.Path {
	case "/v1/responses":
	case "/v1/responses/compact":
		compact = true
	default:
		return nil, fmt.Errorf("invalid OpenAI Responses endpoint: %s %s", requestMethod(req), requestPath(req))
	}
	body, err := pkghttputil.ReadRequestBodyWithPreallocLimit(req, d.maxBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("read OpenAI Responses request body: %w", err)
	}
	model, _, err := service.ParseOpenAIResponsesRequest(body)
	if err != nil {
		return nil, err
	}
	if model != dispatch.Invocation.Model {
		return nil, fmt.Errorf("%w: invocation=%q body=%q", ErrInvocationModelMismatch, dispatch.Invocation.Model, model)
	}

	req = req.Clone(ctx)
	req.Header = req.Header.Clone()
	req.Header.Set("session_id", dispatch.Invocation.SessionID)
	exchange := gatewaytransport.NewHTTPExchange(w, req)
	poolID := dispatch.Pool.ID
	failedAccountIDs := make(map[int64]struct{})
	sameAccountRetries := make(map[int64]int)
	switchCount := 0
	var lastFailover *service.UpstreamFailoverError

	for {
		var selection *service.AccountSelectionResult
		var selectErr error
		if compact {
			selection, selectErr = d.gateway.SelectTechnicalResponsesCompactAccountWithLoadAwareness(
				ctx,
				&poolID,
				dispatch.Invocation.SessionID,
				dispatch.Invocation.Model,
				failedAccountIDs,
			)
		} else {
			selection, selectErr = d.gateway.SelectTechnicalResponsesAccountWithLoadAwareness(
				ctx,
				&poolID,
				dispatch.Invocation.SessionID,
				dispatch.Invocation.Model,
				failedAccountIDs,
			)
		}
		if selectErr != nil {
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
		if account.Platform != service.PlatformOpenAI {
			release()
			return nil, fmt.Errorf("%w: scheduler selected %s account %d for OpenAI Responses", ErrUnsupportedPoolPlatform, account.Platform, account.ID)
		}

		attemptCtx := ctx
		if switchCount > 0 {
			attemptCtx = service.WithAccountSwitchCount(attemptCtx, switchCount, false)
		}
		writtenBefore := exchange.Response().Written()
		sizeBefore := exchange.Response().Size()
		startedAt := time.Now()
		var result *service.OpenAIForwardResult
		var forwardErr error
		if compact {
			result, forwardErr = d.gateway.ForwardResponsesCompactExchange(attemptCtx, exchange, account, body)
		} else {
			result, forwardErr = d.gateway.ForwardResponsesExchange(attemptCtx, exchange, account, body)
		}
		release()
		if forwardErr == nil {
			measurement, measureErr := openAIResponsesMeasurement(exchange, dispatch, account, result, startedAt)
			if measureErr != nil {
				return nil, measureErr
			}
			return measurement, d.gateway.BindStickySession(ctx, &poolID, dispatch.Invocation.SessionID, account.ID)
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

func (d *OpenAIResponsesDispatcher) Close(context.Context) error { return nil }

func (d *OpenAIResponsesDispatcher) forwardWebSocket(
	ctx context.Context,
	w http.ResponseWriter,
	req *http.Request,
	dispatch gatewaycore.DispatchRequest,
) (*gatewaycore.Measurement, error) {
	poolID := dispatch.Pool.ID
	failedAccountIDs := make(map[int64]struct{})
	switchCount := 0
	var lastFailover *service.UpstreamFailoverError

	for {
		selection, selectErr := d.gateway.SelectTechnicalResponsesWebSocketAccountWithLoadAwareness(
			ctx,
			&poolID,
			dispatch.Invocation.SessionID,
			dispatch.Invocation.Model,
			failedAccountIDs,
		)
		if selectErr != nil {
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
		if account.Platform != service.PlatformOpenAI {
			release()
			return nil, fmt.Errorf("%w: scheduler selected %s account %d for OpenAI Responses WebSocket", ErrUnsupportedPoolPlatform, account.Platform, account.ID)
		}

		result, forwardErr := d.gateway.ForwardResponsesWebSocket(
			ctx,
			w,
			req.Clone(ctx),
			account,
			dispatch.Invocation.Model,
			dispatch.Invocation.SessionID,
		)
		release()
		if result != nil {
			measurement, measureErr := openAIResponsesWebSocketMeasurement(account, result)
			if measureErr != nil {
				return nil, errors.Join(forwardErr, measureErr)
			}
			if forwardErr == nil {
				forwardErr = d.gateway.BindStickySession(ctx, &poolID, dispatch.Invocation.SessionID, account.ID)
			}
			return measurement, forwardErr
		}
		var failoverErr *service.UpstreamFailoverError
		if !errors.As(forwardErr, &failoverErr) || !failoverErr.ShouldRetryNextAccount() {
			return nil, forwardErr
		}
		lastFailover = failoverErr
		failedAccountIDs[account.ID] = struct{}{}
		if switchCount >= d.maxAccountSwitches {
			return nil, lastFailover
		}
		switchCount++
	}
}

func isOpenAIResponsesWebSocketUpgrade(req *http.Request) bool {
	if req == nil || !strings.EqualFold(strings.TrimSpace(req.Header.Get("Upgrade")), "websocket") {
		return false
	}
	for _, token := range strings.Split(req.Header.Get("Connection"), ",") {
		if strings.EqualFold(strings.TrimSpace(token), "upgrade") {
			return true
		}
	}
	return false
}

func openAIResponsesWebSocketMeasurement(
	account *service.Account,
	result *service.OpenAIResponsesWebSocketResult,
) (*gatewaycore.Measurement, error) {
	if result == nil {
		return nil, errors.New("OpenAI Responses WebSocket forward returned no result")
	}
	upstreamModel := strings.TrimSpace(result.UpstreamModel)
	if upstreamModel == "" {
		return nil, errors.New("OpenAI Responses WebSocket forward omitted upstream model")
	}
	return &gatewaycore.Measurement{
		AccountID:               account.ID,
		Endpoint:                "/v1/responses",
		UpstreamModel:           upstreamModel,
		InputTokens:             int64(result.Usage.InputTokens),
		OutputTokens:            int64(result.Usage.OutputTokens),
		CacheReadInputTokens:    int64(result.Usage.CacheReadInputTokens),
		CacheWriteInputTokens:   int64(result.Usage.CacheCreationInputTokens),
		ImageCount:              result.ImageCount,
		WebSearchCalls:          result.WebSearchCalls,
		UpstreamStatusCode:      result.UpstreamStatusCode,
		StartedAt:               result.StartedAt,
		Duration:                result.Duration,
		TimeToFirstResponseByte: result.TimeToFirstResponse,
	}, nil
}

func openAIResponsesMeasurement(
	exchange gatewaytransport.Exchange,
	dispatch gatewaycore.DispatchRequest,
	account *service.Account,
	result *service.OpenAIForwardResult,
	startedAt time.Time,
) (*gatewaycore.Measurement, error) {
	if result == nil {
		return nil, errors.New("OpenAI Responses forward returned no result")
	}
	status := exchange.Response().Status()
	if status < 100 || status > 599 {
		return nil, fmt.Errorf("OpenAI Responses forward returned invalid downstream status %d", status)
	}
	upstreamModel := strings.TrimSpace(result.UpstreamModel)
	if upstreamModel == "" {
		upstreamModel = strings.TrimSpace(result.Model)
	}
	if upstreamModel == "" {
		upstreamModel = dispatch.Invocation.Model
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
		OutputTokens:            int64(result.Usage.OutputTokens),
		CacheReadInputTokens:    int64(result.Usage.CacheReadInputTokens),
		CacheWriteInputTokens:   int64(result.Usage.CacheCreationInputTokens),
		ImageCount:              result.ImageCount,
		VideoDurationSeconds:    result.VideoDurationSeconds,
		WebSearchCalls:          result.WebSearchCalls,
		UpstreamStatusCode:      status,
		StartedAt:               startedAt,
		Duration:                result.Duration,
		TimeToFirstResponseByte: firstResponse,
	}, nil
}

var _ gatewaycore.Dispatcher = (*OpenAIResponsesDispatcher)(nil)
