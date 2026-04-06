package response

import (
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOK_Status(t *testing.T) {
	r := OK()
	assert.Equal(t, StatusOK, r.Status)
}

func TestOK_NoError(t *testing.T) {
	r := OK()
	assert.Empty(t, r.Error)
}

func TestError_WithMessage(t *testing.T) {
	r := Error("something went wrong")
	assert.Equal(t, StatusError, r.Status)
	assert.Equal(t, "something went wrong", r.Error)
}

func TestError_EmptyMessage(t *testing.T) {
	r := Error("")
	assert.Equal(t, StatusError, r.Status)
	assert.Empty(t, r.Error)
}

func TestError_LongMessage(t *testing.T) {
	longMsg := strings.Repeat("a", 10000)
	r := Error(longMsg)
	assert.Equal(t, StatusError, r.Status)
	assert.Equal(t, longMsg, r.Error)
}

// Helper structs for triggering real validation errors.

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

func TestValidationError_RequiredTag(t *testing.T) {
	v := validator.New()
	err := v.Struct(requiredStruct{Name: ""})
	require.Error(t, err)

	r := ValidationError(err.(validator.ValidationErrors))
	assert.Equal(t, StatusError, r.Status)
	assert.Contains(t, r.Error, "is a required field")
}

func TestValidationError_UrlTag(t *testing.T) {
	v := validator.New()
	err := v.Struct(urlStruct{Link: "not-a-url"})
	require.Error(t, err)

	r := ValidationError(err.(validator.ValidationErrors))
	assert.Equal(t, StatusError, r.Status)
	assert.Contains(t, r.Error, "is not a valid URL")
}

func TestValidationError_DefaultTag(t *testing.T) {
	v := validator.New()
	err := v.Struct(minStruct{Value: 1})
	require.Error(t, err)

	r := ValidationError(err.(validator.ValidationErrors))
	assert.Equal(t, StatusError, r.Status)
	assert.Contains(t, r.Error, "is not valid")
}

func TestValidationError_MultipleErrors(t *testing.T) {
	v := validator.New()
	err := v.Struct(multiFieldStruct{Name: "", Link: "bad"})
	require.Error(t, err)

	r := ValidationError(err.(validator.ValidationErrors))
	assert.Equal(t, StatusError, r.Status)
	assert.Contains(t, r.Error, ", ")
}

func TestValidationError_SingleError(t *testing.T) {
	v := validator.New()
	err := v.Struct(requiredStruct{Name: ""})
	require.Error(t, err)

	r := ValidationError(err.(validator.ValidationErrors))
	assert.NotContains(t, r.Error, ", ")
}

func TestValidationError_EmptySlice(t *testing.T) {
	var errs validator.ValidationErrors
	r := ValidationError(errs)
	assert.Equal(t, StatusError, r.Status)
	assert.Empty(t, r.Error)
}
