package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/tjorri/observability-federation-proxy/internal/metrics"
)

// responseWriter wraps http.ResponseWriter to capture status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    int64
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.written += int64(n)
	return n, err
}

// Metrics returns middleware that records Prometheus metrics.
func Metrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		metrics.HTTPRequestsInFlight.Inc()
		defer metrics.HTTPRequestsInFlight.Dec()

		rw := newResponseWriter(w)
		next.ServeHTTP(rw, r)

		duration := time.Since(start).Seconds()
		path := normalizePath(r.URL.Path)
		status := strconv.Itoa(rw.statusCode)

		metrics.HTTPRequestsTotal.WithLabelValues(r.Method, path, status).Inc()
		metrics.HTTPRequestDuration.WithLabelValues(r.Method, path).Observe(duration)
	})
}

// Logging returns middleware that logs HTTP requests.
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		rw := newResponseWriter(w)
		next.ServeHTTP(rw, r)

		duration := time.Since(start)

		// Skip logging for health check endpoints at debug level
		logEvent := log.Debug()
		if rw.statusCode >= 400 {
			logEvent = log.Warn()
		}
		if rw.statusCode >= 500 {
			logEvent = log.Error()
		}

		// Skip verbose logging for health endpoints
		path := r.URL.Path
		if path == "/healthz" || path == "/readyz" || path == "/metrics" {
			logEvent = log.Trace()
		}

		logEvent.
			Str("method", r.Method).
			Str("path", path).
			Int("status", rw.statusCode).
			Int64("bytes", rw.written).
			Dur("duration", duration).
			Str("remote_addr", r.RemoteAddr).
			Str("user_agent", r.UserAgent()).
			Msg("http request")
	})
}

// Recovery returns middleware that recovers from panics.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Error().
					Interface("panic", err).
					Str("method", r.Method).
					Str("path", r.URL.Path).
					Msg("recovered from panic")

				http.Error(w, `{"error": "internal server error"}`, http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// Chain chains multiple middleware together.
func Chain(handler http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

// normalizePath normalizes the path for metrics labels to prevent cardinality explosion.
func normalizePath(path string) string {
	// Replace dynamic path segments with placeholders
	parts := strings.Split(path, "/")
	for i, part := range parts {
		// Replace cluster names in /clusters/{cluster}/...
		if i > 0 && parts[i-1] == "clusters" && part != "" {
			parts[i] = "{cluster}"
		}
		// Replace label names in /label/{name}/values
		if i > 0 && parts[i-1] == "label" && part != "" && i+1 < len(parts) && parts[i+1] == "values" {
			parts[i] = "{name}"
		}
	}
	return strings.Join(parts, "/")
}
