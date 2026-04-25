package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizeAllowedOriginsCanonicalizesDefaultPorts(t *testing.T) {
	allowed := normalizeAllowedOrigins([]string{
		"https://SMB.EXAMPLE.COM:443/admin",
		"http://smb.example.com:80",
		"https://smb.example.com:8443",
		"smb.example.com",
	})

	for _, origin := range []string{
		"https://smb.example.com",
		"http://smb.example.com",
		"https://smb.example.com:8443",
	} {
		if _, ok := allowed[origin]; !ok {
			t.Fatalf("expected %q to be allowed; got %#v", origin, allowed)
		}
	}
	if _, ok := allowed["smb.example.com"]; ok {
		t.Fatalf("bare host must not be treated as an origin")
	}
}

func TestIsTrustedRequestCanonicalizesRequestOrigin(t *testing.T) {
	allowed := normalizeAllowedOrigins([]string{"https://smb.example.com"})
	req := httptest.NewRequest(http.MethodGet, "http://internal/", nil)
	req.RemoteAddr = "127.0.0.1:45678"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "SMB.EXAMPLE.COM:443")

	if !isTrustedRequest(req, allowed) {
		t.Fatal("expected request with default forwarded HTTPS port to be trusted")
	}
}

func TestIsTrustedRequestAcceptsFirstForwardedValue(t *testing.T) {
	allowed := normalizeAllowedOrigins([]string{"https://smb.example.com"})
	req := httptest.NewRequest(http.MethodGet, "http://internal/", nil)
	req.RemoteAddr = "127.0.0.1:45678"
	req.Header.Set("X-Forwarded-Proto", "https, http")
	req.Header.Set("X-Forwarded-Host", "smb.example.com, internal")

	if !isTrustedRequest(req, allowed) {
		t.Fatal("expected first forwarded origin to be trusted")
	}
}

func TestIsTrustedRequestAcceptsStandardForwardedHeader(t *testing.T) {
	allowed := normalizeAllowedOrigins([]string{"https://smb.example.com"})
	req := httptest.NewRequest(http.MethodGet, "http://internal/", nil)
	req.RemoteAddr = "127.0.0.1:45678"
	req.Header.Set("Forwarded", `for=192.0.2.10;proto=https;host="smb.example.com:443"`)

	if !isTrustedRequest(req, allowed) {
		t.Fatal("expected standard Forwarded origin to be trusted")
	}
}

func TestIsTrustedRequestIgnoresForwardedHeadersFromNonLoopback(t *testing.T) {
	allowed := normalizeAllowedOrigins([]string{"https://smb.example.com"})
	req := httptest.NewRequest(http.MethodGet, "http://internal/", nil)
	req.RemoteAddr = "192.0.2.55:45678"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "smb.example.com")

	if isTrustedRequest(req, allowed) {
		t.Fatal("forwarded headers from non-loopback clients must not be trusted")
	}
}

func TestIsSecureRequestAcceptsForwardedHTTPS(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://internal/api/v1/auth/login", nil)
	req.RemoteAddr = "127.0.0.1:45678"
	req.Header.Set("X-Forwarded-Proto", "https, http")

	if !isSecureRequest(req) {
		t.Fatal("expected loopback forwarded HTTPS request to be secure")
	}
}

func TestIsSecureRequestAcceptsStandardForwardedHTTPS(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://internal/api/v1/auth/login", nil)
	req.RemoteAddr = "127.0.0.1:45678"
	req.Header.Set("Forwarded", `for=192.0.2.10;proto=https;host=smb.example.com`)

	if !isSecureRequest(req) {
		t.Fatal("expected standard forwarded HTTPS request to be secure")
	}
}
