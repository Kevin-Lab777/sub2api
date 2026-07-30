package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/gatewaytransport"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func nativeOpenAIEmbeddingsAPIKeyAccount() *Account {
	return &Account{
		ID:       701,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "sk-embeddings",
			"base_url": "https://embeddings.example/v1",
			"model_mapping": map[string]any{
				"embedding-alias": "text-embedding-3-large",
			},
		},
	}
}

func TestParseOpenAIEmbeddingsRequestRequiresExactModel(t *testing.T) {
	t.Parallel()

	for _, body := range []string{
		``,
		`[]`,
		`{"input":"hello"}`,
		`{"model":null,"input":"hello"}`,
		`{"model":" text-embedding-3-small","input":"hello"}`,
	} {
		_, err := ParseOpenAIEmbeddingsRequest([]byte(body))
		require.Error(t, err)
	}
	model, err := ParseOpenAIEmbeddingsRequest([]byte(`{"model":"text-embedding-3-small","input":"hello"}`))
	require.NoError(t, err)
	require.Equal(t, "text-embedding-3-small", model)
}

func TestExtractNativeOpenAIEmbeddingsUsageRejectsMissingAndContradictoryCounts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body string
		want string
	}{
		{body: `{"object":"list"}`, want: "omitted usage"},
		{body: `{"usage":{"total_tokens":3}}`, want: "omitted prompt_tokens or input_tokens"},
		{body: `{"usage":{"prompt_tokens":3,"input_tokens":4,"total_tokens":3}}`, want: "contradictory"},
		{body: `{"usage":{"prompt_tokens":3,"total_tokens":4}}`, want: "does not equal"},
		{body: `{"usage":{"prompt_tokens":-1,"total_tokens":-1}}`, want: "cannot be negative"},
		{body: `{"usage":{"prompt_tokens":3.5,"total_tokens":3}}`, want: "parse OpenAI Embeddings response"},
		{body: `{"usage":{"prompt_tokens":3,"total_tokens":3,"prompt_tokens_details":{"cached_tokens":4}}}`, want: "exceeds input token count"},
	}
	for _, tt := range tests {
		_, err := extractNativeOpenAIEmbeddingsUsage([]byte(tt.body))
		require.ErrorContains(t, err, tt.want)
	}
}

func TestForwardEmbeddingsExchangePreservesProtocolAndExactUsage(t *testing.T) {
	responseBody := `{
		"object":"list",
		"data":[{"object":"embedding","index":0,"embedding":[0.1,0.2]}],
		"model":"text-embedding-3-large",
		"usage":{
			"prompt_tokens":13,
			"total_tokens":13,
			"prompt_tokens_details":{"cached_tokens":3,"cache_write_tokens":1,"image_tokens":2}
		}
	}`
	upstream := &openAIResponsesHTTPStub{
		status: http.StatusOK,
		header: http.Header{"Content-Type": []string{"application/json"}, "X-Request-Id": []string{"emb-req-1"}},
		body:   responseBody,
	}
	svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}}
	body := []byte(`{"model":"embedding-alias","input":["hello","world"],"encoding_format":"float","dimensions":256}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", bytes.NewReader(body))
	req.Header.Set("Accept-Language", "en-AU")
	recorder := httptest.NewRecorder()

	result, err := svc.ForwardEmbeddingsExchange(context.Background(), gatewaytransport.NewHTTPExchange(recorder, req), nativeOpenAIEmbeddingsAPIKeyAccount(), body)
	require.NoError(t, err)
	require.JSONEq(t, responseBody, recorder.Body.String())
	require.Equal(t, "emb-req-1", result.RequestID)
	require.Equal(t, "embedding-alias", result.Model)
	require.Equal(t, "text-embedding-3-large", result.UpstreamModel)
	require.Equal(t, nativeOpenAIEmbeddingsEndpoint, result.UpstreamEndpoint)
	require.Equal(t, 13, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.ImageInputTokens)
	require.Equal(t, 3, result.Usage.CacheReadInputTokens)
	require.Equal(t, 1, result.Usage.CacheCreationInputTokens)
	require.Equal(t, "https://embeddings.example/v1/embeddings", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer sk-embeddings", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, "application/json", upstream.lastReq.Header.Get("Accept"))
	require.Equal(t, "en-AU", upstream.lastReq.Header.Get("Accept-Language"))
	posted, err := io.ReadAll(upstream.lastReq.Body)
	require.NoError(t, err)
	require.Equal(t, "text-embedding-3-large", gjson.GetBytes(posted, "model").String())
	require.Equal(t, int64(2), gjson.GetBytes(posted, "input.#").Int())
	require.Equal(t, int64(256), gjson.GetBytes(posted, "dimensions").Int())
}

func TestForwardEmbeddingsExchangeMissingUsageCanFailOverBeforeCommit(t *testing.T) {
	upstream := &openAIResponsesHTTPStub{
		status: http.StatusOK,
		header: http.Header{"Content-Type": []string{"application/json"}},
		body:   `{"object":"list","data":[],"model":"text-embedding-3-large"}`,
	}
	svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}}
	body := []byte(`{"model":"embedding-alias","input":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", bytes.NewReader(body))
	recorder := httptest.NewRecorder()

	result, err := svc.ForwardEmbeddingsExchange(context.Background(), gatewaytransport.NewHTTPExchange(recorder, req), nativeOpenAIEmbeddingsAPIKeyAccount(), body)
	require.Nil(t, result)
	var failover *UpstreamFailoverError
	require.ErrorAs(t, err, &failover)
	require.Empty(t, recorder.Body.String())
}

func TestForwardEmbeddingsExchangeSemanticErrorIsCommittedWithoutFailover(t *testing.T) {
	upstream := &openAIResponsesHTTPStub{
		status: http.StatusBadRequest,
		header: http.Header{"Content-Type": []string{"application/json"}},
		body:   `{"error":{"type":"invalid_request_error","message":"invalid dimensions"}}`,
	}
	svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}}
	body := []byte(`{"model":"embedding-alias","input":"hello","dimensions":-1}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", bytes.NewReader(body))
	recorder := httptest.NewRecorder()

	result, err := svc.ForwardEmbeddingsExchange(context.Background(), gatewaytransport.NewHTTPExchange(recorder, req), nativeOpenAIEmbeddingsAPIKeyAccount(), body)
	require.Nil(t, result)
	require.Error(t, err)
	var failover *UpstreamFailoverError
	require.False(t, errors.As(err, &failover))
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.JSONEq(t, upstream.body, recorder.Body.String())
}
