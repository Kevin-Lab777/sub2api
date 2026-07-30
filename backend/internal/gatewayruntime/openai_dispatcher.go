package gatewayruntime

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/Wei-Shaw/sub2api/gatewaycore"
)

// OpenAIEndpointDispatchers contains one strict dispatcher for every OpenAI
// endpoint family exposed by the in-process runtime.
type OpenAIEndpointDispatchers struct {
	Responses       gatewaycore.Dispatcher
	ChatCompletions gatewaycore.Dispatcher
	Embeddings      gatewaycore.Dispatcher
	Images          gatewaycore.Dispatcher
	Live            gatewaycore.Dispatcher
}

func (d OpenAIEndpointDispatchers) validate() error {
	missing := make([]string, 0, 5)
	if d.Responses == nil {
		missing = append(missing, "Responses")
	}
	if d.ChatCompletions == nil {
		missing = append(missing, "Chat Completions")
	}
	if d.Embeddings == nil {
		missing = append(missing, "Embeddings")
	}
	if d.Images == nil {
		missing = append(missing, "Images")
	}
	if d.Live == nil {
		missing = append(missing, "Live")
	}
	if len(missing) > 0 {
		return fmt.Errorf("OpenAI endpoint dispatchers are incomplete: %s", strings.Join(missing, ", "))
	}
	return nil
}

// OpenAIDispatcher routes an exact inbound endpoint to its protocol-preserving
// implementation. It does not rewrite paths or try another endpoint family.
type OpenAIDispatcher struct {
	dispatchers OpenAIEndpointDispatchers
	closeOnce   sync.Once
	closeErr    error
}

func NewOpenAIDispatcher(dispatchers OpenAIEndpointDispatchers) (*OpenAIDispatcher, error) {
	if err := dispatchers.validate(); err != nil {
		return nil, err
	}
	return &OpenAIDispatcher{dispatchers: dispatchers}, nil
}

func (d *OpenAIDispatcher) Forward(
	ctx context.Context,
	w http.ResponseWriter,
	req *http.Request,
	dispatch gatewaycore.DispatchRequest,
) (*gatewaycore.Measurement, error) {
	if d == nil {
		return nil, errors.New("OpenAI dispatcher is not initialized")
	}
	endpointDispatcher, err := d.dispatcherFor(req)
	if err != nil {
		return nil, err
	}
	return endpointDispatcher.Forward(ctx, w, req, dispatch)
}

func (d *OpenAIDispatcher) dispatcherFor(req *http.Request) (gatewaycore.Dispatcher, error) {
	if req == nil || req.URL == nil {
		return nil, fmt.Errorf("unsupported OpenAI endpoint: %s %s", requestMethod(req), requestPath(req))
	}
	switch req.URL.Path {
	case "/v1/responses", "/v1/responses/compact":
		return d.dispatchers.Responses, nil
	case "/v1/chat/completions", "/v1/completions":
		return d.dispatchers.ChatCompletions, nil
	case "/v1/embeddings":
		return d.dispatchers.Embeddings, nil
	case "/v1/images/generations", "/v1/images/edits":
		return d.dispatchers.Images, nil
	case "/v1/live", "/backend-api/codex/realtime/calls":
		return d.dispatchers.Live, nil
	}
	if strings.HasPrefix(req.URL.Path, "/v1/live/") || strings.HasPrefix(req.URL.Path, "/backend-api/codex/") {
		return d.dispatchers.Live, nil
	}
	return nil, fmt.Errorf("unsupported OpenAI endpoint: %s %s", req.Method, req.URL.Path)
}

func (d *OpenAIDispatcher) Close(ctx context.Context) error {
	if d == nil {
		return nil
	}
	d.closeOnce.Do(func() {
		d.closeErr = errors.Join(
			d.dispatchers.Responses.Close(ctx),
			d.dispatchers.ChatCompletions.Close(ctx),
			d.dispatchers.Embeddings.Close(ctx),
			d.dispatchers.Images.Close(ctx),
			d.dispatchers.Live.Close(ctx),
		)
	})
	return d.closeErr
}

var _ gatewaycore.Dispatcher = (*OpenAIDispatcher)(nil)
