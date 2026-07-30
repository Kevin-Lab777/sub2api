package gatewayruntime

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/gatewaycore"
)

type endpointDispatcherStub struct {
	name       string
	forwarded  int
	closed     int
	closeError error
}

func (s *endpointDispatcherStub) Forward(_ context.Context, _ http.ResponseWriter, _ *http.Request, _ gatewaycore.DispatchRequest) (*gatewaycore.Measurement, error) {
	s.forwarded++
	return &gatewaycore.Measurement{Endpoint: s.name}, nil
}

func (s *endpointDispatcherStub) Close(context.Context) error {
	s.closed++
	return s.closeError
}

func newOpenAIEndpointDispatcherTestSet() (OpenAIEndpointDispatchers, map[string]*endpointDispatcherStub) {
	stubs := map[string]*endpointDispatcherStub{
		"responses":  {name: "responses"},
		"chat":       {name: "chat"},
		"embeddings": {name: "embeddings"},
		"images":     {name: "images"},
		"live":       {name: "live"},
	}
	return OpenAIEndpointDispatchers{
		Responses:       stubs["responses"],
		ChatCompletions: stubs["chat"],
		Embeddings:      stubs["embeddings"],
		Images:          stubs["images"],
		Live:            stubs["live"],
	}, stubs
}

func TestOpenAIDispatcherRoutesExactEndpointFamilies(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   string
	}{
		{method: http.MethodPost, path: "/v1/responses", want: "responses"},
		{method: http.MethodGet, path: "/v1/responses", want: "responses"},
		{method: http.MethodPost, path: "/v1/responses/compact", want: "responses"},
		{method: http.MethodPost, path: "/v1/chat/completions", want: "chat"},
		{method: http.MethodPost, path: "/v1/completions", want: "chat"},
		{method: http.MethodPost, path: "/v1/embeddings", want: "embeddings"},
		{method: http.MethodPost, path: "/v1/images/generations", want: "images"},
		{method: http.MethodPost, path: "/v1/images/edits", want: "images"},
		{method: http.MethodPost, path: "/v1/live", want: "live"},
		{method: http.MethodPost, path: "/backend-api/codex/realtime/calls", want: "live"},
		{method: http.MethodGet, path: "/v1/live/call-id", want: "live"},
		{method: http.MethodGet, path: "/backend-api/codex/call-id", want: "live"},
	}
	for _, test := range tests {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			dispatchers, stubs := newOpenAIEndpointDispatcherTestSet()
			dispatcher, err := NewOpenAIDispatcher(dispatchers)
			if err != nil {
				t.Fatalf("NewOpenAIDispatcher: %v", err)
			}
			measurement, err := dispatcher.Forward(
				context.Background(),
				httptest.NewRecorder(),
				httptest.NewRequest(test.method, test.path, nil),
				gatewaycore.DispatchRequest{},
			)
			if err != nil || measurement == nil || measurement.Endpoint != test.want {
				t.Fatalf("route result: measurement=%+v err=%v", measurement, err)
			}
			for name, stub := range stubs {
				wantCalls := 0
				if name == test.want {
					wantCalls = 1
				}
				if stub.forwarded != wantCalls {
					t.Fatalf("dispatcher %s calls=%d want=%d", name, stub.forwarded, wantCalls)
				}
			}
		})
	}
}

func TestOpenAIDispatcherRejectsUnsupportedEndpointWithoutDispatch(t *testing.T) {
	dispatchers, stubs := newOpenAIEndpointDispatcherTestSet()
	dispatcher, err := NewOpenAIDispatcher(dispatchers)
	if err != nil {
		t.Fatalf("NewOpenAIDispatcher: %v", err)
	}
	measurement, err := dispatcher.Forward(
		context.Background(),
		httptest.NewRecorder(),
		httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", nil),
		gatewaycore.DispatchRequest{},
	)
	if err == nil || measurement != nil {
		t.Fatalf("unsupported endpoint result: measurement=%+v err=%v", measurement, err)
	}
	for name, stub := range stubs {
		if stub.forwarded != 0 {
			t.Fatalf("unsupported endpoint reached %s dispatcher", name)
		}
	}
}

func TestOpenAIDispatcherRequiresEveryEndpointFamily(t *testing.T) {
	dispatchers, _ := newOpenAIEndpointDispatcherTestSet()
	dispatchers.Images = nil
	if _, err := NewOpenAIDispatcher(dispatchers); err == nil {
		t.Fatal("expected missing Images dispatcher to be rejected")
	}
}

func TestOpenAIDispatcherClosesEachEndpointFamilyOnce(t *testing.T) {
	dispatchers, stubs := newOpenAIEndpointDispatcherTestSet()
	closeErr := errors.New("live close failed")
	stubs["live"].closeError = closeErr
	dispatcher, err := NewOpenAIDispatcher(dispatchers)
	if err != nil {
		t.Fatalf("NewOpenAIDispatcher: %v", err)
	}
	if err := dispatcher.Close(context.Background()); !errors.Is(err, closeErr) {
		t.Fatalf("Close error=%v", err)
	}
	if err := dispatcher.Close(context.Background()); !errors.Is(err, closeErr) {
		t.Fatalf("second Close error=%v", err)
	}
	for name, stub := range stubs {
		if stub.closed != 1 {
			t.Fatalf("dispatcher %s closed %d times", name, stub.closed)
		}
	}
}
