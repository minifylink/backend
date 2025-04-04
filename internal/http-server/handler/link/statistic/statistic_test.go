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

	// Тест на 404, потому что chi не передаёт пустой параметр, а возвращает 404
	assert.Equal(t, http.StatusNotFound, rr.Code)
}
