package gatewaytransport

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHTTPExchangeTracksResponseAndTypedValues(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	request.Header.Set("Anthropic-Beta", "test-beta")
	exchange := NewHTTPExchange(recorder, request)

	key := NewKey[int]("attempt")
	Store(exchange.Values(), key, 3)
	if value, ok := Load(exchange.Values(), key); !ok || value != 3 {
		t.Fatalf("typed value = %d, %v; want 3, true", value, ok)
	}
	if got := exchange.RequestHeader("anthropic-beta"); got != "test-beta" {
		t.Fatalf("request header = %q, want test-beta", got)
	}
	if err := exchange.WriteJSON(http.StatusAccepted, map[string]string{"status": "ok"}); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}
	if !exchange.Response().Written() || exchange.Response().Status() != http.StatusAccepted {
		t.Fatalf("response state = written:%v status:%d", exchange.Response().Written(), exchange.Response().Status())
	}
	if got := exchange.Response().Size(); got != int64(len(`{"status":"ok"}`)) {
		t.Fatalf("response size = %d", got)
	}
}

func TestGinExchangeUsesOriginalGinState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	exchange := NewGinExchange(ctx)

	key := NewKey[string]("request-id")
	Store(exchange.Values(), key, "req-1")
	if raw, ok := ctx.Get("request-id"); !ok || raw != "req-1" {
		t.Fatalf("gin value = %v, %v", raw, ok)
	}
	if err := exchange.WriteData(http.StatusCreated, "application/json", []byte(`{"ok":true}`)); err != nil {
		t.Fatalf("WriteData() error = %v", err)
	}
	if !ctx.Writer.Written() || exchange.Response().Status() != http.StatusCreated {
		t.Fatalf("gin response state = written:%v status:%d", ctx.Writer.Written(), exchange.Response().Status())
	}
}
