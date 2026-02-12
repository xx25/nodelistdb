package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/nodelistdb/internal/logging"
)

// EnableCORS wraps a handler to add CORS headers.
func (s *Server) EnableCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

// MethodCheck ensures only specified HTTP methods are allowed.
func MethodCheck(allowedMethod string) func(http.ResponseWriter, *http.Request) bool {
	return func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method != allowedMethod {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return false
		}
		return true
	}
}

// CheckMethod is a helper to check if the request method is allowed.
func CheckMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method != method {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	return true
}

// WriteJSON writes a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, data interface{}, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		logging.Error("Failed to encode JSON response", slog.Any("error", err))
	}
}

// WriteJSONError writes a JSON error response.
func WriteJSONError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"error":  message,
		"status": statusCode,
		"time":   time.Now().UTC(),
	}); err != nil {
		logging.Error("Failed to encode JSON error response", slog.Any("error", err))
	}
}

// WriteJSONSuccess writes a successful JSON response.
func WriteJSONSuccess(w http.ResponseWriter, data interface{}) {
	WriteJSON(w, data, http.StatusOK)
}

// CORSMiddleware handles CORS (if needed)
func (s *Server) CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
