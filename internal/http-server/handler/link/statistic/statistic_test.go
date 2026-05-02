package statistic_test

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

	"backend/internal/http-server/handler/link/statistic"
	"backend/internal/http-server/handler/link/statistic/mocks"
	"backend/internal/lib/logger/slogdiscard"
	"backend/internal/repository"
)

// newRouter — общий setup для unit-тестов хэндлера статистики.
func newRouter(t *testing.T) (*chi.Mux, *mocks.StatisticGetter) {
	t.Helper()
	mock := mocks.NewStatisticGetter(t)
	r := chi.NewRouter()
	r.Get("/{short_id}", statistic.New(slogdiscard.NewDiscardLogger(), mock))
	return r, mock
}

func doGet(t *testing.T, r http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}

// TestStatisticHandler_TableDriven — основной table-driven с покрытием
// классов эквивалентности (валидный/невалидный short_id),
// ошибочных путей (not found / internal error) и базового success.
func TestStatisticHandler_TableDriven(t *testing.T) {
	cases := []struct {
		name         string
		shortID      string
		mockResponse *repository.StatisticResponse
		mockError    error
		wantStatus   string // "OK" — для success возвращает stats; "Error" — error response
		wantErr      string
	}{
		{
			name:    "Success",
			shortID: "test_id",
			mockResponse: &repository.StatisticResponse{
				Clicks:    10,
				Devices:   map[string]string{"desktop": "80%", "mobile": "20%"},
				Countries: []string{"Russia", "USA"},
			},
		},
		{
			name:       "Not Found",
			shortID:    "not_exists",
			mockError:  errors.New("repository.GetStatistic: link not found"),
			wantStatus: "Error",
			wantErr:    "not found",
		},
		{
			name:       "Internal Error masked as not found",
			shortID:    "internal_err",
			mockError:  errors.New("database connection lost"),
			wantStatus: "Error",
			wantErr:    "not found", // хэндлер маскирует все ошибки под "not found"; см. statistic.go
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			r, mock := newRouter(t)
			if tc.mockError != nil {
				mock.On("GetStatistic", tc.shortID).Return(nil, tc.mockError).Once()
			} else {
				mock.On("GetStatistic", tc.shortID).Return(tc.mockResponse, nil).Once()
			}

			rr := doGet(t, r, "/"+tc.shortID)
			require.Equal(t, http.StatusOK, rr.Code)

			if tc.wantErr != "" {
				var body map[string]interface{}
				require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
				assert.Equal(t, tc.wantStatus, body["status"])
				assert.Equal(t, tc.wantErr, body["error"])
			} else {
				var got repository.StatisticResponse
				require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
				assert.Equal(t, tc.mockResponse.Clicks, got.Clicks)
				assert.Equal(t, tc.mockResponse.Devices, got.Devices)
				assert.Equal(t, tc.mockResponse.Countries, got.Countries)
			}
		})
	}
}

// TestStatisticHandler_EmptyShortID — два пути с одной целью:
//  1. через chi-роутер: пустой short_id не матчится → 404 от роутера;
//  2. при прямом вызове хэндлера (без chi): хэндлер сам отвечает "invalid request".
func TestStatisticHandler_EmptyShortID(t *testing.T) {
	t.Run("via_router_returns_404", func(t *testing.T) {
		r, _ := newRouter(t)
		rr := doGet(t, r, "/")
		assert.Equal(t, http.StatusNotFound, rr.Code,
			"chi не находит route без параметра")
	})

	t.Run("direct_call_returns_invalid_request", func(t *testing.T) {
		handler := statistic.New(slogdiscard.NewDiscardLogger(), nil)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.JSONEq(t, `{"status":"Error","error":"invalid request"}`, rr.Body.String())
	})
}

// TestStatisticHandler_ResponseFields — поля clicks/devices/countries
// одновременно сериализуются в JSON в одном пути данных.
func TestStatisticHandler_ResponseFields(t *testing.T) {
	mockResp := &repository.StatisticResponse{
		Clicks:    42,
		Devices:   map[string]string{"desktop": "60%", "mobile": "40%"},
		Countries: []string{"US", "RU", "DE"},
	}

	r, mock := newRouter(t)
	mock.On("GetStatistic", "abc").Return(mockResp, nil).Once()

	rr := doGet(t, r, "/abc")
	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Header().Get("Content-Type"), "application/json")

	var got repository.StatisticResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	assert.Equal(t, mockResp.Clicks, got.Clicks)
	assert.Equal(t, mockResp.Devices, got.Devices)
	assert.Equal(t, mockResp.Countries, got.Countries)
}

// TestStatisticHandler_ZeroClicks — граничный случай: ссылка без переходов.
// Ожидаем clicks=0, пустые devices и countries (а не null/missing).
func TestStatisticHandler_ZeroClicks(t *testing.T) {
	r, mock := newRouter(t)
	mock.On("GetStatistic", "empty").Return(&repository.StatisticResponse{
		Clicks:    0,
		Devices:   map[string]string{},
		Countries: []string{},
	}, nil).Once()

	rr := doGet(t, r, "/empty")
	require.Equal(t, http.StatusOK, rr.Code)

	var got repository.StatisticResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	assert.Equal(t, 0, got.Clicks)
	assert.Empty(t, got.Devices)
	assert.Empty(t, got.Countries)
}

// TestStatisticHandler_ShortIDFormats — длинный/со спецсимволами short_id пробрасывается без валидации.
// Реальное ограничение БД (VARCHAR(20)) проверяется в integration-тесте.
func TestStatisticHandler_ShortIDFormats(t *testing.T) {
	cases := []struct {
		name string
		id   string
	}{
		{"long_120_chars", strings.Repeat("a", 120)},
		{"with_dash_and_underscore", "test-id_123"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			r, mock := newRouter(t)
			mock.On("GetStatistic", tc.id).Return(&repository.StatisticResponse{
				Clicks: 1, Devices: map[string]string{}, Countries: []string{},
			}, nil).Once()

			rr := doGet(t, r, "/"+tc.id)
			assert.Equal(t, http.StatusOK, rr.Code)
		})
	}
}
