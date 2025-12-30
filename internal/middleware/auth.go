package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"
)

// AuthConfig holds authentication configuration.
type AuthConfig struct {
	// Enabled controls whether authentication is required.
	Enabled bool
	// BearerTokens is a list of valid bearer tokens.
	BearerTokens []string
	// SkipPaths are paths that don't require authentication.
	SkipPaths []string
}

// Auth returns middleware that validates authentication.
func Auth(cfg AuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !cfg.Enabled {
				next.ServeHTTP(w, r)
				return
			}

			// Check if path should skip auth
			for _, skip := range cfg.SkipPaths {
				if r.URL.Path == skip || strings.HasPrefix(r.URL.Path, skip+"/") {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Get authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				log.Debug().
					Str("path", r.URL.Path).
					Str("remote_addr", r.RemoteAddr).
					Msg("missing authorization header")
				http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
				return
			}

			// Check for bearer token
			if !strings.HasPrefix(authHeader, "Bearer ") {
				log.Debug().
					Str("path", r.URL.Path).
					Str("remote_addr", r.RemoteAddr).
					Msg("invalid authorization header format")
				http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
				return
			}

			token := strings.TrimPrefix(authHeader, "Bearer ")

			// Validate token
			valid := false
			for _, validToken := range cfg.BearerTokens {
				if subtle.ConstantTimeCompare([]byte(token), []byte(validToken)) == 1 {
					valid = true
					break
				}
			}

			if !valid {
				log.Debug().
					Str("path", r.URL.Path).
					Str("remote_addr", r.RemoteAddr).
					Msg("invalid bearer token")
				http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
