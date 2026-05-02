//go:build integration

package repository

import (
	"backend/internal/config"
	"backend/internal/lib/logger/slogdiscard"
	"context"
	"fmt"
	"os"
	"strings"
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

// Сценарий 9: Процентное распределение устройств — граничный.
//
// Тест разбит на два подкейса:
//  1. целочисленное деление (75/25) — проверяет базовую формулу;
//  2. нецелое деление (1/3 desktop / mobile) — проверяет округление до целого
//     процента. ВАЖНО: при %.0f Go округляет half-to-even (banker's rounding):
//     33.333 → 33, 66.666 → 67. Сумма может дать НЕ 100%.
//     Проверяем актуальное поведение, чтобы случайное изменение формата
//     (например, переход на %.1f) сразу подсветило breaking change.
func TestIntegration_Statistics_DevicePercentages(t *testing.T) {
	t.Run("integer_division_75_25", func(t *testing.T) {
		cleanTables(t)
		require.NoError(t, sharedStorage.SaveLink("https://example.com", "devpct"))
		for i := 0; i < 3; i++ {
			_, err := sharedStorage.GetLink("devpct", "Russia", "desktop", "Chrome")
			require.NoError(t, err)
		}
		_, err := sharedStorage.GetLink("devpct", "Russia", "mobile", "Safari")
		require.NoError(t, err)

		stats, err := sharedStorage.GetStatistic("devpct")
		require.NoError(t, err)
		assert.Equal(t, 4, stats.Clicks)
		assert.Equal(t, "75%", stats.Devices["desktop"])
		assert.Equal(t, "25%", stats.Devices["mobile"])
	})

	t.Run("rounding_one_third", func(t *testing.T) {
		cleanTables(t)
		require.NoError(t, sharedStorage.SaveLink("https://example.com", "devrnd"))
		// 1 desktop + 2 mobile → 33.33% / 66.66%
		_, err := sharedStorage.GetLink("devrnd", "Russia", "desktop", "Chrome")
		require.NoError(t, err)
		for i := 0; i < 2; i++ {
			_, err := sharedStorage.GetLink("devrnd", "Russia", "mobile", "Safari")
			require.NoError(t, err)
		}

		stats, err := sharedStorage.GetStatistic("devrnd")
		require.NoError(t, err)
		assert.Equal(t, 3, stats.Clicks)
		// Sprintf("%.0f", 33.333) → "33", Sprintf("%.0f", 66.666) → "67".
		assert.Equal(t, "33%", stats.Devices["desktop"])
		assert.Equal(t, "67%", stats.Devices["mobile"])
	})
}

// TestIntegration_SaveLink_ShortIDBoundary_VARCHAR20 — BVA по реальной границе
// schema links.short_id VARCHAR(20). Unit-тесты длину не валидируют, поэтому
// единственное место, где «21 символ → ошибка» проверяется — здесь.
func TestIntegration_SaveLink_ShortIDBoundary_VARCHAR20(t *testing.T) {
	cases := []struct {
		name      string
		length    int
		wantError bool
	}{
		{"len=19_below_limit", 19, false},
		{"len=20_at_limit", 20, false},
		{"len=21_above_limit", 21, true}, // VARCHAR(20) — превышение значения
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cleanTables(t)
			id := strings.Repeat("a", tc.length)
			err := sharedStorage.SaveLink("https://example.com", id)
			if tc.wantError {
				require.Error(t, err, "БД должна отвергнуть short_id длиннее 20 символов")
			} else {
				require.NoError(t, err)
				link, err := sharedStorage.GetLink(id, "local", "desktop", "Chrome")
				require.NoError(t, err)
				assert.Equal(t, "https://example.com", link)
			}
		})
	}
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

// Сценарий 12: идемпотентность создания таблиц через сравнение снимков
// information_schema.columns до/после повторного New(). DDL не должен меняться.
func TestIntegration_New_IdempotentTableCreation(t *testing.T) {
	cleanTables(t)
	require.NoError(t, sharedStorage.SaveLink("https://test.com", "idem"))

	type colInfo struct {
		name     string
		dataType string
		nullable string
	}
	snapshot := func(t *testing.T, table string) []colInfo {
		t.Helper()
		rows, err := sharedStorage.db.Query(`
			SELECT column_name, data_type, is_nullable
			FROM information_schema.columns
			WHERE table_name = $1 ORDER BY ordinal_position`, table)
		require.NoError(t, err)
		defer rows.Close()
		var out []colInfo
		for rows.Next() {
			var c colInfo
			require.NoError(t, rows.Scan(&c.name, &c.dataType, &c.nullable))
			out = append(out, c)
		}
		return out
	}

	tablesBefore := map[string][]colInfo{
		"links":     snapshot(t, "links"),
		"users":     snapshot(t, "users"),
		"analytics": snapshot(t, "analytics"),
	}

	// Повторный вызов: New() должен идемпотентно создать таблицы (CREATE TABLE IF NOT EXISTS).
	storage2, err := New(sharedCfg, slogdiscard.NewDiscardLogger())
	require.NoError(t, err)
	require.NotNil(t, storage2)

	for tbl, before := range tablesBefore {
		assert.Equal(t, before, snapshot(t, tbl),
			"схема таблицы %s не должна измениться после повторного New()", tbl)
	}

	// Данные тоже на месте.
	link, err := storage2.GetLink("idem", "local", "desktop", "test")
	require.NoError(t, err)
	assert.Equal(t, "https://test.com", link)
}
