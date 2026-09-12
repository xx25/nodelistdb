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
	// Encode before writing the status: once the header is out, an encoding
	// failure (a NaN in a float field is the realistic one) could only be
	// reported by truncating a 200 body.
	body, err := json.Marshal(data)
	if err != nil {
		logging.Error("Failed to encode JSON response", slog.Any("error", err))
		WriteJSONError(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write(append(body, '\n'))
}

// errorBody is every JSON error the API writes.
type errorBody struct {
	Error  string    `json:"error"`
	Status int       `json:"status"`
	Time   time.Time `json:"time"`
}

// WriteJSONError writes a JSON error response.
func WriteJSONError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(errorBody{
		Error:  message,
		Status: statusCode,
		Time:   time.Now().UTC(),
	}); err != nil {
		logging.Error("Failed to encode JSON error response", slog.Any("error", err))
	}
}

// WriteJSONSuccess writes a successful JSON response.
func WriteJSONSuccess(w http.ResponseWriter, data interface{}) {
	WriteJSON(w, data, http.StatusOK)
}
