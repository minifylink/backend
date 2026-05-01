package slogdiscard

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDiscardHandler_AlwaysDisabled — главное контрактное свойство:
// `Enabled` ВСЕГДА возвращает false, для любого уровня. Из этого следует,
// что любой вызов `slog.Info`/`Debug`/`Error` с этим хэндлером не приведёт
// ни к какому I/O — что и нужно в тестах.
func TestDiscardHandler_AlwaysDisabled(t *testing.T) {
	h := NewDiscardHandler()
	levels := []slog.Level{
		slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError,
		slog.Level(-100), slog.Level(100),
	}
	for _, lvl := range levels {
		assert.Falsef(t, h.Enabled(context.Background(), lvl),
			"уровень %d должен быть disabled", lvl)
	}
}

// TestDiscardHandler_HandleIsNoop — `Handle` ничего не делает и не возвращает ошибку.
func TestDiscardHandler_HandleIsNoop(t *testing.T) {
	h := NewDiscardHandler()
	assert.NoError(t, h.Handle(context.Background(), slog.Record{}))
}

// TestDiscardHandler_WithAttrsAndGroup_PreserveDiscardSemantics — главная
// проверка для `WithAttrs` и `WithGroup`: возвращённый handler ДОЛЖЕН
// сохранять discard-семантику (Enabled=false). Прежние тесты проверяли только
// not-nil — и пропустили бы баг, при котором WithAttrs возвращает «обычный» handler.
func TestDiscardHandler_WithAttrsAndGroup_PreserveDiscardSemantics(t *testing.T) {
	h := NewDiscardHandler()

	t.Run("WithAttrs", func(t *testing.T) {
		result := h.WithAttrs([]slog.Attr{slog.String("key", "value")})
		assert.NotNil(t, result)
		assert.False(t, result.Enabled(context.Background(), slog.LevelInfo),
			"WithAttrs должен сохранить discard-семантику")
	})

	t.Run("WithGroup", func(t *testing.T) {
		result := h.WithGroup("test-group")
		assert.NotNil(t, result)
		assert.False(t, result.Enabled(context.Background(), slog.LevelInfo),
			"WithGroup должен сохранить discard-семантику")
	})
}

// TestNewDiscardLogger_DoesNotPanicOnAnyLevel — фабрика создаёт *slog.Logger,
// и любой вызов Info/Debug/Error на нём не должен паниковать (в т. ч. с атрибутами
// и группами). Заменяет прежний `_NotNil`, который проверял только nil-ность.
func TestNewDiscardLogger_DoesNotPanicOnAnyLevel(t *testing.T) {
	log := NewDiscardLogger()
	assert.NotNil(t, log)
	assert.NotPanics(t, func() {
		log.Debug("debug", "k", "v")
		log.Info("info", "k", "v")
		log.Warn("warn", "k", "v")
		log.Error("error", "k", "v")
		log.With("a", 1).WithGroup("g").Info("nested")
	})
}
