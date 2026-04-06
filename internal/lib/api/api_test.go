package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRedirect_Success_302(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://example.com")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()

	location, err := GetRedirect(srv.URL)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com", location)
}

func TestGetRedirect_ReturnsLocationHeader(t *testing.T) {
	target := "https://google.com/search?q=test"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", target)
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()

	location, err := GetRedirect(srv.URL)
	require.NoError(t, err)
	assert.Equal(t, target, location)
}

func TestGetRedirect_NoRedirect_200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, err := GetRedirect(srv.URL)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidStatusCode))
}

func TestGetRedirect_ServerError_500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := GetRedirect(srv.URL)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidStatusCode))
}

func TestGetRedirect_NotFound_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := GetRedirect(srv.URL)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidStatusCode))
}

func TestGetRedirect_InvalidURL(t *testing.T) {
	_, err := GetRedirect("://invalid-url")
	require.Error(t, err)
}

func TestGetRedirect_EmptyLocation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()

	location, err := GetRedirect(srv.URL)
	require.NoError(t, err)
	assert.Empty(t, location)
}

func TestGetRedirect_ErrorContainsStatusCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, err := GetRedirect(srv.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "200")
}

func TestGetRedirect_DoubleRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://final.com")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()

	location, err := GetRedirect(srv.URL)
	require.NoError(t, err)
	assert.Equal(t, "https://final.com", location)
}
