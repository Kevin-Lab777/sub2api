package gatewaytransport

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
)

// Values is request-scoped state shared by request and response transforms.
type Values interface {
	Get(key string) (any, bool)
	Set(key string, value any)
}

// Key provides typed access to request-scoped values.
type Key[T any] struct {
	name string
}

func NewKey[T any](name string) Key[T] {
	if name == "" {
		panic("gateway transport value key cannot be empty")
	}
	return Key[T]{name: name}
}

func Store[T any](values Values, key Key[T], value T) {
	if values == nil {
		return
	}
	values.Set(key.name, value)
}

func Load[T any](values Values, key Key[T]) (T, bool) {
	var zero T
	if values == nil {
		return zero, false
	}
	value, ok := values.Get(key.name)
	if !ok {
		return zero, false
	}
	typed, ok := value.(T)
	if !ok {
		return zero, false
	}
	return typed, true
}

// ResponseWriter exposes response state without depending on a web framework.
type ResponseWriter interface {
	http.ResponseWriter
	Flush() error
	SupportsFlush() bool
	Written() bool
	Status() int
	Size() int64
	Unwrap() http.ResponseWriter
}

// Exchange is the provider-facing HTTP transport boundary.
type Exchange interface {
	Request() *http.Request
	Response() ResponseWriter
	Values() Values
	RequestHeader(name string) string
	SetResponseHeader(name, value string)
	WriteJSON(status int, value any) error
	WriteData(status int, contentType string, data []byte) error
}

type valueStore struct {
	mu     sync.RWMutex
	values map[string]any
}

func newValueStore() *valueStore {
	return &valueStore{values: make(map[string]any)}
}

func (s *valueStore) Get(key string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.values[key]
	return value, ok
}

func (s *valueStore) Set(key string, value any) {
	s.mu.Lock()
	s.values[key] = value
	s.mu.Unlock()
}

type httpResponseWriter struct {
	writer  http.ResponseWriter
	status  int
	size    int64
	written bool
}

func newHTTPResponseWriter(writer http.ResponseWriter) *httpResponseWriter {
	if writer == nil {
		panic("gateway transport response writer cannot be nil")
	}
	return &httpResponseWriter{writer: writer}
}

func (w *httpResponseWriter) Header() http.Header { return w.writer.Header() }

func (w *httpResponseWriter) WriteHeader(status int) {
	if w.written {
		return
	}
	w.status = status
	w.written = true
	w.writer.WriteHeader(status)
}

func (w *httpResponseWriter) Write(data []byte) (int, error) {
	if !w.written {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.writer.Write(data)
	w.size += int64(n)
	return n, err
}

func (w *httpResponseWriter) Flush() error {
	if !w.written {
		w.WriteHeader(http.StatusOK)
	}
	return http.NewResponseController(w.writer).Flush()
}

func (w *httpResponseWriter) SupportsFlush() bool         { return supportsFlush(w.writer) }
func (w *httpResponseWriter) Written() bool               { return w.written }
func (w *httpResponseWriter) Status() int                 { return w.status }
func (w *httpResponseWriter) Size() int64                 { return w.size }
func (w *httpResponseWriter) Unwrap() http.ResponseWriter { return w.writer }

type HTTPExchange struct {
	request  *http.Request
	response *httpResponseWriter
	values   *valueStore
}

func NewHTTPExchange(writer http.ResponseWriter, request *http.Request) *HTTPExchange {
	if request == nil {
		panic("gateway transport request cannot be nil")
	}
	return &HTTPExchange{
		request:  request,
		response: newHTTPResponseWriter(writer),
		values:   newValueStore(),
	}
}

func (e *HTTPExchange) Request() *http.Request           { return e.request }
func (e *HTTPExchange) Response() ResponseWriter         { return e.response }
func (e *HTTPExchange) Values() Values                   { return e.values }
func (e *HTTPExchange) RequestHeader(name string) string { return e.request.Header.Get(name) }
func (e *HTTPExchange) SetResponseHeader(name, value string) {
	e.response.Header().Set(name, value)
}
func (e *HTTPExchange) WriteJSON(status int, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal gateway response: %w", err)
	}
	return e.WriteData(status, "application/json; charset=utf-8", data)
}
func (e *HTTPExchange) WriteData(status int, contentType string, data []byte) error {
	return writeData(e.response, status, contentType, data)
}

type ginValues struct {
	ctx *gin.Context
}

func (v ginValues) Get(key string) (any, bool) { return v.ctx.Get(key) }
func (v ginValues) Set(key string, value any)  { v.ctx.Set(key, value) }

type ginResponseWriter struct {
	writer gin.ResponseWriter
}

func (w ginResponseWriter) Header() http.Header            { return w.writer.Header() }
func (w ginResponseWriter) Write(data []byte) (int, error) { return w.writer.Write(data) }
func (w ginResponseWriter) WriteHeader(status int)         { w.writer.WriteHeader(status) }
func (w ginResponseWriter) Flush() error                   { w.writer.Flush(); return nil }
func (w ginResponseWriter) SupportsFlush() bool            { return supportsFlush(w.writer) }
func (w ginResponseWriter) Written() bool                  { return w.writer.Written() }
func (w ginResponseWriter) Status() int                    { return w.writer.Status() }
func (w ginResponseWriter) Size() int64                    { return int64(w.writer.Size()) }
func (w ginResponseWriter) Unwrap() http.ResponseWriter    { return w.writer }

type GinExchange struct {
	ctx *gin.Context
}

func NewGinExchange(ctx *gin.Context) *GinExchange {
	if ctx == nil {
		panic("gateway transport gin context cannot be nil")
	}
	return &GinExchange{ctx: ctx}
}

func (e *GinExchange) Request() *http.Request   { return e.ctx.Request }
func (e *GinExchange) Response() ResponseWriter { return ginResponseWriter{writer: e.ctx.Writer} }
func (e *GinExchange) Values() Values           { return ginValues{ctx: e.ctx} }
func (e *GinExchange) RequestHeader(name string) string {
	return e.ctx.GetHeader(name)
}
func (e *GinExchange) SetResponseHeader(name, value string) {
	e.ctx.Header(name, value)
}
func (e *GinExchange) WriteJSON(status int, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal gateway response: %w", err)
	}
	return e.WriteData(status, "application/json; charset=utf-8", data)
}
func (e *GinExchange) WriteData(status int, contentType string, data []byte) error {
	return writeData(e.Response(), status, contentType, data)
}

func writeData(writer ResponseWriter, status int, contentType string, data []byte) error {
	if writer == nil {
		return errors.New("gateway transport response writer is nil")
	}
	if contentType != "" && writer.Header().Get("Content-Type") == "" {
		writer.Header().Set("Content-Type", contentType)
	}
	writer.WriteHeader(status)
	_, err := writer.Write(data)
	return err
}

func supportsFlush(writer http.ResponseWriter) bool {
	for writer != nil {
		if _, ok := writer.(interface{ FlushError() error }); ok {
			return true
		}
		if _, ok := writer.(http.Flusher); ok {
			return true
		}
		unwrapper, ok := writer.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return false
		}
		writer = unwrapper.Unwrap()
	}
	return false
}

var _ Exchange = (*HTTPExchange)(nil)
var _ Exchange = (*GinExchange)(nil)
