package statistic_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"backend/internal/http-server/handler/link/statistic"
	"backend/internal/http-server/handler/link/statistic/mocks"
	"backend/internal/lib/logger/slogdiscard"
	"backend/internal/repository"
)

func TestStatisticHandler(t *testing.T) {
	cases := []struct {
		name         string
		shortID      string
		mockResponse *repository.StatisticResponse
		mockError    error
		expectedCode int
		expectedBody string
	}{
		{
			name:    "Success",
			shortID: "test_id",
			mockResponse: &repository.StatisticResponse{
				Clicks:    10,
				Devices:   map[string]string{"desktop": "80%", "mobile": "20%"},
				Countries: []string{"Russia", "USA"},
			},
			expectedCode: http.StatusOK,
		},
		{
			name:         "Not Found",
			shortID:      "not_exists",
			mockError:    errors.New("repository.GetStatistic: link not found"),
			expectedCode: http.StatusOK,
			expectedBody: `{"status":"Error","error":"not found"}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			statisticGetterMock := mocks.NewStatisticGetter(t)

			if tc.mockError != nil {
				statisticGetterMock.On("GetStatistic", tc.shortID).
					Return(nil, tc.mockError).Once()
			} else {
				statisticGetterMock.On("GetStatistic", tc.shortID).
					Return(tc.mockResponse, nil).Once()
			}

			r := chi.NewRouter()
			r.Get("/{short_id}", statistic.New(slogdiscard.NewDiscardLogger(), statisticGetterMock))

			req, err := http.NewRequest(http.MethodGet, "/"+tc.shortID, nil)
			require.NoError(t, err)

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			assert.Equal(t, tc.expectedCode, rr.Code)

			if tc.expectedBody != "" {
				assert.JSONEq(t, tc.expectedBody, rr.Body.String())
			} else if tc.mockResponse != nil {
				var response repository.StatisticResponse
				err = json.Unmarshal(rr.Body.Bytes(), &response)
				require.NoError(t, err)

				assert.Equal(t, tc.mockResponse.Clicks, response.Clicks)
				assert.Equal(t, tc.mockResponse.Devices, response.Devices)
				assert.Equal(t, tc.mockResponse.Countries, response.Countries)
			}
		})
	}
}

func TestStatisticHandlerEmptyShortID(t *testing.T) {
	r := chi.NewRouter()
	r.Get("/{short_id}", statistic.New(slogdiscard.NewDiscardLogger(), nil))

	req, err := http.NewRequest(http.MethodGet, "/", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestStatisticHandlerEmptyShortIDDirect(t *testing.T) {
	handler := statistic.New(slogdiscard.NewDiscardLogger(), nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.JSONEq(t, `{"status":"Error","error":"invalid request"}`, rr.Body.String())
}

// New tests

func TestStatisticHandler_ReturnsClicks(t *testing.T) {
	mock := mocks.NewStatisticGetter(t)
	mock.On("GetStatistic", "abc").Return(&repository.StatisticResponse{
		Clicks:    42,
		Devices:   map[string]string{},
		Countries: []string{},
	}, nil).Once()

	r := chi.NewRouter()
	r.Get("/{short_id}", statistic.New(slogdiscard.NewDiscardLogger(), mock))

	req := httptest.NewRequest(http.MethodGet, "/abc", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	var resp repository.StatisticResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, 42, resp.Clicks)
}

func TestStatisticHandler_ReturnsDevices(t *testing.T) {
	devices := map[string]string{"desktop": "60%", "mobile": "40%"}
	mock := mocks.NewStatisticGetter(t)
	mock.On("GetStatistic", "abc").Return(&repository.StatisticResponse{
		Clicks:    5,
		Devices:   devices,
		Countries: []string{},
	}, nil).Once()

	r := chi.NewRouter()
	r.Get("/{short_id}", statistic.New(slogdiscard.NewDiscardLogger(), mock))

	req := httptest.NewRequest(http.MethodGet, "/abc", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	var resp repository.StatisticResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, devices, resp.Devices)
}

func TestStatisticHandler_ReturnsCountries(t *testing.T) {
	countries := []string{"US", "RU", "DE"}
	mock := mocks.NewStatisticGetter(t)
	mock.On("GetStatistic", "abc").Return(&repository.StatisticResponse{
		Clicks:    3,
		Devices:   map[string]string{},
		Countries: countries,
	}, nil).Once()

	r := chi.NewRouter()
	r.Get("/{short_id}", statistic.New(slogdiscard.NewDiscardLogger(), mock))

	req := httptest.NewRequest(http.MethodGet, "/abc", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	var resp repository.StatisticResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, countries, resp.Countries)
}

func TestStatisticHandler_ZeroClicks(t *testing.T) {
	mock := mocks.NewStatisticGetter(t)
	mock.On("GetStatistic", "empty").Return(&repository.StatisticResponse{
		Clicks:    0,
		Devices:   map[string]string{},
		Countries: []string{},
	}, nil).Once()

	r := chi.NewRouter()
	r.Get("/{short_id}", statistic.New(slogdiscard.NewDiscardLogger(), mock))

	req := httptest.NewRequest(http.MethodGet, "/empty", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	var resp repository.StatisticResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, 0, resp.Clicks)
	assert.Empty(t, resp.Devices)
}

func TestStatisticHandler_MultipleDevices(t *testing.T) {
	mock := mocks.NewStatisticGetter(t)
	mock.On("GetStatistic", "multi").Return(&repository.StatisticResponse{
		Clicks:    4,
		Devices:   map[string]string{"desktop": "75%", "mobile": "25%"},
		Countries: []string{},
	}, nil).Once()

	r := chi.NewRouter()
	r.Get("/{short_id}", statistic.New(slogdiscard.NewDiscardLogger(), mock))

	req := httptest.NewRequest(http.MethodGet, "/multi", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	var resp repository.StatisticResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, "75%", resp.Devices["desktop"])
	assert.Equal(t, "25%", resp.Devices["mobile"])
}

func TestStatisticHandler_MultipleCountries(t *testing.T) {
	countries := []string{"US", "RU", "DE"}
	mock := mocks.NewStatisticGetter(t)
	mock.On("GetStatistic", "geo").Return(&repository.StatisticResponse{
		Clicks:    10,
		Devices:   map[string]string{},
		Countries: countries,
	}, nil).Once()

	r := chi.NewRouter()
	r.Get("/{short_id}", statistic.New(slogdiscard.NewDiscardLogger(), mock))

	req := httptest.NewRequest(http.MethodGet, "/geo", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	var resp repository.StatisticResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Len(t, resp.Countries, 3)
}

func TestStatisticHandler_InternalError(t *testing.T) {
	mock := mocks.NewStatisticGetter(t)
	mock.On("GetStatistic", "err").Return(nil, errors.New("database connection lost")).Once()

	r := chi.NewRouter()
	r.Get("/{short_id}", statistic.New(slogdiscard.NewDiscardLogger(), mock))

	req := httptest.NewRequest(http.MethodGet, "/err", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	var respBody map[string]interface{}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &respBody))
	assert.Equal(t, "not found", respBody["error"])
}

func TestStatisticHandler_VeryLongShortID(t *testing.T) {
	longID := "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz01234567890123456789"
	mock := mocks.NewStatisticGetter(t)
	mock.On("GetStatistic", longID).Return(&repository.StatisticResponse{
		Clicks: 0, Devices: map[string]string{}, Countries: []string{},
	}, nil).Once()

	r := chi.NewRouter()
	r.Get("/{short_id}", statistic.New(slogdiscard.NewDiscardLogger(), mock))

	req := httptest.NewRequest(http.MethodGet, "/"+longID, nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestStatisticHandler_SpecialCharsInShortID(t *testing.T) {
	mock := mocks.NewStatisticGetter(t)
	mock.On("GetStatistic", "test-id_123").Return(&repository.StatisticResponse{
		Clicks: 1, Devices: map[string]string{}, Countries: []string{},
	}, nil).Once()

	r := chi.NewRouter()
	r.Get("/{short_id}", statistic.New(slogdiscard.NewDiscardLogger(), mock))

	req := httptest.NewRequest(http.MethodGet, "/test-id_123", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestStatisticHandler_ResponseContentType(t *testing.T) {
	mock := mocks.NewStatisticGetter(t)
	mock.On("GetStatistic", "ct").Return(&repository.StatisticResponse{
		Clicks: 0, Devices: map[string]string{}, Countries: []string{},
	}, nil).Once()

	r := chi.NewRouter()
	r.Get("/{short_id}", statistic.New(slogdiscard.NewDiscardLogger(), mock))

	req := httptest.NewRequest(http.MethodGet, "/ct", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Contains(t, rr.Header().Get("Content-Type"), "application/json")
}

func TestStatisticHandler_StatusCode200(t *testing.T) {
	mock := mocks.NewStatisticGetter(t)
	mock.On("GetStatistic", "ok").Return(&repository.StatisticResponse{
		Clicks: 5, Devices: map[string]string{}, Countries: []string{},
	}, nil).Once()

	r := chi.NewRouter()
	r.Get("/{short_id}", statistic.New(slogdiscard.NewDiscardLogger(), mock))

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}
