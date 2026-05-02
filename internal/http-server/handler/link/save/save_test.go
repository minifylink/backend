package save_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"backend/internal/http-server/handler/link/save"
	"backend/internal/http-server/handler/link/save/mocks"
	"backend/internal/lib/logger/slogdiscard"
)

// newHandler — общий конструктор для unit-тестов сохранения.
func newHandler(t *testing.T) (http.HandlerFunc, *mocks.LinkSaver) {
	t.Helper()
	mock := mocks.NewLinkSaver(t)
	return save.New(slogdiscard.NewDiscardLogger(), mock), mock
}

// doSave — отправляет POST /save с заданным телом и возвращает (статус, тело).
func doSave(t *testing.T, handler http.HandlerFunc, body []byte) (int, save.Response) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/save", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var resp save.Response
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	return rr.Code, resp
}

func mustJSON(link, shortID string) []byte {
	return []byte(fmt.Sprintf(`{"link": %q, "short_id": %q}`, link, shortID))
}

// TestSaveHandler_TableDriven — основной table-driven с применением:
//   - класса эквивалентности (валидный/невалидный URL/short_id),
//   - граничных условий (пустые поля, невалидный JSON, отсутствующее тело),
//   - негативных сценариев (ошибки от LinkSaver).
//
// Для каждого case проверяются ВСЕ значимые поля ответа:
// статус-код, status="OK"/"Error", error-сообщение, short_id (для успеха).
func TestSaveHandler_TableDriven(t *testing.T) {
	cases := []struct {
		name           string
		alias          string
		url            string
		rawBody        *string
		mockError      error
		expectMockCall bool
		wantStatus     string
		wantErrSubstr  string // для ошибок проверяем substring (не хардкодим английский хвост валидатора)
		wantShortID    string
	}{
		{
			name:           "Success",
			alias:          "ok_alias",
			url:            "https://google.com",
			expectMockCall: true,
			wantStatus:     "OK",
			wantShortID:    "ok_alias",
		},
		{
			name:           "SaveURL Error",
			alias:          "test_alias",
			url:            "https://google.com",
			mockError:      errors.New("unexpected error"),
			expectMockCall: true,
			wantStatus:     "Error",
			wantErrSubstr:  "failed to add link",
		},
		{
			name:           "ShortID Already Exists",
			alias:          "existing_alias",
			url:            "https://google.com",
			mockError:      errors.New("repository.SaveLink: short id already exists"),
			expectMockCall: true,
			wantStatus:     "Error",
			wantErrSubstr:  "shortID already exists",
		},
		{
			name:          "Empty alias",
			alias:         "",
			url:           "https://google.com",
			wantStatus:    "Error",
			wantErrSubstr: "shortID cannot be empty",
		},
		{
			name:          "Empty URL",
			url:           "",
			alias:         "some_alias",
			wantStatus:    "Error",
			wantErrSubstr: "field Link", // имя поля стабильно; английский хвост валидатора не хардкодим
		},
		{
			name:          "Invalid URL",
			url:           "some invalid URL",
			alias:         "some_alias",
			wantStatus:    "Error",
			wantErrSubstr: "field Link",
		},
		{
			name:          "Empty Body",
			rawBody:       strPtr(""),
			wantStatus:    "Error",
			wantErrSubstr: "empty request",
		},
		{
			name:          "Invalid JSON",
			rawBody:       strPtr("{invalid}"),
			wantStatus:    "Error",
			wantErrSubstr: "failed to decode",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			handler, mock := newHandler(t)
			if tc.expectMockCall {
				mock.On("SaveLink", tc.url, tc.alias).Return(tc.mockError).Once()
			}

			var body []byte
			if tc.rawBody != nil {
				body = []byte(*tc.rawBody)
			} else {
				body = mustJSON(tc.url, tc.alias)
			}

			code, resp := doSave(t, handler, body)

			require.Equal(t, http.StatusOK, code,
				"API всегда отдаёт 200; см. примечание про антипаттерн REST в README")
			assert.Equal(t, tc.wantStatus, resp.Status)

			if tc.wantErrSubstr != "" {
				assert.Contains(t, resp.Error, tc.wantErrSubstr,
					"ожидаем подстроку %q в error", tc.wantErrSubstr)
			} else {
				assert.Empty(t, resp.Error)
			}

			if tc.wantShortID != "" {
				assert.Equal(t, tc.wantShortID, resp.ShortID)
			}
		})
	}
}

func strPtr(s string) *string { return &s }

// TestSaveHandler_Success_FullResponse — позитивный: проверяем сразу status, error и short_id.
func TestSaveHandler_Success_FullResponse(t *testing.T) {
	handler, mock := newHandler(t)
	mock.On("SaveLink", "https://example.com", "myid").Return(nil).Once()

	code, resp := doSave(t, handler, mustJSON("https://example.com", "myid"))

	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "OK", resp.Status)
	assert.Empty(t, resp.Error)
	assert.Equal(t, "myid", resp.ShortID)
}

// TestSaveHandler_ShortIDBoundary_VARCHAR20 — BVA для длины short_id,
// привязанная к РЕАЛЬНОМУ ограничению БД: schema `links.short_id VARCHAR(20)`.
//
// Хэндлер сам длину не валидирует — поэтому unit-тест проверяет
// только то, что значение пробрасывается в LinkSaver без модификации.
// Что именно происходит при длине > 20 — задача integration-теста
// `TestIntegration_SaveLink_ShortIDBoundary_VARCHAR20`.
func TestSaveHandler_ShortIDBoundary_VARCHAR20(t *testing.T) {
	cases := []struct {
		name   string
		length int
	}{
		{"len=19_below_limit", 19},
		{"len=20_at_limit", 20},
		{"len=21_above_limit", 21},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			id := strings.Repeat("x", tc.length)
			handler, mock := newHandler(t)
			mock.On("SaveLink", "https://example.com", id).Return(nil).Once()

			code, resp := doSave(t, handler, mustJSON("https://example.com", id))

			assert.Equal(t, http.StatusOK, code)
			assert.Equal(t, "OK", resp.Status,
				"unit-уровень: длина не валидируется хэндлером, ожидаем успех")
			assert.Equal(t, id, resp.ShortID)
		})
	}
}

// TestSaveHandler_VeryLongURL — URL длиной 2000+ символов. Хэндлер длину URL
// не лимитирует; проверяем, что full-response корректен.
//
// NB: 2000 — взятая с потолка длина. Если в FS появится явный лимит на URL,
// этот тест надо переделать как BVA по этому лимиту (N-1, N, N+1).
func TestSaveHandler_VeryLongURL(t *testing.T) {
	longURL := "https://example.com/" + strings.Repeat("a", 2000)
	handler, mock := newHandler(t)
	mock.On("SaveLink", longURL, "longurl").Return(nil).Once()

	code, resp := doSave(t, handler, mustJSON(longURL, "longurl"))

	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "OK", resp.Status)
	assert.Empty(t, resp.Error)
	assert.Equal(t, "longurl", resp.ShortID)
}

// TestSaveHandler_AcceptsValidShortIDFormats — валидные short_id: ASCII со спецсимволами и юникод.
func TestSaveHandler_AcceptsValidShortIDFormats(t *testing.T) {
	cases := []struct {
		name string
		id   string
	}{
		{"ascii_with_dash_underscore_digits", "test-alias_123"},
		{"unicode_cyrillic", "тест"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			handler, mock := newHandler(t)
			mock.On("SaveLink", "https://example.com", tc.id).Return(nil).Once()

			code, resp := doSave(t, handler, mustJSON("https://example.com", tc.id))

			assert.Equal(t, http.StatusOK, code)
			assert.Equal(t, "OK", resp.Status)
			assert.Equal(t, tc.id, resp.ShortID)
		})
	}
}

// TestSaveHandler_AcceptsValidURLFormats — http/query/fragment — все валидны для validator.url.
func TestSaveHandler_AcceptsValidURLFormats(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{"http_no_tls", "http://example.com"},
		{"with_query", "https://example.com/path?a=1&b=2"},
		{"with_fragment", "https://example.com/page#section"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			handler, mock := newHandler(t)
			mock.On("SaveLink", tc.url, "vid").Return(nil).Once()

			code, resp := doSave(t, handler, mustJSON(tc.url, "vid"))

			assert.Equal(t, http.StatusOK, code)
			assert.Equal(t, "OK", resp.Status)
			assert.Empty(t, resp.Error)
		})
	}
}

// TestSaveHandler_FTPUrl_IsAccepted — validator.url принимает ftp://.
// Если появится политика «только http/https» — тест инвертировать.
func TestSaveHandler_FTPUrl_IsAccepted(t *testing.T) {
	handler, mock := newHandler(t)
	mock.On("SaveLink", "ftp://example.com", "ftp_id").Return(nil).Once()

	code, resp := doSave(t, handler, mustJSON("ftp://example.com", "ftp_id"))

	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "OK", resp.Status, "ftp:// сейчас принимается валидатором")
	assert.Empty(t, resp.Error)
	assert.Equal(t, "ftp_id", resp.ShortID)
}

// TestSaveHandler_ContentType_IsIgnored — render.DecodeJSON парсит тело независимо
// от Content-Type: text/plain с JSON в теле → успешно сохраняем.
func TestSaveHandler_ContentType_IsIgnored(t *testing.T) {
	handler, mock := newHandler(t)
	mock.On("SaveLink", "https://example.com", "ctid").Return(nil).Once()

	body := mustJSON("https://example.com", "ctid")
	req := httptest.NewRequest(http.MethodPost, "/save", bytes.NewReader(body))
	req.Header.Set("Content-Type", "text/plain")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var resp save.Response
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "OK", resp.Status, "Content-Type должен игнорироваться при разборе тела")
	assert.Empty(t, resp.Error)
}

// TestSaveHandler_MissingFields — отсутствие обязательных полей в JSON: link и short_id.
func TestSaveHandler_MissingFields(t *testing.T) {
	cases := []struct {
		name          string
		body          string
		wantErrSubstr string
	}{
		{
			name:          "missing_link",
			body:          `{"short_id": "abc"}`,
			wantErrSubstr: "field Link", // имя поля стабильно; не хардкодим хвост
		},
		{
			name:          "missing_short_id",
			body:          `{"link": "https://example.com"}`,
			wantErrSubstr: "shortID cannot be empty",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			handler, _ := newHandler(t)

			code, resp := doSave(t, handler, []byte(tc.body))

			assert.Equal(t, http.StatusOK, code)
			assert.Equal(t, "Error", resp.Status)
			assert.Contains(t, resp.Error, tc.wantErrSubstr)
		})
	}
}

// TestSaveHandler_ExtraFields — лишние поля в JSON игнорируются (decoder
// пропускает их). Mock явно проверяет, что в SaveLink ушли ровно (link, short_id),
// а не что-то ещё.
func TestSaveHandler_ExtraFields(t *testing.T) {
	handler, mock := newHandler(t)
	mock.On("SaveLink", "https://example.com", "extra").Return(nil).Once()

	body := []byte(`{"link": "https://example.com", "short_id": "extra", "foo": "bar", "baz": 123}`)
	code, resp := doSave(t, handler, body)

	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "OK", resp.Status)
	assert.Equal(t, "extra", resp.ShortID)
}

// TestSaveHandler_NullBody — POST без тела (nil reader) → "empty request".
func TestSaveHandler_NullBody(t *testing.T) {
	handler, _ := newHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/save", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var resp save.Response
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))

	assert.Equal(t, "Error", resp.Status)
	assert.Contains(t, resp.Error, "empty")
}
