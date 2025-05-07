package api

import (
	"encoding/json"
	"fmt"
	"github.com/phillarmonic/syncopate-db/internal/about"
	"github.com/phillarmonic/syncopate-db/internal/settings"
	"github.com/sirupsen/logrus"
	"net/http"
	"os"
	"strconv"
	"strings"
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

// handleSettings returns the current configuration settings
func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	// Create a settings view that's safe to expose
	settingsView := map[string]interface{}{
		"debug":         settings.Config.Debug,
		"logLevel":      settings.Config.LogLevel,
		"port":          settings.Config.Port,
		"enableWAL":     settings.Config.EnableWAL,
		"enableZSTD":    settings.Config.EnableZSTD,
		"colorizedLogs": settings.Config.ColorizedLogs,
		"serverTime":    time.Now().Format(time.RFC3339),
		"version":       about.About().Version,
		"environment":   determineEnvironment(),
	}

	s.respondWithJSON(w, http.StatusOK, settingsView, true)
}

// handleWelcome provides a welcome message for the root path
func (s *Server) handleWelcome(w http.ResponseWriter, r *http.Request) {
	welcomeMessage := WelcomeResponse{
		Name:          about.About().Name,
		Version:       about.About().Version,
		Description:   about.About().Description,
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

	// Note: If IDGenerator is an empty string, auto_increment will be used as default
	if err := s.engine.RegisterEntityType(def); err != nil {
		s.respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Get the actual definition with any defaults applied
	updatedDef, err := s.engine.GetEntityDefinition(def.Name)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "Entity type created but could not retrieve it")
		return
	}

	s.respondWithJSON(w, http.StatusCreated, map[string]interface{}{
		"message":    "Entity type created successfully",
		"entityType": updatedDef,
	})
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

	// Get the entity definition to determine ID type
	def, err := s.engine.GetEntityDefinition(entityType)
	if err != nil {
		s.respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Filter internal fields from response data, ensure all fields are included, and convert IDs
	filteredData := make([]interface{}, len(response.Data))
	for i, entity := range response.Data {
		// Filter internal fields first
		filteredEntity := s.filterInternalFields(entity)
		// Ensure all fields from definition are included
		completeEntity := s.includeAllDefinedFields(filteredEntity, def)
		// Then convert to representation with proper ID type
		filteredData[i] = common.ConvertToRepresentation(completeEntity, def.IDGenerator)
	}

	// Create a new response with the filtered and converted data
	convertedResponse := struct {
		Total      int           `json:"total"`
		Count      int           `json:"count"`
		Limit      int           `json:"limit"`
		Offset     int           `json:"offset"`
		HasMore    bool          `json:"hasMore"`
		EntityType string        `json:"entityType"`
		Data       []interface{} `json:"data"`
	}{
		Total:      response.Total,
		Count:      response.Count,
		Limit:      response.Limit,
		Offset:     response.Offset,
		HasMore:    response.HasMore,
		EntityType: response.EntityType,
		Data:       filteredData,
	}

	s.respondWithJSON(w, http.StatusOK, convertedResponse)
}

// handleCreateEntity creates a new entity
// Todo user should not be able to point an ID on auto increment on create/update
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

	// Convert ID to string if provided as a number for auto_increment
	// (This is a defensive measure in case the client sends a numeric ID)
	rawID := entityData.ID

	// Insert the entity - ID will be generated if not provided
	if err := s.engine.Insert(entityType, rawID, entityData.Fields); err != nil {
		// Check if this is a unique constraint violation
		if strings.Contains(err.Error(), "unique constraint violation") {
			s.respondWithError(w, http.StatusConflict, err.Error())
			return
		}
		s.respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// For auto-generated IDs, we need to find the ID that was generated
	var responseID interface{}

	if rawID == "" {
		// We need to find the entity that was just inserted
		// This is a bit inefficient, but it works for the response
		// A better approach would be to modify Insert to return the generated ID
		entities, err := s.engine.GetAllEntitiesOfType(entityType)
		if err != nil {
			s.respondWithError(w, http.StatusInternalServerError, "Failed to retrieve entity after creation")
			return
		}

		// Find the most recently inserted entity by looking at _created_at timestamp
		var newestEntity common.Entity
		var newestTime time.Time

		for _, e := range entities {
			if createdAt, ok := e.Fields["_created_at"].(time.Time); ok {
				if newestEntity.ID == "" || createdAt.After(newestTime) {
					newestEntity = e
					newestTime = createdAt
				}
			}
		}

		if newestEntity.ID != "" {
			rawID = newestEntity.ID
		}
	}

	// Format the response ID based on entity type's ID generator
	responseID = rawID

	// For auto_increment, convert ID to int for the response
	if def.IDGenerator == common.IDTypeAutoIncrement {
		if id, err := strconv.Atoi(rawID); err == nil {
			responseID = id
		}
	}

	s.respondWithJSON(w, http.StatusCreated, map[string]interface{}{
		"message": "Entity created successfully",
		"id":      responseID,
	})
}

// handleGetEntity retrieves a specific entity
func (s *Server) handleGetEntity(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	rawID := vars["id"]
	entityType := vars["type"]

	// Normalize the ID based on entity type's ID generator
	normalizedID, err := s.normalizeEntityID(entityType, rawID)
	if err != nil {
		s.respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Add debugging information in development mode
	if s.config.DebugMode {
		s.logger.WithFields(logrus.Fields{
			"entityType":   entityType,
			"rawID":        rawID,
			"normalizedID": normalizedID,
		}).Debug("Getting entity")
	}

	// Use a type-specific get method if available
	var entity common.Entity
	var getErr error

	if engine, ok := s.engine.(*datastore.Engine); ok {
		entity, getErr = engine.GetByType(normalizedID, entityType)
	} else {
		// Fallback for other implementations
		entity, getErr = s.engine.Get(normalizedID)
		// Check the type matches what we're looking for
		if getErr == nil && entity.Type != entityType {
			getErr = fmt.Errorf("entity with ID %s and type %s not found", normalizedID, entityType)
		}
	}

	if getErr != nil {
		s.respondWithError(w, http.StatusNotFound, getErr.Error())
		return
	}

	// Filter out internal fields and convert ID to appropriate type for response
	filteredEntity := s.filterInternalFieldsWithIDConversion(entity)
	s.respondWithJSON(w, http.StatusOK, filteredEntity)
}

// handleUpdateEntity updates a specific entity
func (s *Server) handleUpdateEntity(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	rawID := vars["id"]
	entityType := vars["type"]

	var updateData struct {
		Fields map[string]interface{} `json:"fields"`
	}

	if err := json.NewDecoder(r.Body).Decode(&updateData); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	defer r.Body.Close()

	// Normalize the ID based on entity type's ID generator
	normalizedID, err := s.normalizeEntityID(entityType, rawID)
	if err != nil {
		s.respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Add debugging information in development mode
	if s.config.DebugMode {
		s.logger.WithFields(logrus.Fields{
			"entityType":   entityType,
			"rawID":        rawID,
			"normalizedID": normalizedID,
		}).Debug("Updating entity")
	}

	// Use the new type-safe Update method
	if err := s.engine.Update(entityType, normalizedID, updateData.Fields); err != nil {
		// Check if this is a unique constraint violation
		if strings.Contains(err.Error(), "unique constraint violation") {
			s.respondWithError(w, http.StatusConflict, err.Error())
			return
		}
		s.respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Get entity definition to determine how to format the response ID
	def, err := s.engine.GetEntityDefinition(entityType)
	if err == nil && def.IDGenerator == common.IDTypeAutoIncrement {
		// For auto-increment, convert back to int for the response
		if intID, err := strconv.Atoi(rawID); err == nil {
			s.respondWithJSON(w, http.StatusOK, map[string]interface{}{
				"message": "Entity updated successfully",
				"id":      intID,
			})
			return
		}
	}

	// For other types or if conversion fails, use the raw ID
	s.respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Entity updated successfully",
		"id":      rawID,
	})
}

// handleDeleteEntity deletes a specific entity
func (s *Server) handleDeleteEntity(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	rawID := vars["id"]
	entityType := vars["type"]

	// Get entity definition to determine ID type
	def, err := s.engine.GetEntityDefinition(entityType)
	if err != nil {
		s.respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Special handling for auto-increment IDs
	var entityID string = rawID
	if def.IDGenerator == common.IDTypeAutoIncrement {
		// IMPORTANT: For auto-increment, we need to make sure we're using
		// exactly the same string format as when it was stored
		// Try multiple formats to find the entity
		formats := []string{rawID}

		// Only try conversion if it's a valid number
		if _, err := strconv.Atoi(rawID); err == nil {
			// Try multiple formatting approaches
			id, _ := strconv.ParseUint(rawID, 10, 64)
			formats = append(formats, strconv.FormatUint(id, 10))

			id2, _ := strconv.ParseInt(rawID, 10, 64)
			formats = append(formats, strconv.FormatInt(id2, 10))

			intID, _ := strconv.Atoi(rawID)
			formats = append(formats, strconv.Itoa(intID))
		}

		// Try each format until we find the entity
		entityFound := false
		for _, fmt := range formats {
			// Try to get the entity with this format
			if _, err := s.engine.Get(fmt); err == nil {
				// Found it! Use this format for deletion
				entityID = fmt
				entityFound = true
				break
			}
		}

		if !entityFound {
			// If no format worked, use the direct approach - check the debug endpoint for clues
			engine, ok := s.engine.(*datastore.Engine)
			if ok {
				engine.DebugInspectEntities(func(entities map[string]common.Entity) {
					// Look for an entity with matching ID and type
					for key, entity := range entities {
						if entity.Type == entityType && entity.ID == rawID {
							entityID = key
							entityFound = true
							break
						}
					}
				})
			}

			if !entityFound {
				s.respondWithError(w, http.StatusNotFound, "Entity not found with any ID format")
				return
			}
		}
	}

	// At this point, we should have the right entityID format
	if s.config.DebugMode {
		s.logger.WithFields(logrus.Fields{
			"entityType": entityType,
			"rawID":      rawID,
			"entityID":   entityID,
		}).Debug("Deleting entity")
	}

	// Use the properly formatted entity ID for deletion
	if err := s.engine.Delete(entityID); err != nil {
		s.respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Format the response ID based on entity type's ID generator
	var responseID interface{} = rawID

	// If it's an auto_increment type, convert to int for API response
	if def.IDGenerator == common.IDTypeAutoIncrement {
		if intID, err := strconv.Atoi(rawID); err == nil {
			responseID = intID
		}
	}

	s.respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Entity deleted successfully",
		"id":      responseID,
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

	// Get the entity definition to determine ID type
	def, err := s.engine.GetEntityDefinition(queryOpts.EntityType)
	if err != nil {
		s.respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Filter internal fields from response data, ensure all fields are included, and convert IDs
	filteredData := make([]interface{}, len(response.Data))
	for i, entity := range response.Data {
		// Create a filtered copy of the entity
		filteredEntity := common.Entity{
			ID:     entity.ID,
			Type:   entity.Type,
			Fields: make(map[string]interface{}),
		}

		// Copy non-internal fields (those not starting with underscore)
		for name, value := range entity.Fields {
			if !strings.HasPrefix(name, "_") {
				filteredEntity.Fields[name] = value
			}
		}

		// Ensure all fields from definition are included
		completeEntity := s.includeAllDefinedFields(filteredEntity, def)

		// Then convert to representation with proper ID type
		filteredData[i] = common.ConvertToRepresentation(completeEntity, def.IDGenerator)
	}

	// Create a new response with the filtered and converted data
	convertedResponse := struct {
		Total      int           `json:"total"`
		Count      int           `json:"count"`
		Limit      int           `json:"limit"`
		Offset     int           `json:"offset"`
		HasMore    bool          `json:"hasMore"`
		EntityType string        `json:"entityType"`
		Data       []interface{} `json:"data"`
	}{
		Total:      response.Total,
		Count:      response.Count,
		Limit:      response.Limit,
		Offset:     response.Offset,
		HasMore:    response.HasMore,
		EntityType: response.EntityType,
		Data:       filteredData,
	}

	s.respondWithJSON(w, http.StatusOK, convertedResponse)
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

// filterInternalFields removes internal fields from entity data based on debug settings
func (s *Server) filterInternalFields(entity common.Entity) common.Entity {
	// Check if debug mode is enabled via environment variable
	debugMode := os.Getenv("SYNCOPATE_DEBUG") == "true"

	// If debug mode is enabled, return the full entity with internal fields
	if debugMode {
		return entity
	}

	// Create a filtered copy of the entity
	filteredEntity := common.Entity{
		ID:     entity.ID,
		Type:   entity.Type,
		Fields: make(map[string]interface{}),
	}

	// Copy only non-internal fields (those not starting with underscore)
	for name, value := range entity.Fields {
		if !strings.HasPrefix(name, "_") {
			filteredEntity.Fields[name] = value
		}
	}

	return filteredEntity
}

// filterInternalFieldsWithIDConversion removes internal fields from entity data
// and converts the ID to the appropriate type based on the entity's ID generator
func (s *Server) filterInternalFieldsWithIDConversion(entity common.Entity) interface{} {
	// Get entity definition to check the ID generator type
	def, err := s.engine.GetEntityDefinition(entity.Type)
	if err != nil {
		// If we can't get the definition, use string ID (fallback)
		return s.filterInternalFields(entity)
	}

	// First, filter out internal fields
	filteredEntity := s.filterInternalFields(entity)

	// Then, ensure all fields from definition are included
	completeEntity := s.includeAllDefinedFields(filteredEntity, def)

	// Convert to representation with proper ID type
	return common.ConvertToRepresentation(completeEntity, def.IDGenerator)
}

// determineEnvironment tries to detect the current deployment environment
func determineEnvironment() string {
	// Check for common environment variables
	if env := os.Getenv("APP_ENV"); env != "" {
		return env
	}

	if env := os.Getenv("ENV"); env != "" {
		return env
	}

	// Check for debug mode
	if settings.Config.Debug {
		return "development"
	}

	// Default to production
	return "production"
}

func (s *Server) handleUpdateEntityType(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	// First check if the entity type exists
	originalDef, err := s.engine.GetEntityDefinition(name)
	if err != nil {
		s.respondWithError(w, http.StatusNotFound, fmt.Sprintf("Entity type '%s' not found", name))
		return
	}

	// Parse the updated definition
	var updatedDef common.EntityDefinition
	if err := json.NewDecoder(r.Body).Decode(&updatedDef); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	defer r.Body.Close()

	// Ensure the name in the payload matches the URL
	if updatedDef.Name != name {
		s.respondWithError(w, http.StatusBadRequest,
			"Entity type name in payload doesn't match URL parameter")
		return
	}

	// Prevent changing the ID generator - this is a design decision to avoid
	// complex ID migration issues
	if updatedDef.IDGenerator != "" && updatedDef.IDGenerator != originalDef.IDGenerator {
		s.respondWithError(w, http.StatusBadRequest,
			"Cannot change the ID generator after entity type creation")
		return
	}

	// Include the original ID generator if not specified
	if updatedDef.IDGenerator == "" {
		updatedDef.IDGenerator = originalDef.IDGenerator
	}

	// Check for uniqueness constraint changes
	oldUniqueFields := make(map[string]bool)
	for _, field := range originalDef.Fields {
		if field.Unique {
			oldUniqueFields[field.Name] = true
		}
	}

	newUniqueFields := make(map[string]bool)
	for _, field := range updatedDef.Fields {
		if field.Unique {
			newUniqueFields[field.Name] = true
		}
	}

	// Update the entity type
	if err := s.engine.UpdateEntityType(updatedDef); err != nil {
		s.respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Get the actual updated definition with any modifications applied
	updatedDef, err = s.engine.GetEntityDefinition(name)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError,
			"Entity type updated but could not retrieve it")
		return
	}

	// Provide a detailed response with information about the update
	response := map[string]interface{}{
		"message":    "Entity type updated successfully",
		"entityType": updatedDef,
	}

	// If unique constraints were added, mention it in the response
	addedUniqueFields := make([]string, 0)
	for field := range newUniqueFields {
		if !oldUniqueFields[field] {
			addedUniqueFields = append(addedUniqueFields, field)
		}
	}

	if len(addedUniqueFields) > 0 {
		response["uniqueConstraintsAdded"] = addedUniqueFields
	}

	// If unique constraints were removed, mention it in the response
	removedUniqueFields := make([]string, 0)
	for field := range oldUniqueFields {
		if !newUniqueFields[field] {
			removedUniqueFields = append(removedUniqueFields, field)
		}
	}

	if len(removedUniqueFields) > 0 {
		response["uniqueConstraintsRemoved"] = removedUniqueFields
	}

	s.respondWithJSON(w, http.StatusOK, response)
}

func (s *Server) handleDebugSchema(w http.ResponseWriter, r *http.Request) {
	// Get entity type from query parameter
	entityType := r.URL.Query().Get("type")

	if entityType == "" {
		// If no specific type requested, show all entity types
		types := s.engine.ListEntityTypes()
		schemas := make(map[string]interface{})

		for _, typeName := range types {
			def, err := s.engine.GetEntityDefinition(typeName)
			if err != nil {
				continue
			}
			schemas[typeName] = def
		}

		s.respondWithJSON(w, http.StatusOK, map[string]interface{}{
			"entity_types": schemas,
		}, true)
		return
	}

	// Get definition for specific entity type
	def, err := s.engine.GetEntityDefinition(entityType)
	if err != nil {
		s.respondWithError(w, http.StatusNotFound, fmt.Sprintf("Entity type '%s' not found", entityType))
		return
	}

	// Show detailed schema information
	fieldMap := make(map[string]map[string]interface{})
	for _, field := range def.Fields {
		fieldMap[field.Name] = map[string]interface{}{
			"type":     field.Type,
			"indexed":  field.Indexed,
			"required": field.Required,
			"nullable": field.Nullable,
			"internal": field.Internal,
			"unique":   field.Unique,
		}
	}

	// Get count of entities with this type
	count, _ := s.engine.GetEntityCount(entityType)

	s.respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"entity_type":  def.Name,
		"id_generator": def.IDGenerator,
		"fields":       fieldMap,
		"entity_count": count,
	}, true)
}

// handleCountQuery handles count queries without returning the actual data
func (s *Server) handleCountQuery(w http.ResponseWriter, r *http.Request) {
	var queryOpts datastore.QueryOptions
	if err := json.NewDecoder(r.Body).Decode(&queryOpts); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	defer r.Body.Close()

	// Log the request if in debug mode
	if s.config.DebugMode {
		s.logger.WithFields(logrus.Fields{
			"entityType": queryOpts.EntityType,
			"filters":    len(queryOpts.Filters),
			"joins":      len(queryOpts.Joins),
		}).Debug("Executing count query")
	}

	// Track execution time
	startTime := time.Now()

	// Execute the auto-optimizing count query
	count, err := s.queryService.ExecuteCountQuery(queryOpts)
	if err != nil {
		s.respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Calculate execution time
	executionTime := time.Since(startTime)

	// Determine query type
	queryType := "simple"
	if len(queryOpts.Joins) > 0 {
		queryType = "join"
	}

	// Create response
	response := CountResponse{
		Count:         count,
		EntityType:    queryOpts.EntityType,
		QueryType:     queryType,
		FiltersCount:  len(queryOpts.Filters),
		JoinsApplied:  len(queryOpts.Joins),
		ExecutionTime: executionTime.String(),
	}

	s.respondWithJSON(w, http.StatusOK, response)
}

// CountResponse structure for count query responses
type CountResponse struct {
	Count         int    `json:"count"`
	EntityType    string `json:"entityType"`
	QueryType     string `json:"queryType"`
	FiltersCount  int    `json:"filtersCount"`
	JoinsApplied  int    `json:"joinsApplied"`
	ExecutionTime string `json:"executionTime,omitempty"`
}
