package api

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/phillarmonic/syncopate-db/internal/common"
	"github.com/phillarmonic/syncopate-db/internal/datastore"
)

type WelcomeResponse struct {
	Name          string `json:"name"`
	Version       string `json:"version"`
	Description   string `json:"description"`
	Documentation string `json:"documentation"`
	HealthCheck   string `json:"healthCheck"`
	Status        string `json:"status"`
	ServerTime    string `json:"serverTime"`
}

// handleWelcome provides a welcome message for the root path
func (s *Server) handleWelcome(w http.ResponseWriter, r *http.Request) {
	welcomeMessage := WelcomeResponse{
		Name:          "SyncopateDB",
		Version:       "0.0.1",
		Description:   "A flexible, lightweight data store with advanced query capabilities",
		Documentation: "/api/v1",
		HealthCheck:   "/health",
		Status:        "running",
		ServerTime:    time.Now().Format(time.RFC3339),
	}

	// Use pretty-printed JSON for the welcome message
	s.respondWithJSON(w, http.StatusOK, welcomeMessage, true)
}

// handleHealthCheck handles health check requests
func (s *Server) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	s.respondWithJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleGetEntityTypes lists all entity types
func (s *Server) handleGetEntityTypes(w http.ResponseWriter, r *http.Request) {
	types := s.engine.ListEntityTypes()
	s.respondWithJSON(w, http.StatusOK, types)
}

// handleCreateEntityType creates a new entity type
func (s *Server) handleCreateEntityType(w http.ResponseWriter, r *http.Request) {
	var def common.EntityDefinition
	if err := json.NewDecoder(r.Body).Decode(&def); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	defer r.Body.Close()

	if err := s.engine.RegisterEntityType(def); err != nil {
		s.respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.respondWithJSON(w, http.StatusCreated, map[string]string{"message": "Entity type created successfully"})
}

// handleGetEntityType retrieves a specific entity type
func (s *Server) handleGetEntityType(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	def, err := s.engine.GetEntityDefinition(name)
	if err != nil {
		s.respondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	s.respondWithJSON(w, http.StatusOK, def)
}

// handleListEntities lists entities of a specific type
func (s *Server) handleListEntities(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	entityType := vars["type"]

	// Parse query parameters
	limit, offset, orderBy, orderDesc := s.parseQueryParams(r)

	// Create query options
	queryOpts := datastore.QueryOptions{
		EntityType: entityType,
		Limit:      limit,
		Offset:     offset,
		OrderBy:    orderBy,
		OrderDesc:  orderDesc,
	}

	// Execute query
	response, err := s.queryService.ExecutePaginatedQuery(queryOpts)
	if err != nil {
		s.respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.respondWithJSON(w, http.StatusOK, response)
}

// handleCreateEntity creates a new entity
func (s *Server) handleCreateEntity(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	entityType := vars["type"]

	var entityData struct {
		ID     string                 `json:"id"`
		Fields map[string]interface{} `json:"fields"`
	}

	if err := json.NewDecoder(r.Body).Decode(&entityData); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	defer r.Body.Close()

	// Get entity definition to check the ID generator type
	def, err := s.engine.GetEntityDefinition(entityType)
	if err != nil {
		s.respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Custom ID is required only if the ID generation type is custom
	if entityData.ID == "" && def.IDGenerator == common.IDTypeCustom {
		s.respondWithError(w, http.StatusBadRequest, "Entity ID is required for custom ID generation")
		return
	}

	// Insert the entity - ID will be generated if not provided based on the entity type's generator
	if err := s.engine.Insert(entityType, entityData.ID, entityData.Fields); err != nil {
		s.respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// For consistency with the original handler, get the entity to retrieve its ID
	// In a real implementation, we might want to modify Insert to return the generated ID
	var entityID string
	if entityData.ID != "" {
		entityID = entityData.ID
	} else {
		// We need to find the entity that was just inserted
		// This is a bit inefficient, but it works for the response
		// A better approach would be to modify Insert to return the generated ID
		entities, err := s.engine.GetAllEntitiesOfType(entityType)
		if err != nil {
			s.respondWithError(w, http.StatusInternalServerError, "Failed to retrieve entity after creation")
			return
		}

		// Find the most recently inserted entity
		// This is a heuristic and not 100% reliable in a concurrent environment
		// Again, a better approach would be to modify Insert to return the generated ID
		for _, e := range entities {
			if reflect.DeepEqual(e.Fields, entityData.Fields) {
				entityID = e.ID
				break
			}
		}
	}

	s.respondWithJSON(w, http.StatusCreated, map[string]string{
		"message": "Entity created successfully",
		"id":      entityID,
	})
}

// handleGetEntity retrieves a specific entity
func (s *Server) handleGetEntity(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	entity, err := s.engine.Get(id)
	if err != nil {
		s.respondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	s.respondWithJSON(w, http.StatusOK, entity)
}

// handleUpdateEntity updates a specific entity
func (s *Server) handleUpdateEntity(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var updateData struct {
		Fields map[string]interface{} `json:"fields"`
	}

	if err := json.NewDecoder(r.Body).Decode(&updateData); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	defer r.Body.Close()

	if err := s.engine.Update(id, updateData.Fields); err != nil {
		s.respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.respondWithJSON(w, http.StatusOK, map[string]string{
		"message": "Entity updated successfully",
		"id":      id,
	})
}

// handleDeleteEntity deletes a specific entity
func (s *Server) handleDeleteEntity(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	if err := s.engine.Delete(id); err != nil {
		s.respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.respondWithJSON(w, http.StatusOK, map[string]string{
		"message": "Entity deleted successfully",
		"id":      id,
	})
}

// handleQuery handles complex query requests
func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	var queryOpts datastore.QueryOptions
	if err := json.NewDecoder(r.Body).Decode(&queryOpts); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	defer r.Body.Close()

	response, err := s.queryService.ExecutePaginatedQuery(queryOpts)
	if err != nil {
		s.respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.respondWithJSON(w, http.StatusOK, response)
}

// parseQueryParams extracts common query parameters
func (s *Server) parseQueryParams(r *http.Request) (limit int, offset int, orderBy string, orderDesc bool) {
	// Default values
	limit = 100
	offset = 0
	orderDesc = false

	// Parse limit
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	// Parse offset
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	// Parse orderBy
	orderBy = r.URL.Query().Get("orderBy")

	// Parse orderDesc
	orderDesc = r.URL.Query().Get("orderDesc") == "true"

	return
}
