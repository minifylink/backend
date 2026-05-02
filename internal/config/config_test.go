package config

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func clearEnvVars(t *testing.T) {
	t.Helper()
	// Не даём подгрузить настоящий .env в тесте.
	t.Setenv("ENV_FILE", "/dev/null")
	// Чистим все env-переменные конфига, чтобы получить дефолты.
	for _, key := range []string{
		"ENV", "POSTGRES_HOST", "POSTGRES_PORT", "POSTGRES_USER",
		"POSTGRES_PASSWORD", "POSTGRES_DB", "POSTGRES_SSL_MODE",
		"HTTP_SERVER_ADDRESS", "HTTP_SERVER_TIMEOUT", "HTTP_SERVER_IDLE_TIMEOUT",
	} {
		t.Setenv(key, "")
		os.Unsetenv(key)
	}
}

func TestMustLoad_DefaultEnv(t *testing.T) {
	clearEnvVars(t)
	cfg := MustLoad()
	assert.Equal(t, "local", cfg.Env)
}

func TestMustLoad_DefaultPostgres(t *testing.T) {
	clearEnvVars(t)
	cfg := MustLoad()
	assert.Equal(t, "postgres", cfg.PostgresConfig.Host)
	assert.Equal(t, "5432", cfg.PostgresConfig.Port)
	assert.Equal(t, "postgres", cfg.PostgresConfig.Username)
	assert.Equal(t, "postgres", cfg.PostgresConfig.Password)
	assert.Equal(t, "shortener", cfg.PostgresConfig.DBName)
	assert.Equal(t, "disable", cfg.PostgresConfig.SSLMode)
}

func TestMustLoad_DefaultHTTPServer(t *testing.T) {
	clearEnvVars(t)
	cfg := MustLoad()
	assert.Equal(t, "0.0.0.0:8082", cfg.HTTPServer.Address)
}

func TestMustLoad_CustomEnv(t *testing.T) {
	clearEnvVars(t)
	t.Setenv("ENV", "prod")
	cfg := MustLoad()
	assert.Equal(t, "prod", cfg.Env)
}

func TestMustLoad_CustomPostgres(t *testing.T) {
	clearEnvVars(t)
	t.Setenv("POSTGRES_HOST", "myhost")
	t.Setenv("POSTGRES_PORT", "5433")
	t.Setenv("POSTGRES_USER", "admin")
	t.Setenv("POSTGRES_PASSWORD", "secret")
	t.Setenv("POSTGRES_DB", "mydb")
	t.Setenv("POSTGRES_SSL_MODE", "require")

	cfg := MustLoad()
	assert.Equal(t, "myhost", cfg.PostgresConfig.Host)
	assert.Equal(t, "5433", cfg.PostgresConfig.Port)
	assert.Equal(t, "admin", cfg.PostgresConfig.Username)
	assert.Equal(t, "secret", cfg.PostgresConfig.Password)
	assert.Equal(t, "mydb", cfg.PostgresConfig.DBName)
	assert.Equal(t, "require", cfg.PostgresConfig.SSLMode)
}

func TestMustLoad_CustomHTTPAddress(t *testing.T) {
	clearEnvVars(t)
	t.Setenv("HTTP_SERVER_ADDRESS", "127.0.0.1:9090")
	cfg := MustLoad()
	assert.Equal(t, "127.0.0.1:9090", cfg.HTTPServer.Address)
}

func TestMustLoad_Timeout(t *testing.T) {
	clearEnvVars(t)
	cfg := MustLoad()
	assert.Equal(t, 4*time.Second, cfg.HTTPServer.Timeout)

	t.Setenv("HTTP_SERVER_TIMEOUT", "10s")
	cfg = MustLoad()
	assert.Equal(t, 10*time.Second, cfg.HTTPServer.Timeout)
}

func TestMustLoad_IdleTimeout(t *testing.T) {
	clearEnvVars(t)
	cfg := MustLoad()
	assert.Equal(t, 60*time.Second, cfg.HTTPServer.IdleTimeout)
}

func TestLoadEnv_MissingFile(t *testing.T) {
	t.Setenv("ENV_FILE", "/nonexistent/path/.env")
	require.NotPanics(t, func() {
		LoadEnv()
	})
}

func TestLoadEnv_ValidFile(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-env-*.env")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString("TEST_LOAD_ENV_VAR=hello\n")
	require.NoError(t, err)
	tmpFile.Close()

	t.Setenv("ENV_FILE", tmpFile.Name())
	LoadEnv()

	assert.Equal(t, "hello", os.Getenv("TEST_LOAD_ENV_VAR"))
	os.Unsetenv("TEST_LOAD_ENV_VAR")
}

// MustLoad использует log.Fatalf -> os.Exit(1), поэтому НЕ паникует и
// assert.Panics его не ловит. Стандартный приём — re-exec тестового
// бинаря в subprocess: запускаем helper-test с env-флагом и проверяем
// ненулевой код выхода + сообщение в выводе.

// TestMustLoad_FailLoudHelper — точка входа для subprocess. Запускается
// только когда выставлен TEST_MUSTLOAD_FAIL_LOUD=1, иначе скипается.
func TestMustLoad_FailLoudHelper(t *testing.T) {
	if os.Getenv("TEST_MUSTLOAD_FAIL_LOUD") != "1" {
		t.Skip("helper subprocess only — not run directly")
	}
	// В subprocess реальный .env подгружать не нужно.
	os.Setenv("ENV_FILE", "/dev/null")
	MustLoad()
}

// runMustLoadSubprocess re-execает текущий тестовый бинарь и запускает
// только helper-test выше. Возвращает combined output и код выхода.
func runMustLoadSubprocess(t *testing.T, env map[string]string) (string, int) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run", "^TestMustLoad_FailLoudHelper$")
	cmd.Env = append(os.Environ(), "TEST_MUSTLOAD_FAIL_LOUD=1")
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out, err := cmd.CombinedOutput()
	exitCode := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("unexpected subprocess error: %v\noutput: %s", err, out)
	}
	return string(out), exitCode
}

// TestMustLoad_InvalidEnvVars — table-driven проверка fail-loud поведения
// MustLoad для разных невалидных env-переменных. Один кейс — один env.
//
// Замечание про POSTGRES_PORT: поле объявлено как string, поэтому
// envconfig принимает любое значение и MustLoad НЕ падает. Этот кейс
// фиксирует фактическое поведение (shouldFail=false), чтобы будущий
// рефакторинг типа поля в int обязательно зацепил тест.
func TestMustLoad_InvalidEnvVars(t *testing.T) {
	cases := []struct {
		name       string
		envKey     string
		envValue   string
		shouldFail bool   // ожидаем ли ненулевой exit code
		wantInLog  string // подстрока, которая должна встретиться в выводе при fail
	}{
		{
			name:       "HTTP_SERVER_TIMEOUT не парсится как time.Duration",
			envKey:     "HTTP_SERVER_TIMEOUT",
			envValue:   "not-a-duration",
			shouldFail: true,
			wantInLog:  "HTTP_SERVER_TIMEOUT",
		},
		{
			name:       "HTTP_SERVER_IDLE_TIMEOUT не парсится как time.Duration",
			envKey:     "HTTP_SERVER_IDLE_TIMEOUT",
			envValue:   "not-a-duration",
			shouldFail: true,
			wantInLog:  "HTTP_SERVER_IDLE_TIMEOUT",
		},
		{
			// Поле POSTGRES_PORT — string, поэтому envconfig валидацию не
			// делает и MustLoad спокойно отрабатывает. Фиксируем это.
			name:       "POSTGRES_PORT нечисловой — НЕ падает (поле string)",
			envKey:     "POSTGRES_PORT",
			envValue:   "not-a-number",
			shouldFail: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, code := runMustLoadSubprocess(t, map[string]string{
				tc.envKey: tc.envValue,
			})
			if tc.shouldFail {
				assert.NotEqual(t, 0, code,
					"MustLoad должен упасть с ненулевым кодом; output:\n%s", out)
				assert.True(t,
					strings.Contains(out, "Failed to process config") ||
						strings.Contains(out, tc.wantInLog),
					"в логе ожидалось упоминание ошибки/переменной %q; output:\n%s",
					tc.wantInLog, out)
			} else {
				assert.Equal(t, 0, code,
					"MustLoad НЕ должен падать на этом значении; output:\n%s", out)
			}
		})
	}
}
