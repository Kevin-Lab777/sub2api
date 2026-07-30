package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/gatewaytransport"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func nativeOpenAIImagesDirectAccount() *Account {
	return &Account{
		ID:       901,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "sk-images",
			"base_url": "https://images.example/v1",
			"model_mapping": map[string]any{
				"image-alias": "gpt-image-2",
			},
		},
	}
}

func TestParseOpenAIImagesRuntimeRequestRejectsAmbiguousRoutingFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		path        string
		contentType string
		body        string
	}{
		{name: "missing model", path: "/v1/images/generations", contentType: "application/json", body: `{"prompt":"cat"}`},
		{name: "model whitespace", path: "/v1/images/generations", contentType: "application/json", body: `{"model":" image-alias","prompt":"cat"}`},
		{name: "null stream", path: "/v1/images/generations", contentType: "application/json", body: `{"model":"image-alias","stream":null}`},
		{name: "null n", path: "/v1/images/generations", contentType: "application/json", body: `{"model":"image-alias","n":null}`},
		{name: "fractional n", path: "/v1/images/generations", contentType: "application/json", body: `{"model":"image-alias","n":1.5}`},
		{name: "multipart generation", path: "/v1/images/generations", contentType: "multipart/form-data; boundary=x", body: "--x--\r\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", tt.contentType)
			_, err := ParseOpenAIImagesRuntimeRequest(req, []byte(tt.body))
			require.Error(t, err)
		})
	}
}

func TestParseOpenAIImagesRuntimeRequestReadsMultipartRoutingWithoutChangingBody(t *testing.T) {
	t.Parallel()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "image-alias"))
	require.NoError(t, writer.WriteField("stream", "true"))
	require.NoError(t, writer.WriteField("n", "2"))
	file, err := writer.CreateFormFile("image[]", "source.png")
	require.NoError(t, err)
	_, err = file.Write([]byte("image-bytes"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	raw := append([]byte(nil), body.Bytes()...)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(raw))
	req.Header.Set("Content-Type", writer.FormDataContentType())

	parsed, err := ParseOpenAIImagesRuntimeRequest(req, raw)
	require.NoError(t, err)
	require.Equal(t, "image-alias", parsed.Model)
	require.True(t, parsed.Stream)
	require.Equal(t, 2, parsed.N)
	require.True(t, parsed.Multipart)
	require.Equal(t, raw, body.Bytes())
}

func TestParseNativeOpenAIImagesUsageRequiresExactOfficialSchema(t *testing.T) {
	t.Parallel()

	usage, err := parseNativeOpenAIImagesUsage(jsonRaw(`{"input_tokens":50,"output_tokens":50,"total_tokens":100,"input_tokens_details":{"text_tokens":10,"image_tokens":40}}`), true)
	require.NoError(t, err)
	require.Equal(t, OpenAIUsage{InputTokens: 50, ImageInputTokens: 40, OutputTokens: 50, ImageOutputTokens: 50}, usage)

	for _, raw := range []string{
		`null`,
		`{"input_tokens":50,"output_tokens":50,"total_tokens":99,"input_tokens_details":{"text_tokens":10,"image_tokens":40}}`,
		`{"input_tokens":50,"output_tokens":50,"total_tokens":100,"input_tokens_details":{"text_tokens":11,"image_tokens":40}}`,
		`{"input_tokens":50,"output_tokens":-1,"total_tokens":49,"input_tokens_details":{"text_tokens":10,"image_tokens":40}}`,
		`{"input_tokens":50.5,"output_tokens":50,"total_tokens":100,"input_tokens_details":{"text_tokens":10,"image_tokens":40}}`,
	} {
		_, err := parseNativeOpenAIImagesUsage(jsonRaw(raw), true)
		require.Error(t, err)
	}
}

func jsonRaw(value string) []byte { return []byte(value) }

func TestForwardImagesDirectExchangePreservesJSONAndMeasuresActualImages(t *testing.T) {
	responseBody := `{
		"created":1785000000,
		"data":[
			{"b64_json":"aW1hZ2Ux","revised_prompt":"cat one","size":"1024x1024"},
			{"b64_json":"aW1hZ2Uy","revised_prompt":"cat two"}
		],
		"usage":{"input_tokens":12,"output_tokens":20,"total_tokens":32,"input_tokens_details":{"text_tokens":12,"image_tokens":0}}
	}`
	upstream := &openAIResponsesHTTPStub{
		status: http.StatusOK,
		header: http.Header{"Content-Type": []string{"application/json"}, "X-Request-Id": []string{"img-req-1"}},
		body:   responseBody,
	}
	svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}}
	body := []byte(`{"model":"image-alias","prompt":"draw cats","n":2,"quality":"high"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	result, err := svc.ForwardImagesDirectExchange(context.Background(), gatewaytransport.NewHTTPExchange(recorder, req), nativeOpenAIImagesDirectAccount(), body)
	require.NoError(t, err)
	require.JSONEq(t, responseBody, recorder.Body.String())
	require.Equal(t, "img-req-1", result.RequestID)
	require.Equal(t, "image-alias", result.Model)
	require.Equal(t, "gpt-image-2", result.UpstreamModel)
	require.Equal(t, 2, result.ImageCount)
	require.Equal(t, 12, result.Usage.InputTokens)
	require.Equal(t, 20, result.Usage.OutputTokens)
	require.Equal(t, 20, result.Usage.ImageOutputTokens)
	require.Equal(t, []string{"1024x1024"}, result.ImageOutputSizes)
	require.Equal(t, "https://images.example/v1/images/generations", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer sk-images", upstream.lastReq.Header.Get("Authorization"))
	posted, err := io.ReadAll(upstream.lastReq.Body)
	require.NoError(t, err)
	require.Equal(t, "gpt-image-2", gjson.GetBytes(posted, "model").String())
	require.Equal(t, int64(2), gjson.GetBytes(posted, "n").Int())
}

func TestForwardImagesDirectExchangeForwardsMultipartEditWithMappedModel(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "image-alias"))
	require.NoError(t, writer.WriteField("prompt", "replace background"))
	file, err := writer.CreateFormFile("image", "source.png")
	require.NoError(t, err)
	_, err = file.Write([]byte("png-image-content"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	upstream := &openAIResponsesHTTPStub{
		status: http.StatusOK,
		header: http.Header{"Content-Type": []string{"application/json"}},
		body:   `{"created":1785000001,"data":[{"b64_json":"ZWRpdGVk"}],"usage":{"input_tokens":30,"output_tokens":10,"total_tokens":40,"input_tokens_details":{"text_tokens":5,"image_tokens":25}}}`,
	}
	svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}}
	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(body.Bytes()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()

	result, err := svc.ForwardImagesDirectExchange(context.Background(), gatewaytransport.NewHTTPExchange(recorder, req), nativeOpenAIImagesDirectAccount(), body.Bytes())
	require.NoError(t, err)
	require.Equal(t, 1, result.ImageCount)
	require.Equal(t, 25, result.Usage.ImageInputTokens)
	require.Equal(t, 10, result.Usage.ImageOutputTokens)
	require.Equal(t, "https://images.example/v1/images/edits", upstream.lastReq.URL.String())
	mediaType, params, err := mime.ParseMediaType(upstream.lastReq.Header.Get("Content-Type"))
	require.NoError(t, err)
	require.Equal(t, "multipart/form-data", mediaType)
	reader := multipart.NewReader(bytes.NewReader(readRequestBody(t, upstream.lastReq)), params["boundary"])
	fields := map[string]string{}
	files := map[string]string{}
	for {
		part, partErr := reader.NextPart()
		if partErr == io.EOF {
			break
		}
		require.NoError(t, partErr)
		data, readErr := io.ReadAll(part)
		require.NoError(t, readErr)
		if part.FileName() == "" {
			fields[part.FormName()] = string(data)
		} else {
			files[part.FormName()] = string(data)
		}
	}
	require.Equal(t, "gpt-image-2", fields["model"])
	require.Equal(t, "replace background", fields["prompt"])
	require.Equal(t, "png-image-content", files["image"])
}

func readRequestBody(t *testing.T, req *http.Request) []byte {
	t.Helper()
	body, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	return body
}

func TestForwardImagesDirectExchangeStreamsOfficialEventsAndAggregatesPerImageUsage(t *testing.T) {
	streamBody := strings.Join([]string{
		`event: image_generation.partial_image`,
		`data: {"type":"image_generation.partial_image","b64_json":"cGFydDE=","partial_image_index":0}`,
		``,
		`event: image_generation.completed`,
		`data: {"type":"image_generation.completed","b64_json":"ZmluYWwx","size":"1024x1024","usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30,"input_tokens_details":{"text_tokens":10,"image_tokens":0}}}`,
		``,
		`event: image_generation.completed`,
		`data: {"type":"image_generation.completed","b64_json":"ZmluYWwy","size":"1024x1536","usage":{"input_tokens":11,"output_tokens":21,"total_tokens":32,"input_tokens_details":{"text_tokens":11,"image_tokens":0}}}`,
		``,
	}, "\n")
	upstream := &openAIResponsesHTTPStub{status: http.StatusOK, header: http.Header{"Content-Type": []string{"text/event-stream"}}, body: streamBody}
	svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}}
	body := []byte(`{"model":"image-alias","prompt":"draw cats","n":2,"stream":true}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	result, err := svc.ForwardImagesDirectExchange(context.Background(), gatewaytransport.NewHTTPExchange(recorder, req), nativeOpenAIImagesDirectAccount(), body)
	require.NoError(t, err)
	require.Equal(t, streamBody, recorder.Body.String())
	require.True(t, result.Stream)
	require.Equal(t, 2, result.ImageCount)
	require.Equal(t, 21, result.Usage.InputTokens)
	require.Equal(t, 41, result.Usage.OutputTokens)
	require.Equal(t, 41, result.Usage.ImageOutputTokens)
	require.Equal(t, []string{"1024x1024", "1024x1536"}, result.ImageOutputSizes)
	require.NotNil(t, result.FirstTokenMs)
}

func TestForwardImagesDirectExchangeMissingUsageStaysVisibleWithoutRetryClassification(t *testing.T) {
	responseBody := `{"created":1785000000,"data":[{"b64_json":"aW1hZ2U="}]}`
	upstream := &openAIResponsesHTTPStub{status: http.StatusOK, header: http.Header{"Content-Type": []string{"application/json"}}, body: responseBody}
	svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}}
	body := []byte(`{"model":"image-alias","prompt":"draw cat"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	result, err := svc.ForwardImagesDirectExchange(context.Background(), gatewaytransport.NewHTTPExchange(recorder, req), nativeOpenAIImagesDirectAccount(), body)
	require.Nil(t, result)
	require.ErrorContains(t, err, "omitted exact usage")
	var failover *UpstreamFailoverError
	require.False(t, errors.As(err, &failover))
	require.JSONEq(t, responseBody, recorder.Body.String())
}

func TestForwardImagesDirectExchangeTruncatedStreamIsNotFinalized(t *testing.T) {
	streamBody := "event: image_edit.partial_image\ndata: {\"type\":\"image_edit.partial_image\",\"b64_json\":\"cGFydA==\",\"partial_image_index\":0}\n\n"
	upstream := &openAIResponsesHTTPStub{status: http.StatusOK, header: http.Header{"Content-Type": []string{"text/event-stream"}}, body: streamBody}
	svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}}
	body := []byte(`{"model":"image-alias","prompt":"edit","images":[{"image_url":"https://example.com/source.png"}],"stream":true}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	result, err := svc.ForwardImagesDirectExchange(context.Background(), gatewaytransport.NewHTTPExchange(recorder, req), nativeOpenAIImagesDirectAccount(), body)
	require.Nil(t, result)
	require.ErrorContains(t, err, "without a completed image")
	require.Equal(t, streamBody, recorder.Body.String())
	require.NotContains(t, recorder.Body.String(), "image_edit.completed")
}

func TestForwardImagesDirectExchangePreservesCompletedUsageBeforeLaterBadEvent(t *testing.T) {
	streamBody := strings.Join([]string{
		`event: image_generation.completed`,
		`data: {"type":"image_generation.completed","b64_json":"ZmluYWw=","usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30,"input_tokens_details":{"text_tokens":10,"image_tokens":0}}}`,
		``,
		`event: unexpected.event`,
		`data: {"type":"unexpected.event"}`,
		``,
	}, "\n")
	upstream := &openAIResponsesHTTPStub{status: http.StatusOK, header: http.Header{"Content-Type": []string{"text/event-stream"}}, body: streamBody}
	svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}}
	body := []byte(`{"model":"image-alias","prompt":"draw cat","stream":true}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	result, err := svc.ForwardImagesDirectExchange(context.Background(), gatewaytransport.NewHTTPExchange(recorder, req), nativeOpenAIImagesDirectAccount(), body)
	require.NotNil(t, result)
	require.ErrorContains(t, err, "unsupported event")
	require.Equal(t, 1, result.ImageCount)
	require.Equal(t, 10, result.Usage.InputTokens)
	require.Equal(t, 20, result.Usage.ImageOutputTokens)
	require.Equal(t, streamBody, recorder.Body.String())
}
