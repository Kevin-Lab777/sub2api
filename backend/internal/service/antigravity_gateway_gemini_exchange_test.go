package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/gatewaytransport"
)

func strictAntigravityGeminiTestAccount() *Account {
	return &Account{
		ID:          7101,
		Name:        "strict-antigravity",
		Platform:    PlatformAntigravity,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 2,
		Credentials: map[string]any{
			"access_token": "provider-token",
			"project_id":   "provider-project",
			"model_mapping": map[string]any{
				"gemini-client": "gemini-provider",
			},
		},
		Extra: map[string]any{"mixed_scheduling": true},
	}
}

func strictAntigravityGeminiResponse(status int, contentType, body string) *http.Response {
	header := make(http.Header)
	header.Set("Content-Type", contentType)
	header.Set("x-goog-request-id", "provider-request")
	return &http.Response{
		StatusCode: status,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestForwardGeminiExchangePreservesRequestAndReportsRawStreamingTelemetry(t *testing.T) {
	requestBody := []byte(`{
		"contents":[{"role":"model","parts":[{"text":"reason","thoughtSignature":"real-signature"}]}],
		"systemInstruction":{"parts":[{"text":"user-system"}]},
		"tools":[{"functionDeclarations":[{"name":"f","parameters":{"type":"object","additionalProperties":false,"$defs":{"value":{"type":"string"}}}}]}]
	}`)
	upstreamStream := strings.Join([]string{
		`data: {"response":{"candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"hello"}]}}]}}`,
		"",
		`data: {"response":{"candidates":[{"index":0,"content":{"role":"model","parts":[{"inlineData":{"mimeType":"image/png","data":"aW1hZ2U="}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":12,"cachedContentTokenCount":2,"candidatesTokenCount":5,"thoughtsTokenCount":3,"candidatesTokensDetails":[{"modality":"IMAGE","tokenCount":4}]}}}`,
		"",
	}, "\n")

	var capturedURL string
	var capturedAuthorization string
	upstream := &queuedHTTPUpstreamStub{
		responses: []*http.Response{strictAntigravityGeminiResponse(http.StatusOK, "text/event-stream", upstreamStream)},
		onCall: func(req *http.Request, _ *queuedHTTPUpstreamStub) {
			capturedURL = req.URL.String()
			capturedAuthorization = req.Header.Get("Authorization")
		},
	}
	svc := &AntigravityGatewayService{tokenProvider: &AntigravityTokenProvider{}, httpUpstream: upstream}
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-client:streamGenerateContent", bytes.NewReader(requestBody))
	exchange := gatewaytransport.NewHTTPExchange(recorder, req)

	result, err := svc.ForwardGeminiExchange(context.Background(), exchange, strictAntigravityGeminiTestAccount(), "gemini-client", "streamGenerateContent", true, requestBody)
	if err != nil {
		t.Fatalf("ForwardGeminiExchange: %v", err)
	}
	if upstream.callCount != 1 {
		t.Fatalf("upstream calls = %d, want 1", upstream.callCount)
	}
	if !strings.HasSuffix(capturedURL, "/v1internal:streamGenerateContent?alt=sse") || capturedAuthorization != "Bearer provider-token" {
		t.Fatalf("unexpected provider request: url=%q authorization=%q", capturedURL, capturedAuthorization)
	}

	var envelope map[string]any
	if err := json.Unmarshal(upstream.requestBodies[0], &envelope); err != nil {
		t.Fatalf("decode provider envelope: %v", err)
	}
	if envelope["project"] != "provider-project" || envelope["model"] != "gemini-provider" || envelope["requestType"] != "agent" {
		t.Fatalf("unexpected provider envelope: %#v", envelope)
	}
	providerRequest, ok := envelope["request"].(map[string]any)
	if !ok {
		t.Fatalf("provider request is not an object: %#v", envelope["request"])
	}
	contents := providerRequest["contents"].([]any)
	parts := contents[0].(map[string]any)["parts"].([]any)
	if parts[0].(map[string]any)["thoughtSignature"] != "real-signature" {
		t.Fatalf("thought signature was modified: %#v", parts[0])
	}
	tools := providerRequest["tools"].([]any)
	declarations := tools[0].(map[string]any)["functionDeclarations"].([]any)
	parameters := declarations[0].(map[string]any)["parameters"].(map[string]any)
	if parameters["additionalProperties"] != false || parameters["$defs"] == nil {
		t.Fatalf("tool schema was cleaned or rewritten: %#v", parameters)
	}
	systemParts := providerRequest["systemInstruction"].(map[string]any)["parts"].([]any)
	if len(systemParts) != 2 || systemParts[1].(map[string]any)["text"] != "user-system" {
		t.Fatalf("required identity was not prepended faithfully: %#v", systemParts)
	}

	downstream := recorder.Body.String()
	if strings.Contains(downstream, `"response":`) || !strings.Contains(downstream, `data: {"candidates"`) || !strings.Contains(downstream, `"inlineData"`) {
		t.Fatalf("unexpected downstream Gemini SSE: %s", downstream)
	}
	if result.RequestID != "provider-request" || result.Model != "gemini-client" || result.UpstreamModel != "gemini-provider" || !result.Stream {
		t.Fatalf("unexpected forward result: %+v", result)
	}
	if result.Usage.InputTokens != 10 || result.Usage.CacheReadInputTokens != 2 || result.Usage.OutputTokens != 8 || result.Usage.ImageOutputTokens != 4 || result.ImageCount != 1 {
		t.Fatalf("unexpected raw telemetry: %+v", result)
	}
}

func TestForwardGeminiExchangeAggregatesNonStreamingPartsWithoutDeduplication(t *testing.T) {
	upstreamStream := strings.Join([]string{
		`data: {"response":{"candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"A"},{"functionCall":{"name":"lookup","args":{"q":"x"}}}]}}]}}`,
		"",
		`data: {"response":{"candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"B"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":2}}}`,
		"",
	}, "\n")
	upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{strictAntigravityGeminiResponse(http.StatusOK, "text/event-stream", upstreamStream)}}
	svc := &AntigravityGatewayService{tokenProvider: &AntigravityTokenProvider{}, httpUpstream: upstream}
	recorder := httptest.NewRecorder()
	body := []byte(`{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`)
	exchange := gatewaytransport.NewHTTPExchange(recorder, httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-client:generateContent", bytes.NewReader(body)))

	result, err := svc.ForwardGeminiExchange(context.Background(), exchange, strictAntigravityGeminiTestAccount(), "gemini-client", "generateContent", false, body)
	if err != nil {
		t.Fatalf("ForwardGeminiExchange: %v", err)
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode downstream response: %v body=%s", err, recorder.Body.String())
	}
	candidates := response["candidates"].([]any)
	candidate := candidates[0].(map[string]any)
	parts := candidate["content"].(map[string]any)["parts"].([]any)
	if len(parts) != 3 || parts[0].(map[string]any)["text"] != "A" || parts[1].(map[string]any)["functionCall"] == nil || parts[2].(map[string]any)["text"] != "B" {
		t.Fatalf("incremental parts were changed: %#v", parts)
	}
	if candidate["finishReason"] != "STOP" || result.Usage.InputTokens != 4 || result.Usage.OutputTokens != 2 || result.Stream {
		t.Fatalf("unexpected aggregate result: candidate=%#v result=%+v", candidate, result)
	}
}

func TestForwardGeminiExchangeDoesNotRetryFallbackOrRepairProviderErrors(t *testing.T) {
	errorBody := `{"error":{"code":404,"message":"model not found"}}`
	upstream := &queuedHTTPUpstreamStub{
		responses: []*http.Response{
			strictAntigravityGeminiResponse(http.StatusNotFound, "application/json", errorBody),
			strictAntigravityGeminiResponse(http.StatusOK, "text/event-stream", `data: {"response":{"candidates":[]}}\n\n`),
		},
	}
	svc := &AntigravityGatewayService{tokenProvider: &AntigravityTokenProvider{}, httpUpstream: upstream}
	recorder := httptest.NewRecorder()
	body := []byte(`{"contents":[]}`)
	exchange := gatewaytransport.NewHTTPExchange(recorder, httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-client:generateContent", bytes.NewReader(body)))

	result, err := svc.ForwardGeminiExchange(context.Background(), exchange, strictAntigravityGeminiTestAccount(), "gemini-client", "generateContent", false, body)
	if err == nil || result != nil {
		t.Fatalf("expected provider error, result=%+v err=%v", result, err)
	}
	if upstream.callCount != 1 {
		t.Fatalf("provider error triggered retry or fallback: calls=%d", upstream.callCount)
	}
	if recorder.Code != http.StatusNotFound || recorder.Body.String() != errorBody {
		t.Fatalf("provider error was modified: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestForwardGeminiExchangeReturnsRawFailoverBeforeResponseCommit(t *testing.T) {
	errorBody := `{"error":{"code":429,"message":"quota exhausted"}}`
	upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{strictAntigravityGeminiResponse(http.StatusTooManyRequests, "application/json", errorBody)}}
	svc := &AntigravityGatewayService{tokenProvider: &AntigravityTokenProvider{}, httpUpstream: upstream}
	recorder := httptest.NewRecorder()
	body := []byte(`{"contents":[]}`)
	exchange := gatewaytransport.NewHTTPExchange(recorder, httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-client:generateContent", bytes.NewReader(body)))

	_, err := svc.ForwardGeminiExchange(context.Background(), exchange, strictAntigravityGeminiTestAccount(), "gemini-client", "generateContent", false, body)
	var failoverErr *UpstreamFailoverError
	if !errors.As(err, &failoverErr) || failoverErr.StatusCode != http.StatusTooManyRequests || string(failoverErr.ResponseBody) != errorBody {
		t.Fatalf("unexpected failover error: %v", err)
	}
	if exchange.Response().Written() || recorder.Body.Len() != 0 {
		t.Fatalf("failover committed a downstream response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestForwardGeminiExchangeRejectsUnsupportedCountTokensWithoutSyntheticResult(t *testing.T) {
	upstream := &queuedHTTPUpstreamStub{}
	svc := &AntigravityGatewayService{tokenProvider: &AntigravityTokenProvider{}, httpUpstream: upstream}
	recorder := httptest.NewRecorder()
	body := []byte(`{"contents":[]}`)
	exchange := gatewaytransport.NewHTTPExchange(recorder, httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-client:countTokens", bytes.NewReader(body)))

	result, err := svc.ForwardGeminiExchange(context.Background(), exchange, strictAntigravityGeminiTestAccount(), "gemini-client", "countTokens", false, body)
	if err == nil || result != nil || upstream.callCount != 0 || exchange.Response().Written() {
		t.Fatalf("countTokens must fail closed without fake data: result=%+v err=%v calls=%d written=%v", result, err, upstream.callCount, exchange.Response().Written())
	}
}

func TestForwardGeminiExchangeRejectsMalformedSuccessBeforeCommit(t *testing.T) {
	upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{strictAntigravityGeminiResponse(http.StatusOK, "text/event-stream", "data: {\"unexpected\":true}\n\n")}}
	svc := &AntigravityGatewayService{tokenProvider: &AntigravityTokenProvider{}, httpUpstream: upstream}
	recorder := httptest.NewRecorder()
	body := []byte(`{"contents":[]}`)
	exchange := gatewaytransport.NewHTTPExchange(recorder, httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-client:streamGenerateContent", bytes.NewReader(body)))

	result, err := svc.ForwardGeminiExchange(context.Background(), exchange, strictAntigravityGeminiTestAccount(), "gemini-client", "streamGenerateContent", true, body)
	if err == nil || result != nil || exchange.Response().Written() {
		t.Fatalf("malformed success must fail before commit: result=%+v err=%v written=%v", result, err, exchange.Response().Written())
	}
}
