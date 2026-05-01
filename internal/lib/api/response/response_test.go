package response

import (
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResponseConstructors — единый КОНТРАКТНЫЙ тест на публичные функции
// OK() и Error(msg). Цель — НЕ покрыть две строки кода, а зафиксировать
// контракт значений в JSON-ответе:
//
//	OK()        → {"status": "OK"}
//	Error(msg)  → {"status": "Error", "error": <msg>}
//
// Если кто-то изменит StatusOK = "OK" на "ok" / "success" — компилятор
// этого не поймает (это просто значение константы), а фронт сломается.
// Этот тест защищает от такого breaking change.
//
// Прежде это был перебор: TestOK_Status + TestOK_NoError + TestError_*
// с подкейсом «10000 символов» (последний — overtesting: компилятор Go
// строки сам не обрезает, реалистичных багов кейс не находит).
func TestResponseConstructors(t *testing.T) {
	t.Run("OK", func(t *testing.T) {
		r := OK()
		assert.Equal(t, StatusOK, r.Status)
		assert.Empty(t, r.Error)
	})

	t.Run("Error_with_message", func(t *testing.T) {
		r := Error("something went wrong")
		assert.Equal(t, StatusError, r.Status)
		assert.Equal(t, "something went wrong", r.Error)
	})

	// Граничный случай: пустое сообщение. Контракт — Status="Error" даже когда
	// текста ошибки нет. Защищает от соблазна «оптимизировать» через
	// `if msg == "" { return OK() }`.
	t.Run("Error_empty_message", func(t *testing.T) {
		r := Error("")
		assert.Equal(t, StatusError, r.Status)
		assert.Empty(t, r.Error)
	})
}

// Helper structs для генерации реальных ошибок валидатора.

type requiredStruct struct {
	Name string `validate:"required"`
}

type urlStruct struct {
	Link string `validate:"url"`
}

type minStruct struct {
	Value int `validate:"min=10"`
}

type multiFieldStruct struct {
	Name string `validate:"required"`
	Link string `validate:"url"`
}

// TestValidationError_PerTag — три ветки `switch err.ActualTag()` в ValidationError.
// Объединяет прежние `_RequiredTag` / `_UrlTag` / `_DefaultTag` в табличный тест,
// где видно, что это **классы эквивалентности по тегу валидатора**.
//
// Хвосты сообщений ("is a required field" и т. п.) проверяются как substring,
// потому что они ЗАДАНЫ ИМЕННО В response.go (не в чужой либе) — то есть
// тест корректно отражает контракт, который задаёт сам код.
func TestValidationError_PerTag(t *testing.T) {
	cases := []struct {
		name        string
		input       interface{}
		expectField string
		expectTail  string
	}{
		{"required_tag", requiredStruct{Name: ""}, "Name", "is a required field"},
		{"url_tag", urlStruct{Link: "not-a-url"}, "Link", "is not a valid URL"},
		{"default_tag", minStruct{Value: 1}, "Value", "is not valid"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			v := validator.New()
			err := v.Struct(tc.input)
			require.Error(t, err)

			r := ValidationError(err.(validator.ValidationErrors))
			assert.Equal(t, StatusError, r.Status)
			assert.Contains(t, r.Error, "field "+tc.expectField,
				"имя поля должно присутствовать в сообщении")
			assert.Contains(t, r.Error, tc.expectTail,
				"хвост контролируется самим response.go (см. ValidationError)")
		})
	}
}

// TestValidationError_JoinsMultiple — две ошибки валидации объединяются через ", ".
// Сравнение со случаем одной ошибки показывает, что разделитель появляется
// тогда и только тогда, когда ошибок > 1.
func TestValidationError_JoinsMultiple(t *testing.T) {
	v := validator.New()

	t.Run("single_error_no_separator", func(t *testing.T) {
		err := v.Struct(requiredStruct{Name: ""})
		require.Error(t, err)
		r := ValidationError(err.(validator.ValidationErrors))
		assert.NotContains(t, r.Error, ", ")
	})

	t.Run("two_errors_joined_by_comma", func(t *testing.T) {
		err := v.Struct(multiFieldStruct{Name: "", Link: "bad"})
		require.Error(t, err)
		r := ValidationError(err.(validator.ValidationErrors))
		assert.Contains(t, r.Error, ", ")
		// проверяем, что обе ошибки реально присутствуют
		assert.Contains(t, r.Error, "field Name")
		assert.Contains(t, r.Error, "field Link")
	})
}

// TestValidationError_EmptySlice — граничный случай: вызов с пустым списком ошибок.
// Status="Error" по контракту, но Error="" (нечего объединять).
func TestValidationError_EmptySlice(t *testing.T) {
	var errs validator.ValidationErrors
	r := ValidationError(errs)
	assert.Equal(t, StatusError, r.Status)
	assert.Empty(t, r.Error)
}
