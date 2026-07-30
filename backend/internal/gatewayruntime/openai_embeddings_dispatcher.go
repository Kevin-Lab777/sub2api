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

type OpenAIEmbeddingsDispatcherConfig struct {
	MaxAccountSwitches int
	MaxBodyBytes       int64
}

func OpenAIEmbeddingsDispatcherConfigFromApplication(cfg *config.Config) (OpenAIEmbeddingsDispatcherConfig, error) {
	if cfg == nil {
		return OpenAIEmbeddingsDispatcherConfig{}, errors.New("gateway application config is required")
	}
	return OpenAIEmbeddingsDispatcherConfig{
		MaxAccountSwitches: cfg.Gateway.MaxAccountSwitches,
		MaxBodyBytes:       cfg.Gateway.MaxBodySize,
	}, nil
}

type openAIEmbeddingsGateway interface {
	ValidateTechnicalRuntime() error
	SelectTechnicalEmbeddingsAccountWithLoadAwareness(context.Context, *int64, string, string, map[int64]struct{}) (*service.AccountSelectionResult, error)
	AcquireSelection(context.Context, *service.AccountSelectionResult) (func(), error)
	ForwardEmbeddingsExchange(context.Context, gatewaytransport.Exchange, *service.Account, []byte) (*service.OpenAIForwardResult, error)
}

// OpenAIEmbeddingsDispatcher owns the native Embeddings endpoint for exact
// technical pools. It does not perform customer routing, limits, or billing.
type OpenAIEmbeddingsDispatcher struct {
	gateway            openAIEmbeddingsGateway
	maxAccountSwitches int
	maxBodyBytes       int64
}

func NewOpenAIEmbeddingsDispatcher(gateway openAIEmbeddingsGateway, cfg OpenAIEmbeddingsDispatcherConfig) (*OpenAIEmbeddingsDispatcher, error) {
	if gateway == nil {
		return nil, errors.New("OpenAI Embeddings gateway is required")
	}
	if err := gateway.ValidateTechnicalRuntime(); err != nil {
		return nil, fmt.Errorf("validate OpenAI Embeddings gateway: %w", err)
	}
	if cfg.MaxAccountSwitches <= 0 {
		return nil, errors.New("OpenAI max account switches must be positive")
	}
	if cfg.MaxBodyBytes <= 0 {
		return nil, errors.New("OpenAI max body bytes must be positive")
	}
	return &OpenAIEmbeddingsDispatcher{
		gateway:            gateway,
		maxAccountSwitches: cfg.MaxAccountSwitches,
		maxBodyBytes:       cfg.MaxBodyBytes,
	}, nil
}

func (d *OpenAIEmbeddingsDispatcher) Forward(
	ctx context.Context,
	w http.ResponseWriter,
	req *http.Request,
	dispatch gatewaycore.DispatchRequest,
) (*gatewaycore.Measurement, error) {
	if d == nil || d.gateway == nil {
		return nil, errors.New("OpenAI Embeddings dispatcher is not initialized")
	}
	if dispatch.Pool.Platform != service.PlatformOpenAI {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedPoolPlatform, dispatch.Pool.Platform)
	}
	if req == nil || req.URL == nil || req.Method != http.MethodPost || req.URL.Path != "/v1/embeddings" {
		return nil, fmt.Errorf("invalid OpenAI Embeddings endpoint: %s %s", requestMethod(req), requestPath(req))
	}
	body, err := pkghttputil.ReadRequestBodyWithPreallocLimit(req, d.maxBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("read OpenAI Embeddings request body: %w", err)
	}
	model, err := service.ParseOpenAIEmbeddingsRequest(body)
	if err != nil {
		return nil, err
	}
	if model != dispatch.Invocation.Model {
		return nil, fmt.Errorf("%w: invocation=%q body=%q", ErrInvocationModelMismatch, dispatch.Invocation.Model, model)
	}

	req = req.Clone(ctx)
	exchange := gatewaytransport.NewHTTPExchange(w, req)
	poolID := dispatch.Pool.ID
	failedAccountIDs := make(map[int64]struct{})
	sameAccountRetries := make(map[int64]int)
	switchCount := 0
	var lastFailover *service.UpstreamFailoverError

	for {
		selection, selectErr := d.gateway.SelectTechnicalEmbeddingsAccountWithLoadAwareness(
			ctx,
			&poolID,
			"",
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
		if account.Platform != service.PlatformOpenAI || account.Type != service.AccountTypeAPIKey {
			release()
			return nil, fmt.Errorf("scheduler selected unsupported account %d for OpenAI Embeddings", account.ID)
		}

		writtenBefore := exchange.Response().Written()
		sizeBefore := exchange.Response().Size()
		startedAt := time.Now()
		result, forwardErr := d.gateway.ForwardEmbeddingsExchange(ctx, exchange, account, body)
		release()
		if result != nil {
			measurement, measureErr := openAIEmbeddingsMeasurement(exchange, account, result, startedAt)
			if measureErr != nil {
				return nil, errors.Join(forwardErr, measureErr)
			}
			return measurement, forwardErr
		}
		if forwardErr == nil {
			return nil, errors.New("OpenAI Embeddings forward returned neither result nor error")
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

func (d *OpenAIEmbeddingsDispatcher) Close(context.Context) error { return nil }

func openAIEmbeddingsMeasurement(exchange gatewaytransport.Exchange, account *service.Account, result *service.OpenAIForwardResult, startedAt time.Time) (*gatewaycore.Measurement, error) {
	if result == nil {
		return nil, errors.New("OpenAI Embeddings forward returned no result")
	}
	status := exchange.Response().Status()
	if status < 100 || status > 599 {
		return nil, fmt.Errorf("OpenAI Embeddings forward returned invalid downstream status %d", status)
	}
	upstreamModel := strings.TrimSpace(result.UpstreamModel)
	if upstreamModel == "" {
		return nil, errors.New("OpenAI Embeddings forward omitted upstream model")
	}
	return &gatewaycore.Measurement{
		AccountID:             account.ID,
		Endpoint:              "/v1/embeddings",
		UpstreamModel:         upstreamModel,
		InputTokens:           int64(result.Usage.InputTokens),
		ImageInputTokens:      int64(result.Usage.ImageInputTokens),
		OutputTokens:          int64(result.Usage.OutputTokens),
		ImageOutputTokens:     int64(result.Usage.ImageOutputTokens),
		CacheReadInputTokens:  int64(result.Usage.CacheReadInputTokens),
		CacheWriteInputTokens: int64(result.Usage.CacheCreationInputTokens),
		UpstreamStatusCode:    status,
		StartedAt:             startedAt,
		Duration:              result.Duration,
	}, nil
}

var _ gatewaycore.Dispatcher = (*OpenAIEmbeddingsDispatcher)(nil)
