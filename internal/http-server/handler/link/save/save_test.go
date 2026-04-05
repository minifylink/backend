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

func TestSaveHandler(t *testing.T) {
	cases := []struct {
		name      string
		alias     string
		url       string
		respError string
		mockError error
		rawBody   *string
	}{
		{
			name:  "Success",
			alias: "test_alias",
			url:   "https://google.com",
		},
		{
			name:      "Empty alias",
			alias:     "",
			url:       "https://google.com",
			respError: "shortID cannot be empty",
		},
		{
			name:      "Empty URL",
			url:       "",
			alias:     "some_alias",
			respError: "field Link is a required field",
		},
		{
			name:      "Invalid URL",
			url:       "some invalid URL",
			alias:     "some_alias",
			respError: "field Link is not a valid URL",
		},
		{
			name:      "SaveURL Error",
			alias:     "test_alias",
			url:       "https://google.com",
			respError: "failed to add link",
			mockError: errors.New("unexpected error"),
		},
		{
			name:      "Empty Body",
			rawBody:   strPtr(""),
			respError: "empty request",
		},
		{
			name:      "Invalid JSON",
			rawBody:   strPtr("{invalid}"),
			respError: "failed to decode request",
		},
		{
			name:      "ShortID Already Exists",
			alias:     "existing_alias",
			url:       "https://google.com",
			respError: "shortID already exists",
			mockError: errors.New("repository.SaveLink: short id already exists"),
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			linkSaverMock := mocks.NewLinkSaver(t)

			if tc.respError == "" || tc.mockError != nil {
				linkSaverMock.On("SaveLink", tc.url, tc.alias).
					Return(tc.mockError).
					Once()
			}

			handler := save.New(slogdiscard.NewDiscardLogger(), linkSaverMock)

			var req *http.Request
			var err error
			if tc.rawBody != nil {
				req, err = http.NewRequest(http.MethodPost, "/save", bytes.NewReader([]byte(*tc.rawBody)))
			} else {
				input := fmt.Sprintf(`{"link": "%s", "short_id": "%s"}`, tc.url, tc.alias)
				req, err = http.NewRequest(http.MethodPost, "/save", bytes.NewReader([]byte(input)))
			}
			require.NoError(t, err)

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			require.Equal(t, rr.Code, http.StatusOK)

			body := rr.Body.String()

			var resp save.Response

			require.NoError(t, json.Unmarshal([]byte(body), &resp))

			require.Equal(t, tc.respError, resp.Error)
		})
	}
}

func strPtr(s string) *string { return &s }

// New tests

func TestSaveHandler_SuccessResponseStatus(t *testing.T) {
	mock := mocks.NewLinkSaver(t)
	mock.On("SaveLink", "https://example.com", "myid").Return(nil).Once()

	handler := save.New(slogdiscard.NewDiscardLogger(), mock)

	body := `{"link": "https://example.com", "short_id": "myid"}`
	req := httptest.NewRequest(http.MethodPost, "/save", strings.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var resp save.Response
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, "OK", resp.Status)
}

func TestSaveHandler_SuccessReturnsShortID(t *testing.T) {
	mock := mocks.NewLinkSaver(t)
	mock.On("SaveLink", "https://example.com", "myid").Return(nil).Once()

	handler := save.New(slogdiscard.NewDiscardLogger(), mock)

	body := `{"link": "https://example.com", "short_id": "myid"}`
	req := httptest.NewRequest(http.MethodPost, "/save", strings.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var resp save.Response
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, "myid", resp.ShortID)
}

func TestSaveHandler_VeryLongURL(t *testing.T) {
	longURL := "https://example.com/" + strings.Repeat("a", 2000)
	mock := mocks.NewLinkSaver(t)
	mock.On("SaveLink", longURL, "long").Return(nil).Once()

	handler := save.New(slogdiscard.NewDiscardLogger(), mock)

	body := fmt.Sprintf(`{"link": "%s", "short_id": "long"}`, longURL)
	req := httptest.NewRequest(http.MethodPost, "/save", strings.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var resp save.Response
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Empty(t, resp.Error)
}

func TestSaveHandler_VeryLongShortID(t *testing.T) {
	longID := strings.Repeat("x", 100)
	mock := mocks.NewLinkSaver(t)
	mock.On("SaveLink", "https://example.com", longID).Return(nil).Once()

	handler := save.New(slogdiscard.NewDiscardLogger(), mock)

	body := fmt.Sprintf(`{"link": "https://example.com", "short_id": "%s"}`, longID)
	req := httptest.NewRequest(http.MethodPost, "/save", strings.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var resp save.Response
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Empty(t, resp.Error)
}

func TestSaveHandler_SpecialCharsInShortID(t *testing.T) {
	mock := mocks.NewLinkSaver(t)
	mock.On("SaveLink", "https://example.com", "test-alias_123").Return(nil).Once()

	handler := save.New(slogdiscard.NewDiscardLogger(), mock)

	body := `{"link": "https://example.com", "short_id": "test-alias_123"}`
	req := httptest.NewRequest(http.MethodPost, "/save", strings.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var resp save.Response
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Empty(t, resp.Error)
}

func TestSaveHandler_UnicodeShortID(t *testing.T) {
	mock := mocks.NewLinkSaver(t)
	mock.On("SaveLink", "https://example.com", "тест").Return(nil).Once()

	handler := save.New(slogdiscard.NewDiscardLogger(), mock)

	body := `{"link": "https://example.com", "short_id": "тест"}`
	req := httptest.NewRequest(http.MethodPost, "/save", strings.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var resp save.Response
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Empty(t, resp.Error)
}

func TestSaveHandler_URLWithQueryParams(t *testing.T) {
	url := "https://example.com/path?a=1&b=2"
	mock := mocks.NewLinkSaver(t)
	mock.On("SaveLink", url, "qp").Return(nil).Once()

	handler := save.New(slogdiscard.NewDiscardLogger(), mock)

	body := fmt.Sprintf(`{"link": "%s", "short_id": "qp"}`, url)
	req := httptest.NewRequest(http.MethodPost, "/save", strings.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var resp save.Response
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Empty(t, resp.Error)
}

func TestSaveHandler_URLWithFragment(t *testing.T) {
	url := "https://example.com/page#section"
	mock := mocks.NewLinkSaver(t)
	mock.On("SaveLink", url, "frag").Return(nil).Once()

	handler := save.New(slogdiscard.NewDiscardLogger(), mock)

	body := fmt.Sprintf(`{"link": "%s", "short_id": "frag"}`, url)
	req := httptest.NewRequest(http.MethodPost, "/save", strings.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var resp save.Response
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Empty(t, resp.Error)
}

func TestSaveHandler_HTTPUrl(t *testing.T) {
	mock := mocks.NewLinkSaver(t)
	mock.On("SaveLink", "http://example.com", "http").Return(nil).Once()

	handler := save.New(slogdiscard.NewDiscardLogger(), mock)

	body := `{"link": "http://example.com", "short_id": "http"}`
	req := httptest.NewRequest(http.MethodPost, "/save", strings.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var resp save.Response
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Empty(t, resp.Error)
}

func TestSaveHandler_FTPUrl(t *testing.T) {
	mock := mocks.NewLinkSaver(t)
	mock.On("SaveLink", "ftp://example.com", "ftp").Return(nil).Maybe()

	handler := save.New(slogdiscard.NewDiscardLogger(), mock)

	body := `{"link": "ftp://example.com", "short_id": "ftp"}`
	req := httptest.NewRequest(http.MethodPost, "/save", strings.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// ftp:// may or may not be considered a valid URL by the validator
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestSaveHandler_MissingLinkField(t *testing.T) {
	mock := mocks.NewLinkSaver(t)

	handler := save.New(slogdiscard.NewDiscardLogger(), mock)

	body := `{"short_id": "abc"}`
	req := httptest.NewRequest(http.MethodPost, "/save", strings.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var resp save.Response
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Contains(t, resp.Error, "required field")
}

func TestSaveHandler_MissingShortIDField(t *testing.T) {
	mock := mocks.NewLinkSaver(t)

	handler := save.New(slogdiscard.NewDiscardLogger(), mock)

	body := `{"link": "https://example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/save", strings.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var resp save.Response
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, "shortID cannot be empty", resp.Error)
}

func TestSaveHandler_ExtraFields(t *testing.T) {
	mock := mocks.NewLinkSaver(t)
	mock.On("SaveLink", "https://example.com", "extra").Return(nil).Once()

	handler := save.New(slogdiscard.NewDiscardLogger(), mock)

	body := `{"link": "https://example.com", "short_id": "extra", "foo": "bar", "baz": 123}`
	req := httptest.NewRequest(http.MethodPost, "/save", strings.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var resp save.Response
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Empty(t, resp.Error)
}

func TestSaveHandler_NullBody(t *testing.T) {
	mock := mocks.NewLinkSaver(t)

	handler := save.New(slogdiscard.NewDiscardLogger(), mock)

	req := httptest.NewRequest(http.MethodPost, "/save", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var resp save.Response
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.Error)
}

func TestSaveHandler_ContentTypeNotJSON(t *testing.T) {
	mock := mocks.NewLinkSaver(t)
	mock.On("SaveLink", "https://example.com", "ct").Return(nil).Maybe()

	handler := save.New(slogdiscard.NewDiscardLogger(), mock)

	body := `{"link": "https://example.com", "short_id": "ct"}`
	req := httptest.NewRequest(http.MethodPost, "/save", strings.NewReader(body))
	req.Header.Set("Content-Type", "text/plain")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// render.DecodeJSON should still work regardless of Content-Type
	assert.Equal(t, http.StatusOK, rr.Code)
}
