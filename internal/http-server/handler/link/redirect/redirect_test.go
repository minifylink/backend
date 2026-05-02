package redirect

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
func newRouter(t *testing.T, opts ...Option) (*chi.Mux, *mocks.LinkGetter) {
	t.Helper()
	mock := mocks.NewLinkGetter(t)
	r := chi.NewRouter()
	r.Get("/{minilink}", New(slogdiscard.NewDiscardLogger(), mock, opts...))
	return r, mock
}

// TestRedirectHandler_HappyPath — позитивный путь через настоящий httptest-сервер.
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

// TestRedirectHandler_PairwiseDeviceBrowser — pairwise device×browser (2×3 = 6 пар).
// Mock проверяет, что хэндлер распознал пару из User-Agent и передал её в LinkGetter.
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
			var opts []Option
			if tc.countryGetter != nil {
				opts = append(opts, WithCountryGetter(tc.countryGetter))
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
	handler := New(slogdiscard.NewDiscardLogger(), nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var respBody map[string]interface{}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &respBody))
	assert.Equal(t, "Error", respBody["status"])
	assert.Equal(t, "invalid request", respBody["error"])
}

// TestRedirectHandler_StatusFoundAndLocation — sanity: 302 + Location в одной проверке.
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

// TestRedirectHandler_PreservesURLAsIs — http/https/URL с query+fragment проходят без модификации.
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

	r, mock := newRouter(t, WithCountryGetter(countryFn))
	mock.On("GetLink", alias, "TestCountry", "desktop", "").
		Return("https://example.com", nil).Once()

	req := httptest.NewRequest(http.MethodGet, "/"+alias, nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusFound, rr.Code)
}

// TestRedirectHandler_AliasBoundary_VARCHAR20 — BVA 19/20/21 по границе links.short_id VARCHAR(20).
// Хэндлер длину не валидирует — все три проходят; реальное отклонение проверяется в integration-тесте.
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

// TestIsPrivateIP — табличный тест по 6 классам эквивалентности.
// Принцип EP: один представитель на класс. Дополнительные представители
// внутри класса не находят новых багов.
func TestIsPrivateIP(t *testing.T) {
	cases := []struct {
		name string
		ip   string
		want bool
	}{
		// EQ_invalid: парсер net.ParseIP вернёт nil → функция возвращает false
		{"invalid", "garbage", false},
		// EQ_private_v4: попадает под net.IP.IsPrivate() (RFC 1918)
		{"private_v4", "192.168.1.1", true},
		// EQ_loopback_v4: попадает под net.IP.IsLoopback() (127.0.0.0/8)
		{"loopback_v4", "127.0.0.1", true},
		// EQ_loopback_v6: граничный случай — IPv6 loopback (::1)
		{"loopback_v6", "::1", true},
		// EQ_private_v6: IPv6 ULA (fc00::/7) — net.IP.IsPrivate() тоже возвращает true
		// для этой подсети. Отдельный класс: внутри стандартной библиотеки это
		// другая ветка проверки (RFC 4193), не RFC 1918.
		{"private_v6", "fd00::1", true},
		// EQ_public: ни IsPrivate, ни IsLoopback — возвращаем false
		{"public", "8.8.8.8", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isPrivateIP(tc.ip))
		})
	}
}

// TestIsPrivateIP_RangeBoundaries — BVA по границам приватных диапазонов
// (RFC 1918: 10/8, 172.16/12, 192.168/16; RFC 1122: 127/8). Для каждого — below / at / above.
func TestIsPrivateIP_RangeBoundaries(t *testing.T) {
	cases := []struct {
		name string
		ip   string
		want bool
	}{
		// boundary: 10.0.0.0/8
		{"10/8_below_9.255.255.255", "9.255.255.255", false},
		{"10/8_at_lower_10.0.0.0", "10.0.0.0", true},
		{"10/8_at_upper_10.255.255.255", "10.255.255.255", true},
		{"10/8_above_11.0.0.0", "11.0.0.0", false},

		// boundary: 172.16.0.0/12 (172.16.0.0 .. 172.31.255.255)
		{"172.16/12_below_172.15.255.255", "172.15.255.255", false},
		{"172.16/12_at_lower_172.16.0.0", "172.16.0.0", true},
		{"172.16/12_at_upper_172.31.255.255", "172.31.255.255", true},
		{"172.16/12_above_172.32.0.0", "172.32.0.0", false},

		// boundary: 192.168.0.0/16
		{"192.168/16_below_192.167.255.255", "192.167.255.255", false},
		{"192.168/16_at_lower_192.168.0.0", "192.168.0.0", true},
		{"192.168/16_at_upper_192.168.255.255", "192.168.255.255", true},
		{"192.168/16_above_192.169.0.0", "192.169.0.0", false},

		// boundary: 127.0.0.0/8 (loopback)
		{"127/8_below_126.255.255.255", "126.255.255.255", false},
		{"127/8_at_lower_127.0.0.0", "127.0.0.0", true},
		{"127/8_at_upper_127.255.255.255", "127.255.255.255", true},
		{"127/8_above_128.0.0.0", "128.0.0.0", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isPrivateIP(tc.ip),
				"граница диапазона: ip=%s ожидаем=%v", tc.ip, tc.want)
		})
	}
}

// Tests for getIP

func TestGetIP_XRealIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Real-IP", "203.0.113.1")

	ip := getIP(req)
	assert.Equal(t, "203.0.113.1", ip)
}

// EQ_xff_pick_first: XFF из нескольких IP — берётся первый.
func TestGetIP_XForwardedFor_Multiple(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "198.51.100.1, 10.0.0.1, 172.16.0.1")

	ip := getIP(req)
	assert.Equal(t, "198.51.100.1", ip)
}

func TestGetIP_XForwardedFor_WithSpaces(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "  198.51.100.1 , 10.0.0.1")

	ip := getIP(req)
	assert.Equal(t, "198.51.100.1", ip)
}

// TestGetIP_XForwardedFor_EmptyFirstFallsBackToRemoteAddr — XFF задан, но первый элемент
// пустой (например, ",10.0.0.1") → fallthrough к RemoteAddr.
func TestGetIP_XForwardedFor_EmptyFirstFallsBackToRemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", ",10.0.0.1") // первый элемент пустой
	req.RemoteAddr = "203.0.113.99:54321"

	ip := getIP(req)
	assert.Equal(t, "203.0.113.99", ip,
		"при пустом первом элементе XFF должен быть fallback к RemoteAddr")
}

func TestGetIP_RemoteAddr_WithPort(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"

	ip := getIP(req)
	assert.Equal(t, "10.0.0.1", ip)
}

func TestGetIP_RemoteAddr_NoPort(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1"

	ip := getIP(req)
	assert.Equal(t, "10.0.0.1", ip)
}

func TestGetIP_Priority(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Real-IP", "1.1.1.1")
	req.Header.Set("X-Forwarded-For", "2.2.2.2")
	req.RemoteAddr = "3.3.3.3:1234"

	ip := getIP(req)
	assert.Equal(t, "1.1.1.1", ip, "X-Real-IP should take precedence")
}

func TestGetIP_EmptyHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = ""

	ip := getIP(req)
	assert.Equal(t, "", ip)
}

// Tests for getCountry

// EQ_isPrivateIP_true: ранний возврат "local" без вызова fetcher.
// Подклассы IsPrivate/IsLoopback уже покрыты в TestIsPrivateIP.
func TestGetCountry_PrivateIP(t *testing.T) {
	country, err := getCountry("192.168.1.1")
	require.NoError(t, err)
	assert.Equal(t, "local", country)
}

// fakeCountryFetcher — тестовая подмена CountryFetcher: фиксирует факт вызова
// и возвращает заранее заданные значения. Используется через подмену пакетной
// переменной countryFetcher с восстановлением через t.Cleanup.
type fakeCountryFetcher struct {
	result string
	err    error
	calls  int
	lastIP string
}

func (f *fakeCountryFetcher) Fetch(ip string) (string, error) {
	f.calls++
	f.lastIP = ip
	return f.result, f.err
}

// TestGetCountry_NonPrivate — нелокальный путь делегирует CountryFetcher;
// приватный IP — ранний возврат без вызова fetcher.
func TestGetCountry_NonPrivate(t *testing.T) {
	cases := []struct {
		name      string
		ip        string
		fake      *fakeCountryFetcher
		want      string
		wantErr   bool
		wantCalls int
	}{
		{
			name:      "Success",
			ip:        "8.8.8.8",
			fake:      &fakeCountryFetcher{result: "USA"},
			want:      "USA",
			wantErr:   false,
			wantCalls: 1,
		},
		{
			name:      "Error",
			ip:        "8.8.8.8",
			fake:      &fakeCountryFetcher{err: errors.New("network down")},
			want:      "",
			wantErr:   true,
			wantCalls: 1,
		},
		{
			name:      "Private-IP-doesnt-call-fetcher",
			ip:        "192.168.1.1",
			fake:      &fakeCountryFetcher{},
			want:      "local",
			wantErr:   false,
			wantCalls: 0,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			old := countryFetcher
			countryFetcher = tc.fake
			t.Cleanup(func() { countryFetcher = old })

			got, err := getCountry(tc.ip)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.want, got)
			}
			assert.Equal(t, tc.wantCalls, tc.fake.calls,
				"количество вызовов fetcher.Fetch")
			if tc.wantCalls > 0 {
				assert.Equal(t, tc.ip, tc.fake.lastIP,
					"fetcher должен получить ровно тот ip, который передан в getCountry")
			}
		})
	}
}
