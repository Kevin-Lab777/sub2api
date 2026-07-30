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
)

func TestParseOpenAIChatCompletionsRequestIsStrict(t *testing.T) {
	t.Parallel()
	for _, body := range []string{
		`{"model":"gpt-5.4","stream":null}`,
		`{"model":"gpt-5.4","stream":1}`,
		`{"model":" gpt-5.4"}`,
		`[]`,
		`not-json`,
	} {
		_, _, err := ParseOpenAIChatCompletionsRequest([]byte(body))
		require.Error(t, err, body)
	}
	model, stream, err := ParseOpenAIChatCompletionsRequest([]byte(`{"model":"gpt-5.4","stream":true}`))
	require.NoError(t, err)
	require.Equal(t, "gpt-5.4", model)
	require.True(t, stream)
}

func TestPrepareNativeOpenAIChatCompletionsBodyOnlyMapsModelAndRequestsUsage(t *testing.T) {
	body := []byte(`{"model":"alias","stream":true,"stream_options":{"include_usage":false,"future":"preserve"},"messages":[{"role":"user","content":"hello"}]}`)
	updated, err := prepareNativeOpenAIChatCompletionsBody(body, "gpt-5.4", true)
	require.NoError(t, err)
	require.JSONEq(t, `{"model":"gpt-5.4","stream":true,"stream_options":{"include_usage":true,"future":"preserve"},"messages":[{"role":"user","content":"hello"}]}`, string(updated))

	_, err = prepareNativeOpenAIChatCompletionsBody([]byte(`{"model":"alias","stream":true,"stream_options":null}`), "gpt-5.4", true)
	require.ErrorContains(t, err, "stream_options must be an object")
	_, err = prepareNativeOpenAIChatCompletionsBody([]byte(`{"model":"alias","stream":true,"stream_options":{"include_usage":null}}`), "gpt-5.4", true)
	require.ErrorContains(t, err, "include_usage must be a boolean")
}

func TestExtractNativeOpenAIChatUsageRejectsAliasesAmbiguityAndNegatives(t *testing.T) {
	_, found, err := extractNativeOpenAIChatUsage([]byte(`{"usage":{"input_tokens":3,"output_tokens":2}}`))
	require.ErrorContains(t, err, "omitted prompt_tokens or completion_tokens")
	require.False(t, found)

	_, found, err = extractNativeOpenAIChatUsage([]byte(`{"usage":{"prompt_tokens":-1,"completion_tokens":2}}`))
	require.ErrorContains(t, err, "negative token count")
	require.False(t, found)

	_, found, err = extractNativeOpenAIChatUsage([]byte(`{"usage":{"prompt_tokens":3,"completion_tokens":2,"prompt_tokens_details":{"cache_creation_tokens":0,"cache_write_tokens":1}}}`))
	require.ErrorContains(t, err, "ambiguous cache creation")
	require.False(t, found)
}

func TestForwardChatCompletionsExchangeAPIKeyPreservesUnaryProtocol(t *testing.T) {
	upstream := &openAIResponsesHTTPStub{
		status: http.StatusOK,
		header: http.Header{
			"Content-Type": []string{"application/json"},
			"X-Request-Id": []string{"chat-request-1"},
		},
		body: `{"id":"chatcmpl_1","object":"chat.completion","model":"gpt-5.4-upstream","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":4,"prompt_tokens_details":{"cached_tokens":3}}}`,
	}
	svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}}
	account := &Account{
		ID:       401,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "sk-chat",
			"model_mapping": map[string]any{
				"chat-alias": "gpt-5.4-upstream",
			},
		},
	}
	body := []byte(`{"model":"chat-alias","messages":[{"role":"user","content":"hello"}],"stream":false,"future_field":{"keep":true}}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	recorder := httptest.NewRecorder()

	result, err := svc.ForwardChatCompletionsExchange(context.Background(), gatewaytransport.NewHTTPExchange(recorder, req), account, body)
	require.NoError(t, err)
	require.JSONEq(t, upstream.body, recorder.Body.String())
	require.Equal(t, 12, result.Usage.InputTokens)
	require.Equal(t, 4, result.Usage.OutputTokens)
	require.Equal(t, 3, result.Usage.CacheReadInputTokens)
	require.Equal(t, "chat-alias", result.Model)
	require.Equal(t, "gpt-5.4-upstream", result.UpstreamModel)
	require.Equal(t, nativeOpenAIChatCompletionsEndpoint, result.UpstreamEndpoint)
	require.Equal(t, "https://api.openai.com/v1/chat/completions", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer sk-chat", upstream.lastReq.Header.Get("Authorization"))
	posted, err := io.ReadAll(upstream.lastReq.Body)
	require.NoError(t, err)
	require.JSONEq(t, `{"model":"gpt-5.4-upstream","messages":[{"role":"user","content":"hello"}],"stream":false,"future_field":{"keep":true}}`, string(posted))
}

func TestForwardChatCompletionsExchangeStreamsRawFramesAndExactUsage(t *testing.T) {
	streamBody := strings.Join([]string{
		`data: {"id":"chatcmpl_stream","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":null}]}`,
		``,
		`data: {"id":"chatcmpl_stream","object":"chat.completion.chunk","model":"gpt-5.4","choices":[],"usage":{"prompt_tokens":8,"completion_tokens":3,"prompt_tokens_details":{"cached_tokens":2}}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	upstream := &openAIResponsesHTTPStub{
		status: http.StatusOK,
		header: http.Header{"Content-Type": []string{"text/event-stream"}},
		body:   streamBody,
	}
	svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}}
	account := &Account{ID: 402, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-chat"}}
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":true}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	recorder := httptest.NewRecorder()

	result, err := svc.ForwardChatCompletionsExchange(context.Background(), gatewaytransport.NewHTTPExchange(recorder, req), account, body)
	require.NoError(t, err)
	require.Equal(t, streamBody, recorder.Body.String())
	require.Equal(t, 8, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.OutputTokens)
	require.Equal(t, 2, result.Usage.CacheReadInputTokens)
	require.True(t, result.Stream)
	require.NotNil(t, result.FirstTokenMs)
	posted, err := io.ReadAll(upstream.lastReq.Body)
	require.NoError(t, err)
	require.JSONEq(t, `{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":true,"stream_options":{"include_usage":true}}`, string(posted))
}

func TestForwardChatCompletionsExchangeStreamingMissingUsageStaysVisibleAfterCommit(t *testing.T) {
	streamBody := "data: {\"id\":\"chatcmpl_stream\",\"choices\":[]}\n\ndata: [DONE]\n\n"
	upstream := &openAIResponsesHTTPStub{
		status: http.StatusOK,
		header: http.Header{"Content-Type": []string{"text/event-stream"}},
		body:   streamBody,
	}
	svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}}
	account := &Account{ID: 403, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-chat"}}
	body := []byte(`{"model":"gpt-5.4","messages":[],"stream":true}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	recorder := httptest.NewRecorder()

	result, err := svc.ForwardChatCompletionsExchange(context.Background(), gatewaytransport.NewHTTPExchange(recorder, req), account, body)
	require.ErrorContains(t, err, "without [DONE] and exact usage")
	require.NotNil(t, result)
	require.Equal(t, streamBody, recorder.Body.String())
}

func TestForwardChatCompletionsExchangeInvalidUnaryPayloadCanFailOverBeforeCommit(t *testing.T) {
	upstream := &openAIResponsesHTTPStub{
		status: http.StatusOK,
		header: http.Header{"Content-Type": []string{"application/json"}},
		body:   `not-json`,
	}
	svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}}
	account := &Account{ID: 404, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-chat"}}
	body := []byte(`{"model":"gpt-5.4","messages":[]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	recorder := httptest.NewRecorder()

	result, err := svc.ForwardChatCompletionsExchange(context.Background(), gatewaytransport.NewHTTPExchange(recorder, req), account, body)
	require.Nil(t, result)
	var failover *UpstreamFailoverError
	require.True(t, errors.As(err, &failover))
	require.Empty(t, recorder.Body.String())
}
