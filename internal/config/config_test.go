package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func clearEnvVars(t *testing.T) {
	t.Helper()
	// Prevent loading any real .env file
	t.Setenv("ENV_FILE", "/dev/null")
	// Clear all config-related env vars to get defaults
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
