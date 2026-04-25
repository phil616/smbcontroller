package handler

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strings"

	"smb-controller/internal/session"
)

type contextKey string

const sessionContextKey contextKey = "session"

func AuthMiddleware(store *session.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("session_id")
			if err != nil {
				writeError(w, http.StatusUnauthorized, "Unauthorized")
				return
			}
			sess, ok := store.Get(cookie.Value)
			if !ok {
				writeError(w, http.StatusUnauthorized, "Session expired")
				return
			}
			ctx := context.WithValue(r.Context(), sessionContextKey, sess)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func CurrentSession(r *http.Request) (*session.Session, bool) {
	sess, ok := r.Context().Value(sessionContextKey).(*session.Session)
	return sess, ok
}

func TrustedDomainMiddleware(domains []string) func(http.Handler) http.Handler {
	allowed := normalizeAllowedOrigins(domains)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(allowed) == 0 || isTrustedRequest(r, allowed) {
				next.ServeHTTP(w, r)
				return
			}
			if strings.HasPrefix(r.URL.Path, "/api/") {
				writeError(w, http.StatusForbidden, "环境不可信：当前访问域名不在服务端配置的可信域名列表中")
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>环境不可信</title><style>body{margin:0;min-height:100vh;display:grid;place-items:center;font:15px system-ui;background:#f5f7f9;color:#1f2933}.panel{max-width:560px;background:#fff;border:1px solid #d9e0e6;border-radius:8px;padding:28px;box-shadow:0 14px 40px rgba(31,41,51,.08)}h1{margin:0 0 12px;font-size:24px}p{line-height:1.6;color:#64717d}</style></head><body><main class="panel"><h1>环境不可信</h1><p>当前访问地址不在服务端配置的可信域名列表中。请使用管理员配置的域名访问 SMB Controller，或联系管理员检查 server.domain 配置。</p></main></body></html>`))
		})
	}
}

func normalizeAllowedOrigins(domains []string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, domain := range domains {
		domain = strings.TrimSpace(domain)
		if domain == "" {
			continue
		}
		u, err := url.Parse(domain)
		if err != nil || u.Scheme == "" || u.Host == "" {
			continue
		}
		if origin, ok := canonicalOrigin(u.Scheme, u.Host); ok {
			out[origin] = struct{}{}
		}
	}
	return out
}

func isTrustedRequest(r *http.Request, allowed map[string]struct{}) bool {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	host := r.Host
	if requestFromLoopback(r) {
		forwardedProto, forwardedHost := forwardedRequestOrigin(r)
		if forwardedProto != "" {
			scheme = forwardedProto
		}
		if forwardedHost != "" {
			host = forwardedHost
		}
	}
	origin, ok := canonicalOrigin(scheme, host)
	if !ok {
		return false
	}
	_, ok = allowed[origin]
	return ok
}

func canonicalOrigin(scheme, host string) (string, bool) {
	scheme = strings.ToLower(strings.TrimSpace(firstHeaderValue(scheme)))
	host = strings.TrimSpace(firstHeaderValue(host))
	if scheme != "http" && scheme != "https" || host == "" {
		return "", false
	}
	if strings.Contains(host, "://") {
		u, err := url.Parse(host)
		if err != nil || u.Host == "" {
			return "", false
		}
		host = u.Host
	}
	if hostOnly, port, err := net.SplitHostPort(host); err == nil {
		defaultPort := scheme == "http" && port == "80" || scheme == "https" && port == "443"
		if defaultPort {
			host = hostOnly
		}
	}
	host = strings.ToLower(host)
	return scheme + "://" + host, true
}

func forwardedRequestOrigin(r *http.Request) (scheme, host string) {
	if forwardedProto := r.Header.Get("X-Forwarded-Proto"); forwardedProto != "" {
		scheme = forwardedProto
	}
	if forwardedHost := r.Header.Get("X-Forwarded-Host"); forwardedHost != "" {
		host = forwardedHost
	}
	if scheme != "" || host != "" {
		return scheme, host
	}
	return parseForwardedOrigin(r.Header.Get("Forwarded"))
}

func parseForwardedOrigin(header string) (scheme, host string) {
	first := firstHeaderValue(header)
	for _, part := range strings.Split(first, ";") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"`)
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "proto":
			scheme = value
		case "host":
			host = value
		}
	}
	return scheme, host
}

func firstHeaderValue(value string) string {
	value, _, _ = strings.Cut(value, ",")
	return strings.TrimSpace(value)
}

func requestFromLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isSecureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if !requestFromLoopback(r) {
		return false
	}
	forwardedProto, _ := forwardedRequestOrigin(r)
	return strings.EqualFold(firstHeaderValue(forwardedProto), "https")
}
