package api

import (
	"encoding/json"
	"net/http"
	"time"
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
	json.NewEncoder(w).Encode(data)
}

// WriteJSONError writes a JSON error response.
func WriteJSONError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error":  message,
		"status": statusCode,
		"time":   time.Now().UTC(),
	})
}

// WriteJSONSuccess writes a successful JSON response.
func WriteJSONSuccess(w http.ResponseWriter, data interface{}) {
	WriteJSON(w, data, http.StatusOK)
}
