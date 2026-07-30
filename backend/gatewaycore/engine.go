package gatewaycore

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

var (
	ErrInvalidPool        = errors.New("invalid gateway pool")
	ErrPoolUnavailable    = errors.New("gateway pool unavailable")
	ErrInvalidMeasurement = errors.New("invalid gateway measurement")
)

// Pool is the technical account-pool identity resolved by Next API.
type Pool struct {
	ID       int64
	Name     string
	Platform string
	Active   bool
}

// PoolResolver loads the technical pool selected by New API.
type PoolResolver interface {
	ResolvePool(ctx context.Context, poolID int64) (context.Context, Pool, error)
}

// DispatchRequest is the provider-facing request after pool resolution.
type DispatchRequest struct {
	Pool       Pool
	Invocation Invocation
}

// Measurement contains raw upstream facts. Engine attaches invocation identity
// before reporting Usage to New API.
type Measurement struct {
	AccountID               int64
	Endpoint                string
	UpstreamModel           string
	InputTokens             int64
	ImageInputTokens        int64
	OutputTokens            int64
	ImageOutputTokens       int64
	CacheReadInputTokens    int64
	CacheWriteInputTokens   int64
	ImageCount              int
	ImageOutputSizes        []string
	VideoDurationSeconds    int
	WebSearchCalls          int
	UpstreamStatusCode      int
	StartedAt               time.Time
	Duration                time.Duration
	TimeToFirstResponseByte time.Duration
}

func (m Measurement) validate() error {
	if m.AccountID <= 0 {
		return fmt.Errorf("%w: account ID must be positive", ErrInvalidMeasurement)
	}
	if strings.TrimSpace(m.Endpoint) == "" {
		return fmt.Errorf("%w: endpoint is required", ErrInvalidMeasurement)
	}
	if strings.TrimSpace(m.UpstreamModel) == "" {
		return fmt.Errorf("%w: upstream model is required", ErrInvalidMeasurement)
	}
	if m.InputTokens < 0 || m.ImageInputTokens < 0 || m.OutputTokens < 0 || m.ImageOutputTokens < 0 || m.CacheReadInputTokens < 0 || m.CacheWriteInputTokens < 0 {
		return fmt.Errorf("%w: token counts cannot be negative", ErrInvalidMeasurement)
	}
	if m.ImageInputTokens > m.InputTokens || m.ImageOutputTokens > m.OutputTokens {
		return fmt.Errorf("%w: image token counts must be subsets of total token counts", ErrInvalidMeasurement)
	}
	if m.ImageCount < 0 || m.VideoDurationSeconds < 0 || m.WebSearchCalls < 0 {
		return fmt.Errorf("%w: media and tool usage cannot be negative", ErrInvalidMeasurement)
	}
	if len(m.ImageOutputSizes) > m.ImageCount {
		return fmt.Errorf("%w: image output sizes exceed image count", ErrInvalidMeasurement)
	}
	for _, size := range m.ImageOutputSizes {
		if size == "" || size != strings.TrimSpace(size) {
			return fmt.Errorf("%w: image output sizes must be exact non-empty strings", ErrInvalidMeasurement)
		}
	}
	if m.UpstreamStatusCode < 100 || m.UpstreamStatusCode > 599 {
		return fmt.Errorf("%w: upstream status code must be a valid HTTP status", ErrInvalidMeasurement)
	}
	if m.StartedAt.IsZero() {
		return fmt.Errorf("%w: start time is required", ErrInvalidMeasurement)
	}
	if m.Duration < 0 || m.TimeToFirstResponseByte < 0 || m.TimeToFirstResponseByte > m.Duration {
		return fmt.Errorf("%w: response timing is inconsistent", ErrInvalidMeasurement)
	}
	return nil
}

// Dispatcher executes one client protocol against an account in the resolved
// pool. It does not authenticate or bill customers.
type Dispatcher interface {
	Forward(
		ctx context.Context,
		w http.ResponseWriter,
		req *http.Request,
		dispatch DispatchRequest,
	) (*Measurement, error)
	Close(ctx context.Context) error
}

// Dispatchers declares an exact dispatcher for every supported protocol.
type Dispatchers struct {
	Anthropic Dispatcher
	OpenAI    Dispatcher
	Gemini    Dispatcher
}

func (d Dispatchers) validate() error {
	if d.Anthropic == nil {
		return fmt.Errorf("%w: Anthropic dispatcher is required", ErrInvalidInvocation)
	}
	if d.OpenAI == nil {
		return fmt.Errorf("%w: OpenAI dispatcher is required", ErrInvalidInvocation)
	}
	if d.Gemini == nil {
		return fmt.Errorf("%w: Gemini dispatcher is required", ErrInvalidInvocation)
	}
	return nil
}

func (d Dispatchers) forProtocol(protocol Protocol) Dispatcher {
	switch protocol {
	case ProtocolAnthropic:
		return d.Anthropic
	case ProtocolOpenAI:
		return d.OpenAI
	case ProtocolGemini:
		return d.Gemini
	default:
		return nil
	}
}

// Engine is the protocol-independent in-process Next API runtime.
type Engine struct {
	resolver    PoolResolver
	dispatchers Dispatchers
	closeOnce   sync.Once
	closeErr    error
}

func NewEngine(resolver PoolResolver, dispatchers Dispatchers) (*Engine, error) {
	if resolver == nil {
		return nil, fmt.Errorf("%w: pool resolver is required", ErrInvalidInvocation)
	}
	if err := dispatchers.validate(); err != nil {
		return nil, err
	}
	return &Engine{resolver: resolver, dispatchers: dispatchers}, nil
}

func (e *Engine) Invoke(
	ctx context.Context,
	w http.ResponseWriter,
	req *http.Request,
	invocation Invocation,
) error {
	if e == nil {
		return fmt.Errorf("%w: runtime is nil", ErrInvalidInvocation)
	}
	if ctx == nil {
		return fmt.Errorf("%w: context is required", ErrInvalidInvocation)
	}
	if w == nil {
		return fmt.Errorf("%w: response writer is required", ErrInvalidInvocation)
	}
	if req == nil {
		return fmt.Errorf("%w: request is required", ErrInvalidInvocation)
	}
	if err := invocation.Validate(); err != nil {
		return err
	}

	resolvedCtx, pool, err := e.resolver.ResolvePool(ctx, invocation.PoolID)
	if err != nil {
		return fmt.Errorf("resolve gateway pool %d: %w", invocation.PoolID, err)
	}
	if resolvedCtx == nil {
		return fmt.Errorf("%w: pool resolver returned a nil context", ErrInvalidPool)
	}
	if pool.ID != invocation.PoolID || strings.TrimSpace(pool.Platform) == "" {
		return fmt.Errorf("%w: resolver returned pool %d for requested pool %d", ErrInvalidPool, pool.ID, invocation.PoolID)
	}
	if !pool.Active {
		return fmt.Errorf("%w: pool %d is not active", ErrPoolUnavailable, pool.ID)
	}

	dispatcher := e.dispatchers.forProtocol(invocation.Protocol)
	measurement, dispatchErr := dispatcher.Forward(resolvedCtx, w, req, DispatchRequest{
		Pool:       pool,
		Invocation: invocation,
	})
	if measurement == nil {
		return dispatchErr
	}
	if err := measurement.validate(); err != nil {
		return errors.Join(dispatchErr, err)
	}

	usageErr := invocation.OnUsage(resolvedCtx, Usage{
		PoolID:                  pool.ID,
		AccountID:               measurement.AccountID,
		RequestID:               invocation.RequestID,
		SessionID:               invocation.SessionID,
		Protocol:                invocation.Protocol,
		Endpoint:                measurement.Endpoint,
		Model:                   invocation.Model,
		UpstreamModel:           measurement.UpstreamModel,
		InputTokens:             measurement.InputTokens,
		ImageInputTokens:        measurement.ImageInputTokens,
		OutputTokens:            measurement.OutputTokens,
		ImageOutputTokens:       measurement.ImageOutputTokens,
		CacheReadInputTokens:    measurement.CacheReadInputTokens,
		CacheWriteInputTokens:   measurement.CacheWriteInputTokens,
		ImageCount:              measurement.ImageCount,
		ImageOutputSizes:        append([]string(nil), measurement.ImageOutputSizes...),
		VideoDurationSeconds:    measurement.VideoDurationSeconds,
		WebSearchCalls:          measurement.WebSearchCalls,
		UpstreamStatusCode:      measurement.UpstreamStatusCode,
		StartedAt:               measurement.StartedAt,
		Duration:                measurement.Duration,
		TimeToFirstResponseByte: measurement.TimeToFirstResponseByte,
	})
	return errors.Join(dispatchErr, usageErr)
}

func (e *Engine) Close(ctx context.Context) error {
	if e == nil {
		return nil
	}
	e.closeOnce.Do(func() {
		e.closeErr = errors.Join(
			e.dispatchers.Anthropic.Close(ctx),
			e.dispatchers.OpenAI.Close(ctx),
			e.dispatchers.Gemini.Close(ctx),
		)
	})
	return e.closeErr
}

var _ Runtime = (*Engine)(nil)
