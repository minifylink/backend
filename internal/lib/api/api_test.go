package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRedirectServer — поднимает тестовый сервер, отвечающий заданным статусом
// и (опционально) Location-заголовком.
func fakeRedirectServer(t *testing.T, status int, location string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if location != "" {
			w.Header().Set("Location", location)
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestGetRedirect_ReturnsLocation — позитивный путь: 302 + Location → возвращаем URL.
func TestGetRedirect_ReturnsLocation(t *testing.T) {
	const target = "https://google.com/search?q=test"
	srv := fakeRedirectServer(t, http.StatusFound, target)

	location, err := GetRedirect(srv.URL)
	require.NoError(t, err)
	assert.Equal(t, target, location)
}

// TestGetRedirect_NonRedirectStatuses — табличный тест по классам эквивалентности
// "не-редиректных" статусов: 200, 404, 500. Все должны приводить к ErrInvalidStatusCode
// и сообщение должно содержать сам код.
func TestGetRedirect_NonRedirectStatuses(t *testing.T) {
	cases := []struct {
		name   string
		status int
	}{
		{"200_OK", http.StatusOK},
		{"404_NotFound", http.StatusNotFound},
		{"500_InternalServerError", http.StatusInternalServerError},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			srv := fakeRedirectServer(t, tc.status, "")

			_, err := GetRedirect(srv.URL)
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrInvalidStatusCode))
			// статус-код должен попадать в сообщение об ошибке —
			// без этого диагностика реальных багов в проде сильно затруднится.
			assert.Contains(t, err.Error(), strconv.Itoa(tc.status))
		})
	}
}

// TestGetRedirect_StatusBoundary_302 — BVA точечной границы (api.go:29): принимаем
// ровно 302, 301 и 303 отвергаем. 301/303 тоже редиректы по HTTP, но реализация их не пропускает.
func TestGetRedirect_StatusBoundary_302(t *testing.T) {
	const target = "https://example.com/destination"
	cases := []struct {
		name      string
		status    int
		wantError bool
	}{
		{"301_below_boundary_MovedPermanently", http.StatusMovedPermanently, true}, // 301 — отвергается
		{"302_at_boundary_Found", http.StatusFound, false},                         // 302 — принимается
		{"303_above_boundary_SeeOther", http.StatusSeeOther, true},                 // 303 — отвергается
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			srv := fakeRedirectServer(t, tc.status, target)

			location, err := GetRedirect(srv.URL)
			if tc.wantError {
				require.Error(t, err, "статус %d не равен 302 → должен быть ErrInvalidStatusCode", tc.status)
				assert.True(t, errors.Is(err, ErrInvalidStatusCode))
				assert.Contains(t, err.Error(), strconv.Itoa(tc.status))
			} else {
				require.NoError(t, err, "статус 302 — единственный принимаемый")
				assert.Equal(t, target, location)
			}
		})
	}
}

// TestGetRedirect_InvalidURL — невалидный URL → сетевая/парс-ошибка от http.Client.
func TestGetRedirect_InvalidURL(t *testing.T) {
	_, err := GetRedirect("://invalid-url")
	require.Error(t, err)
}

// TestGetRedirect_EmptyLocation — 302 без заголовка Location → возвращаем "" без ошибки.
// Граничный случай: технически редирект корректен, но клиенту некуда идти.
func TestGetRedirect_EmptyLocation(t *testing.T) {
	srv := fakeRedirectServer(t, http.StatusFound, "")

	location, err := GetRedirect(srv.URL)
	require.NoError(t, err)
	assert.Empty(t, location)
}

// TestGetRedirect_StopsAfterFirstRedirect — цепочка из двух серверов:
// srv1 (302 → srv2.URL) и srv2 (200). GetRedirect должен вернуть Location от srv1
// благодаря CheckRedirect=ErrUseLastResponse.
func TestGetRedirect_StopsAfterFirstRedirect(t *testing.T) {
	srv2 := fakeRedirectServer(t, http.StatusOK, "")
	srv1 := fakeRedirectServer(t, http.StatusFound, srv2.URL)

	location, err := GetRedirect(srv1.URL)
	require.NoError(t, err)
	assert.Equal(t, srv2.URL, location,
		"клиент должен остановиться на первом редиректе и вернуть его Location")
}
