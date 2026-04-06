package logger

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/internal/lib/logger/slogdiscard"

	"github.com/stretchr/testify/assert"
)

func TestLoggerMiddleware_CallsNext(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	log := slogdiscard.NewDiscardLogger()
	handler := New(log)(next)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.True(t, called)
}

func TestLoggerMiddleware_PreservesBody(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	})

	log := slogdiscard.NewDiscardLogger()
	handler := New(log)(next)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, "hello", rr.Body.String())
}

func TestLoggerMiddleware_PreservesStatusCode(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	log := slogdiscard.NewDiscardLogger()
	handler := New(log)(next)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)
}

func TestLoggerMiddleware_DefaultStatusCode(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	log := slogdiscard.NewDiscardLogger()
	handler := New(log)(next)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestLoggerMiddleware_GET(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	log := slogdiscard.NewDiscardLogger()
	handler := New(log)(next)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestLoggerMiddleware_POST(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	log := slogdiscard.NewDiscardLogger()
	handler := New(log)(next)

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestLoggerMiddleware_ErrorStatus500(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	log := slogdiscard.NewDiscardLogger()
	handler := New(log)(next)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}
