package redirect_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"backend/internal/http-server/handler/link/redirect"
	"backend/internal/http-server/handler/link/redirect/mocks"
	"backend/internal/lib/api"
	"backend/internal/lib/logger/slogdiscard"
)

// User-Agents, для которых mssola/user_agent гарантированно даёт известную пару (device, browser).
const (
	uaDesktopChrome  = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	uaDesktopFirefox = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:120.0) Gecko/20100101 Firefox/120.0"
	uaDesktopSafari  = "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15"
	uaMobileChrome   = "Mozilla/5.0 (Linux; Android 13; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36"
	uaMobileFirefox  = "Mozilla/5.0 (Android 13; Mobile; rv:120.0) Gecko/120.0 Firefox/120.0"
	uaMobileSafari   = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1"
)

// newRouter — общий конструктор для unit-тестов хэндлера.
// Возвращает router и mock, чтобы тесту не приходилось дублировать setup.
func newRouter(t *testing.T, opts ...redirect.Option) (*chi.Mux, *mocks.LinkGetter) {
	t.Helper()
	mock := mocks.NewLinkGetter(t)
	r := chi.NewRouter()
	r.Get("/{minilink}", redirect.New(slogdiscard.NewDiscardLogger(), mock, opts...))
	return r, mock
}

// TestRedirectHandler_HappyPath — позитивный сценарий: mock отдаёт URL, хэндлер
// возвращает 302 + правильный Location. Заменяет прежний `TestSaveHandler`,
// у которого было путаное имя (тестировал redirect, но назывался Save).
func TestRedirectHandler_HappyPath(t *testing.T) {
	const alias = "happy"
	const target = "https://www.google.com/"

	r, mock := newRouter(t)
	mock.On("GetLink", alias, "local", "desktop", "Go-http-client").
		Return(target, nil).Once()

	ts := httptest.NewServer(r)
	defer ts.Close()

	location, err := api.GetRedirect(ts.URL + "/" + alias)
	require.NoError(t, err)
	assert.Equal(t, target, location)
}

// TestRedirectHandler_PairwiseDeviceBrowser — настоящий pairwise по двум параметрам:
// device ∈ {desktop, mobile} и browser ∈ {Chrome, Firefox, Safari}.
// Полная сетка: 2×3 = 6 пар; pairwise здесь совпадает с полным перебором.
//
// Цель — убедиться, что для ЛЮБОЙ пары (device, browser) хэндлер
//  1. распознаёт их корректно из User-Agent,
//  2. передаёт ровно эти значения в LinkGetter,
//  3. отвечает 302 с переданным Location.
func TestRedirectHandler_PairwiseDeviceBrowser(t *testing.T) {
	cases := []struct {
		name    string
		ua      string
		device  string
		browser string
	}{
		{"desktop+Chrome", uaDesktopChrome, "desktop", "Chrome"},
		{"desktop+Firefox", uaDesktopFirefox, "desktop", "Firefox"},
		{"desktop+Safari", uaDesktopSafari, "desktop", "Safari"},
		{"mobile+Chrome", uaMobileChrome, "mobile", "Chrome"},
		{"mobile+Firefox", uaMobileFirefox, "mobile", "Firefox"},
		{"mobile+Safari", uaMobileSafari, "mobile", "Safari"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			alias := "pw_" + tc.device + "_" + tc.browser
			r, mock := newRouter(t)
			mock.On("GetLink", alias, "local", tc.device, tc.browser).
				Return("https://example.com", nil).Once()

			req := httptest.NewRequest(http.MethodGet, "/"+alias, nil)
			req.Header.Set("User-Agent", tc.ua)
			req.Header.Set("X-Real-IP", "127.0.0.1")

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			assert.Equal(t, http.StatusFound, rr.Code)
			assert.Equal(t, "https://example.com", rr.Header().Get("Location"))
			// mock.AssertExpectations вызывается в finalizer'е mocks.NewLinkGetter(t):
			// если хэндлер вызовет GetLink с другими параметрами,
			// тест упадёт с явным сообщением о mismatch.
		})
	}
}

// TestRedirectHandler_HandlerErrors — ошибочные пути обработчика
// (link not found / internal error / geo error).
func TestRedirectHandler_HandlerErrors(t *testing.T) {
	cases := []struct {
		name          string
		alias         string
		mockError     error
		respError     string
		headerKey     string
		headerVal     string
		countryGetter func(string) (string, error)
		mockCountry   string
	}{
		{
			name:        "Link Not Found",
			alias:       "not_exists",
			mockError:   errors.New("repository.GetLink: link not found"),
			respError:   "not found",
			headerKey:   "X-Real-IP",
			headerVal:   "127.0.0.1",
			mockCountry: "local",
		},
		{
			name:        "Internal Error",
			alias:       "test_alias",
			mockError:   errors.New("unexpected error"),
			respError:   "internal error",
			headerKey:   "X-Forwarded-For",
			headerVal:   "127.0.0.1",
			mockCountry: "local",
		},
		{
			name:      "Country Getter Error",
			alias:     "test_alias",
			mockError: errors.New("unexpected error"),
			respError: "internal error",
			countryGetter: func(ip string) (string, error) {
				return "", errors.New("geo error")
			},
			mockCountry: "unknown",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var opts []redirect.Option
			if tc.countryGetter != nil {
				opts = append(opts, redirect.WithCountryGetter(tc.countryGetter))
			}
			r, mock := newRouter(t, opts...)
			mock.On("GetLink", tc.alias, tc.mockCountry, "desktop", "").
				Return("", tc.mockError).Once()

			req := httptest.NewRequest(http.MethodGet, "/"+tc.alias, nil)
			if tc.headerKey != "" {
				req.Header.Set(tc.headerKey, tc.headerVal)
			}
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			var respBody map[string]interface{}
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &respBody))
			assert.Equal(t, "Error", respBody["status"])
			assert.Equal(t, tc.respError, respBody["error"])
		})
	}
}

// TestRedirectHandler_EmptyAlias — обращение без minilink-параметра идёт мимо роутера.
// Проверяем, что хэндлер сам отдаёт корректную ошибку.
func TestRedirectHandler_EmptyAlias(t *testing.T) {
	handler := redirect.New(slogdiscard.NewDiscardLogger(), nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var respBody map[string]interface{}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &respBody))
	assert.Equal(t, "Error", respBody["status"])
	assert.Equal(t, "invalid request", respBody["error"])
}

// TestRedirectHandler_StatusFoundAndLocation — единый позитивный sanity-тест,
// объединяющий прежние `_ReturnsStatusFound` и `_LocationHeader`.
// Один тест → один сценарий → две связанные проверки.
func TestRedirectHandler_StatusFoundAndLocation(t *testing.T) {
	const alias = "loc"
	const target = "https://target.com/path"

	r, mock := newRouter(t)
	mock.On("GetLink", alias, "local", "desktop", "Go-http-client").
		Return(target, nil).Once()

	ts := httptest.NewServer(r)
	defer ts.Close()

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(ts.URL + "/" + alias)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusFound, resp.StatusCode)
	assert.Equal(t, target, resp.Header.Get("Location"))
}

// TestRedirectHandler_PreservesURLAsIs — хэндлер не трогает полученный из БД URL.
// Заменяет прежние `_LinkGetterReturnsHTTP` и `_LinkGetterReturnsHTTPS`,
// которые с точки зрения хэндлера принадлежали ОДНОМУ классу эквивалентности
// (хэндлер протокол не различает).
//
// Здесь проверяется, что:
//  1. http и https одинаково пробрасываются;
//  2. сложные URL (query, fragment, путь) не модифицируются.
func TestRedirectHandler_PreservesURLAsIs(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{"http", "http://insecure.example.com"},
		{"https", "https://secure.example.com"},
		{"https-with-query-and-fragment", "https://example.com/path?a=1&b=2#section"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			alias := "preserve_" + tc.name
			r, mock := newRouter(t)
			mock.On("GetLink", alias, "local", "desktop", "").
				Return(tc.url, nil).Once()

			req := httptest.NewRequest(http.MethodGet, "/"+alias, nil)
			req.Header.Set("X-Real-IP", "127.0.0.1")
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			assert.Equal(t, http.StatusFound, rr.Code)
			assert.Equal(t, tc.url, rr.Header().Get("Location"))
		})
	}
}

// TestRedirectHandler_CustomCountryGetter — Option-паттерн `WithCountryGetter`
// действительно подменяет функцию определения страны,
// и значение, которое она возвращает, доходит до LinkGetter.
func TestRedirectHandler_CustomCountryGetter(t *testing.T) {
	const alias = "opt"
	countryFn := func(ip string) (string, error) { return "TestCountry", nil }

	r, mock := newRouter(t, redirect.WithCountryGetter(countryFn))
	mock.On("GetLink", alias, "TestCountry", "desktop", "").
		Return("https://example.com", nil).Once()

	req := httptest.NewRequest(http.MethodGet, "/"+alias, nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusFound, rr.Code)
}

// TestRedirectHandler_AliasBoundary_VARCHAR20 — граничные условия по длине alias,
// привязанные к РЕАЛЬНОМУ ограничению БД: schema `links.short_id VARCHAR(20)`.
// Хэндлер сам длину не валидирует, поэтому unit-тест проверяет, что для alias'ов
// длиной 19 / 20 / 21 хэндлер одинаково пробрасывает их в LinkGetter без модификации.
// Реальное отклонение длиннее 20 проверяется в integration-тесте на репозитории.
func TestRedirectHandler_AliasBoundary_VARCHAR20(t *testing.T) {
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
			alias := strings.Repeat("a", tc.length)
			r, mock := newRouter(t)
			mock.On("GetLink", alias, "local", "desktop", "").
				Return("https://example.com", nil).Once()

			req := httptest.NewRequest(http.MethodGet, "/"+alias, nil)
			req.Header.Set("X-Real-IP", "127.0.0.1")
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			assert.Equal(t, http.StatusFound, rr.Code,
				"хэндлер не валидирует длину; ожидаем 302 для всех трёх случаев")
		})
	}
}

// TestRedirectHandler_SpecialCharsAlias — alias с допустимыми по chi-роутеру символами.
func TestRedirectHandler_SpecialCharsAlias(t *testing.T) {
	const alias = "my-link_123"
	r, mock := newRouter(t)
	mock.On("GetLink", alias, "local", "desktop", "").
		Return("https://example.com", nil).Once()

	req := httptest.NewRequest(http.MethodGet, "/"+alias, nil)
	req.Header.Set("X-Real-IP", "127.0.0.1")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusFound, rr.Code)
}

// TestRedirectHandler_EmptyUserAgent — отсутствие UA: device по умолчанию = "desktop",
// browser = "" (пустая строка). Граничный случай для парсера.
func TestRedirectHandler_EmptyUserAgent(t *testing.T) {
	const alias = "noua"
	r, mock := newRouter(t)
	mock.On("GetLink", alias, "local", "desktop", "").
		Return("https://example.com", nil).Once()

	req := httptest.NewRequest(http.MethodGet, "/"+alias, nil)
	req.Header.Set("X-Real-IP", "127.0.0.1")
	req.Header.Del("User-Agent")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusFound, rr.Code)
}
