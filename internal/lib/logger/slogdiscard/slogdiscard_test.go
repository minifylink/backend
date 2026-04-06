package slogdiscard

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDiscardLogger_NotNil(t *testing.T) {
	log := NewDiscardLogger()
	require.NotNil(t, log)
}

func TestDiscardHandler_Enabled_ReturnsFalse(t *testing.T) {
	h := NewDiscardHandler()
	assert.False(t, h.Enabled(context.Background(), slog.LevelInfo))
	assert.False(t, h.Enabled(context.Background(), slog.LevelDebug))
	assert.False(t, h.Enabled(context.Background(), slog.LevelError))
}

func TestDiscardHandler_Handle_ReturnsNil(t *testing.T) {
	h := NewDiscardHandler()
	err := h.Handle(context.Background(), slog.Record{})
	assert.NoError(t, err)
}

func TestDiscardHandler_WithAttrs_ReturnsHandler(t *testing.T) {
	h := NewDiscardHandler()
	result := h.WithAttrs([]slog.Attr{slog.String("key", "value")})
	assert.NotNil(t, result)
}

func TestDiscardHandler_WithGroup_ReturnsHandler(t *testing.T) {
	h := NewDiscardHandler()
	result := h.WithGroup("test-group")
	assert.NotNil(t, result)
}
