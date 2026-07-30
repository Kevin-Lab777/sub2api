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
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func nativeOpenAIChatSubscriptionAccount() *Account {
	return &Account{
		ID:       601,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"auth_mode":          OpenAIAuthModePersonalAccessToken,
			"access_token":       "oauth-subscription-token",
			"chatgpt_account_id": "chatgpt-account-601",
			"model_mapping": map[string]any{
				"chat-alias": "gpt-5.4-upstream",
			},
		},
	}
}

func TestParseStrictNativeOpenAIChatRequestRejectsLossyShapes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
	}{
		{name: "unknown stop", body: `{"model":"gpt-5.4","messages":[{"role":"user","content":"hi"}],"stop":"END"}`},
		{name: "sampling field", body: `{"model":"gpt-5.4","messages":[{"role":"user","content":"hi"}],"temperature":0.3}`},
		{name: "developer role", body: `{"model":"gpt-5.4","messages":[{"role":"developer","content":"hi"}]}`},
		{name: "assistant structured content", body: `{"model":"gpt-5.4","messages":[{"role":"assistant","content":[{"type":"text","text":"hi"}]}]}`},
		{name: "empty assistant content", body: `{"model":"gpt-5.4","messages":[{"role":"assistant","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{}"}}]}]}`},
		{name: "null parallel tools", body: `{"model":"gpt-5.4","messages":[{"role":"user","content":"hi"}],"parallel_tool_calls":null}`},
		{name: "small token limit", body: `{"model":"gpt-5.4","messages":[{"role":"user","content":"hi"}],"max_completion_tokens":8}`},
		{name: "ambiguous limits", body: `{"model":"gpt-5.4","messages":[{"role":"user","content":"hi"}],"max_tokens":32,"max_completion_tokens":32}`},
		{name: "unsupported tool", body: `{"model":"gpt-5.4","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"web_search"}]}`},
		{name: "empty base64 image", body: `{"model":"gpt-5.4","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,"}}]}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parseStrictNativeOpenAIChatRequest([]byte(tt.body))
			require.Error(t, err)
		})
	}
}

func TestBuildNativeOpenAIChatSubscriptionRequestConvertsValidatedProtocol(t *testing.T) {
	body := []byte(`{
		"model":"chat-alias",
		"messages":[
			{"role":"system","content":"be concise"},
			{"role":"user","content":[{"type":"text","text":"hello"},{"type":"image_url","image_url":{"url":"data:image/png;base64,abc123","detail":"high"}}]},
			{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"q\":\"x\"}"}}]},
			{"role":"tool","tool_call_id":"call_1","content":"result"}
		],
		"max_completion_tokens":128,
		"stream":true,
		"stream_options":{"include_usage":true},
		"tools":[{"type":"function","function":{"name":"lookup","description":"lookup","parameters":{"type":"object"},"strict":true}}],
		"parallel_tool_calls":false,
		"tool_choice":"auto",
		"reasoning_effort":"high",
		"service_tier":"priority"
	}`)
	request, err := buildNativeOpenAIChatSubscriptionRequest(nativeOpenAIChatSubscriptionAccount(), body)
	require.NoError(t, err)
	require.Equal(t, "chat-alias", request.originalModel)
	require.Equal(t, "gpt-5.4-upstream", request.upstreamModel)
	require.True(t, request.clientStream)
	require.True(t, request.clientIncludeUsage)
	require.Equal(t, "gpt-5.4-upstream", gjson.GetBytes(request.responsesBody, "model").String())
	require.True(t, gjson.GetBytes(request.responsesBody, "stream").Bool())
	require.False(t, gjson.GetBytes(request.responsesBody, "store").Bool())
	require.Equal(t, int64(128), gjson.GetBytes(request.responsesBody, "max_output_tokens").Int())
	require.Equal(t, "high", gjson.GetBytes(request.responsesBody, "reasoning.effort").String())
	require.Equal(t, "priority", gjson.GetBytes(request.responsesBody, "service_tier").String())
	require.Len(t, gjson.GetBytes(request.responsesBody, "input").Array(), 4)
	require.Equal(t, "high", gjson.GetBytes(request.responsesBody, "input.1.content.1.detail").String())
	require.False(t, gjson.GetBytes(request.responsesBody, "include").Exists())
}

func TestConvertStrictNativeOpenAIResponseToChatRejectsFabricatedMetadataAndUnrepresentableState(t *testing.T) {
	usage := OpenAIUsage{InputTokens: 1, OutputTokens: 1}
	_, _, _, err := convertStrictNativeOpenAIResponseToChat(
		[]byte(`{"id":"resp_1","model":"gpt-5.4","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`),
		"gpt-5.4",
		"gpt-5.4",
		usage,
	)
	require.ErrorContains(t, err, "created_at")

	_, _, _, err = convertStrictNativeOpenAIResponseToChat(
		[]byte(`{"id":"resp_1","created_at":1784000000,"model":"gpt-5.4","status":"completed","output":[{"type":"reasoning","encrypted_content":"secret","summary":[]}],"usage":{"input_tokens":1,"output_tokens":1}}`),
		"gpt-5.4",
		"gpt-5.4",
		usage,
	)
	require.ErrorContains(t, err, "encrypted reasoning")

	_, _, _, err = convertStrictNativeOpenAIResponseToChat(
		[]byte(`{"id":"resp_1","created_at":1784000000,"model":"gpt-5.5","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`),
		"chat-alias",
		"gpt-5.4",
		usage,
	)
	require.ErrorContains(t, err, "model mismatch")
}

func TestForwardChatCompletionsSubscriptionConvertsTerminalResponseWithoutFabrication(t *testing.T) {
	createdAt := int64(1_784_000_123)
	streamBody := strings.Join([]string{
		`event: response.completed`,
		`data: {"type":"response.completed","response":{"id":"resp_subscription_1","object":"response","created_at":1784000123,"model":"gpt-5.4-upstream","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]},{"type":"web_search_call"}],"usage":{"input_tokens":11,"output_tokens":4,"input_tokens_details":{"cached_tokens":3}}}}`,
		``,
	}, "\n")
	upstream := &openAIResponsesHTTPStub{
		status: http.StatusOK,
		header: http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"upstream-sub-1"}},
		body:   streamBody,
	}
	svc := &OpenAIGatewayService{
		httpUpstream:        upstream,
		cfg:                 &config.Config{},
		openAITokenProvider: NewOpenAITokenProvider(nil, nil, nil),
	}
	body := []byte(`{"model":"chat-alias","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	recorder := httptest.NewRecorder()

	result, err := svc.ForwardChatCompletionsExchange(context.Background(), gatewaytransport.NewHTTPExchange(recorder, req), nativeOpenAIChatSubscriptionAccount(), body)
	require.NoError(t, err)
	require.Equal(t, "resp_subscription_1", gjson.Get(recorder.Body.String(), "id").String())
	require.Equal(t, createdAt, gjson.Get(recorder.Body.String(), "created").Int())
	require.Equal(t, "chat-alias", gjson.Get(recorder.Body.String(), "model").String())
	require.Equal(t, "hello", gjson.Get(recorder.Body.String(), "choices.0.message.content").String())
	require.Equal(t, int64(11), gjson.Get(recorder.Body.String(), "usage.prompt_tokens").Int())
	require.Equal(t, int64(4), gjson.Get(recorder.Body.String(), "usage.completion_tokens").Int())
	require.Equal(t, 11, result.Usage.InputTokens)
	require.Equal(t, 4, result.Usage.OutputTokens)
	require.Equal(t, 3, result.Usage.CacheReadInputTokens)
	require.Equal(t, 1, result.WebSearchCalls)
	require.Equal(t, nativeOpenAIResponsesEndpoint, result.UpstreamEndpoint)
	require.Equal(t, "https://chatgpt.com/backend-api/codex/responses", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer oauth-subscription-token", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, "chatgpt-account-601", upstream.lastReq.Header.Get("chatgpt-account-id"))
	require.Equal(t, "next-api", upstream.lastReq.Header.Get("originator"))
	posted, err := io.ReadAll(upstream.lastReq.Body)
	require.NoError(t, err)
	require.Equal(t, "gpt-5.4-upstream", gjson.GetBytes(posted, "model").String())
	require.True(t, gjson.GetBytes(posted, "stream").Bool())
}

func TestForwardChatCompletionsSubscriptionStreamsDeterministicChatEvents(t *testing.T) {
	streamBody := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_stream_sub","object":"response","created_at":1784000456,"model":"gpt-5.4-upstream","status":"in_progress","output":[]}}`,
		``,
		`data: {"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"hi"}`,
		``,
		`data: {"type":"response.completed","response":{"id":"resp_stream_sub","object":"response","created_at":1784000456,"model":"gpt-5.4-upstream","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hi"}]}],"usage":{"input_tokens":7,"output_tokens":2,"input_tokens_details":{"cached_tokens":1}}}}`,
		``,
	}, "\n")
	upstream := &openAIResponsesHTTPStub{
		status: http.StatusOK,
		header: http.Header{"Content-Type": []string{"text/event-stream"}},
		body:   streamBody,
	}
	svc := &OpenAIGatewayService{
		httpUpstream:        upstream,
		cfg:                 &config.Config{},
		openAITokenProvider: NewOpenAITokenProvider(nil, nil, nil),
	}
	body := []byte(`{"model":"chat-alias","messages":[{"role":"user","content":"hello"}],"stream":true,"stream_options":{"include_usage":true}}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	recorder := httptest.NewRecorder()

	result, err := svc.ForwardChatCompletionsExchange(context.Background(), gatewaytransport.NewHTTPExchange(recorder, req), nativeOpenAIChatSubscriptionAccount(), body)
	require.NoError(t, err)
	require.Contains(t, recorder.Body.String(), `"role":"assistant"`)
	require.Contains(t, recorder.Body.String(), `"content":"hi"`)
	require.Contains(t, recorder.Body.String(), `"finish_reason":"stop"`)
	require.Contains(t, recorder.Body.String(), `"prompt_tokens":7`)
	require.True(t, strings.HasSuffix(recorder.Body.String(), "data: [DONE]\n\n"))
	require.Equal(t, 7, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
	require.Equal(t, 1, result.Usage.CacheReadInputTokens)
	require.True(t, result.Stream)
	require.NotNil(t, result.FirstTokenMs)
}

func TestForwardChatCompletionsSubscriptionInvalidFirstEventCanFailOverBeforeCommit(t *testing.T) {
	upstream := &openAIResponsesHTTPStub{
		status: http.StatusOK,
		header: http.Header{"Content-Type": []string{"text/event-stream"}},
		body:   "data: {\"type\":\"response.output_text.delta\",\"delta\":\"orphan\"}\n\n",
	}
	svc := &OpenAIGatewayService{
		httpUpstream:        upstream,
		cfg:                 &config.Config{},
		openAITokenProvider: NewOpenAITokenProvider(nil, nil, nil),
	}
	body := []byte(`{"model":"chat-alias","messages":[{"role":"user","content":"hello"}],"stream":true}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	recorder := httptest.NewRecorder()
	result, err := svc.ForwardChatCompletionsExchange(context.Background(), gatewaytransport.NewHTTPExchange(recorder, req), nativeOpenAIChatSubscriptionAccount(), body)
	require.Nil(t, result)
	var failover *UpstreamFailoverError
	require.ErrorAs(t, err, &failover)
	require.Empty(t, recorder.Body.String())
}

func TestForwardChatCompletionsSubscriptionRejectsTerminalMetadataMismatchAfterCommit(t *testing.T) {
	streamBody := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_created","object":"response","created_at":1784000456,"model":"gpt-5.4-upstream","status":"in_progress","output":[]}}`,
		``,
		`data: {"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"hi"}`,
		``,
		`data: {"type":"response.completed","response":{"id":"resp_changed","object":"response","created_at":1784000456,"model":"gpt-5.4-upstream","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hi"}]}],"usage":{"input_tokens":7,"output_tokens":2}}}`,
		``,
	}, "\n")
	upstream := &openAIResponsesHTTPStub{status: http.StatusOK, header: http.Header{"Content-Type": []string{"text/event-stream"}}, body: streamBody}
	svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}, openAITokenProvider: NewOpenAITokenProvider(nil, nil, nil)}
	body := []byte(`{"model":"chat-alias","messages":[{"role":"user","content":"hello"}],"stream":true}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	recorder := httptest.NewRecorder()

	result, err := svc.ForwardChatCompletionsExchange(context.Background(), gatewaytransport.NewHTTPExchange(recorder, req), nativeOpenAIChatSubscriptionAccount(), body)
	require.NotNil(t, result)
	require.ErrorContains(t, err, "metadata does not match")
	require.Contains(t, recorder.Body.String(), `"content":"hi"`)
	require.NotContains(t, recorder.Body.String(), `"finish_reason":"stop"`)
	require.NotContains(t, recorder.Body.String(), "[DONE]")
}

func TestForwardChatCompletionsSubscriptionFailedResponseDoesNotSwitchAccounts(t *testing.T) {
	upstream := &openAIResponsesHTTPStub{
		status: http.StatusOK,
		header: http.Header{"Content-Type": []string{"text/event-stream"}},
		body:   "data: {\"type\":\"response.failed\",\"response\":{\"id\":\"resp_failed\",\"status\":\"failed\"}}\n\n",
	}
	svc := &OpenAIGatewayService{
		httpUpstream:        upstream,
		cfg:                 &config.Config{},
		openAITokenProvider: NewOpenAITokenProvider(nil, nil, nil),
	}
	body := []byte(`{"model":"chat-alias","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	result, err := svc.ForwardChatCompletionsExchange(context.Background(), gatewaytransport.NewHTTPExchange(httptest.NewRecorder(), req), nativeOpenAIChatSubscriptionAccount(), body)
	require.Nil(t, result)
	var failover *UpstreamFailoverError
	require.False(t, errors.As(err, &failover))
	var semantic *nativeOpenAIChatSemanticError
	require.ErrorAs(t, err, &semantic)
}
