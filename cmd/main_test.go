package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupLogger_Local(t *testing.T) {
	log := setupLogger("local")
	require.NotNil(t, log)
	assert.True(t, log.Enabled(nil, -4)) // Debug level
}

func TestSetupLogger_Dev(t *testing.T) {
	log := setupLogger("dev")
	require.NotNil(t, log)
	assert.True(t, log.Enabled(nil, -4)) // Debug level
}

func TestSetupLogger_Prod(t *testing.T) {
	log := setupLogger("prod")
	require.NotNil(t, log)
	assert.True(t, log.Enabled(nil, 0))  // Info level
	assert.False(t, log.Enabled(nil, -4)) // Debug level disabled in prod
}

func TestSetupLogger_Unknown(t *testing.T) {
	log := setupLogger("unknown_env")
	require.NotNil(t, log)
	assert.True(t, log.Enabled(nil, 0))  // Info level (defaults to prod)
	assert.False(t, log.Enabled(nil, -4)) // Debug disabled
}

func TestSetupLogger_EmptyString(t *testing.T) {
	log := setupLogger("")
	require.NotNil(t, log)
	assert.True(t, log.Enabled(nil, 0))  // Info level (defaults to prod)
	assert.False(t, log.Enabled(nil, -4)) // Debug disabled
}
