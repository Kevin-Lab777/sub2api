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
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type GeminiDispatcherConfig struct {
	MaxAccountSwitches int
	MaxBodyBytes       int64
}

func GeminiDispatcherConfigFromApplication(cfg *config.Config) (GeminiDispatcherConfig, error) {
	if cfg == nil {
		return GeminiDispatcherConfig{}, errors.New("gateway application config is required")
	}
	return GeminiDispatcherConfig{
		MaxAccountSwitches: cfg.Gateway.MaxAccountSwitchesGemini,
		MaxBodyBytes:       cfg.Gateway.MaxBodySize,
	}, nil
}

type geminiScheduler interface {
	ValidateTechnicalSchedulerRuntime() error
	SelectAccountWithLoadAwareness(context.Context, *int64, string, string, map[int64]struct{}) (*service.AccountSelectionResult, error)
	AcquireSelection(context.Context, *service.AccountSelectionResult) (func(), error)
	TempUnscheduleRetryableError(context.Context, int64, *service.UpstreamFailoverError)
	IncrementAccountRPM(context.Context, int64) error
	BindStickySession(context.Context, *int64, string, int64) error
}

type geminiForwarder interface {
	ValidateTechnicalRuntime() error
	ForwardNativeExchange(context.Context, gatewaytransport.Exchange, *service.Account, string, string, bool, []byte) (*service.ForwardResult, error)
}

// GeminiDispatcher owns native Gemini endpoint parsing, technical-pool
// scheduling, account concurrency, and provider forwarding.
type GeminiDispatcher struct {
	scheduler          geminiScheduler
	forwarder          geminiForwarder
	maxAccountSwitches int
	maxBodyBytes       int64
}

func NewGeminiDispatcher(scheduler geminiScheduler, forwarder geminiForwarder, cfg GeminiDispatcherConfig) (*GeminiDispatcher, error) {
	if scheduler == nil {
		return nil, errors.New("gemini account scheduler is required")
	}
	if err := scheduler.ValidateTechnicalSchedulerRuntime(); err != nil {
		return nil, fmt.Errorf("validate Gemini scheduler: %w", err)
	}
	if forwarder == nil {
		return nil, errors.New("gemini forwarder is required")
	}
	if err := forwarder.ValidateTechnicalRuntime(); err != nil {
		return nil, fmt.Errorf("validate Gemini forwarder: %w", err)
	}
	if cfg.MaxAccountSwitches <= 0 {
		return nil, errors.New("gemini max account switches must be positive")
	}
	if cfg.MaxBodyBytes <= 0 {
		return nil, errors.New("gemini max body bytes must be positive")
	}
	return &GeminiDispatcher{
		scheduler:          scheduler,
		forwarder:          forwarder,
		maxAccountSwitches: cfg.MaxAccountSwitches,
		maxBodyBytes:       cfg.MaxBodyBytes,
	}, nil
}

func (d *GeminiDispatcher) Forward(
	ctx context.Context,
	w http.ResponseWriter,
	req *http.Request,
	dispatch gatewaycore.DispatchRequest,
) (*gatewaycore.Measurement, error) {
	if d == nil || d.scheduler == nil || d.forwarder == nil {
		return nil, errors.New("gemini dispatcher is not initialized")
	}
	if dispatch.Pool.Platform != service.PlatformGemini {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedPoolPlatform, dispatch.Pool.Platform)
	}
	model, action, stream, err := parseNativeGeminiEndpoint(req)
	if err != nil {
		return nil, err
	}
	if model != dispatch.Invocation.Model {
		return nil, fmt.Errorf("%w: invocation=%q path=%q", ErrInvocationModelMismatch, dispatch.Invocation.Model, model)
	}

	body, err := pkghttputil.ReadLenientJSONRequestBodyWithPrealloc(req, d.maxBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("read Gemini request body: %w", err)
	}
	if len(body) == 0 {
		return nil, errors.New("Gemini request body is empty")
	}

	// Native Gemini dispatch deliberately selects native Gemini accounts. The
	// exact pool ID remains mandatory; this platform constraint never broadens
	// account lookup beyond the resolved pool.
	ctx = context.WithValue(ctx, ctxkey.ForcePlatform, service.PlatformGemini)
	req = req.WithContext(ctx)
	exchange := gatewaytransport.NewHTTPExchange(w, req)
	poolID := dispatch.Pool.ID
	failedAccountIDs := make(map[int64]struct{})
	sameAccountRetries := make(map[int64]int)
	switchCount := 0
	var lastFailover *service.UpstreamFailoverError

	for {
		selection, selectErr := d.scheduler.SelectAccountWithLoadAwareness(
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
		release, acquireErr := d.scheduler.AcquireSelection(ctx, selection)
		if acquireErr != nil {
			return nil, acquireErr
		}
		release = releaseOnContextDone(ctx, release)
		if account.Platform != service.PlatformGemini {
			release()
			return nil, fmt.Errorf("%w: scheduler selected %s account %d for native Gemini", ErrUnsupportedPoolPlatform, account.Platform, account.ID)
		}

		attemptCtx := ctx
		if switchCount > 0 {
			attemptCtx = service.WithAccountSwitchCount(attemptCtx, switchCount, false)
		}
		writtenBefore := exchange.Response().Written()
		sizeBefore := exchange.Response().Size()
		forwardStarted := time.Now()
		result, forwardErr := d.forwarder.ForwardNativeExchange(
			attemptCtx,
			exchange,
			account,
			model,
			action,
			stream,
			body,
		)
		release()

		if forwardErr == nil {
			measurement, measureErr := geminiMeasurement(exchange, dispatch, account, result, forwardStarted)
			if measureErr != nil {
				return nil, measureErr
			}
			var stateErr error
			if account.GetBaseRPM() > 0 {
				stateErr = errors.Join(stateErr, d.scheduler.IncrementAccountRPM(ctx, account.ID))
			}
			stateErr = errors.Join(stateErr, d.scheduler.BindStickySession(ctx, &poolID, dispatch.Invocation.SessionID, account.ID))
			return measurement, stateErr
		}

		if exchange.Response().Size() != sizeBefore || (!writtenBefore && exchange.Response().Written()) {
			return nil, forwardErr
		}
		var failoverErr *service.UpstreamFailoverError
		if !errors.As(forwardErr, &failoverErr) {
			return nil, forwardErr
		}
		lastFailover = failoverErr
		if !failoverErr.ShouldRetryNextAccount() {
			return nil, forwardErr
		}

		retryLimit := account.GetPoolModeRetryCount()
		if failoverErr.RetryableOnSameAccount && sameAccountRetries[account.ID] < retryLimit {
			sameAccountRetries[account.ID]++
			if err := waitForContext(ctx, sameAccountRetryDelay); err != nil {
				return nil, err
			}
			continue
		}
		if failoverErr.RetryableOnSameAccount {
			d.scheduler.TempUnscheduleRetryableError(ctx, account.ID, failoverErr)
		}
		failedAccountIDs[account.ID] = struct{}{}
		if switchCount >= d.maxAccountSwitches {
			return nil, lastFailover
		}
		switchCount++
	}
}

func (d *GeminiDispatcher) Close(context.Context) error { return nil }

func parseNativeGeminiEndpoint(req *http.Request) (model, action string, stream bool, err error) {
	if req == nil || req.Method != http.MethodPost || req.URL == nil {
		return "", "", false, fmt.Errorf("invalid Gemini endpoint: %s %s", requestMethod(req), requestPath(req))
	}
	const prefix = "/v1beta/models/"
	if !strings.HasPrefix(req.URL.Path, prefix) {
		return "", "", false, fmt.Errorf("invalid Gemini endpoint: %s %s", req.Method, req.URL.Path)
	}
	modelAction := strings.TrimPrefix(req.URL.Path, prefix)
	separator := strings.LastIndexByte(modelAction, ':')
	if separator <= 0 || separator == len(modelAction)-1 {
		return "", "", false, fmt.Errorf("invalid Gemini model action path: %s", req.URL.Path)
	}
	model = strings.TrimSpace(modelAction[:separator])
	action = strings.TrimSpace(modelAction[separator+1:])
	if model == "" || !validGeminiModelPath(model) {
		return "", "", false, fmt.Errorf("invalid Gemini model path: %s", req.URL.Path)
	}
	switch action {
	case "generateContent", "countTokens":
		return model, action, false, nil
	case "streamGenerateContent":
		return model, action, true, nil
	default:
		return "", "", false, fmt.Errorf("unsupported Gemini action %q", action)
	}
}

func validGeminiModelPath(model string) bool {
	for _, segment := range strings.Split(model, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

func geminiMeasurement(
	exchange gatewaytransport.Exchange,
	dispatch gatewaycore.DispatchRequest,
	account *service.Account,
	result *service.ForwardResult,
	startedAt time.Time,
) (*gatewaycore.Measurement, error) {
	if result == nil {
		return nil, errors.New("Gemini forward returned no result")
	}
	status := exchange.Response().Status()
	if status < 100 || status > 599 {
		return nil, fmt.Errorf("Gemini forward returned invalid downstream status %d", status)
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
		ImageOutputTokens:       int64(result.Usage.ImageOutputTokens),
		CacheReadInputTokens:    int64(result.Usage.CacheReadInputTokens),
		CacheWriteInputTokens:   int64(result.Usage.CacheCreationInputTokens),
		ImageCount:              result.ImageCount,
		ImageOutputSizes:        append([]string(nil), result.ImageOutputSizes...),
		WebSearchCalls:          result.WebSearchCalls,
		UpstreamStatusCode:      status,
		StartedAt:               startedAt,
		Duration:                result.Duration,
		TimeToFirstResponseByte: firstResponse,
	}, nil
}

func requestMethod(req *http.Request) string {
	if req == nil {
		return ""
	}
	return req.Method
}

var _ gatewaycore.Dispatcher = (*GeminiDispatcher)(nil)
