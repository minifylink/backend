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
// Объединяет прежние `_Success_302` и `_ReturnsLocationHeader` (одна логика).
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

// TestGetRedirect_StopsAfterFirstRedirect — НАСТОЯЩАЯ цепочка из двух серверов.
// Прежний тест с тем же именем поднимал ОДИН сервер и не проверял
// поведение клиента при нескольких хопах — был зелёный, но ничего не доказывал.
//
// Здесь:
//   - srv2 — финальная страница, возвращает 200 (для GetRedirect это ошибка, ОК).
//   - srv1 — первый редирект 302 → srv2.URL.
//
// GetRedirect должен вернуть URL первого редиректа (Location срыв srv1),
// потому что клиент настроен останавливаться после ПЕРВОГО редиректа
// (`return http.ErrUseLastResponse`).
func TestGetRedirect_StopsAfterFirstRedirect(t *testing.T) {
	srv2 := fakeRedirectServer(t, http.StatusOK, "")
	srv1 := fakeRedirectServer(t, http.StatusFound, srv2.URL)

	location, err := GetRedirect(srv1.URL)
	require.NoError(t, err)
	assert.Equal(t, srv2.URL, location,
		"клиент должен остановиться на первом редиректе и вернуть его Location")
}
