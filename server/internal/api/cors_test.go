package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewCORSMiddleware_AllowedOrigin(t *testing.T) {
	origins := []string{"http://localhost:5173", "https://app.example.com"}
	handler := newCORSMiddleware(origins)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, origin := range origins {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/agents", nil)
		req.Header.Set("Origin", origin)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, origin, rr.Header().Get("Access-Control-Allow-Origin"))
		assert.NotEmpty(t, rr.Header().Get("Access-Control-Allow-Methods"))
		assert.NotEmpty(t, rr.Header().Get("Access-Control-Allow-Headers"))
		assert.Equal(t, "Origin", rr.Header().Get("Vary"))
	}
}

func TestNewCORSMiddleware_DisallowedOrigin(t *testing.T) {
	origins := []string{"http://localhost:5173"}
	handler := newCORSMiddleware(origins)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/agents", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))
	assert.Empty(t, rr.Header().Get("Access-Control-Allow-Methods"))
	assert.Empty(t, rr.Header().Get("Access-Control-Allow-Headers"))
}

func TestNewCORSMiddleware_NoOriginHeader(t *testing.T) {
	origins := []string{"http://localhost:5173"}
	handler := newCORSMiddleware(origins)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Same-origin requests carry no Origin header; they must not be affected.
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/agents", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))
	assert.Empty(t, rr.Header().Get("Access-Control-Allow-Methods"))
}

func TestNewCORSMiddleware_PreflightAllowed(t *testing.T) {
	origins := []string{"http://localhost:5173"}
	handler := newCORSMiddleware(origins)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK) // should not be reached
	}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodOptions, "/api/agents", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNoContent, rr.Code)
	assert.Equal(t, "http://localhost:5173", rr.Header().Get("Access-Control-Allow-Origin"))
}

func TestNewCORSMiddleware_PreflightDisallowed(t *testing.T) {
	origins := []string{"http://localhost:5173"}
	handler := newCORSMiddleware(origins)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK) // should not be reached
	}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodOptions, "/api/agents", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Pre-flight from a disallowed origin must be rejected outright (no CORS headers).
	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))
}

func TestNewCORSMiddleware_EmptyOriginList(t *testing.T) {
	handler := newCORSMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/agents", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))
}

func TestNewCORSMiddleware_PreflightNoOriginPassedThrough(t *testing.T) {
	origins := []string{"http://localhost:5173"}
	called := false
	handler := newCORSMiddleware(origins)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	// OPTIONS without an Origin header is a plain HTTP OPTIONS request (not a CORS
	// pre-flight) and must be handled normally (short-circuit with 204, not 403).
	req := httptest.NewRequestWithContext(context.Background(), http.MethodOptions, "/api/agents", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNoContent, rr.Code)
	assert.False(t, called, "underlying handler should not be invoked for OPTIONS")
}
