// Package gatewaycore defines the in-process account gateway contract.
//
// It intentionally contains no customer identity, payment, subscription, or
// billing concepts. Those belong to the caller (New API). Implementations are
// responsible only for selecting and operating upstream provider accounts.
package gatewaycore

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Protocol identifies the client protocol that must be preserved end to end.
type Protocol string

const (
	ProtocolAnthropic Protocol = "anthropic"
	ProtocolOpenAI    Protocol = "openai"
	ProtocolGemini    Protocol = "gemini"
)

// ErrInvalidInvocation is returned when an invocation is missing a required
// routing or accounting field.
var ErrInvalidInvocation = errors.New("invalid gateway invocation")

// Invocation contains the technical routing information supplied by New API.
// PoolID identifies a Next API account-pool group, not a commercial user group.
type Invocation struct {
	PoolID    int64
	RequestID string
	SessionID string
	Protocol  Protocol
	Model     string
	OnUsage   func(context.Context, Usage) error
}

// Validate rejects incomplete calls before account scheduling begins.
func (i Invocation) Validate() error {
	if i.PoolID <= 0 {
		return fmt.Errorf("%w: pool ID must be positive", ErrInvalidInvocation)
	}
	if strings.TrimSpace(i.RequestID) == "" {
		return fmt.Errorf("%w: request ID is required", ErrInvalidInvocation)
	}
	if strings.TrimSpace(i.SessionID) == "" {
		return fmt.Errorf("%w: session ID is required", ErrInvalidInvocation)
	}
	if !i.Protocol.valid() {
		return fmt.Errorf("%w: unsupported protocol %q", ErrInvalidInvocation, i.Protocol)
	}
	if strings.TrimSpace(i.Model) == "" {
		return fmt.Errorf("%w: model is required", ErrInvalidInvocation)
	}
	if i.OnUsage == nil {
		return fmt.Errorf("%w: usage callback is required", ErrInvalidInvocation)
	}
	return nil
}

func (p Protocol) valid() bool {
	switch p {
	case ProtocolAnthropic, ProtocolOpenAI, ProtocolGemini:
		return true
	default:
		return false
	}
}

// Usage is account-level upstream telemetry returned to New API. It contains
// no price or customer balance fields; New API remains the billing authority.
type Usage struct {
	PoolID                  int64
	AccountID               int64
	RequestID               string
	SessionID               string
	Protocol                Protocol
	Endpoint                string
	Model                   string
	UpstreamModel           string
	InputTokens             int64
	ImageInputTokens        int64
	OutputTokens            int64
	ImageOutputTokens       int64
	CacheReadInputTokens    int64
	CacheWriteInputTokens   int64
	ImageCount              int
	VideoDurationSeconds    int
	WebSearchCalls          int
	UpstreamStatusCode      int
	StartedAt               time.Time
	Duration                time.Duration
	TimeToFirstResponseByte time.Duration
}

// Runtime forwards a request without TCP or a second customer-facing gateway.
// The original writer and request preserve streaming, WebSocket upgrades,
// multipart bodies, cancellation, and backpressure.
type Runtime interface {
	Invoke(
		ctx context.Context,
		w http.ResponseWriter,
		req *http.Request,
		invocation Invocation,
	) error
	Close(ctx context.Context) error
}
