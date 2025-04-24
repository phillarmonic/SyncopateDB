package api

import (
	"fmt"
	"github.com/phillarmonic/syncopate-db/internal/common"
	"strconv"
	"strings"
)

// normalizeEntityID ensures that an ID is in the correct format for internal storage
// based on the entity type's ID generator
func (s *Server) normalizeEntityID(entityType string, rawID string) (string, error) {
	// Get entity definition to determine ID type
	def, err := s.engine.GetEntityDefinition(entityType)
	if err != nil {
		return "", fmt.Errorf("entity type not found: %w", err)
	}

	switch def.IDGenerator {
	case common.IDTypeAutoIncrement:
		// For auto-increment, ensure it's a valid number
		id, err := strconv.ParseUint(rawID, 10, 64)
		if err != nil {
			return "", fmt.Errorf("invalid auto-increment ID format: %w", err)
		}
		// Format exactly as the generator would
		return strconv.FormatUint(id, 10), nil

	case common.IDTypeUUID:
		// For UUID, normalize to lowercase as per RFC 4122
		return strings.ToLower(rawID), nil

	case common.IDTypeCUID:
		// CUIDs should already be properly formatted, but validate format
		if !strings.HasPrefix(rawID, "c") {
			return "", fmt.Errorf("invalid CUID format: must start with 'c'")
		}
		return rawID, nil

	case common.IDTypeCustom:
		// For custom IDs, use as-is
		return rawID, nil

	default:
		// Unknown ID type, use as-is but log a warning
		s.logger.Warnf("Unknown ID generator type: %s, using raw ID", def.IDGenerator)
		return rawID, nil
	}
}
