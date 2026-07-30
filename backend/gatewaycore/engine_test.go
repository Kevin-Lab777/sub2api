package gatewaycore

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type poolResolverFunc func(context.Context, int64) (Pool, error)

func (f poolResolverFunc) ResolvePool(ctx context.Context, poolID int64) (Pool, error) {
	return f(ctx, poolID)
}

type dispatcherStub struct {
	forward  func(context.Context, http.ResponseWriter, *http.Request, DispatchRequest) (*Measurement, error)
	closed   int
	closeErr error
}

func (d *dispatcherStub) Forward(ctx context.Context, w http.ResponseWriter, req *http.Request, dispatch DispatchRequest) (*Measurement, error) {
	return d.forward(ctx, w, req, dispatch)
}

func (d *dispatcherStub) Close(context.Context) error {
	d.closed++
	return d.closeErr
}

func validMeasurement() *Measurement {
	return &Measurement{
		AccountID:               91,
		Endpoint:                "/backend-api/codex/responses",
		UpstreamModel:           "gpt-5.4-codex",
		InputTokens:             120,
		OutputTokens:            30,
		CacheReadInputTokens:    40,
		UpstreamStatusCode:      http.StatusOK,
		StartedAt:               time.Unix(100, 0),
		Duration:                2 * time.Second,
		TimeToFirstResponseByte: 300 * time.Millisecond,
	}
}

func validInvocation(onUsage func(context.Context, Usage) error) Invocation {
	return Invocation{
		PoolID:    17,
		RequestID: "req-1",
		SessionID: "session-2",
		Protocol:  ProtocolOpenAI,
		Model:     "gpt-5.4",
		OnUsage:   onUsage,
	}
}

func newTestEngine(t *testing.T, openAI *dispatcherStub) (*Engine, *dispatcherStub, *dispatcherStub) {
	t.Helper()
	anthropic := &dispatcherStub{forward: func(context.Context, http.ResponseWriter, *http.Request, DispatchRequest) (*Measurement, error) {
		t.Fatal("Anthropic dispatcher must not be selected for an OpenAI invocation")
		return nil, nil
	}}
	gemini := &dispatcherStub{forward: func(context.Context, http.ResponseWriter, *http.Request, DispatchRequest) (*Measurement, error) {
		t.Fatal("Gemini dispatcher must not be selected for an OpenAI invocation")
		return nil, nil
	}}
	engine, err := NewEngine(
		poolResolverFunc(func(_ context.Context, poolID int64) (Pool, error) {
			return Pool{ID: poolID, Name: "Codex", Platform: "openai", Active: true}, nil
		}),
		Dispatchers{Anthropic: anthropic, OpenAI: openAI, Gemini: gemini},
	)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}
	return engine, anthropic, gemini
}

func TestEngineInvokeDispatchesResolvedPoolAndReportsRawUsage(t *testing.T) {
	var gotDispatch DispatchRequest
	openAI := &dispatcherStub{forward: func(_ context.Context, _ http.ResponseWriter, _ *http.Request, dispatch DispatchRequest) (*Measurement, error) {
		gotDispatch = dispatch
		return validMeasurement(), nil
	}}
	engine, _, _ := newTestEngine(t, openAI)

	var gotUsage Usage
	err := engine.Invoke(
		context.Background(),
		httptest.NewRecorder(),
		httptest.NewRequest(http.MethodPost, "/v1/responses", nil),
		validInvocation(func(_ context.Context, usage Usage) error {
			gotUsage = usage
			return nil
		}),
	)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if gotDispatch.Pool.ID != 17 || gotDispatch.Invocation.RequestID != "req-1" {
		t.Fatalf("dispatcher received %#v", gotDispatch)
	}
	if gotUsage.PoolID != 17 || gotUsage.AccountID != 91 || gotUsage.RequestID != "req-1" || gotUsage.Model != "gpt-5.4" {
		t.Fatalf("usage = %#v", gotUsage)
	}
}

func TestEngineInvokeRejectsUnavailablePoolBeforeDispatch(t *testing.T) {
	called := false
	dispatcher := &dispatcherStub{forward: func(context.Context, http.ResponseWriter, *http.Request, DispatchRequest) (*Measurement, error) {
		called = true
		return nil, nil
	}}
	engine, err := NewEngine(
		poolResolverFunc(func(_ context.Context, poolID int64) (Pool, error) {
			return Pool{ID: poolID, Platform: "openai", Active: false}, nil
		}),
		Dispatchers{Anthropic: dispatcher, OpenAI: dispatcher, Gemini: dispatcher},
	)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	err = engine.Invoke(context.Background(), httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/responses", nil), validInvocation(func(context.Context, Usage) error { return nil }))
	if !errors.Is(err, ErrPoolUnavailable) {
		t.Fatalf("Invoke() error = %v, want ErrPoolUnavailable", err)
	}
	if called {
		t.Fatal("dispatcher was called for an inactive pool")
	}
}

func TestEngineInvokeRejectsInvalidMeasurementWithoutUsageCallback(t *testing.T) {
	openAI := &dispatcherStub{forward: func(context.Context, http.ResponseWriter, *http.Request, DispatchRequest) (*Measurement, error) {
		measurement := validMeasurement()
		measurement.AccountID = 0
		return measurement, nil
	}}
	engine, _, _ := newTestEngine(t, openAI)
	usageCalled := false

	err := engine.Invoke(context.Background(), httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/responses", nil), validInvocation(func(context.Context, Usage) error {
		usageCalled = true
		return nil
	}))
	if !errors.Is(err, ErrInvalidMeasurement) {
		t.Fatalf("Invoke() error = %v, want ErrInvalidMeasurement", err)
	}
	if usageCalled {
		t.Fatal("usage callback was called for an invalid measurement")
	}
}

func TestEngineInvokeReturnsDispatchAndUsageErrors(t *testing.T) {
	dispatchErr := errors.New("upstream failed")
	usageErr := errors.New("usage persistence failed")
	openAI := &dispatcherStub{forward: func(context.Context, http.ResponseWriter, *http.Request, DispatchRequest) (*Measurement, error) {
		return validMeasurement(), dispatchErr
	}}
	engine, _, _ := newTestEngine(t, openAI)

	err := engine.Invoke(context.Background(), httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/responses", nil), validInvocation(func(context.Context, Usage) error {
		return usageErr
	}))
	if !errors.Is(err, dispatchErr) || !errors.Is(err, usageErr) {
		t.Fatalf("Invoke() error = %v, want both dispatch and usage errors", err)
	}
}

func TestEngineCloseClosesEachProtocolOnce(t *testing.T) {
	openAI := &dispatcherStub{forward: func(context.Context, http.ResponseWriter, *http.Request, DispatchRequest) (*Measurement, error) {
		return nil, nil
	}}
	engine, anthropic, gemini := newTestEngine(t, openAI)

	if err := engine.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := engine.Close(context.Background()); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if anthropic.closed != 1 || openAI.closed != 1 || gemini.closed != 1 {
		t.Fatalf("close counts = anthropic:%d openai:%d gemini:%d", anthropic.closed, openAI.closed, gemini.closed)
	}
}
