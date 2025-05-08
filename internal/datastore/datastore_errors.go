package datastore

import (
	stderrors "errors"
	"fmt"
	"github.com/phillarmonic/syncopate-db/internal/errors"
	"strings"
)

// Common error transformations for entity type operations
func entityTypeNotFoundError(entityType string) error {
	return errors.NewError(
		errors.ErrCodeEntityTypeNotFound,
		"entity type '"+entityType+"' not registered",
	)
}

func entityTypeExistsError(entityType string) error {
	return errors.NewError(
		errors.ErrCodeEntityTypeExists,
		"entity type '"+entityType+"' already exists",
	)
}

func invalidFieldNameError(fieldName string) error {
	return errors.NewError(
		errors.ErrCodeFieldNameReserved,
		"field name '"+fieldName+"' is not allowed: names starting with underscore are reserved for internal use",
	)
}

func idGeneratorChangeError() error {
	return errors.NewError(
		errors.ErrCodeIDGeneratorChange,
		"cannot change the ID generator after entity type creation",
	)
}

// Common error transformations for entity operations
func EntityNotFoundError(entityType, id string) error {
	return errors.NewError(
		errors.ErrCodeEntityNotFound,
		fmt.Sprintf("entity with ID '%s' and type '%s' not found", id, entityType),
	)
}

func entityAlreadyExistsError(entityType, id string) error {
	return errors.NewError(
		errors.ErrCodeEntityAlreadyExists,
		"entity with ID '"+id+"' already exists for entity type '"+entityType+"'",
	)
}

func invalidIDError(entityType, id string) error {
	return errors.NewError(
		errors.ErrCodeInvalidID,
		"invalid ID format for entity type '"+entityType+"': "+id,
	)
}

func idGenerationFailedError(err error) error {
	return errors.WrapError(
		err,
		errors.ErrCodeIDGenerationFailed,
		"failed to generate ID",
	)
}

func requiredFieldMissingError(fieldName string) error {
	return errors.NewError(
		errors.ErrCodeRequiredFieldMissing,
		"required field '"+fieldName+"' is missing",
	)
}

func fieldTypeMismatchError(fieldName string, err error) error {
	return errors.WrapError(
		err,
		errors.ErrCodeFieldTypeMismatch,
		"field '"+fieldName+"' has invalid type",
	)
}

func nullableViolationError(fieldName string) error {
	return errors.NewError(
		errors.ErrCodeNullableViolation,
		"field '"+fieldName+"' cannot be null",
	)
}

func uniqueConstraintViolationError(fieldName string, value interface{}, existingID string) error {
	return errors.NewError(
		errors.ErrCodeUniqueConstraint,
		"unique constraint violation: field '"+fieldName+"' with value '"+fmt.Sprintf("%v", value)+"' already exists in entity ID '"+existingID+"'",
	)
}

// Query errors
func invalidQueryError(message string) error {
	return errors.NewError(
		errors.ErrCodeInvalidQuery,
		message,
	)
}

func invalidFilterError(message string) error {
	return errors.NewError(
		errors.ErrCodeInvalidFilter,
		message,
	)
}

func invalidJoinError(message string) error {
	return errors.NewError(
		errors.ErrCodeInvalidJoin,
		message,
	)
}

func joinTargetNotFoundError(entityType string) error {
	return errors.NewError(
		errors.ErrCodeJoinTargetNotFound,
		"join target entity type '"+entityType+"' not found",
	)
}

// Persistence errors
func persistenceFailedError(err error) error {
	return errors.WrapError(
		err,
		errors.ErrCodePersistenceFailed,
		"persistence operation failed",
	)
}

func snapshotFailedError(err error) error {
	return errors.WrapError(
		err,
		errors.ErrCodeSnapshotFailed,
		"failed to create snapshot",
	)
}

func walWriteFailedError(err error) error {
	return errors.WrapError(
		err,
		errors.ErrCodeWALWriteFailed,
		"failed to write to WAL",
	)
}

// ConvertToSyncopateError ensures an error is a SyncopateError
// This is useful for functions that return standard Go errors
func ConvertToSyncopateError(err error) error {
	if err == nil {
		return nil
	}

	// Already a SyncopateError, return as is
	var synErr *errors.SyncopateError
	if stderrors.As(err, &synErr) {
		return err
	}

	// Handle common error patterns and convert them
	errMsg := err.Error()

	// Entity not found errors
	if strings.Contains(errMsg, "not found") && strings.Contains(errMsg, "entity") {
		return errors.WrapError(err, errors.ErrCodeEntityNotFound, errMsg)
	}

	// Entity type not found errors
	if strings.Contains(errMsg, "not registered") && strings.Contains(errMsg, "entity type") {
		return errors.WrapError(err, errors.ErrCodeEntityTypeNotFound, errMsg)
	}

	// Unique constraint errors
	if strings.Contains(errMsg, "unique constraint violation") {
		return errors.WrapError(err, errors.ErrCodeUniqueConstraint, errMsg)
	}

	// Invalid ID errors
	if strings.Contains(errMsg, "invalid ID format") {
		return errors.WrapError(err, errors.ErrCodeInvalidID, errMsg)
	}

	// ID generation errors
	if strings.Contains(errMsg, "failed to generate ID") {
		return errors.WrapError(err, errors.ErrCodeIDGenerationFailed, errMsg)
	}

	// Default to general error
	return errors.WrapError(err, errors.ErrCodeGeneral, errMsg)
}
