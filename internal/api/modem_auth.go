package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/nodelistdb/internal/config"
)

// contextKey is a type for context keys to avoid collisions
type contextKey string

const (
	// CallerIDContextKey is the context key for the authenticated caller ID
	CallerIDContextKey contextKey = "modem_caller_id"
)

// ModemAuthMiddleware creates a middleware that validates Bearer tokens against config
func ModemAuthMiddleware(cfg *config.ModemAPIConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract Bearer token
			apiKey := extractBearerToken(r)
			if apiKey == "" {
				WriteJSONError(w, "missing or invalid Authorization header", http.StatusUnauthorized)
				return
			}

			// Validate API key and get caller ID
			callerID, err := validateAPIKey(cfg, apiKey)
			if err != nil {
				WriteJSONError(w, "invalid API key", http.StatusUnauthorized)
				return
			}

			// Store caller ID in context for handlers to use
			ctx := context.WithValue(r.Context(), CallerIDContextKey, callerID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// extractBearerToken extracts the API key from the Authorization header
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}

	// Check for "Bearer " prefix (case-insensitive)
	if len(auth) < 7 {
		return ""
	}

	prefix := strings.ToLower(auth[:7])
	if prefix != "bearer " {
		return ""
	}

	return strings.TrimSpace(auth[7:])
}

// validateAPIKey validates the API key against configured callers
// Returns the caller_id if valid, error if invalid
func validateAPIKey(cfg *config.ModemAPIConfig, apiKey string) (string, error) {
	// Compute SHA256 hash of the provided API key
	hash := sha256.Sum256([]byte(apiKey))
	keyHash := "sha256:" + hex.EncodeToString(hash[:])

	// Check against all configured callers
	for _, caller := range cfg.Callers {
		// Use constant-time comparison to prevent timing attacks
		if subtle.ConstantTimeCompare([]byte(caller.APIKeyHash), []byte(keyHash)) == 1 {
			return caller.CallerID, nil
		}
	}

	return "", fmt.Errorf("invalid API key")
}

// GetCallerIDFromContext retrieves the authenticated caller ID from the request context
func GetCallerIDFromContext(ctx context.Context) string {
	callerID, ok := ctx.Value(CallerIDContextKey).(string)
	if !ok {
		return ""
	}
	return callerID
}

// AuthMiddleware returns the authentication middleware for the handler's config
func (h *ModemHandler) AuthMiddleware() func(http.Handler) http.Handler {
	return ModemAuthMiddleware(h.config)
}

// SizeLimitMiddleware returns the request size limit middleware for the handler's config
func (h *ModemHandler) SizeLimitMiddleware() func(http.Handler) http.Handler {
	maxBytes := int64(h.config.MaxBodySizeMB) * 1024 * 1024
	if maxBytes <= 0 {
		maxBytes = 1 * 1024 * 1024 // Default 1MB
	}
	return RequestSizeLimitMiddleware(maxBytes)
}

// HashAPIKey generates a hash string for a given API key
// Use this to generate the api_key_hash value for config.yaml
func HashAPIKey(apiKey string) string {
	hash := sha256.Sum256([]byte(apiKey))
	return "sha256:" + hex.EncodeToString(hash[:])
}

// RequestSizeLimitMiddleware creates a middleware that limits request body size
func RequestSizeLimitMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > maxBytes {
				WriteJSONError(w, fmt.Sprintf("request body too large (max %d bytes)", maxBytes), http.StatusRequestEntityTooLarge)
				return
			}
			// Limit the reader to prevent reading more than maxBytes
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}
