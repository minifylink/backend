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

func TestSaveHandler(t *testing.T) {
	cases := []struct {
		name      string
		alias     string
		url       string
		respError string
		mockError error
	}{
		{
			name:  "Success",
			alias: "test_alias",
			url:   "https://www.google.com/",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			linkGetterMock := mocks.NewLinkGetter(t)

			if tc.respError == "" || tc.mockError != nil {
				linkGetterMock.On("GetLink", tc.alias, "local", "desktop", "Go-http-client").
					Return(tc.url, tc.mockError).Once()
			}

			r := chi.NewRouter()
			r.Get("/{minilink}", redirect.New(slogdiscard.NewDiscardLogger(), linkGetterMock))

			ts := httptest.NewServer(r)
			defer ts.Close()

			redirectedToURL, err := api.GetRedirect(ts.URL + "/" + tc.alias)
			require.NoError(t, err)

			// Check the final URL after redirection.
			assert.Equal(t, tc.url, redirectedToURL)
		})
	}
}

func TestRedirectHandlerErrors(t *testing.T) {
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
		t.Run(tc.name, func(t *testing.T) {
			linkGetterMock := mocks.NewLinkGetter(t)
			linkGetterMock.On("GetLink", tc.alias, tc.mockCountry, "desktop", "").
				Return("", tc.mockError).Once()

			var opts []redirect.Option
			if tc.countryGetter != nil {
				opts = append(opts, redirect.WithCountryGetter(tc.countryGetter))
			}

			r := chi.NewRouter()
			r.Get("/{minilink}", redirect.New(slogdiscard.NewDiscardLogger(), linkGetterMock, opts...))

			req := httptest.NewRequest(http.MethodGet, "/"+tc.alias, nil)
			if tc.headerKey != "" {
				req.Header.Set(tc.headerKey, tc.headerVal)
			}

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			var respBody map[string]interface{}
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &respBody))
			assert.Equal(t, tc.respError, respBody["error"])
		})
	}
}

func TestRedirectHandlerEmptyAlias(t *testing.T) {
	handler := redirect.New(slogdiscard.NewDiscardLogger(), nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var respBody map[string]interface{}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &respBody))
	assert.Equal(t, "invalid request", respBody["error"])
}

// New tests

func TestRedirectHandler_ReturnsStatusFound(t *testing.T) {
	mock := mocks.NewLinkGetter(t)
	mock.On("GetLink", "abc", "local", "desktop", "Go-http-client").
		Return("https://example.com", nil).Once()

	r := chi.NewRouter()
	r.Get("/{minilink}", redirect.New(slogdiscard.NewDiscardLogger(), mock))

	ts := httptest.NewServer(r)
	defer ts.Close()

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	resp, err := client.Get(ts.URL + "/abc")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusFound, resp.StatusCode)
}

func TestRedirectHandler_LocationHeader(t *testing.T) {
	mock := mocks.NewLinkGetter(t)
	mock.On("GetLink", "loc", "local", "desktop", "Go-http-client").
		Return("https://target.com/path", nil).Once()

	r := chi.NewRouter()
	r.Get("/{minilink}", redirect.New(slogdiscard.NewDiscardLogger(), mock))

	ts := httptest.NewServer(r)
	defer ts.Close()

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	resp, err := client.Get(ts.URL + "/loc")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, "https://target.com/path", resp.Header.Get("Location"))
}

func TestRedirectHandler_DesktopDevice(t *testing.T) {
	mock := mocks.NewLinkGetter(t)
	mock.On("GetLink", "dev", "local", "desktop", "Chrome").
		Return("https://example.com", nil).Once()

	r := chi.NewRouter()
	r.Get("/{minilink}", redirect.New(slogdiscard.NewDiscardLogger(), mock))

	req := httptest.NewRequest(http.MethodGet, "/dev", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("X-Real-IP", "127.0.0.1")

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusFound, rr.Code)
}

func TestRedirectHandler_MobileDevice(t *testing.T) {
	mock := mocks.NewLinkGetter(t)
	mock.On("GetLink", "mob", "local", "mobile", "Safari").
		Return("https://example.com", nil).Once()

	r := chi.NewRouter()
	r.Get("/{minilink}", redirect.New(slogdiscard.NewDiscardLogger(), mock))

	req := httptest.NewRequest(http.MethodGet, "/mob", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1")
	req.Header.Set("X-Real-IP", "127.0.0.1")

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusFound, rr.Code)
}

func TestRedirectHandler_ChromeBrowser(t *testing.T) {
	mock := mocks.NewLinkGetter(t)
	mock.On("GetLink", "chrome", "local", "desktop", "Chrome").
		Return("https://example.com", nil).Once()

	r := chi.NewRouter()
	r.Get("/{minilink}", redirect.New(slogdiscard.NewDiscardLogger(), mock))

	req := httptest.NewRequest(http.MethodGet, "/chrome", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("X-Real-IP", "127.0.0.1")

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusFound, rr.Code)
}

func TestRedirectHandler_FirefoxBrowser(t *testing.T) {
	mock := mocks.NewLinkGetter(t)
	mock.On("GetLink", "ff", "local", "desktop", "Firefox").
		Return("https://example.com", nil).Once()

	r := chi.NewRouter()
	r.Get("/{minilink}", redirect.New(slogdiscard.NewDiscardLogger(), mock))

	req := httptest.NewRequest(http.MethodGet, "/ff", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:120.0) Gecko/20100101 Firefox/120.0")
	req.Header.Set("X-Real-IP", "127.0.0.1")

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusFound, rr.Code)
}

func TestRedirectHandler_CountryGetterError(t *testing.T) {
	mock := mocks.NewLinkGetter(t)
	mock.On("GetLink", "geo", "unknown", "desktop", "").
		Return("https://example.com", nil).Once()

	countryFn := func(ip string) (string, error) {
		return "", errors.New("service unavailable")
	}

	r := chi.NewRouter()
	r.Get("/{minilink}", redirect.New(slogdiscard.NewDiscardLogger(), mock, redirect.WithCountryGetter(countryFn)))

	req := httptest.NewRequest(http.MethodGet, "/geo", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusFound, rr.Code)
}

func TestRedirectHandler_WithCountryGetterOption(t *testing.T) {
	mock := mocks.NewLinkGetter(t)
	mock.On("GetLink", "opt", "TestCountry", "desktop", "").
		Return("https://example.com", nil).Once()

	countryFn := func(ip string) (string, error) {
		return "TestCountry", nil
	}

	r := chi.NewRouter()
	r.Get("/{minilink}", redirect.New(slogdiscard.NewDiscardLogger(), mock, redirect.WithCountryGetter(countryFn)))

	req := httptest.NewRequest(http.MethodGet, "/opt", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusFound, rr.Code)
}

func TestRedirectHandler_VeryLongAlias(t *testing.T) {
	longAlias := strings.Repeat("a", 200)
	mock := mocks.NewLinkGetter(t)
	mock.On("GetLink", longAlias, "local", "desktop", "").
		Return("https://example.com", nil).Once()

	r := chi.NewRouter()
	r.Get("/{minilink}", redirect.New(slogdiscard.NewDiscardLogger(), mock))

	req := httptest.NewRequest(http.MethodGet, "/"+longAlias, nil)
	req.Header.Set("X-Real-IP", "127.0.0.1")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusFound, rr.Code)
}

func TestRedirectHandler_SpecialCharsAlias(t *testing.T) {
	mock := mocks.NewLinkGetter(t)
	mock.On("GetLink", "my-link_123", "local", "desktop", "").
		Return("https://example.com", nil).Once()

	r := chi.NewRouter()
	r.Get("/{minilink}", redirect.New(slogdiscard.NewDiscardLogger(), mock))

	req := httptest.NewRequest(http.MethodGet, "/my-link_123", nil)
	req.Header.Set("X-Real-IP", "127.0.0.1")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusFound, rr.Code)
}

func TestRedirectHandler_LinkGetterReturnsHTTP(t *testing.T) {
	mock := mocks.NewLinkGetter(t)
	mock.On("GetLink", "httplink", "local", "desktop", "").
		Return("http://insecure.example.com", nil).Once()

	r := chi.NewRouter()
	r.Get("/{minilink}", redirect.New(slogdiscard.NewDiscardLogger(), mock))

	req := httptest.NewRequest(http.MethodGet, "/httplink", nil)
	req.Header.Set("X-Real-IP", "127.0.0.1")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusFound, rr.Code)
	assert.Equal(t, "http://insecure.example.com", rr.Header().Get("Location"))
}

func TestRedirectHandler_LinkGetterReturnsHTTPS(t *testing.T) {
	mock := mocks.NewLinkGetter(t)
	mock.On("GetLink", "httpslink", "local", "desktop", "").
		Return("https://secure.example.com", nil).Once()

	r := chi.NewRouter()
	r.Get("/{minilink}", redirect.New(slogdiscard.NewDiscardLogger(), mock))

	req := httptest.NewRequest(http.MethodGet, "/httpslink", nil)
	req.Header.Set("X-Real-IP", "127.0.0.1")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusFound, rr.Code)
	assert.Equal(t, "https://secure.example.com", rr.Header().Get("Location"))
}

func TestRedirectHandler_EmptyUserAgent(t *testing.T) {
	mock := mocks.NewLinkGetter(t)
	mock.On("GetLink", "noua", "local", "desktop", "").
		Return("https://example.com", nil).Once()

	r := chi.NewRouter()
	r.Get("/{minilink}", redirect.New(slogdiscard.NewDiscardLogger(), mock))

	req := httptest.NewRequest(http.MethodGet, "/noua", nil)
	req.Header.Set("X-Real-IP", "127.0.0.1")
	req.Header.Del("User-Agent")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusFound, rr.Code)
}
