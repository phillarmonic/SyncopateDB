package api

import (
	"encoding/json"
	"net/http"
)

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
	Code    int    `json:"code"`
}

// respondWithError sends an error response with the given status code and message
func (s *Server) respondWithError(w http.ResponseWriter, code int, message string) {
	errorResponse := ErrorResponse{
		Error:   http.StatusText(code),
		Message: message,
		Code:    code,
	}
	s.respondWithJSON(w, code, errorResponse, true)
}

// respondWithJSON sends a JSON response with the given status code and data
func (s *Server) respondWithJSON(w http.ResponseWriter, code int, data interface{}, prettyPrint ...bool) {
	// Set headers
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	// Marshal data to JSON, with optional pretty printing
	if data != nil {
		var response []byte
		var err error

		// Check if pretty printing is requested
		isPrettyPrint := false
		if len(prettyPrint) > 0 && prettyPrint[0] {
			isPrettyPrint = true
		}

		if isPrettyPrint {
			// Pretty print with indentation
			response, err = json.MarshalIndent(data, "", "  ")
		} else {
			// Regular compact JSON
			response, err = json.Marshal(data)
		}

		if err != nil {
			s.logger.Errorf("Error marshaling JSON response: %v", err)
			http.Error(w, `{"error":"Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		// Write response
		if _, err := w.Write(response); err != nil {
			s.logger.Errorf("Error writing JSON response: %v", err)
		}
	}
}
