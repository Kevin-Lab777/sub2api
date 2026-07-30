package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const nativeOpenAIResponsesWebSocketEndpoint = "/v1/responses"

// OpenAIResponsesWebSocketResult contains connection-level upstream facts.
// Usage is the exact sum of successfully completed Responses turns observed on
// the connection; customer pricing remains the caller's responsibility.
type OpenAIResponsesWebSocketResult struct {
	Usage                  OpenAIUsage
	UpstreamModel          string
	ImageCount             int
	WebSearchCalls         int
	UpstreamStatusCode     int
	StartedAt              time.Time
	Duration               time.Duration
	TimeToFirstResponse    time.Duration
	CompletedResponseCount int
}

type nativeOpenAIResponsesFrameConn interface {
	ReadFrame(context.Context) (coderws.MessageType, []byte, error)
	WriteFrame(context.Context, coderws.MessageType, []byte) error
	Close() error
}

type nativeOpenAIResponsesDownstreamConn struct {
	conn *coderws.Conn
}

func (c *nativeOpenAIResponsesDownstreamConn) ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error) {
	if c == nil || c.conn == nil {
		return coderws.MessageText, nil, errors.New("OpenAI Responses downstream WebSocket is nil")
	}
	return c.conn.Read(ctx)
}

func (c *nativeOpenAIResponsesDownstreamConn) WriteFrame(ctx context.Context, messageType coderws.MessageType, payload []byte) error {
	if c == nil || c.conn == nil {
		return errors.New("OpenAI Responses downstream WebSocket is nil")
	}
	return c.conn.Write(ctx, messageType, payload)
}

func (c *nativeOpenAIResponsesDownstreamConn) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.CloseNow()
}

// ForwardResponsesWebSocket establishes a dedicated upstream Responses
// WebSocket before accepting the downstream upgrade, then relays application
// frames without an HTTP bridge, replay, recovery, or account switching.
func (s *OpenAIGatewayService) ForwardResponsesWebSocket(
	ctx context.Context,
	w http.ResponseWriter,
	req *http.Request,
	account *Account,
	invocationModel string,
	sessionID string,
) (*OpenAIResponsesWebSocketResult, error) {
	if s == nil {
		return nil, errors.New("OpenAI gateway service is nil")
	}
	if ctx == nil || w == nil || req == nil || req.URL == nil {
		return nil, errors.New("OpenAI Responses WebSocket transport is incomplete")
	}
	if req.Method != http.MethodGet || req.URL.Path != nativeOpenAIResponsesWebSocketEndpoint || !isNativeOpenAIResponsesWebSocketUpgrade(req) {
		return nil, fmt.Errorf("invalid OpenAI Responses WebSocket endpoint: %s %s", req.Method, req.URL.Path)
	}
	if account == nil || account.Platform != PlatformOpenAI {
		return nil, errors.New("OpenAI Responses WebSocket requires an OpenAI account")
	}
	if !account.SupportsTechnicalOpenAIResponsesWebSocketV2() {
		return nil, fmt.Errorf("OpenAI account %d has no explicit direct Responses WebSocket v2 capability", account.ID)
	}
	if sessionID == "" || sessionID != strings.TrimSpace(sessionID) {
		return nil, errors.New("OpenAI Responses WebSocket session ID must be an exact non-empty value")
	}
	upstreamModel, err := resolveExactOpenAIAccountModel(account, invocationModel)
	if err != nil {
		return nil, err
	}
	startedAt := time.Now()
	token, err := s.nativeOpenAIResponsesCredential(ctx, account)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, nativeOpenAIAccountFailoverError(
			http.StatusBadGateway,
			GatewayFailureStageAccountAuth,
			openAIResponsesCredentialUnavailableReason,
			fmt.Errorf("get OpenAI Responses WebSocket credential: %w", err),
		)
	}
	headers, err := s.buildNativeOpenAIResponsesWebSocketHeaders(ctx, account, token, sessionID)
	if err != nil {
		return nil, nativeOpenAIAccountFailoverError(
			http.StatusBadGateway,
			GatewayFailureStageAccountAuth,
			openAIResponsesCredentialUnavailableReason,
			fmt.Errorf("build OpenAI Responses WebSocket headers: %w", err),
		)
	}
	wsURL, err := s.buildOpenAIResponsesWSURL(account)
	if err != nil {
		return nil, err
	}
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	dialer := s.getOpenAIWSPassthroughDialer()
	if dialer == nil {
		return nil, errors.New("OpenAI Responses WebSocket dialer is unavailable")
	}
	dialCtx, cancelDial := context.WithTimeout(ctx, s.openAIWSDialTimeout())
	upstreamClient, statusCode, _, dialErr := dialer.Dial(dialCtx, wsURL, headers, proxyURL)
	cancelDial()
	if dialErr != nil {
		if statusCode < 100 || statusCode > 599 {
			statusCode = http.StatusBadGateway
		}
		return nil, nativeOpenAIAccountFailoverError(
			statusCode,
			GatewayFailureStageInference,
			openAIResponsesTransportUnavailableReason,
			fmt.Errorf("dial OpenAI Responses WebSocket: %w", dialErr),
		)
	}
	if upstreamClient == nil {
		return nil, nativeOpenAIAccountFailoverError(
			http.StatusBadGateway,
			GatewayFailureStageInference,
			openAIResponsesTransportUnavailableReason,
			errors.New("OpenAI Responses WebSocket dialer returned a nil connection"),
		)
	}
	upstream, ok := upstreamClient.(nativeOpenAIResponsesFrameConn)
	if !ok {
		_ = upstreamClient.Close()
		return nil, errors.New("OpenAI Responses WebSocket connection does not support raw frames")
	}
	defer func() { _ = upstream.Close() }()

	downstreamConn, err := coderws.Accept(w, req, &coderws.AcceptOptions{
		CompressionMode: coderws.CompressionContextTakeover,
	})
	if err != nil {
		return nil, fmt.Errorf("accept OpenAI Responses WebSocket: %w", err)
	}
	downstreamConn.SetReadLimit(ResolveOpenAIWSClientReadLimitBytes(s.cfg))
	downstream := &nativeOpenAIResponsesDownstreamConn{conn: downstreamConn}
	defer func() { _ = downstream.Close() }()

	result := &OpenAIResponsesWebSocketResult{
		UpstreamModel:      upstreamModel,
		UpstreamStatusCode: http.StatusSwitchingProtocols,
		StartedAt:          startedAt,
	}
	firstCtx, cancelFirst := context.WithTimeout(ctx, ResolveOpenAIWSClientFirstMessageTimeout(s.cfg))
	firstType, firstPayload, firstErr := downstream.ReadFrame(firstCtx)
	cancelFirst()
	if firstErr != nil {
		result.Duration = time.Since(startedAt)
		return result, fmt.Errorf("read first OpenAI Responses WebSocket frame: %w", firstErr)
	}
	mappedFirst, err := mapNativeOpenAIResponsesWebSocketCreateFrame(firstPayload, invocationModel, upstreamModel, true)
	if err != nil {
		result.Duration = time.Since(startedAt)
		return result, err
	}
	writeCtx, cancelWrite := context.WithTimeout(ctx, s.openAIWSWriteTimeout())
	err = upstream.WriteFrame(writeCtx, firstType, mappedFirst)
	cancelWrite()
	if err != nil {
		result.Duration = time.Since(startedAt)
		return result, fmt.Errorf("write first OpenAI Responses WebSocket frame upstream: %w", err)
	}

	relayErr := s.relayNativeOpenAIResponsesWebSocket(ctx, downstream, upstream, invocationModel, upstreamModel, result)
	result.Duration = time.Since(startedAt)
	return result, relayErr
}

func (s *OpenAIGatewayService) buildNativeOpenAIResponsesWebSocketHeaders(ctx context.Context, account *Account, token, sessionID string) (http.Header, error) {
	headers, err := s.buildOpenAIAuthenticationHeaders(ctx, account, token)
	if err != nil {
		return nil, err
	}
	if account.Type == AccountTypeOAuth {
		if err := resolveAndSetOpenAIChatGPTAccountHeaders(ctx, s.accountRepo, headers, account); err != nil {
			return nil, err
		}
	}
	if customUA := strings.TrimSpace(account.GetOpenAIUserAgent()); customUA != "" {
		headers.Set("User-Agent", customUA)
	}
	account.ApplyHeaderOverrides(headers)
	// Transport and invocation identity are runtime invariants, not account
	// override points.
	headers.Set("OpenAI-Beta", openAIWSBetaV2Value)
	headers.Set("originator", "next-api")
	headers.Set("session_id", sessionID)
	return headers, nil
}

func nativeOpenAIAccountFailoverError(statusCode int, stage GatewayFailureStage, reason GatewayFailureReason, cause error) error {
	return errors.Join(&UpstreamFailoverError{
		StatusCode:        statusCode,
		Stage:             stage,
		Scope:             GatewayFailureScopeAccount,
		Reason:            reason,
		NextAccountAction: NextAccountRetry,
	}, cause)
}

func isNativeOpenAIResponsesWebSocketUpgrade(req *http.Request) bool {
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

func mapNativeOpenAIResponsesWebSocketCreateFrame(payload []byte, invocationModel, upstreamModel string, first bool) ([]byte, error) {
	if !json.Valid(payload) {
		if first {
			return nil, errors.New("first OpenAI Responses WebSocket frame must be valid JSON")
		}
		return payload, nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(payload, &object); err != nil || object == nil {
		if first {
			return nil, errors.New("first OpenAI Responses WebSocket frame must be a JSON object")
		}
		return payload, nil
	}
	var eventType string
	typeRaw, hasType := object["type"]
	if hasType {
		if err := json.Unmarshal(typeRaw, &eventType); err != nil {
			if first {
				return nil, errors.New("first OpenAI Responses WebSocket frame type must be a string")
			}
			return payload, nil
		}
	}
	if first && eventType != "response.create" {
		return nil, errors.New("first OpenAI Responses WebSocket frame type must be response.create")
	}
	if eventType != "response.create" {
		return payload, nil
	}
	modelRaw, hasModel := object["model"]
	if !hasModel {
		if first {
			return nil, errors.New("first OpenAI Responses WebSocket response.create frame requires model")
		}
		return payload, nil
	}
	var model string
	if err := json.Unmarshal(modelRaw, &model); err != nil || model == "" || model != strings.TrimSpace(model) {
		return nil, errors.New("OpenAI Responses WebSocket response.create model must be an exact non-empty string")
	}
	if model != invocationModel {
		return nil, fmt.Errorf("OpenAI Responses WebSocket model mismatch: invocation=%q frame=%q", invocationModel, model)
	}
	if upstreamModel == invocationModel {
		return payload, nil
	}
	mapped, err := sjson.SetBytes(payload, "model", upstreamModel)
	if err != nil {
		return nil, fmt.Errorf("map OpenAI Responses WebSocket model: %w", err)
	}
	return mapped, nil
}

type nativeOpenAIResponsesRelayExit struct {
	stage string
	err   error
}

func (s *OpenAIGatewayService) relayNativeOpenAIResponsesWebSocket(
	ctx context.Context,
	downstream nativeOpenAIResponsesFrameConn,
	upstream nativeOpenAIResponsesFrameConn,
	invocationModel string,
	upstreamModel string,
	result *OpenAIResponsesWebSocketResult,
) error {
	if downstream == nil || upstream == nil || result == nil {
		return errors.New("OpenAI Responses WebSocket relay is incomplete")
	}
	relayCtx, cancelRelay := context.WithCancel(ctx)
	exits := make(chan nativeOpenAIResponsesRelayExit, 2)
	writeTimeout := s.openAIWSWriteTimeout()

	go func() {
		for {
			messageType, payload, err := downstream.ReadFrame(relayCtx)
			if err != nil {
				exits <- nativeOpenAIResponsesRelayExit{stage: "read downstream", err: err}
				return
			}
			mapped, err := mapNativeOpenAIResponsesWebSocketCreateFrame(payload, invocationModel, upstreamModel, false)
			if err != nil {
				exits <- nativeOpenAIResponsesRelayExit{stage: "validate downstream", err: err}
				return
			}
			writeCtx, cancel := context.WithTimeout(relayCtx, writeTimeout)
			err = upstream.WriteFrame(writeCtx, messageType, mapped)
			cancel()
			if err != nil {
				exits <- nativeOpenAIResponsesRelayExit{stage: "write upstream", err: err}
				return
			}
		}
	}()

	aggregate := &nativeOpenAIResponsesWebSocketAggregate{seenResponseIDs: make(map[string]struct{})}
	go func() {
		firstResponseObserved := false
		for {
			messageType, payload, err := upstream.ReadFrame(relayCtx)
			if err != nil {
				exits <- nativeOpenAIResponsesRelayExit{stage: "read upstream", err: err}
				return
			}
			if !firstResponseObserved {
				result.TimeToFirstResponse = time.Since(result.StartedAt)
				firstResponseObserved = true
			}
			observeErr := aggregate.observe(messageType, payload)
			writeCtx, cancel := context.WithTimeout(relayCtx, writeTimeout)
			writeErr := downstream.WriteFrame(writeCtx, messageType, payload)
			cancel()
			if observeErr != nil {
				exits <- nativeOpenAIResponsesRelayExit{stage: "measure upstream", err: observeErr}
				return
			}
			if writeErr != nil {
				exits <- nativeOpenAIResponsesRelayExit{stage: "write downstream", err: writeErr}
				return
			}
		}
	}()

	firstExit := <-exits
	cancelRelay()
	_ = downstream.Close()
	_ = upstream.Close()
	secondExit := <-exits
	result.Usage = aggregate.usage
	result.ImageCount = aggregate.imageCount
	result.WebSearchCalls = aggregate.webSearchCalls
	result.CompletedResponseCount = len(aggregate.seenResponseIDs)
	for _, exit := range [...]nativeOpenAIResponsesRelayExit{firstExit, secondExit} {
		if exit.stage == "validate downstream" || exit.stage == "measure upstream" {
			return fmt.Errorf("OpenAI Responses WebSocket %s: %w", exit.stage, exit.err)
		}
	}
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	if nativeOpenAIResponsesWebSocketGracefulClose(firstExit.err) || nativeOpenAIResponsesWebSocketGracefulClose(secondExit.err) {
		return nil
	}
	return fmt.Errorf("OpenAI Responses WebSocket %s: %w", firstExit.stage, firstExit.err)
}

func nativeOpenAIResponsesWebSocketGracefulClose(err error) bool {
	status := coderws.CloseStatus(err)
	return status == coderws.StatusNormalClosure || status == coderws.StatusGoingAway
}

type nativeOpenAIResponsesWebSocketAggregate struct {
	usage           OpenAIUsage
	imageCount      int
	webSearchCalls  int
	seenResponseIDs map[string]struct{}
}

func (a *nativeOpenAIResponsesWebSocketAggregate) observe(messageType coderws.MessageType, payload []byte) error {
	if messageType != coderws.MessageText || !json.Valid(payload) {
		return nil
	}
	eventType := gjson.GetBytes(payload, "type")
	if eventType.Type != gjson.String || (eventType.String() != "response.completed" && eventType.String() != "response.done") {
		return nil
	}
	response := gjson.GetBytes(payload, "response")
	if !response.Exists() || !response.IsObject() || !json.Valid([]byte(response.Raw)) {
		return fmt.Errorf("OpenAI Responses WebSocket %s event omitted a valid response object", eventType.String())
	}
	responseID := gjson.Get(response.Raw, "id")
	if responseID.Type != gjson.String || responseID.String() == "" || responseID.String() != strings.TrimSpace(responseID.String()) {
		return fmt.Errorf("OpenAI Responses WebSocket %s event omitted an exact response ID", eventType.String())
	}
	if _, duplicate := a.seenResponseIDs[responseID.String()]; duplicate {
		return fmt.Errorf("OpenAI Responses WebSocket emitted multiple terminal events for response %q", responseID.String())
	}
	usage, found, err := extractNativeOpenAIResponsesUsage(payload)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("OpenAI Responses WebSocket %s event omitted usage", eventType.String())
	}
	imageCount, _, webSearchCalls := nativeOpenAIResponsesOutputTelemetry([]byte(response.Raw))
	nextUsage := a.usage
	if err := addNativeOpenAIResponsesUsage(&nextUsage, usage); err != nil {
		return err
	}
	if imageCount > math.MaxInt-a.imageCount || webSearchCalls > math.MaxInt-a.webSearchCalls {
		return errors.New("OpenAI Responses WebSocket media telemetry overflow")
	}
	a.usage = nextUsage
	a.imageCount += imageCount
	a.webSearchCalls += webSearchCalls
	if a.seenResponseIDs == nil {
		a.seenResponseIDs = make(map[string]struct{})
	}
	a.seenResponseIDs[responseID.String()] = struct{}{}
	return nil
}

func addNativeOpenAIResponsesUsage(total *OpenAIUsage, turn OpenAIUsage) error {
	if total == nil {
		return errors.New("OpenAI Responses WebSocket usage accumulator is nil")
	}
	next := *total
	fields := []struct {
		name string
		dst  *int
		src  int
	}{
		{name: "input_tokens", dst: &next.InputTokens, src: turn.InputTokens},
		{name: "output_tokens", dst: &next.OutputTokens, src: turn.OutputTokens},
		{name: "cache_creation_input_tokens", dst: &next.CacheCreationInputTokens, src: turn.CacheCreationInputTokens},
		{name: "cache_read_input_tokens", dst: &next.CacheReadInputTokens, src: turn.CacheReadInputTokens},
		{name: "image_input_tokens", dst: &next.ImageInputTokens, src: turn.ImageInputTokens},
		{name: "image_output_tokens", dst: &next.ImageOutputTokens, src: turn.ImageOutputTokens},
	}
	for _, field := range fields {
		if field.src < 0 || *field.dst > math.MaxInt-field.src {
			return fmt.Errorf("OpenAI Responses WebSocket %s overflow", field.name)
		}
		*field.dst += field.src
	}
	*total = next
	return nil
}
