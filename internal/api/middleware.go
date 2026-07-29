package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/nodelistdb/internal/logging"
)

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
