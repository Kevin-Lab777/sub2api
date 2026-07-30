package service

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/gatewaytransport"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func nativeOpenAIImagesSubscriptionAccount() *Account {
	return &Account{
		ID:       902,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"auth_mode":          OpenAIAuthModePersonalAccessToken,
			"access_token":       "oauth-images-token",
			"chatgpt_account_id": "chatgpt-images-account",
			"model_mapping": map[string]any{
				"image-alias": "gpt-image-2",
			},
		},
	}
}

func TestValidateOpenAIImagesSubscriptionRequestRejectsLossyShapes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		path string
		body string
	}{
		{name: "remote URL response", path: "/v1/images/generations", body: `{"model":"image-alias","prompt":"cat","response_format":"url"}`},
		{name: "streaming aggregate multi image usage", path: "/v1/images/generations", body: `{"model":"image-alias","prompt":"cat","stream":true,"n":2}`},
		{name: "unknown user field", path: "/v1/images/generations", body: `{"model":"image-alias","prompt":"cat","user":"customer"}`},
		{name: "null stream", path: "/v1/images/generations", body: `{"model":"image-alias","prompt":"cat","stream":null}`},
		{name: "fractional compression", path: "/v1/images/generations", body: `{"model":"image-alias","prompt":"cat","output_compression":12.5}`},
		{name: "generation image input", path: "/v1/images/generations", body: `{"model":"image-alias","prompt":"cat","images":[{"image_url":"https://example.com/cat.png"}]}`},
		{name: "edit missing image", path: "/v1/images/edits", body: `{"model":"image-alias","prompt":"edit cat"}`},
		{name: "edit file id", path: "/v1/images/edits", body: `{"model":"image-alias","prompt":"edit cat","images":[{"file_id":"file_1"}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			require.Error(t, ValidateOpenAIImagesSubscriptionRequest(req, []byte(tt.body)))
		})
	}
}

func TestBuildNativeOpenAIImagesSubscriptionRequestPreservesMultipartEdit(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "image-alias"))
	require.NoError(t, writer.WriteField("prompt", "replace the background"))
	require.NoError(t, writer.WriteField("input_fidelity", "high"))
	require.NoError(t, writer.WriteField("output_format", "webp"))
	imageHeader := make(textproto.MIMEHeader)
	imageHeader.Set("Content-Disposition", `form-data; name="image"; filename="source.png"`)
	imageHeader.Set("Content-Type", "image/png")
	imagePart, err := writer.CreatePart(imageHeader)
	require.NoError(t, err)
	_, err = imagePart.Write([]byte("source-image"))
	require.NoError(t, err)
	maskHeader := make(textproto.MIMEHeader)
	maskHeader.Set("Content-Disposition", `form-data; name="mask"; filename="mask.png"`)
	maskHeader.Set("Content-Type", "image/png")
	maskPart, err := writer.CreatePart(maskHeader)
	require.NoError(t, err)
	_, err = maskPart.Write([]byte("mask-image"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(body.Bytes()))
	req.Header.Set("Content-Type", writer.FormDataContentType())

	parsed, err := buildNativeOpenAIImagesSubscriptionRequest(nativeOpenAIImagesSubscriptionAccount(), req, body.Bytes())
	require.NoError(t, err)
	require.Equal(t, "image-alias", parsed.originalModel)
	require.Equal(t, "gpt-image-2", parsed.upstreamModel)
	require.Equal(t, openAIImagesResponsesMainModel, gjson.GetBytes(parsed.responsesBody, "model").String())
	require.True(t, gjson.GetBytes(parsed.responsesBody, "stream").Bool())
	require.Equal(t, "edit", gjson.GetBytes(parsed.responsesBody, "tools.0.action").String())
	require.Equal(t, "gpt-image-2", gjson.GetBytes(parsed.responsesBody, "tools.0.model").String())
	require.Equal(t, "high", gjson.GetBytes(parsed.responsesBody, "tools.0.input_fidelity").String())
	require.Equal(t, "webp", gjson.GetBytes(parsed.responsesBody, "tools.0.output_format").String())
	require.True(t, strings.HasPrefix(gjson.GetBytes(parsed.responsesBody, "input.0.content.1.image_url").String(), "data:image/png;base64,"))
	require.True(t, strings.HasPrefix(gjson.GetBytes(parsed.responsesBody, "tools.0.input_image_mask.image_url").String(), "data:image/png;base64,"))
	require.False(t, gjson.GetBytes(parsed.responsesBody, "include").Exists())
	require.False(t, gjson.GetBytes(parsed.responsesBody, "instructions").Exists())
	require.False(t, gjson.GetBytes(parsed.responsesBody, "reasoning").Exists())
	require.False(t, gjson.GetBytes(parsed.responsesBody, "parallel_tool_calls").Exists())
}

func TestParseNativeOpenAIImagesSubscriptionUsageRequiresExactEditInputTokens(t *testing.T) {
	t.Parallel()
	toolUsage := jsonRaw(`{"input_tokens":50,"output_tokens":100,"input_tokens_details":{"image_tokens":40},"output_tokens_details":{"image_tokens":100},"images":1}`)
	responsesUsage := jsonRaw(`{"input_tokens_details":{"image_tokens":40,"cached_tokens":3}}`)
	usage, _, err := parseNativeOpenAIImagesSubscriptionUsage(toolUsage, responsesUsage, true, 1)
	require.NoError(t, err)
	require.Equal(t, OpenAIUsage{InputTokens: 50, ImageInputTokens: 40, OutputTokens: 100, ImageOutputTokens: 100, CacheReadInputTokens: 3}, usage)

	_, _, err = parseNativeOpenAIImagesSubscriptionUsage(
		jsonRaw(`{"input_tokens":50,"output_tokens":100,"output_tokens_details":{"image_tokens":100},"images":1}`),
		jsonRaw(`{"input_tokens_details":{"cached_tokens":3}}`),
		true,
		1,
	)
	require.ErrorContains(t, err, "omitted exact image input tokens")

	_, _, err = parseNativeOpenAIImagesSubscriptionUsage(
		toolUsage,
		jsonRaw(`{"input_tokens_details":{"image_tokens":39}}`),
		true,
		1,
	)
	require.ErrorContains(t, err, "disagree")
}

func TestForwardImagesSubscriptionExchangeConvertsAuthoritativeTerminalJSON(t *testing.T) {
	streamBody := strings.Join([]string{
		`event: response.completed`,
		`data: {"type":"response.completed","response":{"id":"resp_img_1","model":"gpt-5.4-mini","status":"completed","created_at":1785000100,"tools":[{"type":"image_generation","model":"gpt-image-2","output_format":"png","quality":"high","size":"1024x1024"}],"usage":{"input_tokens":11,"output_tokens":22,"input_tokens_details":{"cached_tokens":3,"image_tokens":0}},"tool_usage":{"image_gen":{"input_tokens":46,"output_tokens":2459,"total_tokens":2505,"output_tokens_details":{"image_tokens":2459},"images":2}},"output":[{"type":"reasoning","summary":[]},{"id":"ig_1","type":"image_generation_call","status":"completed","result":"aW1hZ2Ux","revised_prompt":"cat one"},{"id":"ig_2","type":"image_generation_call","status":"completed","result":"aW1hZ2Uy","revised_prompt":"cat two"}]}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	upstream := &openAIResponsesHTTPStub{
		status: http.StatusOK,
		header: http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"subscription-image-1"}},
		body:   streamBody,
	}
	svc := &OpenAIGatewayService{
		httpUpstream:        upstream,
		cfg:                 &config.Config{},
		openAITokenProvider: NewOpenAITokenProvider(nil, nil, nil),
	}
	body := []byte(`{"model":"image-alias","prompt":"draw two cats","n":2}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	result, err := svc.ForwardImagesSubscriptionExchange(context.Background(), gatewaytransport.NewHTTPExchange(recorder, req), nativeOpenAIImagesSubscriptionAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 2, result.ImageCount)
	require.Equal(t, 46, result.Usage.InputTokens)
	require.Equal(t, 2459, result.Usage.OutputTokens)
	require.Equal(t, 2459, result.Usage.ImageOutputTokens)
	require.Equal(t, 3, result.Usage.CacheReadInputTokens)
	require.Equal(t, []string{"1024x1024", "1024x1024"}, result.ImageOutputSizes)
	require.Equal(t, int64(1785000100), gjson.Get(recorder.Body.String(), "created").Int())
	require.Len(t, gjson.Get(recorder.Body.String(), "data").Array(), 2)
	require.Equal(t, "aW1hZ2Ux", gjson.Get(recorder.Body.String(), "data.0.b64_json").String())
	require.Equal(t, "aW1hZ2Uy", gjson.Get(recorder.Body.String(), "data.1.b64_json").String())
	require.Equal(t, "1024x1024", gjson.Get(recorder.Body.String(), "data.0.size").String())
	require.JSONEq(t, `{"input_tokens":46,"output_tokens":2459,"total_tokens":2505,"output_tokens_details":{"image_tokens":2459},"images":2}`, gjson.Get(recorder.Body.String(), "usage").Raw)
	require.Equal(t, chatgptCodexURL, upstream.lastReq.URL.String())
	require.Equal(t, "chatgpt-images-account", upstream.lastReq.Header.Get("chatgpt-account-id"))
	require.Equal(t, openAIImagesResponsesMainModel, gjson.GetBytes(readRequestBody(t, upstream.lastReq), "model").String())
}

func TestForwardImagesSubscriptionExchangeStreamsOnlyObservedImageEvents(t *testing.T) {
	streamBody := strings.Join([]string{
		`event: response.created`,
		`data: {"type":"response.created","response":{"id":"resp_img_stream","created_at":1785000200,"tools":[{"type":"image_generation","model":"gpt-image-2","output_format":"webp","background":"transparent","quality":"high","size":"1024x1536"}]}}`,
		``,
		`event: response.image_generation_call.partial_image`,
		`data: {"type":"response.image_generation_call.partial_image","partial_image_b64":"cGFydGlhbA==","partial_image_index":0,"output_format":"webp","background":"transparent"}`,
		``,
		`event: response.completed`,
		`data: {"type":"response.completed","response":{"id":"resp_img_stream","model":"gpt-5.4-mini","status":"completed","created_at":1785000200,"tools":[{"type":"image_generation","model":"gpt-image-2","output_format":"webp","background":"transparent","quality":"high","size":"1024x1536"}],"usage":{"input_tokens":9,"output_tokens":10,"input_tokens_details":{"image_tokens":0}},"tool_usage":{"image_gen":{"input_tokens":40,"output_tokens":100,"output_tokens_details":{"image_tokens":100},"images":1}},"output":[{"id":"ig_stream","type":"image_generation_call","status":"completed","result":"ZmluYWw="}]}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	upstream := &openAIResponsesHTTPStub{status: http.StatusOK, header: http.Header{"Content-Type": []string{"text/event-stream"}}, body: streamBody}
	svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}, openAITokenProvider: NewOpenAITokenProvider(nil, nil, nil)}
	body := []byte(`{"model":"image-alias","prompt":"draw a cat","stream":true,"output_format":"webp"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	result, err := svc.ForwardImagesSubscriptionExchange(context.Background(), gatewaytransport.NewHTTPExchange(recorder, req), nativeOpenAIImagesSubscriptionAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Stream)
	require.Equal(t, 1, result.ImageCount)
	require.Equal(t, 100, result.Usage.ImageOutputTokens)
	events := parseOpenAIImageTestSSEEvents(recorder.Body.String())
	require.Len(t, events, 2)
	require.Equal(t, "image_generation.partial_image", events[0].Name)
	require.Equal(t, int64(1785000200), gjson.Get(events[0].Data, "created_at").Int())
	require.Equal(t, "cGFydGlhbA==", gjson.Get(events[0].Data, "b64_json").String())
	require.Equal(t, "image_generation.completed", events[1].Name)
	require.Equal(t, "ZmluYWw=", gjson.Get(events[1].Data, "b64_json").String())
	require.Equal(t, "1024x1536", gjson.Get(events[1].Data, "size").String())
	require.JSONEq(t, `{"input_tokens":40,"output_tokens":100,"output_tokens_details":{"image_tokens":100},"images":1}`, gjson.Get(events[1].Data, "usage").Raw)
	require.NotContains(t, recorder.Body.String(), "[DONE]")
}

func TestForwardImagesSubscriptionExchangeDoesNotFinalizeTruncatedStream(t *testing.T) {
	streamBody := strings.Join([]string{
		`event: response.created`,
		`data: {"type":"response.created","response":{"created_at":1785000300,"tools":[{"type":"image_generation","model":"gpt-image-2","output_format":"png","size":"1024x1024"}]}}`,
		``,
		`event: response.image_generation_call.partial_image`,
		`data: {"type":"response.image_generation_call.partial_image","partial_image_b64":"cGFydA==","partial_image_index":0}`,
		``,
	}, "\n")
	upstream := &openAIResponsesHTTPStub{status: http.StatusOK, header: http.Header{"Content-Type": []string{"text/event-stream"}}, body: streamBody}
	svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}, openAITokenProvider: NewOpenAITokenProvider(nil, nil, nil)}
	body := []byte(`{"model":"image-alias","prompt":"draw a cat","stream":true}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	result, err := svc.ForwardImagesSubscriptionExchange(context.Background(), gatewaytransport.NewHTTPExchange(recorder, req), nativeOpenAIImagesSubscriptionAccount(), body)
	require.Nil(t, result)
	require.ErrorContains(t, err, "without response.completed")
	require.Contains(t, recorder.Body.String(), "image_generation.partial_image")
	require.NotContains(t, recorder.Body.String(), "image_generation.completed")
}

func TestForwardImagesSubscriptionExchangeDoesNotUseOutputItemDoneAsFallback(t *testing.T) {
	streamBody := strings.Join([]string{
		`data: {"type":"response.output_item.done","item":{"id":"ig_fallback","type":"image_generation_call","result":"ZmFrZQ=="}}`,
		``,
		`data: {"type":"response.completed","response":{"status":"completed","created_at":1785000400,"tools":[{"type":"image_generation","model":"gpt-image-2"}],"tool_usage":{"image_gen":{"input_tokens":1,"output_tokens":1,"output_tokens_details":{"image_tokens":1},"images":1}},"output":[]}}`,
		``,
	}, "\n")
	upstream := &openAIResponsesHTTPStub{status: http.StatusOK, header: http.Header{"Content-Type": []string{"text/event-stream"}}, body: streamBody}
	svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}, openAITokenProvider: NewOpenAITokenProvider(nil, nil, nil)}
	body := []byte(`{"model":"image-alias","prompt":"draw a cat"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	result, err := svc.ForwardImagesSubscriptionExchange(context.Background(), gatewaytransport.NewHTTPExchange(recorder, req), nativeOpenAIImagesSubscriptionAccount(), body)
	require.Nil(t, result)
	require.ErrorContains(t, err, "without image output")
	var failover *UpstreamFailoverError
	require.False(t, errors.As(err, &failover))
	require.Empty(t, recorder.Body.String())
}
