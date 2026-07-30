package gatewayruntime

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/gatewaycore"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/gatewaytransport"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

var (
	ErrUnsupportedPoolPlatform = errors.New("unsupported gateway pool platform")
	ErrInvocationModelMismatch = errors.New("gateway invocation model does not match request body")
)

const sameAccountRetryDelay = 500 * time.Millisecond

type AnthropicDispatcherConfig struct {
	MaxAccountSwitches int
	MaxBodyBytes       int64
}

func AnthropicDispatcherConfigFromApplication(cfg *config.Config) (AnthropicDispatcherConfig, error) {
	if cfg == nil {
		return AnthropicDispatcherConfig{}, errors.New("gateway application config is required")
	}
	return AnthropicDispatcherConfig{
		MaxAccountSwitches: cfg.Gateway.MaxAccountSwitches,
		MaxBodyBytes:       cfg.Gateway.MaxBodySize,
	}, nil
}

type anthropicGateway interface {
	ValidateTechnicalRuntime() error
	SelectAccountWithLoadAwareness(context.Context, *int64, string, string, map[int64]struct{}) (*service.AccountSelectionResult, error)
	AcquireSelection(context.Context, *service.AccountSelectionResult) (func(), error)
	ApplyBedrockCCCompatExchange(gatewaytransport.Exchange, []byte, string, *service.Account, *int64) []byte
	ForwardExchange(context.Context, gatewaytransport.Exchange, *service.Account, *service.ParsedRequest) (*service.ForwardResult, error)
	TempUnscheduleRetryableError(context.Context, int64, *service.UpstreamFailoverError)
	IncrementAccountRPM(context.Context, int64) error
	BindStickySession(context.Context, *int64, string, int64) error
}

// AnthropicDispatcher owns only technical-pool scheduling and provider forwarding.
type AnthropicDispatcher struct {
	gateway            anthropicGateway
	maxAccountSwitches int
	maxBodyBytes       int64
}

func NewAnthropicDispatcher(gateway anthropicGateway, cfg AnthropicDispatcherConfig) (*AnthropicDispatcher, error) {
	if gateway == nil {
		return nil, errors.New("anthropic gateway service is required")
	}
	if err := gateway.ValidateTechnicalRuntime(); err != nil {
		return nil, fmt.Errorf("validate Anthropic gateway: %w", err)
	}
	if cfg.MaxAccountSwitches <= 0 {
		return nil, errors.New("anthropic max account switches must be positive")
	}
	if cfg.MaxBodyBytes <= 0 {
		return nil, errors.New("anthropic max body bytes must be positive")
	}
	return &AnthropicDispatcher{
		gateway:            gateway,
		maxAccountSwitches: cfg.MaxAccountSwitches,
		maxBodyBytes:       cfg.MaxBodyBytes,
	}, nil
}

func (d *AnthropicDispatcher) Forward(
	ctx context.Context,
	w http.ResponseWriter,
	req *http.Request,
	dispatch gatewaycore.DispatchRequest,
) (*gatewaycore.Measurement, error) {
	if d == nil || d.gateway == nil {
		return nil, errors.New("anthropic dispatcher is not initialized")
	}
	if dispatch.Pool.Platform != service.PlatformAnthropic {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedPoolPlatform, dispatch.Pool.Platform)
	}
	if req.Method != http.MethodPost || req.URL == nil || req.URL.Path != "/v1/messages" {
		return nil, fmt.Errorf("invalid Anthropic endpoint: %s %s", req.Method, requestPath(req))
	}

	body, err := pkghttputil.ReadLenientJSONRequestBodyWithPrealloc(req, d.maxBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("read Anthropic request body: %w", err)
	}
	if len(body) == 0 {
		return nil, errors.New("Anthropic request body is empty")
	}
	parsed, err := service.ParseGatewayRequest(service.NewRequestBodyRef(body), domain.PlatformAnthropic)
	if err != nil {
		return nil, fmt.Errorf("parse Anthropic request body: %w", err)
	}
	if parsed.Model != dispatch.Invocation.Model {
		return nil, fmt.Errorf("%w: invocation=%q body=%q", ErrInvocationModelMismatch, dispatch.Invocation.Model, parsed.Model)
	}

	ctx = prepareAnthropicClientContext(ctx, req, parsed)
	req = req.WithContext(ctx)
	exchange := gatewaytransport.NewHTTPExchange(w, req)
	poolID := dispatch.Pool.ID
	parsed.GroupID = &poolID

	failedAccountIDs := make(map[int64]struct{})
	sameAccountRetries := make(map[int64]int)
	switchCount := 0
	var lastFailover *service.UpstreamFailoverError

	for {
		selection, selectErr := d.gateway.SelectAccountWithLoadAwareness(
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

		attempt, cloneErr := parsed.CloneForBody(body)
		if cloneErr != nil {
			release()
			return nil, fmt.Errorf("clone Anthropic request: %w", cloneErr)
		}
		attempt.GroupID = &poolID
		if account.IsBedrock() {
			if err := attempt.ReplaceBody(d.gateway.ApplyBedrockCCCompatExchange(exchange, attempt.Body.Bytes(), attempt.Model, account, &poolID)); err != nil {
				release()
				return nil, fmt.Errorf("prepare Bedrock request: %w", err)
			}
		}

		attemptCtx := ctx
		if switchCount > 0 {
			attemptCtx = service.WithAccountSwitchCount(attemptCtx, switchCount, false)
		}
		writtenBefore := exchange.Response().Written()
		sizeBefore := exchange.Response().Size()
		forwardStarted := time.Now()
		result, forwardErr := d.gateway.ForwardExchange(attemptCtx, exchange, account, attempt)
		release()

		if forwardErr == nil {
			measurement, measureErr := anthropicMeasurement(exchange, dispatch, account, result, forwardStarted)
			if measureErr != nil {
				return nil, measureErr
			}
			var stateErr error
			if account.IsAnthropicOAuthOrSetupToken() && account.GetBaseRPM() > 0 {
				stateErr = errors.Join(stateErr, d.gateway.IncrementAccountRPM(ctx, account.ID))
			}
			stateErr = errors.Join(stateErr, d.gateway.BindStickySession(ctx, &poolID, dispatch.Invocation.SessionID, account.ID))
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
			d.gateway.TempUnscheduleRetryableError(ctx, account.ID, failoverErr)
		}
		failedAccountIDs[account.ID] = struct{}{}
		if switchCount >= d.maxAccountSwitches {
			return nil, lastFailover
		}
		switchCount++
	}
}

func (d *AnthropicDispatcher) Close(context.Context) error { return nil }

func prepareAnthropicClientContext(ctx context.Context, req *http.Request, parsed *service.ParsedRequest) context.Context {
	probe := parsed.MaxTokens == 1 && strings.Contains(strings.ToLower(parsed.Model), "haiku")
	ctx = service.WithIsMaxTokensOneHaikuRequest(ctx, probe, false)
	ctx = service.WithThinkingEnabled(ctx, parsed.ThinkingEnabled, false)

	bodyMap := map[string]any{"model": parsed.Model}
	if parsed.HasSystem {
		if system, ok := parsed.SystemValue(); ok {
			bodyMap["system"] = system
		} else {
			bodyMap["system"] = nil
		}
	}
	if parsed.MetadataUserID != "" {
		bodyMap["metadata"] = map[string]any{"user_id": parsed.MetadataUserID}
	}

	validator := service.NewClaudeCodeValidator()
	validationRequest := req.WithContext(ctx)
	isClaudeCode := validator.Validate(validationRequest, bodyMap)
	ctx = service.SetClaudeCodeClient(ctx, isClaudeCode)
	if isClaudeCode {
		if version := validator.ExtractVersion(req.Header.Get("User-Agent")); version != "" {
			ctx = service.SetClaudeCodeVersion(ctx, version)
		}
	}
	return ctx
}

func anthropicMeasurement(
	exchange gatewaytransport.Exchange,
	dispatch gatewaycore.DispatchRequest,
	account *service.Account,
	result *service.ForwardResult,
	startedAt time.Time,
) (*gatewaycore.Measurement, error) {
	if result == nil {
		return nil, errors.New("Anthropic forward returned no result")
	}
	status := exchange.Response().Status()
	if status < 100 || status > 599 {
		return nil, fmt.Errorf("Anthropic forward returned invalid downstream status %d", status)
	}
	upstreamModel := result.UpstreamModel
	if upstreamModel == "" {
		upstreamModel = result.Model
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

func releaseOnContextDone(ctx context.Context, release func()) func() {
	if release == nil {
		return func() {}
	}
	var once sync.Once
	releaseOnce := func() { once.Do(release) }
	stop := context.AfterFunc(ctx, releaseOnce)
	return func() {
		_ = stop()
		releaseOnce()
	}
}

func waitForContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func requestPath(req *http.Request) string {
	if req == nil || req.URL == nil {
		return ""
	}
	return req.URL.Path
}

var _ gatewaycore.Dispatcher = (*AnthropicDispatcher)(nil)
