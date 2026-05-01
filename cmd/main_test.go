package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSetupLogger_KnownEnvs — табличный тест по классам эквивалентности
// допустимых значений ENV: `local` и `dev` → debug включён, `prod` → только Info.
//
// Заменяет три отдельных теста (`_Local`, `_Dev`, `_Prod`) одной таблицей,
// в которой видно различие классов: `(EQ_local|EQ_dev) → debug=on`, `EQ_prod → debug=off`.
func TestSetupLogger_KnownEnvs(t *testing.T) {
	cases := []struct {
		env          string
		debugEnabled bool
		infoEnabled  bool
	}{
		{"local", true, true},
		{"dev", true, true},
		{"prod", false, true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.env, func(t *testing.T) {
			log := setupLogger(tc.env)
			require.NotNil(t, log)
			assert.Equal(t, tc.debugEnabled, log.Enabled(nil, -4),
				"debug-уровень для ENV=%s", tc.env)
			assert.Equal(t, tc.infoEnabled, log.Enabled(nil, 0),
				"info-уровень для ENV=%s", tc.env)
		})
	}
}

// TestSetupLogger_UnknownEnv_FallsBackToProd — все значения ENV вне множества
// {local, dev, prod} попадают в default-ветку switch и трактуются как prod.
//
// Прежний тест-сьют имел ДВА теста для одного класса эквивалентности:
// `_Unknown` ("unknown_env") и `_EmptyString` ("") — оба тестируют ОДИН и тот же
// default-путь. Объединяем в один табличный тест с РАЗНЫМИ свойствами входа:
//   - "" — пустая строка (граничный случай),
//   - "unknown_env" — произвольная строка,
//   - "PROD" — другой регистр (показывает, что switch case-sensitive).
func TestSetupLogger_UnknownEnv_FallsBackToProd(t *testing.T) {
	envs := []string{"", "unknown_env", "PROD"}
	for _, env := range envs {
		env := env
		t.Run("env="+env, func(t *testing.T) {
			log := setupLogger(env)
			require.NotNil(t, log)
			// Поведение должно быть идентично prod:
			assert.True(t, log.Enabled(nil, 0), "info должен быть on")
			assert.False(t, log.Enabled(nil, -4), "debug должен быть off")
		})
	}
}
