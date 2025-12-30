package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuth_Disabled(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	authMiddleware := Auth(AuthConfig{
		Enabled: false,
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	authMiddleware(handler).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestAuth_SkipPaths(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	authMiddleware := Auth(AuthConfig{
		Enabled:      true,
		BearerTokens: []string{"secret"},
		SkipPaths:    []string{"/healthz", "/metrics"},
	})

	tests := []struct {
		path     string
		expected int
	}{
		{"/healthz", http.StatusOK},
		{"/metrics", http.StatusOK},
		{"/api/v1/clusters", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()

			authMiddleware(handler).ServeHTTP(w, req)

			if w.Code != tt.expected {
				t.Errorf("path %s: expected status %d, got %d", tt.path, tt.expected, w.Code)
			}
		})
	}
}

func TestAuth_MissingHeader(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	authMiddleware := Auth(AuthConfig{
		Enabled:      true,
		BearerTokens: []string{"secret"},
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	authMiddleware(handler).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}

func TestAuth_InvalidFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	authMiddleware := Auth(AuthConfig{
		Enabled:      true,
		BearerTokens: []string{"secret"},
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	w := httptest.NewRecorder()

	authMiddleware(handler).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}

func TestAuth_InvalidToken(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	authMiddleware := Auth(AuthConfig{
		Enabled:      true,
		BearerTokens: []string{"secret"},
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer wrongtoken")
	w := httptest.NewRecorder()

	authMiddleware(handler).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}

func TestAuth_ValidToken(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	authMiddleware := Auth(AuthConfig{
		Enabled:      true,
		BearerTokens: []string{"secret", "anothersecret"},
	})

	tests := []struct {
		token    string
		expected int
	}{
		{"secret", http.StatusOK},
		{"anothersecret", http.StatusOK},
		{"wrongtoken", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.token, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set("Authorization", "Bearer "+tt.token)
			w := httptest.NewRecorder()

			authMiddleware(handler).ServeHTTP(w, req)

			if w.Code != tt.expected {
				t.Errorf("token %s: expected status %d, got %d", tt.token, tt.expected, w.Code)
			}
		})
	}
}

func TestAuth_SkipPathPrefix(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	authMiddleware := Auth(AuthConfig{
		Enabled:      true,
		BearerTokens: []string{"secret"},
		SkipPaths:    []string{"/healthz"},
	})

	// Both /healthz and /healthz/ should be skipped
	paths := []string{"/healthz", "/healthz/"}
	for _, path := range paths {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()

		authMiddleware(handler).ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("path %s: expected status 200, got %d", path, w.Code)
		}
	}
}
