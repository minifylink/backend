//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	idCounter atomic.Int64
	runID     string
)

var baseURL string

var client = &http.Client{
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
	Timeout: 10 * time.Second,
}

type saveRequest struct {
	Link    string `json:"link"`
	ShortID string `json:"short_id"`
}

type saveResponse struct {
	Status  string `json:"status"`
	Error   string `json:"error,omitempty"`
	ShortID string `json:"short_id,omitempty"`
}

type errorResponse struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type statsResponse struct {
	Clicks    int               `json:"clicks"`
	Devices   map[string]string `json:"devices"`
	Countries []string          `json:"countries"`
}

const (
	desktopUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	mobileUA  = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1"
	firefoxUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:121.0) Gecko/20100101 Firefox/121.0"
	safariUA  = "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15"
)

func TestMain(m *testing.M) {
	baseURL = os.Getenv("E2E_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8082"
	}

	// Unique prefix per test run to avoid collisions
	runID = fmt.Sprintf("%d", time.Now().UnixNano()%100000)

	// Wait for the service to be ready
	ready := false
	for i := 0; i < 30; i++ {
		resp, err := http.Get(baseURL + "/healthy")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			ready = true
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(1 * time.Second)
	}

	if !ready {
		fmt.Fprintf(os.Stderr, "E2E: service at %s is not ready. Start it with: docker compose up -d --build\n", baseURL)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func uniqueID(prefix string) string {
	// Keep short to fit VARCHAR(20) constraint on links.short_id
	n := idCounter.Add(1)
	return fmt.Sprintf("%s%s%d", prefix, runID, n)
}

func createLink(t *testing.T, link, shortID string) saveResponse {
	t.Helper()
	body, _ := json.Marshal(saveRequest{Link: link, ShortID: shortID})
	resp, err := client.Post(baseURL+"/api/v1/shorten/", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	var result saveResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	return result
}

func doRedirect(t *testing.T, shortID, userAgent string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, baseURL+"/"+shortID, nil)
	require.NoError(t, err)
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	resp, err := client.Do(req)
	require.NoError(t, err)
	return resp
}

func getStats(t *testing.T, shortID string) statsResponse {
	t.Helper()
	resp, err := client.Get(baseURL + "/api/v1/stats/" + shortID)
	require.NoError(t, err)
	defer resp.Body.Close()

	var result statsResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	return result
}

func getError(t *testing.T, resp *http.Response) errorResponse {
	t.Helper()
	defer resp.Body.Close()
	var result errorResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	return result
}

// Сценарий 1: Создание короткой ссылки и переход по ней
func TestE2E_CreateAndRedirect(t *testing.T) {
	id := uniqueID("s1")
	result := createLink(t, "https://github.com", id)
	require.Equal(t, "OK", result.Status)
	require.Equal(t, id, result.ShortID)

	resp := doRedirect(t, id, desktopUA)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusFound, resp.StatusCode)
	assert.Equal(t, "https://github.com", resp.Header.Get("Location"))
}

// Сценарий 2: Создание ссылки и просмотр пустой статистики
func TestE2E_EmptyStats(t *testing.T) {
	id := uniqueID("s2")
	result := createLink(t, "https://google.com", id)
	require.Equal(t, "OK", result.Status)

	stats := getStats(t, id)
	assert.Equal(t, 0, stats.Clicks)
	assert.Empty(t, stats.Devices)
	assert.Empty(t, stats.Countries)
}

// Сценарий 3: Полный цикл — создание, переходы, проверка статистики
func TestE2E_FullCycle_DesktopClicks(t *testing.T) {
	id := uniqueID("s3")
	result := createLink(t, "https://google.com", id)
	require.Equal(t, "OK", result.Status)

	for i := 0; i < 3; i++ {
		resp := doRedirect(t, id, desktopUA)
		resp.Body.Close()
		assert.Equal(t, http.StatusFound, resp.StatusCode)
	}

	stats := getStats(t, id)
	assert.Equal(t, 3, stats.Clicks)
	assert.Equal(t, "100%", stats.Devices["desktop"])
}

// Сценарий 4: Статистика с разных устройств
func TestE2E_MixedDevices(t *testing.T) {
	id := uniqueID("s4")
	result := createLink(t, "https://example.com", id)
	require.Equal(t, "OK", result.Status)

	for i := 0; i < 2; i++ {
		resp := doRedirect(t, id, desktopUA)
		resp.Body.Close()
	}
	for i := 0; i < 2; i++ {
		resp := doRedirect(t, id, mobileUA)
		resp.Body.Close()
	}

	stats := getStats(t, id)
	assert.Equal(t, 4, stats.Clicks)
	assert.Equal(t, "50%", stats.Devices["desktop"])
	assert.Equal(t, "50%", stats.Devices["mobile"])
}

// Сценарий 5: Локальные клики попадают в countries как "local".
//
// История теста: прежняя версия называлась TestE2E_DifferentCountries и претендовала
// на проверку «разных стран», но при локальном запуске ВСЕ клики идут с loopback IP,
// которые getCountry() мапит в фиксированную строку "local". То есть «разных стран»
// в этом окружении нет в принципе.
//
// Теперь тест честно проверяет то, что реально проверяется:
// 3 локальных клика → countries == ["local"], len == 1.
// Покрытие действительно «разных стран» сделано через юнит/интеграцию,
// где можно подменить countryFn без обращения к ip-api.com.
func TestE2E_LocalClicks_AreReportedAsLocalCountry(t *testing.T) {
	id := uniqueID("s5")
	result := createLink(t, "https://example.com", id)
	require.Equal(t, "OK", result.Status)

	for i := 0; i < 3; i++ {
		resp := doRedirect(t, id, desktopUA)
		resp.Body.Close()
	}

	stats := getStats(t, id)
	assert.Equal(t, 3, stats.Clicks)
	assert.Equal(t, []string{"local"}, stats.Countries,
		"в локальном e2e окружении все IP — приватные, geo-сервис не вызывается")
}

// Сценарий 6: Попытка создать дубликат не затирает оригинальную ссылку.
//
// NB: API-уровень здесь возвращает 200 OK + {"status":"Error", "error":"..."}.
// Это антипаттерн REST (по-хорошему должен быть 409 Conflict), но он закреплён
// контрактом save-хэндлера. Тест явно проверяет именно текущее поведение,
// чтобы случайное «улучшение» (переход на 409) сразу подсветило breaking change.
func TestE2E_DuplicateShortID(t *testing.T) {
	id := uniqueID("s6")

	result := createLink(t, "https://a.com", id)
	require.Equal(t, "OK", result.Status)

	// дубликат: HTTP 200, но в теле — Error
	body, _ := json.Marshal(saveRequest{Link: "https://b.com", ShortID: id})
	resp, err := client.Post(baseURL+"/api/v1/shorten/", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"API закрепляет 200+Error вместо 409 Conflict")
	var dup saveResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&dup))
	resp.Body.Close()
	assert.Equal(t, "Error", dup.Status)
	assert.Contains(t, dup.Error, "shortID already exists")

	// оригинальная ссылка не затёрта
	resp2 := doRedirect(t, id, desktopUA)
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusFound, resp2.StatusCode)
	assert.Equal(t, "https://a.com", resp2.Header.Get("Location"))
}

// Сценарий 7: Переход по несуществующей ссылке, затем создание и переход
func TestE2E_NotFoundThenCreate(t *testing.T) {
	id := uniqueID("s7")

	resp := doRedirect(t, id, desktopUA)
	errResp := getError(t, resp)
	assert.Equal(t, "Error", errResp.Status)
	assert.Contains(t, errResp.Error, "not found")

	result := createLink(t, "https://google.com", id)
	require.Equal(t, "OK", result.Status)

	resp2 := doRedirect(t, id, desktopUA)
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusFound, resp2.StatusCode)
	assert.Equal(t, "https://google.com", resp2.Header.Get("Location"))
}

// Сценарий 8: Несколько ссылок с независимой статистикой
func TestE2E_MultipleLinksIndependentStats(t *testing.T) {
	idA := uniqueID("s8a")
	idB := uniqueID("s8b")

	createLink(t, "https://a.com", idA)
	createLink(t, "https://b.com", idB)

	for i := 0; i < 3; i++ {
		resp := doRedirect(t, idA, desktopUA)
		resp.Body.Close()
	}
	resp := doRedirect(t, idB, desktopUA)
	resp.Body.Close()

	statsA := getStats(t, idA)
	statsB := getStats(t, idB)

	assert.Equal(t, 3, statsA.Clicks)
	assert.Equal(t, 1, statsB.Clicks)
}

// Сценарий 9: Накопление статистики при повторных визитах
func TestE2E_AccumulatingStats(t *testing.T) {
	id := uniqueID("s9")
	createLink(t, "https://example.com", id)

	for i := 0; i < 2; i++ {
		resp := doRedirect(t, id, desktopUA)
		resp.Body.Close()
	}

	stats := getStats(t, id)
	assert.Equal(t, 2, stats.Clicks)

	for i := 0; i < 3; i++ {
		resp := doRedirect(t, id, desktopUA)
		resp.Body.Close()
	}

	stats = getStats(t, id)
	assert.Equal(t, 5, stats.Clicks)
}

// Сценарий 10a/10b/10c: валидация данных при создании ссылки.
//
// Прежде это был ОДИН тест с тремя последовательными шагами; падение шага №2
// маскировало результаты шага №3. Здесь — три независимых теста, каждый
// проверяет один класс эквивалентности.
func TestE2E_CreateLink_InvalidURL(t *testing.T) {
	id := uniqueID("s10a")
	result := createLink(t, "not-a-url", id)
	assert.Equal(t, "Error", result.Status)
	assert.NotEmpty(t, result.Error,
		"для невалидного URL должно быть конкретное сообщение")
}

func TestE2E_CreateLink_EmptyURL(t *testing.T) {
	id := uniqueID("s10b")
	result := createLink(t, "", id)
	assert.Equal(t, "Error", result.Status)
	assert.NotEmpty(t, result.Error)
}

func TestE2E_CreateLink_ValidURL_AndRedirect(t *testing.T) {
	id := uniqueID("s10c")
	result := createLink(t, "https://google.com", id)
	require.Equal(t, "OK", result.Status)

	resp := doRedirect(t, id, desktopUA)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusFound, resp.StatusCode)
	assert.Equal(t, "https://google.com", resp.Header.Get("Location"))
}

// Сценарий 11: разные desktop-браузеры одинаково классифицируются как desktop.
//
// Прежнее имя `TestE2E_DifferentBrowsers` вводило в заблуждение: тест
// проверял НЕ "разные браузеры в статистике" (поле devices хранит только
// desktop/mobile, без browser), а то, что три desktop-UA дают devices.desktop=100%.
// Новое имя честно отражает суть.
func TestE2E_DesktopBrowsers_AllClassifiedAsDesktop(t *testing.T) {
	id := uniqueID("s11")
	createLink(t, "https://example.com", id)

	for _, ua := range []string{desktopUA, firefoxUA, safariUA} {
		resp := doRedirect(t, id, ua)
		resp.Body.Close()
	}

	stats := getStats(t, id)
	assert.Equal(t, 3, stats.Clicks)
	assert.Equal(t, "100%", stats.Devices["desktop"],
		"3 разных desktop-UA → 100% desktop, 0% mobile")
	assert.NotContains(t, stats.Devices, "mobile")
}

// Сценарий 12: Статистика несуществующей ссылки, затем её создание и клик
func TestE2E_StatsNotFoundThenCreateAndClick(t *testing.T) {
	id := uniqueID("s12")

	// Статистика до создания — ошибка (ответ будет JSON с error)
	resp, err := client.Get(baseURL + "/api/v1/stats/" + id)
	require.NoError(t, err)
	var errResp errorResponse
	json.NewDecoder(resp.Body).Decode(&errResp)
	resp.Body.Close()
	assert.Equal(t, "Error", errResp.Status)
	assert.Contains(t, errResp.Error, "not found")

	// Создаём ссылку
	result := createLink(t, "https://google.com", id)
	require.Equal(t, "OK", result.Status)

	// Кликаем
	resp2 := doRedirect(t, id, desktopUA)
	resp2.Body.Close()
	assert.Equal(t, http.StatusFound, resp2.StatusCode)

	// Проверяем статистику
	stats := getStats(t, id)
	assert.Equal(t, 1, stats.Clicks)
}
