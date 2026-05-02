package redirect

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIsPrivateIP — табличный тест по 5 классам эквивалентности.
// Принцип EP: один представитель на класс. Дополнительные представители
// внутри класса не находят новых багов.
func TestIsPrivateIP(t *testing.T) {
	cases := []struct {
		name string
		ip   string
		want bool
	}{
		// EQ_invalid: парсер net.ParseIP вернёт nil → функция возвращает false
		{"invalid", "garbage", false},
		// EQ_private_v4: попадает под net.IP.IsPrivate() (RFC 1918)
		{"private_v4", "192.168.1.1", true},
		// EQ_loopback_v4: попадает под net.IP.IsLoopback() (127.0.0.0/8)
		{"loopback_v4", "127.0.0.1", true},
		// EQ_loopback_v6: граничный случай — IPv6 loopback (::1)
		{"loopback_v6", "::1", true},
		// EQ_public: ни IsPrivate, ни IsLoopback — возвращаем false
		{"public", "8.8.8.8", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isPrivateIP(tc.ip))
		})
	}
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
