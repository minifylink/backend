package logger

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"backend/internal/lib/logger/slogdiscard"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newMW — собирает middleware с тихим логгером (для тестов, где сами логи
// не интересуют, важна только обёртка ответа).
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

// TestLoggerMiddleware_PreservesResponse — middleware не модифицирует ответ.
//
// Покрываем ДВА разных code path'а в chi.middleware.WrapResponseWriter:
//
//  1. explicit_status_and_body — next-handler ЯВНО вызывает WriteHeader(N) + Write(...).
//     Проверяет, что middleware не подменяет статус и не съедает body.
//
//  2. implicit_200 — next-handler ТОЛЬКО Write(...), без WriteHeader.
//     Это другая ветка в WrapResponseWriter (implicit-status track).
//
// Один представитель статуса (201) достаточен: middleware не зависит от значения
// статус-кода. Прежние подкейсы preserves_500 и preserves_body были представителями
// тех же двух классов и не находили дополнительных багов.
func TestLoggerMiddleware_PreservesResponse(t *testing.T) {
	t.Run("explicit_status_and_body", func(t *testing.T) {
		mw := newMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte("hello"))
		}))
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/test", nil))

		assert.Equal(t, http.StatusCreated, rr.Code, "явный статус должен сохраниться")
		assert.Equal(t, "hello", rr.Body.String(), "body должно пройти как есть")
	})

	t.Run("implicit_200", func(t *testing.T) {
		mw := newMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("ok"))
		}))
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/test", nil))

		assert.Equal(t, http.StatusOK, rr.Code, "без WriteHeader → 200 по умолчанию")
		assert.Equal(t, "ok", rr.Body.String())
	})
}

// TestLoggerMiddleware_PreservesHeaders — middleware не должен затирать
// заголовки, которые установил next-handler. Заодно использует POST-метод —
// единственный «не GET» в наборе. Метод-агностичность middleware демонстрируется
// тем, что и POST проходит, и code path'ы из других тестов работают на GET.
func TestLoggerMiddleware_PreservesHeaders(t *testing.T) {
	mw := newMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Custom-Header", "test-value")
		w.WriteHeader(http.StatusOK)
	}))
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/test", nil))

	assert.Equal(t, http.StatusOK, rr.Code, "POST-метод тоже должен пройти")
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
	assert.Equal(t, "test-value", rr.Header().Get("X-Custom-Header"))
}

// =============================================================================
// SPY-логгер: вместо тихого slogdiscard собираем записи в память,
// чтобы проверить — middleware действительно пишет то, что обещает.
// =============================================================================

// capturedLog — снимок одной записи лога: уровень + сообщение + плоская мапа атрибутов.
type capturedLog struct {
	Level   slog.Level
	Message string
	Attrs   map[string]string
}

// spyHandler — реализация slog.Handler, которая складывает записи в общий буфер.
// `records` — общий указатель: при WithAttrs/WithGroup создаётся новый handler,
// но он пишет в тот же буфер, плюс наследует атрибуты родителя.
type spyHandler struct {
	records *[]capturedLog
	attrs   []slog.Attr
}

func newSpyHandler() *spyHandler {
	return &spyHandler{records: &[]capturedLog{}}
}

func (h *spyHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *spyHandler) Handle(_ context.Context, r slog.Record) error {
	attrs := make(map[string]string)
	for _, a := range h.attrs {
		attrs[a.Key] = a.Value.String()
	}
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.String()
		return true
	})
	*h.records = append(*h.records, capturedLog{
		Level:   r.Level,
		Message: r.Message,
		Attrs:   attrs,
	})
	return nil
}

func (h *spyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	combined := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	combined = append(combined, h.attrs...)
	combined = append(combined, attrs...)
	return &spyHandler{records: h.records, attrs: combined}
}

func (h *spyHandler) WithGroup(_ string) slog.Handler { return h }

// runWithSpy — запускает middleware вокруг переданного next с заданным запросом
// и возвращает записи лога, которые middleware сделал ИМЕННО ПРИ ОБРАБОТКЕ запроса
// (запись «logger middleware enabled» при инициализации отбрасывается).
func runWithSpy(t *testing.T, req *http.Request, next http.HandlerFunc) []capturedLog {
	t.Helper()
	spy := newSpyHandler()
	log := slog.New(spy)

	mw := New(log)(next)
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	// Первая запись делается при инициализации middleware ("logger middleware enabled").
	// Дальше идут записи, относящиеся к запросу.
	all := *spy.records
	require.NotEmpty(t, all, "spy должен записать хотя бы одну строку")
	return all[1:] // пропускаем "logger middleware enabled"
}

// findRequestCompleted — находит запись с message="request completed" среди
// всех записей. Это та запись, которую middleware пишет в defer'е после next.
func findRequestCompleted(t *testing.T, logs []capturedLog) capturedLog {
	t.Helper()
	for _, l := range logs {
		if l.Message == "request completed" {
			return l
		}
	}
	t.Fatalf("запись 'request completed' не найдена; всего записей: %d", len(logs))
	return capturedLog{}
}

// TestLoggerMiddleware_LogsRequestMetadata — табличный тест по 5 атрибутам,
// которые middleware ОБЯЗАН залогировать в записи "request completed":
// method / path / status / bytes / duration. Для каждого — свой подкейс.
//
// Это критично для observability в проде: если хотя бы одно поле пропадёт,
// диагностика инцидентов сильно затруднится.
func TestLoggerMiddleware_LogsRequestMetadata(t *testing.T) {
	req := httptest.NewRequest(http.MethodPut, "/api/v1/foo", nil)
	logs := runWithSpy(t, req, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("created"))
	})
	completed := findRequestCompleted(t, logs)

	cases := []struct {
		name string
		key  string
		want string
	}{
		{"method", "method", "PUT"},
		{"path", "path", "/api/v1/foo"},
		{"status", "status", "201"},
		{"bytes", "bytes", "7"}, // длина "created"
		// duration в формате time.Duration.String() (например "12.5µs"); проверяем,
		// что атрибут есть и не пустой — конкретное число время-зависимо.
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := completed.Attrs[tc.key]
			require.Truef(t, ok, "атрибут %q должен присутствовать", tc.key)
			assert.Equal(t, tc.want, got)
		})
	}

	t.Run("duration_is_present_and_nonempty", func(t *testing.T) {
		got, ok := completed.Attrs["duration"]
		require.True(t, ok, "атрибут duration должен присутствовать")
		assert.NotEmpty(t, got, "duration не должен быть пустой строкой")
	})
}

// TestLoggerMiddleware_LogsRequestID — middleware подхватывает request ID
// из chi.middleware.RequestIDKey в контексте. Это нужно для трассировки
// одного запроса через все логи.
//
// Проверяем оба сценария:
//  1. ID есть в контексте → попадает в лог;
//  2. ID отсутствует → атрибут есть, но строка пустая (middleware не падает).
func TestLoggerMiddleware_LogsRequestID(t *testing.T) {
	t.Run("with_request_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		ctx := context.WithValue(req.Context(), chimiddleware.RequestIDKey, "trace-abc-123")
		req = req.WithContext(ctx)

		logs := runWithSpy(t, req, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		completed := findRequestCompleted(t, logs)
		assert.Equal(t, "trace-abc-123", completed.Attrs["request_id"])
	})

	t.Run("without_request_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil) // нет RequestIDKey в контексте
		logs := runWithSpy(t, req, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		completed := findRequestCompleted(t, logs)
		// Атрибут есть, но пустой. Главное — middleware не падает на отсутствии ID.
		_, ok := completed.Attrs["request_id"]
		assert.True(t, ok, "атрибут request_id должен присутствовать даже когда пустой")
		assert.Empty(t, completed.Attrs["request_id"])
	})
}

// TestLoggerMiddleware_LogsRemoteAddrAndUserAgent — security-relevant поля:
// remote_addr и user_agent попадают в лог. Без них невозможен post-mortem
// разбор подозрительной активности.
func TestLoggerMiddleware_LogsRemoteAddrAndUserAgent(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.42:54321"
	req.Header.Set("User-Agent", "TestAgent/1.0")

	logs := runWithSpy(t, req, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	completed := findRequestCompleted(t, logs)

	assert.Equal(t, "203.0.113.42:54321", completed.Attrs["remote_addr"])
	assert.Equal(t, "TestAgent/1.0", completed.Attrs["user_agent"])
}

// TestLoggerMiddleware_LogsAtInfoLevel — запись "request completed" должна
// быть на уровне INFO. Если кто-то поменяет на DEBUG, в проде логи пропадут
// (там фильтр LevelInfo, см. cmd/main.go::setupLogger). Если поменяют на
// ERROR — каждый успешный запрос будет триггерить алерт мониторинга.
func TestLoggerMiddleware_LogsAtInfoLevel(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	logs := runWithSpy(t, req, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	completed := findRequestCompleted(t, logs)

	assert.Equal(t, slog.LevelInfo, completed.Level,
		"запись о завершении запроса должна быть на уровне INFO")
}
