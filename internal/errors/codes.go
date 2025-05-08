package errors

import (
	stderrors "errors" // Import standard errors as stderrors to avoid name collision
	"fmt"
)

// ErrorCode represents a unique SyncopateDB error code
type ErrorCode string

// Error categories
const (
	// General errors (SY001-SY099)
	ErrCodeGeneral         ErrorCode = "SY001"
	ErrCodeInternalServer  ErrorCode = "SY002"
	ErrCodeUnauthorized    ErrorCode = "SY003"
	ErrCodeForbidden       ErrorCode = "SY004"
	ErrCodeNotImplemented  ErrorCode = "SY005"
	ErrCodeRequestTimeout  ErrorCode = "SY006"
	ErrCodeInvalidRequest  ErrorCode = "SY007"
	ErrCodeMalformedData   ErrorCode = "SY008"
	ErrCodeTooManyRequests ErrorCode = "SY009"

	// Entity type errors (SY100-SY199)
	ErrCodeEntityTypeNotFound   ErrorCode = "SY100"
	ErrCodeEntityTypeExists     ErrorCode = "SY101"
	ErrCodeInvalidEntityType    ErrorCode = "SY102"
	ErrCodeEntityTypeValidation ErrorCode = "SY103"
	ErrCodeFieldNameReserved    ErrorCode = "SY104"
	ErrCodeIDGeneratorChange    ErrorCode = "SY105"

	// Entity errors (SY200-SY299)
	ErrCodeEntityNotFound       ErrorCode = "SY200"
	ErrCodeEntityAlreadyExists  ErrorCode = "SY201"
	ErrCodeInvalidEntity        ErrorCode = "SY202"
	ErrCodeEntityValidation     ErrorCode = "SY203"
	ErrCodeInvalidID            ErrorCode = "SY204"
	ErrCodeIDGenerationFailed   ErrorCode = "SY205"
	ErrCodeRequiredFieldMissing ErrorCode = "SY206"
	ErrCodeFieldTypeMismatch    ErrorCode = "SY207"
	ErrCodeNullableViolation    ErrorCode = "SY208"
	ErrCodeUniqueConstraint     ErrorCode = "SY209"

	// Query errors (SY300-SY399)
	ErrCodeInvalidQuery       ErrorCode = "SY300"
	ErrCodeInvalidFilter      ErrorCode = "SY301"
	ErrCodeInvalidJoin        ErrorCode = "SY302"
	ErrCodeJoinTargetNotFound ErrorCode = "SY303"
	ErrCodeInvalidSort        ErrorCode = "SY304"
	ErrCodeQueryTimeout       ErrorCode = "SY305"
	ErrCodeQueryTooComplex    ErrorCode = "SY306"

	// Persistence errors (SY400-SY499)
	ErrCodePersistenceFailed  ErrorCode = "SY400"
	ErrCodeSnapshotFailed     ErrorCode = "SY401"
	ErrCodeWALWriteFailed     ErrorCode = "SY402"
	ErrCodeDatabaseCorruption ErrorCode = "SY403"
	ErrCodeBackupFailed       ErrorCode = "SY404"
	ErrCodeRestoreFailed      ErrorCode = "SY405"
)

// SyncopateError represents an error with a code and message
type SyncopateError struct {
	Code    ErrorCode
	Message string
	Err     error // Original error
}

// Error returns the error message
func (e *SyncopateError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the original error
func (e *SyncopateError) Unwrap() error {
	return e.Err
}

// NewError creates a new SyncopateError
func NewError(code ErrorCode, message string) *SyncopateError {
	return &SyncopateError{
		Code:    code,
		Message: message,
		Err:     stderrors.New(message),
	}
}

// WrapError wraps an existing error with a SyncopateDB error code
func WrapError(err error, code ErrorCode, message string) *SyncopateError {
	if err == nil {
		return NewError(code, message)
	}

	// If it's already a SyncopateError, just update the code if needed
	var synErr *SyncopateError
	if stderrors.As(err, &synErr) {
		if synErr.Code == code {
			return synErr
		}
		// Create a new error with the updated code but preserve the chain
		return &SyncopateError{
			Code:    code,
			Message: message,
			Err:     synErr,
		}
	}

	return &SyncopateError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// IsErrorCode checks if an error has a specific error code
func IsErrorCode(err error, code ErrorCode) bool {
	var synErr *SyncopateError
	if stderrors.As(err, &synErr) {
		return synErr.Code == code
	}
	return false
}

// GetErrorCode extracts the error code from an error, or returns a general error code
func GetErrorCode(err error) ErrorCode {
	var synErr *SyncopateError
	if stderrors.As(err, &synErr) {
		return synErr.Code
	}
	return ErrCodeGeneral
}

// Map HTTP status codes to SyncopateDB error codes
func MapHTTPError(statusCode int) ErrorCode {
	switch statusCode {
	case 400:
		return ErrCodeInvalidRequest
	case 401:
		return ErrCodeUnauthorized
	case 403:
		return ErrCodeForbidden
	case 404:
		return ErrCodeEntityNotFound
	case 409:
		return ErrCodeEntityAlreadyExists
	case 429:
		return ErrCodeTooManyRequests
	case 500:
		return ErrCodeInternalServer
	case 501:
		return ErrCodeNotImplemented
	case 504:
		return ErrCodeRequestTimeout
	default:
		return ErrCodeGeneral
	}
}
