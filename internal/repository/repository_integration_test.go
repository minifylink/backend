//go:build integration

package repository

import (
	"backend/internal/config"
	"backend/internal/lib/logger/slogdiscard"
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	sharedStorage *Storage
	sharedCfg     *config.Config
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:17-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start postgres container: %v\n", err)
		os.Exit(1)
	}

	host, _ := pgContainer.Host(ctx)
	port, _ := pgContainer.MappedPort(ctx, "5432")

	sharedCfg = &config.Config{
		PostgresConfig: config.PostgresConfig{
			Host:     host,
			Port:     port.Port(),
			Username: "testuser",
			Password: "testpass",
			DBName:   "testdb",
			SSLMode:  "disable",
		},
	}

	log := slogdiscard.NewDiscardLogger()
	sharedStorage, err = New(sharedCfg, log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to init storage: %v\n", err)
		pgContainer.Terminate(ctx)
		os.Exit(1)
	}

	code := m.Run()

	pgContainer.Terminate(ctx)
	os.Exit(code)
}

// cleanTables очищает все таблицы между тестами
func cleanTables(t *testing.T) {
	t.Helper()
	_, err := sharedStorage.db.Exec("DELETE FROM analytics")
	require.NoError(t, err)
	_, err = sharedStorage.db.Exec("DELETE FROM links")
	require.NoError(t, err)
}

// Сценарий 1: Сохранение ссылки — позитивный
func TestIntegration_SaveLink_Success(t *testing.T) {
	cleanTables(t)

	err := sharedStorage.SaveLink("https://google.com", "ggl")
	require.NoError(t, err)
}

// Сценарий 2: Дубликат short_id — негативный
func TestIntegration_SaveLink_DuplicateShortID(t *testing.T) {
	cleanTables(t)

	err := sharedStorage.SaveLink("https://google.com", "dup")
	require.NoError(t, err)

	err = sharedStorage.SaveLink("https://other.com", "dup")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repository.SaveLink")
}

// Сценарий 3: Сохранение и получение ссылки — позитивный
func TestIntegration_SaveAndGetLink(t *testing.T) {
	cleanTables(t)

	err := sharedStorage.SaveLink("https://example.com", "ex1")
	require.NoError(t, err)

	link, err := sharedStorage.GetLink("ex1", "Russia", "desktop", "Chrome")
	require.NoError(t, err)
	assert.Equal(t, "https://example.com", link)
}

// Сценарий 4: Получение несуществующей ссылки — негативный
func TestIntegration_GetLink_NotFound(t *testing.T) {
	cleanTables(t)

	_, err := sharedStorage.GetLink("nonexistent", "Russia", "desktop", "Chrome")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "link not found")
}

// Сценарий 5: Запись аналитики при получении ссылки — позитивный
func TestIntegration_GetLink_RecordsAnalytics(t *testing.T) {
	cleanTables(t)

	err := sharedStorage.SaveLink("https://example.com", "anl")
	require.NoError(t, err)

	_, err = sharedStorage.GetLink("anl", "Russia", "desktop", "Chrome")
	require.NoError(t, err)

	stats, err := sharedStorage.GetStatistic("anl")
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Clicks)
	assert.Contains(t, stats.Countries, "Russia")
}

// Сценарий 6: Статистика после нескольких кликов — позитивный
func TestIntegration_Statistics_AfterMultipleClicks(t *testing.T) {
	cleanTables(t)

	err := sharedStorage.SaveLink("https://example.com", "multi")
	require.NoError(t, err)

	_, err = sharedStorage.GetLink("multi", "Russia", "desktop", "Chrome")
	require.NoError(t, err)
	_, err = sharedStorage.GetLink("multi", "USA", "mobile", "Safari")
	require.NoError(t, err)
	_, err = sharedStorage.GetLink("multi", "Germany", "desktop", "Firefox")
	require.NoError(t, err)

	stats, err := sharedStorage.GetStatistic("multi")
	require.NoError(t, err)
	assert.Equal(t, 3, stats.Clicks)
	assert.Len(t, stats.Countries, 3)
}

// Сценарий 7: Статистика без кликов — граничный
func TestIntegration_Statistics_ZeroClicks(t *testing.T) {
	cleanTables(t)

	err := sharedStorage.SaveLink("https://example.com", "zero")
	require.NoError(t, err)

	stats, err := sharedStorage.GetStatistic("zero")
	require.NoError(t, err)
	assert.Equal(t, 0, stats.Clicks)
	assert.Empty(t, stats.Devices)
	assert.Empty(t, stats.Countries)
}

// Сценарий 8: Статистика несуществующей ссылки — негативный
func TestIntegration_Statistics_NotFound(t *testing.T) {
	cleanTables(t)

	_, err := sharedStorage.GetStatistic("notexist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "link not found")
}

// Сценарий 9: Процентное распределение устройств — граничный
func TestIntegration_Statistics_DevicePercentages(t *testing.T) {
	cleanTables(t)

	err := sharedStorage.SaveLink("https://example.com", "devpct")
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		_, err = sharedStorage.GetLink("devpct", "Russia", "desktop", "Chrome")
		require.NoError(t, err)
	}
	_, err = sharedStorage.GetLink("devpct", "Russia", "mobile", "Safari")
	require.NoError(t, err)

	stats, err := sharedStorage.GetStatistic("devpct")
	require.NoError(t, err)
	assert.Equal(t, 4, stats.Clicks)
	assert.Equal(t, "75%", stats.Devices["desktop"])
	assert.Equal(t, "25%", stats.Devices["mobile"])
}

// Сценарий 10: Множественные страны — позитивный
func TestIntegration_Statistics_MultipleCountries(t *testing.T) {
	cleanTables(t)

	err := sharedStorage.SaveLink("https://example.com", "geo")
	require.NoError(t, err)

	countries := []string{"Russia", "USA", "Germany", "France"}
	for _, c := range countries {
		_, err = sharedStorage.GetLink("geo", c, "desktop", "Chrome")
		require.NoError(t, err)
	}

	stats, err := sharedStorage.GetStatistic("geo")
	require.NoError(t, err)
	assert.Equal(t, 4, stats.Clicks)
	assert.Len(t, stats.Countries, 4)
}

// Сценарий 11: Пустые поля аналитики — граничный
func TestIntegration_GetLink_EmptyAnalyticsFields(t *testing.T) {
	cleanTables(t)

	err := sharedStorage.SaveLink("https://example.com", "empty")
	require.NoError(t, err)

	link, err := sharedStorage.GetLink("empty", "", "", "")
	require.NoError(t, err)
	assert.Equal(t, "https://example.com", link)

	stats, err := sharedStorage.GetStatistic("empty")
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Clicks)
}

// Сценарий 12: Идемпотентность создания таблиц — позитивный
func TestIntegration_New_IdempotentTableCreation(t *testing.T) {
	cleanTables(t)

	err := sharedStorage.SaveLink("https://test.com", "idem")
	require.NoError(t, err)

	// Повторный вызов New — таблицы уже существуют
	log := slogdiscard.NewDiscardLogger()
	storage2, err := New(sharedCfg, log)
	require.NoError(t, err)

	// Данные сохранились
	link, err := storage2.GetLink("idem", "local", "desktop", "test")
	require.NoError(t, err)
	assert.Equal(t, "https://test.com", link)

	_ = fmt.Sprintf("storage2=%p", storage2)
}
