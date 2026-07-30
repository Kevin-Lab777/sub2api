package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/gatewaytransport"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type openAIResponsesHTTPStub struct {
	status  int
	header  http.Header
	body    string
	err     error
	lastReq *http.Request
}

func TestParseOpenAIResponsesRequestRejectsNonExactProtocolFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{name: "null stream", body: `{"model":"gpt-5.4","stream":null}`},
		{name: "numeric stream", body: `{"model":"gpt-5.4","stream":1}`},
		{name: "model whitespace", body: `{"model":" gpt-5.4","stream":false}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := ParseOpenAIResponsesRequest([]byte(tt.body))
			require.Error(t, err)
		})
	}
}

func TestResolveExactOpenAICompactAccountModelUsesSequentialExactMappings(t *testing.T) {
	t.Parallel()

	account := &Account{ID: 213, Credentials: map[string]any{
		"model_mapping": map[string]any{
			"customer-alias": "gpt-5.4",
		},
		"compact_model_mapping": map[string]any{
			"gpt-5.4": "gpt-5.4-compact",
			"gpt-*":   "wildcard-must-not-run",
		},
	}}
	mapped, err := resolveExactOpenAICompactAccountModel(account, "customer-alias")
	require.NoError(t, err)
	require.Equal(t, "gpt-5.4-compact", mapped)

	account.Credentials["compact_model_mapping"] = map[string]any{"gpt-*": "wildcard-must-not-run"}
	mapped, err = resolveExactOpenAICompactAccountModel(account, "customer-alias")
	require.NoError(t, err)
	require.Equal(t, "gpt-5.4", mapped)
}

func TestExtractNativeOpenAIResponsesUsageRejectsLegacyAliasesAndNegativeValues(t *testing.T) {
	t.Parallel()

	_, ok, err := extractNativeOpenAIResponsesUsage([]byte(`{"usage":{"prompt_tokens":4,"completion_tokens":2}}`))
	require.ErrorContains(t, err, "omitted input_tokens or output_tokens")
	require.False(t, ok)

	_, ok, err = extractNativeOpenAIResponsesUsage([]byte(`{"usage":{"input_tokens":-1,"output_tokens":2}}`))
	require.ErrorContains(t, err, "negative token count")
	require.False(t, ok)
}

func TestNativeOpenAIResponsesOutputTelemetryCountsRawOutputItems(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"output":[
			{"type":"image_generation_call","result":"same-image","size":"1024x1024"},
			{"type":"image_generation_call","result":"same-image","size":"1024x1024"},
			{"type":"web_search_call","id":"search_1"},
			{"type":"message","content":[]}
		]
	}`)
	imageCount, sizes, webSearchCalls := nativeOpenAIResponsesOutputTelemetry(body)
	require.Equal(t, 2, imageCount)
	require.Equal(t, []string{"1024x1024", "1024x1024"}, sizes)
	require.Equal(t, 1, webSearchCalls)
}

func TestClassifyNativeOpenAIResponsesContentTypeIsExact(t *testing.T) {
	t.Parallel()

	kind, err := classifyNativeOpenAIResponsesContentType("application/problem+json; charset=utf-8")
	require.NoError(t, err)
	require.Equal(t, nativeOpenAIResponsesContentJSON, kind)

	kind, err = classifyNativeOpenAIResponsesContentType(`application/json; profile="text/event-stream"`)
	require.NoError(t, err)
	require.Equal(t, nativeOpenAIResponsesContentJSON, kind)

	_, err = classifyNativeOpenAIResponsesContentType("text/plain")
	require.ErrorContains(t, err, "unsupported Content-Type")
}

func (s *openAIResponsesHTTPStub) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	s.lastReq = req
	if s.err != nil {
		return nil, s.err
	}
	return &http.Response{
		StatusCode: s.status,
		Header:     s.header.Clone(),
		Body:       io.NopCloser(strings.NewReader(s.body)),
	}, nil
}

func (s *openAIResponsesHTTPStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, concurrency)
}

func TestOpenAIForwardResponsesExchangeAPIKeyPreservesPayloadAndMeasuresUsage(t *testing.T) {
	upstream := &openAIResponsesHTTPStub{
		status: http.StatusOK,
		header: http.Header{
			"Content-Type": []string{"application/json"},
			"X-Request-Id": []string{"openai-req-1"},
		},
		body: `{"id":"resp_1","object":"response","model":"gpt-5.4-2026-07-01","status":"completed","output":[],"usage":{"input_tokens":13,"output_tokens":5,"input_tokens_details":{"cached_tokens":4}}}`,
	}
	svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}}
	account := &Account{
		ID:       201,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "sk-test",
			"model_mapping": map[string]any{
				"gpt-5.4": "gpt-5.4-2026-07-01",
			},
		},
	}
	body := []byte(`{"model":"gpt-5.4","input":"hello","stream":false}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	recorder := httptest.NewRecorder()

	result, err := svc.ForwardResponsesExchange(context.Background(), gatewaytransport.NewHTTPExchange(recorder, req), account, body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, upstream.body, recorder.Body.String())
	require.Equal(t, 13, result.Usage.InputTokens)
	require.Equal(t, 5, result.Usage.OutputTokens)
	require.Equal(t, 4, result.Usage.CacheReadInputTokens)
	require.Equal(t, "gpt-5.4", result.Model)
	require.Equal(t, "gpt-5.4-2026-07-01", result.UpstreamModel)
	require.Equal(t, "Bearer sk-test", upstream.lastReq.Header.Get("Authorization"))

	posted, err := io.ReadAll(upstream.lastReq.Body)
	require.NoError(t, err)
	require.JSONEq(t, `{"model":"gpt-5.4-2026-07-01","input":"hello","stream":false}`, string(posted))
}

func TestOpenAIForwardResponsesExchangeStreamsWithoutSyntheticEvents(t *testing.T) {
	streamBody := strings.Join([]string{
		`event: response.created`,
		`data: {"type":"response.created","response":{"id":"resp_stream"}}`,
		``,
		`event: response.completed`,
		`data: {"type":"response.completed","response":{"id":"resp_stream","model":"gpt-5.4","status":"completed","output":[],"usage":{"input_tokens":8,"output_tokens":3,"input_tokens_details":{"cached_tokens":2}}}}`,
		``,
	}, "\n")
	upstream := &openAIResponsesHTTPStub{
		status: http.StatusOK,
		header: http.Header{"Content-Type": []string{"text/event-stream"}},
		body:   streamBody,
	}
	svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}}
	account := &Account{ID: 202, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-stream"}}
	body := []byte(`{"model":"gpt-5.4","input":"hello","stream":true}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	recorder := httptest.NewRecorder()

	result, err := svc.ForwardResponsesExchange(context.Background(), gatewaytransport.NewHTTPExchange(recorder, req), account, body)
	require.NoError(t, err)
	require.Equal(t, streamBody, recorder.Body.String())
	require.Equal(t, 8, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.OutputTokens)
	require.Equal(t, 2, result.Usage.CacheReadInputTokens)
	require.Equal(t, "resp_stream", result.ResponseID)
	require.NotNil(t, result.FirstTokenMs)
	require.True(t, result.Stream)
}

func TestOpenAIForwardResponsesExchangeOAuthUsesSubscriptionTransportHeaders(t *testing.T) {
	upstream := &openAIResponsesHTTPStub{
		status: http.StatusOK,
		header: http.Header{"Content-Type": []string{"application/json"}},
		body:   `{"id":"resp_oauth","object":"response","model":"gpt-5.4","status":"completed","output":[],"usage":{"input_tokens":4,"output_tokens":1}}`,
	}
	provider := NewOpenAITokenProvider(nil, nil, nil)
	svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}, openAITokenProvider: provider}
	account := &Account{
		ID:       206,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"auth_mode":          OpenAIAuthModePersonalAccessToken,
			"access_token":       "oauth-test-token",
			"chatgpt_account_id": "chatgpt-account-1",
		},
	}
	body := []byte(`{"model":"gpt-5.4","input":"hello","stream":false}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	req.Header.Set("session_id", "session-oauth")
	req.Header.Set("originator", "codex_cli_rs")
	recorder := httptest.NewRecorder()

	result, err := svc.ForwardResponsesExchange(context.Background(), gatewaytransport.NewHTTPExchange(recorder, req), account, body)
	require.NoError(t, err)
	require.Equal(t, 4, result.Usage.InputTokens)
	require.Equal(t, "https://chatgpt.com/backend-api/codex/responses", upstream.lastReq.URL.String())
	require.Equal(t, "chatgpt.com", upstream.lastReq.Host)
	require.Equal(t, "Bearer oauth-test-token", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, "chatgpt-account-1", upstream.lastReq.Header.Get("chatgpt-account-id"))
	require.Equal(t, "responses=experimental", upstream.lastReq.Header.Get("OpenAI-Beta"))
	require.Equal(t, "next-api", upstream.lastReq.Header.Get("originator"))
	require.Equal(t, "session-oauth", upstream.lastReq.Header.Get("session_id"))
}

func TestOpenAIForwardResponsesCompactExchangePreservesRequestFields(t *testing.T) {
	t.Parallel()

	upstream := &openAIResponsesHTTPStub{
		status: http.StatusOK,
		header: http.Header{"Content-Type": []string{"application/json"}},
		body:   `{"id":"cmp_native","model":"gpt-5.4-compact","output":[{"type":"compaction","encrypted_content":"raw"}],"usage":{"input_tokens":9,"output_tokens":2}}`,
	}
	svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}}
	account := &Account{
		ID:       211,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "sk-compact",
			"base_url": "https://gateway.example/v1",
			"compact_model_mapping": map[string]any{
				"gpt-5.4": "gpt-5.4-compact",
			},
		},
	}
	body := []byte(`{"model":"gpt-5.4","stream":true,"store":true,"prompt_cache_key":"client-key","input":"compact me"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses/compact", bytes.NewReader(body))
	recorder := httptest.NewRecorder()

	result, err := svc.ForwardResponsesCompactExchange(context.Background(), gatewaytransport.NewHTTPExchange(recorder, req), account, body)
	require.NoError(t, err)
	require.Equal(t, nativeOpenAIResponsesCompactEndpoint, result.UpstreamEndpoint)
	require.Equal(t, "gpt-5.4-compact", result.UpstreamModel)
	require.JSONEq(t, upstream.body, recorder.Body.String())
	require.Equal(t, "https://gateway.example/v1/responses/compact", upstream.lastReq.URL.String())
	require.Equal(t, "application/json", upstream.lastReq.Header.Get("Accept"))

	posted, err := io.ReadAll(upstream.lastReq.Body)
	require.NoError(t, err)
	require.JSONEq(t, `{"model":"gpt-5.4-compact","stream":true,"store":true,"prompt_cache_key":"client-key","input":"compact me"}`, string(posted))
}

func TestOpenAIForwardResponsesCompactExchangeRejectsSSEUpstream(t *testing.T) {
	t.Parallel()

	upstream := &openAIResponsesHTTPStub{
		status: http.StatusOK,
		header: http.Header{"Content-Type": []string{"text/event-stream"}},
		body:   "data: {\"type\":\"response.completed\"}\n\n",
	}
	svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}}
	account := &Account{ID: 212, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-compact-sse"}}
	body := []byte(`{"model":"gpt-5.4","input":"compact me"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses/compact", bytes.NewReader(body))
	recorder := httptest.NewRecorder()

	result, err := svc.ForwardResponsesCompactExchange(context.Background(), gatewaytransport.NewHTTPExchange(recorder, req), account, body)
	require.ErrorContains(t, err, "non-JSON upstream content type")
	require.Nil(t, result)
	require.Empty(t, recorder.Body.String())
}

func TestOpenAIForwardResponsesExchangeRejectsWildcardOnlyModelMapping(t *testing.T) {
	t.Parallel()

	upstream := &openAIResponsesHTTPStub{
		status: http.StatusOK,
		header: http.Header{"Content-Type": []string{"application/json"}},
		body:   `{"usage":{"input_tokens":1,"output_tokens":1}}`,
	}
	svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}}
	account := &Account{
		ID:       208,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "sk-wildcard",
			"model_mapping": map[string]any{
				"gpt-*": "gpt-5.4-upstream",
			},
		},
	}
	body := []byte(`{"model":"gpt-5.4","input":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))

	result, err := svc.ForwardResponsesExchange(context.Background(), gatewaytransport.NewHTTPExchange(httptest.NewRecorder(), req), account, body)
	require.ErrorContains(t, err, `no exact mapping for model "gpt-5.4"`)
	require.Nil(t, result)
	require.Nil(t, upstream.lastReq)
}

func TestOpenAIForwardResponsesExchangeCollectsOnlyTerminalResponse(t *testing.T) {
	streamBody := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_collected\",\"model\":\"gpt-5.4\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":6,\"output_tokens\":2}}}\n\n"
	upstream := &openAIResponsesHTTPStub{
		status: http.StatusOK,
		header: http.Header{"Content-Type": []string{"text/event-stream"}},
		body:   streamBody,
	}
	svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}}
	account := &Account{ID: 203, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-collect"}}
	body := []byte(`{"model":"gpt-5.4","input":"hello","stream":false}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	recorder := httptest.NewRecorder()

	result, err := svc.ForwardResponsesExchange(context.Background(), gatewaytransport.NewHTTPExchange(recorder, req), account, body)
	require.NoError(t, err)
	require.JSONEq(t, `{"id":"resp_collected","model":"gpt-5.4","status":"completed","output":[],"usage":{"input_tokens":6,"output_tokens":2}}`, recorder.Body.String())
	require.Equal(t, 6, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
}

func TestOpenAIForwardResponsesExchangeDoesNotFabricateMissingUsage(t *testing.T) {
	upstream := &openAIResponsesHTTPStub{
		status: http.StatusOK,
		header: http.Header{"Content-Type": []string{"application/json"}},
		body:   `{"id":"resp_no_usage","object":"response","status":"completed","output":[]}`,
	}
	svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}}
	account := &Account{ID: 204, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-no-usage"}}
	body := []byte(`{"model":"gpt-5.4","input":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	recorder := httptest.NewRecorder()

	result, err := svc.ForwardResponsesExchange(context.Background(), gatewaytransport.NewHTTPExchange(recorder, req), account, body)
	require.ErrorContains(t, err, "omitted usage")
	require.Nil(t, result)
	require.False(t, recorder.Result().Header.Get("Content-Type") != "" || recorder.Body.Len() > 0)
}

func TestOpenAIForwardResponsesExchangeDoesNotFabricateOversizeResponse(t *testing.T) {
	t.Parallel()

	upstream := &openAIResponsesHTTPStub{
		status: http.StatusOK,
		header: http.Header{"Content-Type": []string{"application/json"}},
		body:   `{"usage":{"input_tokens":1,"output_tokens":1}}`,
	}
	cfg := &config.Config{}
	cfg.Gateway.UpstreamResponseReadMaxBytes = 8
	svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: cfg}
	account := &Account{ID: 209, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-limit"}}
	body := []byte(`{"model":"gpt-5.4","input":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	recorder := httptest.NewRecorder()

	result, err := svc.ForwardResponsesExchange(context.Background(), gatewaytransport.NewHTTPExchange(recorder, req), account, body)
	require.ErrorIs(t, err, ErrUpstreamResponseBodyTooLarge)
	require.Nil(t, result)
	require.Empty(t, recorder.Body.String())
	require.Empty(t, recorder.Header().Get("Content-Type"))
}

func TestOpenAIForwardResponsesExchangeRejectsMalformedMultiDataEvent(t *testing.T) {
	upstream := &openAIResponsesHTTPStub{
		status: http.StatusOK,
		header: http.Header{"Content-Type": []string{"text/event-stream"}},
		body: "data: {\"type\":\"response.completed\"}\n" +
			"data: {\"type\":\"response.completed\"}\n\n",
	}
	svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}}
	account := &Account{ID: 207, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-malformed"}}
	body := []byte(`{"model":"gpt-5.4","input":"hello","stream":false}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	recorder := httptest.NewRecorder()

	result, err := svc.ForwardResponsesExchange(context.Background(), gatewaytransport.NewHTTPExchange(recorder, req), account, body)
	require.ErrorContains(t, err, "non-JSON SSE data")
	require.Nil(t, result)
	require.Empty(t, recorder.Body.String())
}

func TestOpenAIForwardResponsesExchangeUsesStatusOnlyFailoverPolicy(t *testing.T) {
	upstream := &openAIResponsesHTTPStub{
		status: http.StatusTooManyRequests,
		header: http.Header{"Content-Type": []string{"application/json"}},
		body:   `{"error":{"message":"quota"}}`,
	}
	svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}}
	account := &Account{ID: 205, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-rate"}}
	body := []byte(`{"model":"gpt-5.4","input":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	recorder := httptest.NewRecorder()

	result, err := svc.ForwardResponsesExchange(context.Background(), gatewaytransport.NewHTTPExchange(recorder, req), account, body)
	require.Nil(t, result)
	var failover *UpstreamFailoverError
	require.True(t, errors.As(err, &failover))
	require.Equal(t, http.StatusTooManyRequests, failover.StatusCode)
	require.Empty(t, recorder.Body.String())
}

func TestOpenAIForwardResponsesExchangePassesThroughRedirectStatusWithoutFailover(t *testing.T) {
	t.Parallel()

	upstream := &openAIResponsesHTTPStub{
		status: http.StatusTemporaryRedirect,
		header: http.Header{
			"Content-Type": []string{"application/json"},
			"Location":     []string{"https://api.example.invalid/v1/responses"},
		},
		body: `{"error":{"message":"redirect"}}`,
	}
	svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}}
	account := &Account{ID: 210, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-redirect"}}
	body := []byte(`{"model":"gpt-5.4","input":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	recorder := httptest.NewRecorder()

	result, err := svc.ForwardResponsesExchange(context.Background(), gatewaytransport.NewHTTPExchange(recorder, req), account, body)
	require.Error(t, err)
	require.Nil(t, result)
	var failover *UpstreamFailoverError
	require.False(t, errors.As(err, &failover))
	require.Equal(t, http.StatusTemporaryRedirect, recorder.Code)
	require.JSONEq(t, upstream.body, recorder.Body.String())
}
