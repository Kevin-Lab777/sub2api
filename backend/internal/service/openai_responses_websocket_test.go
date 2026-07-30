package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

type nativeOpenAIResponsesTestFrame struct {
	messageType coderws.MessageType
	payload     []byte
}

type nativeOpenAIResponsesTestConn struct {
	reads     chan nativeOpenAIResponsesTestFrame
	writes    chan nativeOpenAIResponsesTestFrame
	closed    chan struct{}
	closeOnce sync.Once
}

func newNativeOpenAIResponsesTestConn() *nativeOpenAIResponsesTestConn {
	return &nativeOpenAIResponsesTestConn{
		reads:  make(chan nativeOpenAIResponsesTestFrame, 8),
		writes: make(chan nativeOpenAIResponsesTestFrame, 8),
		closed: make(chan struct{}),
	}
}

func (c *nativeOpenAIResponsesTestConn) WriteJSON(ctx context.Context, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.WriteFrame(ctx, coderws.MessageText, payload)
}

func (c *nativeOpenAIResponsesTestConn) ReadMessage(ctx context.Context) ([]byte, error) {
	_, payload, err := c.ReadFrame(ctx)
	return payload, err
}

func (c *nativeOpenAIResponsesTestConn) ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error) {
	select {
	case frame := <-c.reads:
		return frame.messageType, append([]byte(nil), frame.payload...), nil
	case <-c.closed:
		return coderws.MessageText, nil, io.EOF
	case <-ctx.Done():
		return coderws.MessageText, nil, ctx.Err()
	}
}

func (c *nativeOpenAIResponsesTestConn) WriteFrame(ctx context.Context, messageType coderws.MessageType, payload []byte) error {
	frame := nativeOpenAIResponsesTestFrame{messageType: messageType, payload: append([]byte(nil), payload...)}
	select {
	case c.writes <- frame:
		return nil
	case <-c.closed:
		return io.EOF
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *nativeOpenAIResponsesTestConn) Ping(context.Context) error { return nil }

func (c *nativeOpenAIResponsesTestConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

type nativeOpenAIResponsesTestDial struct {
	url      string
	headers  http.Header
	proxyURL string
}

type nativeOpenAIResponsesTestDialer struct {
	conn   *nativeOpenAIResponsesTestConn
	dialed chan nativeOpenAIResponsesTestDial
}

func (d *nativeOpenAIResponsesTestDialer) Dial(_ context.Context, wsURL string, headers http.Header, proxyURL string) (openAIWSClientConn, int, http.Header, error) {
	d.dialed <- nativeOpenAIResponsesTestDial{url: wsURL, headers: headers.Clone(), proxyURL: proxyURL}
	return d.conn, 0, nil, nil
}

func TestForwardResponsesWebSocketRelaysRawFramesAndAggregatesCompletedTurns(t *testing.T) {
	upstream := newNativeOpenAIResponsesTestConn()
	dialer := &nativeOpenAIResponsesTestDialer{conn: upstream, dialed: make(chan nativeOpenAIResponsesTestDial, 1)}
	svc := &OpenAIGatewayService{
		cfg:                       &config.Config{},
		openaiWSPassthroughDialer: dialer,
	}
	account := &Account{
		ID:       301,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":    "sk-upstream",
			"user_agent": "next-api-test/1.0",
			"model_mapping": map[string]any{
				"gpt-5.4": "gpt-5.4-upstream",
			},
		},
		Extra: map[string]any{
			"openai_apikey_responses_websockets_v2_enabled": true,
		},
	}
	type serverResult struct {
		result *OpenAIResponsesWebSocketResult
		err    error
	}
	serverResultCh := make(chan serverResult, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		result, err := svc.ForwardResponsesWebSocket(req.Context(), w, req, account, "gpt-5.4", "session-technical")
		serverResultCh <- serverResult{result: result, err: err}
	}))
	defer server.Close()

	client, _, err := coderws.Dial(context.Background(), "ws"+strings.TrimPrefix(server.URL, "http")+"/v1/responses", nil)
	require.NoError(t, err)
	defer client.CloseNow()

	dial := <-dialer.dialed
	require.Equal(t, "wss://api.openai.com/v1/responses", dial.url)
	require.Empty(t, dial.proxyURL)
	require.Equal(t, "Bearer sk-upstream", dial.headers.Get("Authorization"))
	require.Equal(t, openAIWSBetaV2Value, dial.headers.Get("OpenAI-Beta"))
	require.Equal(t, "next-api", dial.headers.Get("originator"))
	require.Equal(t, "session-technical", dial.headers.Get("session_id"))
	require.Equal(t, "next-api-test/1.0", dial.headers.Get("User-Agent"))

	firstCreate := []byte(`{"type":"response.create","model":"gpt-5.4","input":"hello"}`)
	require.NoError(t, client.Write(context.Background(), coderws.MessageText, firstCreate))
	firstUpstream := <-upstream.writes
	require.Equal(t, coderws.MessageText, firstUpstream.messageType)
	require.JSONEq(t, `{"type":"response.create","model":"gpt-5.4-upstream","input":"hello"}`, string(firstUpstream.payload))

	firstTerminal := []byte(`{"type":"response.completed","response":{"id":"resp_1","usage":{"input_tokens":3,"output_tokens":2,"input_tokens_details":{"cached_tokens":1}},"output":[{"type":"image_generation_call","result":"image-a","size":"1024x1024"},{"type":"web_search_call"}]}}`)
	upstream.reads <- nativeOpenAIResponsesTestFrame{messageType: coderws.MessageText, payload: firstTerminal}
	messageType, payload, err := client.Read(context.Background())
	require.NoError(t, err)
	require.Equal(t, coderws.MessageText, messageType)
	require.Equal(t, firstTerminal, payload)

	secondCreate := []byte(`{"type":"response.create","input":"again"}`)
	require.NoError(t, client.Write(context.Background(), coderws.MessageText, secondCreate))
	secondUpstream := <-upstream.writes
	require.Equal(t, secondCreate, secondUpstream.payload)

	secondTerminal := []byte(`{"type":"response.done","response":{"id":"resp_2","usage":{"input_tokens":5,"output_tokens":4,"input_tokens_details":{"cached_tokens":2}},"output":[{"type":"web_search_call"}]}}`)
	upstream.reads <- nativeOpenAIResponsesTestFrame{messageType: coderws.MessageText, payload: secondTerminal}
	messageType, payload, err = client.Read(context.Background())
	require.NoError(t, err)
	require.Equal(t, coderws.MessageText, messageType)
	require.Equal(t, secondTerminal, payload)

	require.NoError(t, client.Close(coderws.StatusNormalClosure, "done"))
	observed := <-serverResultCh
	require.NoError(t, observed.err)
	require.NotNil(t, observed.result)
	require.Equal(t, 8, observed.result.Usage.InputTokens)
	require.Equal(t, 6, observed.result.Usage.OutputTokens)
	require.Equal(t, 3, observed.result.Usage.CacheReadInputTokens)
	require.Equal(t, 1, observed.result.ImageCount)
	require.Equal(t, []string{"1024x1024"}, observed.result.ImageOutputSizes)
	require.Equal(t, 2, observed.result.WebSearchCalls)
	require.Equal(t, 2, observed.result.CompletedResponseCount)
	require.Equal(t, http.StatusSwitchingProtocols, observed.result.UpstreamStatusCode)
	require.Positive(t, observed.result.Duration)
	require.Positive(t, observed.result.TimeToFirstResponse)
	require.LessOrEqual(t, observed.result.TimeToFirstResponse, observed.result.Duration)
}

func TestMapNativeOpenAIResponsesWebSocketCreateFrameIsStrictAndMinimal(t *testing.T) {
	mapped, err := mapNativeOpenAIResponsesWebSocketCreateFrame(
		[]byte(`{"type":"response.create","model":"gpt-5.4","store":false}`),
		"gpt-5.4",
		"gpt-5.4-upstream",
		true,
	)
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"response.create","model":"gpt-5.4-upstream","store":false}`, string(mapped))

	omitted := []byte(`{"type":"response.create","input":"next"}`)
	got, err := mapNativeOpenAIResponsesWebSocketCreateFrame(omitted, "gpt-5.4", "gpt-5.4-upstream", false)
	require.NoError(t, err)
	require.Equal(t, omitted, got)

	_, err = mapNativeOpenAIResponsesWebSocketCreateFrame([]byte(`{"type":"response.create","model":"gpt-5.3"}`), "gpt-5.4", "gpt-5.4-upstream", false)
	require.ErrorContains(t, err, "model mismatch")
	_, err = mapNativeOpenAIResponsesWebSocketCreateFrame([]byte(`{"type":"session.update"}`), "gpt-5.4", "gpt-5.4-upstream", true)
	require.ErrorContains(t, err, "response.create")
	_, err = mapNativeOpenAIResponsesWebSocketCreateFrame([]byte(`not-json`), "gpt-5.4", "gpt-5.4-upstream", true)
	require.ErrorContains(t, err, "valid JSON")
}

func TestBuildNativeOpenAIResponsesWebSocketHeadersKeepsTechnicalIdentity(t *testing.T) {
	svc := &OpenAIGatewayService{}
	apiKeyAccount := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"user_agent":              "credential-agent/1.0",
			"header_override_enabled": true,
			"header_overrides": map[string]any{
				"user-agent":  "override-agent/2.0",
				"originator":  "impersonated-client",
				"openai-beta": "wrong-protocol",
			},
		},
	}
	headers, err := svc.buildNativeOpenAIResponsesWebSocketHeaders(context.Background(), apiKeyAccount, "sk-test", "session-1")
	require.NoError(t, err)
	require.Equal(t, "Bearer sk-test", headers.Get("Authorization"))
	require.Equal(t, "override-agent/2.0", headers.Get("User-Agent"))
	require.Equal(t, "next-api", headers.Get("originator"))
	require.Equal(t, openAIWSBetaV2Value, headers.Get("OpenAI-Beta"))
	require.Equal(t, "session-1", headers.Get("session_id"))

	oauthAccount := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"chatgpt_account_id": "chatgpt-account-1",
		},
	}
	headers, err = svc.buildNativeOpenAIResponsesWebSocketHeaders(context.Background(), oauthAccount, "oauth-token", "session-2")
	require.NoError(t, err)
	require.Equal(t, "Bearer oauth-token", headers.Get("Authorization"))
	require.Equal(t, "chatgpt-account-1", headers.Get("chatgpt-account-id"))
	require.Equal(t, "next-api", headers.Get("originator"))
	require.Equal(t, "session-2", headers.Get("session_id"))
}

func TestNativeOpenAIResponsesWebSocketAggregateRejectsAliasesAndDuplicateTerminals(t *testing.T) {
	aggregate := &nativeOpenAIResponsesWebSocketAggregate{seenResponseIDs: make(map[string]struct{})}
	aliasUsage := []byte(`{"type":"response.done","response":{"id":"resp_alias","usage":{"prompt_tokens":3,"completion_tokens":2}}}`)
	require.ErrorContains(t, aggregate.observe(coderws.MessageText, aliasUsage), "omitted input_tokens or output_tokens")

	terminal := []byte(`{"type":"response.completed","response":{"id":"resp_exact","usage":{"input_tokens":3,"output_tokens":2},"output":[{"type":"image_generation_call","result":"same"},{"type":"image_generation_call","result":"same"}]}}`)
	require.NoError(t, aggregate.observe(coderws.MessageText, terminal))
	require.Equal(t, 2, aggregate.imageCount)
	require.Empty(t, aggregate.imageOutputSizes)
	require.ErrorContains(t, aggregate.observe(coderws.MessageText, terminal), "multiple terminal events")
}

func TestNativeOpenAIResponsesWebSocketDialFailureRemainsPreCommit(t *testing.T) {
	svc := &OpenAIGatewayService{
		cfg: &config.Config{},
		openaiWSPassthroughDialer: openAIResponsesFailingDialer{
			err: errors.New("upstream unavailable"),
		},
	}
	account := &Account{
		ID:          302,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-upstream"},
		Extra:       map[string]any{"responses_websockets_v2_enabled": true},
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	recorder := httptest.NewRecorder()

	result, err := svc.ForwardResponsesWebSocket(context.Background(), recorder, req, account, "gpt-5.4", "session-technical")
	require.Nil(t, result)
	var failover *UpstreamFailoverError
	require.ErrorAs(t, err, &failover)
	require.False(t, recorder.Flushed)
	require.Equal(t, http.StatusOK, recorder.Code)
}

type openAIResponsesFailingDialer struct {
	err error
}

func (d openAIResponsesFailingDialer) Dial(context.Context, string, http.Header, string) (openAIWSClientConn, int, http.Header, error) {
	return nil, http.StatusServiceUnavailable, nil, d.err
}

var _ openAIWSClientDialer = (*nativeOpenAIResponsesTestDialer)(nil)
var _ openAIWSClientConn = (*nativeOpenAIResponsesTestConn)(nil)
var _ nativeOpenAIResponsesFrameConn = (*nativeOpenAIResponsesTestConn)(nil)
