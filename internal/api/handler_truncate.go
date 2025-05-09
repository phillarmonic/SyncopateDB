package api

import (
	"fmt"
	"github.com/gorilla/mux"
	"github.com/phillarmonic/syncopate-db/internal/errors"
	"net/http"
)

// handleTruncateEntityType handles requests to remove all entities of a specific type
func (s *Server) handleTruncateEntityType(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	entityType := vars["type"]

	// Check if the entity type exists
	_, err := s.engine.GetEntityDefinition(entityType)
	if err != nil {
		s.respondWithError(w, http.StatusNotFound,
			fmt.Sprintf("Entity type '%s' not found", entityType),
			errors.NewError(errors.ErrCodeEntityTypeNotFound,
				fmt.Sprintf("Entity type '%s' not found", entityType)))
		return
	}

	// Get the count before truncating for the response
	count, err := s.engine.GetEntityCount(entityType)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError,
			"Failed to get entity count",
			errors.NewError(errors.ErrCodeInternalServer,
				fmt.Sprintf("Failed to get entity count: %v", err)))
		return
	}

	// Perform the truncate operation
	if err := s.engine.TruncateEntityType(entityType); err != nil {
		s.respondWithError(w, http.StatusInternalServerError,
			fmt.Sprintf("Failed to truncate entity type: %v", err),
			errors.NewError(errors.ErrCodeInternalServer,
				fmt.Sprintf("Failed to truncate entity type: %v", err)))
		return
	}

	// Return success with the count of removed entities
	s.respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"message":          fmt.Sprintf("Successfully truncated entity type '%s'", entityType),
		"type":             entityType,
		"entities_removed": count,
	})
}

// handleTruncateDatabase handles requests to remove all entities from all types
func (s *Server) handleTruncateDatabase(w http.ResponseWriter, r *http.Request) {
	// Get counts before truncating for the response
	entityTypes := s.engine.ListEntityTypes()
	typeCounts := make(map[string]int)
	totalCount := 0

	for _, entityType := range entityTypes {
		count, err := s.engine.GetEntityCount(entityType)
		if err == nil {
			typeCounts[entityType] = count
			totalCount += count
		}
	}

	// Perform the truncate operation
	if err := s.engine.TruncateDatabase(); err != nil {
		s.respondWithError(w, http.StatusInternalServerError,
			fmt.Sprintf("Failed to truncate database: %v", err),
			errors.NewError(errors.ErrCodeInternalServer,
				fmt.Sprintf("Failed to truncate database: %v", err)))
		return
	}

	// Return success with the count of removed entities
	s.respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"message":          "Successfully truncated all entities in the database",
		"entities_removed": totalCount,
		"types":            typeCounts,
	})
}
