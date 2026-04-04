package redirect

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Existing test
func TestIsPrivateIP(t *testing.T) {
	assert.False(t, isPrivateIP("not-an-ip"))  // parsedIP == nil
	assert.True(t, isPrivateIP("192.168.1.1")) // IsPrivate()
	assert.True(t, isPrivateIP("127.0.0.1"))   // IsLoopback()
	assert.False(t, isPrivateIP("8.8.8.8"))    // публичный IP → return false
}

// New tests for isPrivateIP

func TestIsPrivateIP_InvalidIP(t *testing.T) {
	assert.False(t, isPrivateIP("garbage"))
	assert.False(t, isPrivateIP(""))
	assert.False(t, isPrivateIP("999.999.999.999"))
}

func TestIsPrivateIP_IPv6Loopback(t *testing.T) {
	assert.True(t, isPrivateIP("::1"))
}

func TestIsPrivateIP_PublicIP(t *testing.T) {
	assert.False(t, isPrivateIP("8.8.8.8"))
	assert.False(t, isPrivateIP("1.1.1.1"))
}

// Tests for getIP

func TestGetIP_XRealIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Real-IP", "203.0.113.1")

	ip := getIP(req)
	assert.Equal(t, "203.0.113.1", ip)
}

func TestGetIP_XForwardedFor_Single(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "198.51.100.1")

	ip := getIP(req)
	assert.Equal(t, "198.51.100.1", ip)
}

func TestGetIP_XForwardedFor_Multiple(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "198.51.100.1, 10.0.0.1, 172.16.0.1")

	ip := getIP(req)
	assert.Equal(t, "198.51.100.1", ip)
}

func TestGetIP_XForwardedFor_WithSpaces(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "  198.51.100.1 , 10.0.0.1")

	ip := getIP(req)
	assert.Equal(t, "198.51.100.1", ip)
}

func TestGetIP_RemoteAddr_WithPort(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"

	ip := getIP(req)
	assert.Equal(t, "10.0.0.1", ip)
}

func TestGetIP_RemoteAddr_NoPort(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1"

	ip := getIP(req)
	assert.Equal(t, "10.0.0.1", ip)
}

func TestGetIP_Priority(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Real-IP", "1.1.1.1")
	req.Header.Set("X-Forwarded-For", "2.2.2.2")
	req.RemoteAddr = "3.3.3.3:1234"

	ip := getIP(req)
	assert.Equal(t, "1.1.1.1", ip, "X-Real-IP should take precedence")
}

func TestGetIP_EmptyHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = ""

	ip := getIP(req)
	assert.Equal(t, "", ip)
}

// Tests for getCountry

func TestGetCountry_PrivateIP(t *testing.T) {
	country, err := getCountry("192.168.1.1")
	require.NoError(t, err)
	assert.Equal(t, "local", country)
}

func TestGetCountry_LoopbackIP(t *testing.T) {
	country, err := getCountry("127.0.0.1")
	require.NoError(t, err)
	assert.Equal(t, "local", country)
}
