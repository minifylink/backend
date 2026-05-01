package logger

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/internal/lib/logger/slogdiscard"

	"github.com/stretchr/testify/assert"
)

// newMW — собирает middleware с тихим логгером.
func newMW(next http.Handler) http.Handler {
	return New(slogdiscard.NewDiscardLogger())(next)
}

// TestLoggerMiddleware_CallsNext — middleware обязан передавать управление дальше.
func TestLoggerMiddleware_CallsNext(t *testing.T) {
	called := false
	mw := newMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	mw.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/test", nil))

	assert.True(t, called, "middleware должен вызвать next.ServeHTTP")
}

// TestLoggerMiddleware_PreservesResponse — middleware не модифицирует ответ:
//   - тело передаётся как есть,
//   - status code сохраняется (для явного WriteHeader),
//   - default 200 без явного WriteHeader.
//
// Заменяет три отдельных теста (`_PreservesBody`, `_PreservesStatusCode`,
// `_DefaultStatusCode`) одним набором, у которого общая суть — «ничего не ломаем».
func TestLoggerMiddleware_PreservesResponse(t *testing.T) {
	cases := []struct {
		name       string
		writer     func(w http.ResponseWriter)
		wantBody   string
		wantStatus int
	}{
		{
			name:       "preserves_body",
			writer:     func(w http.ResponseWriter) { _, _ = w.Write([]byte("hello")) },
			wantBody:   "hello",
			wantStatus: http.StatusOK, // implicit 200 при первом Write
		},
		{
			name:       "preserves_explicit_201",
			writer:     func(w http.ResponseWriter) { w.WriteHeader(http.StatusCreated) },
			wantStatus: http.StatusCreated,
		},
		{
			name:       "preserves_500",
			writer:     func(w http.ResponseWriter) { w.WriteHeader(http.StatusInternalServerError) },
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "default_200_without_writeheader",
			writer:     func(w http.ResponseWriter) { _, _ = w.Write([]byte("ok")) },
			wantBody:   "ok",
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			mw := newMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tc.writer(w)
			}))
			rr := httptest.NewRecorder()
			mw.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/test", nil))

			assert.Equal(t, tc.wantStatus, rr.Code)
			if tc.wantBody != "" {
				assert.Equal(t, tc.wantBody, rr.Body.String())
			}
		})
	}
}

// TestLoggerMiddleware_HTTPMethodAgnostic — middleware должен корректно работать
// со ВСЕМИ HTTP-методами, не только GET и POST. Прежние тесты `_GET` / `_POST`
// проверяли одну и ту же логику дважды, не покрывая остальных методов.
func TestLoggerMiddleware_HTTPMethodAgnostic(t *testing.T) {
	methods := []string{
		http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete,
		http.MethodPatch, http.MethodHead, http.MethodOptions,
	}
	for _, m := range methods {
		m := m
		t.Run(m, func(t *testing.T) {
			mw := newMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			rr := httptest.NewRecorder()
			mw.ServeHTTP(rr, httptest.NewRequest(m, "/test", nil))
			assert.Equal(t, http.StatusOK, rr.Code,
				"middleware должен быть метод-агностичным")
		})
	}
}
