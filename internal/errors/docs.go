package errors

// ErrorCodeDoc represents documentation for an error code
type ErrorCodeDoc struct {
	Code        ErrorCode
	Name        string
	Description string
	HTTPStatus  int
	Example     string
}

// ErrorCodeDocs All error code documentation, organized by category
var ErrorCodeDocs = map[ErrorCode]ErrorCodeDoc{
	// General errors (SY001-SY099)
	ErrCodeGeneral: {
		Code:        ErrCodeGeneral,
		Name:        "General Error",
		Description: "A general error occurred that doesn't fit into other categories",
		HTTPStatus:  500,
		Example:     `{"error":"Internal Server Error","message":"An unexpected error occurred","code":500,"db_code":"SY001"}`,
	},
	ErrCodeInternalServer: {
		Code:        ErrCodeInternalServer,
		Name:        "Internal Server Error",
		Description: "An internal server error occurred",
		HTTPStatus:  500,
		Example:     `{"error":"Internal Server Error","message":"Failed to process request","code":500,"db_code":"SY002"}`,
	},
	ErrCodeUnauthorized: {
		Code:        ErrCodeUnauthorized,
		Name:        "Unauthorized",
		Description: "Authentication is required to access this resource",
		HTTPStatus:  401,
		Example:     `{"error":"Unauthorized","message":"Authentication required","code":401,"db_code":"SY003"}`,
	},
	ErrCodeForbidden: {
		Code:        ErrCodeForbidden,
		Name:        "Forbidden",
		Description: "User does not have permission to access this resource",
		HTTPStatus:  403,
		Example:     `{"error":"Forbidden","message":"Insufficient permissions","code":403,"db_code":"SY004"}`,
	},
	ErrCodeNotImplemented: {
		Code:        ErrCodeNotImplemented,
		Name:        "Not Implemented",
		Description: "The requested feature is not implemented",
		HTTPStatus:  501,
		Example:     `{"error":"Not Implemented","message":"This feature is not yet available","code":501,"db_code":"SY005"}`,
	},
	ErrCodeRequestTimeout: {
		Code:        ErrCodeRequestTimeout,
		Name:        "Request Timeout",
		Description: "The request timed out",
		HTTPStatus:  504,
		Example:     `{"error":"Gateway Timeout","message":"Request timed out","code":504,"db_code":"SY006"}`,
	},
	ErrCodeInvalidRequest: {
		Code:        ErrCodeInvalidRequest,
		Name:        "Invalid Request",
		Description: "The request is invalid",
		HTTPStatus:  400,
		Example:     `{"error":"Bad Request","message":"Invalid request format","code":400,"db_code":"SY007"}`,
	},
	ErrCodeMalformedData: {
		Code:        ErrCodeMalformedData,
		Name:        "Malformed Data",
		Description: "The request data is malformed or cannot be parsed",
		HTTPStatus:  400,
		Example:     `{"error":"Bad Request","message":"Malformed JSON data","code":400,"db_code":"SY008"}`,
	},
	ErrCodeTooManyRequests: {
		Code:        ErrCodeTooManyRequests,
		Name:        "Too Many Requests",
		Description: "Rate limit exceeded",
		HTTPStatus:  429,
		Example:     `{"error":"Too Many Requests","message":"Rate limit exceeded, try again later","code":429,"db_code":"SY009"}`,
	},

	// Entity type errors (SY100-SY199)
	ErrCodeEntityTypeNotFound: {
		Code:        ErrCodeEntityTypeNotFound,
		Name:        "Entity Type Not Found",
		Description: "The requested entity type does not exist",
		HTTPStatus:  404,
		Example:     `{"error":"Not Found","message":"entity type 'products' not registered","code":404,"db_code":"SY100"}`,
	},
	ErrCodeEntityTypeExists: {
		Code:        ErrCodeEntityTypeExists,
		Name:        "Entity Type Exists",
		Description: "The entity type already exists",
		HTTPStatus:  409,
		Example:     `{"error":"Conflict","message":"entity type 'products' already exists","code":409,"db_code":"SY101"}`,
	},
	ErrCodeInvalidEntityType: {
		Code:        ErrCodeInvalidEntityType,
		Name:        "Invalid Entity Type",
		Description: "The entity type is invalid",
		HTTPStatus:  400,
		Example:     `{"error":"Bad Request","message":"Invalid entity type definition","code":400,"db_code":"SY102"}`,
	},
	ErrCodeEntityTypeValidation: {
		Code:        ErrCodeEntityTypeValidation,
		Name:        "Entity Type Validation Failed",
		Description: "The entity type definition failed validation",
		HTTPStatus:  400,
		Example:     `{"error":"Bad Request","message":"Entity type validation failed: invalid field type","code":400,"db_code":"SY103"}`,
	},
	ErrCodeFieldNameReserved: {
		Code:        ErrCodeFieldNameReserved,
		Name:        "Field Name Reserved",
		Description: "The field name is reserved for internal use",
		HTTPStatus:  400,
		Example:     `{"error":"Bad Request","message":"field name '_id' is not allowed: names starting with underscore are reserved for internal use","code":400,"db_code":"SY104"}`,
	},
	ErrCodeIDGeneratorChange: {
		Code:        ErrCodeIDGeneratorChange,
		Name:        "ID Generator Change Not Allowed",
		Description: "Cannot change the ID generator after entity type creation",
		HTTPStatus:  400,
		Example:     `{"error":"Bad Request","message":"Cannot change the ID generator after entity type creation","code":400,"db_code":"SY105"}`,
	},

	// Entity errors (SY200-SY299)
	ErrCodeEntityNotFound: {
		Code:        ErrCodeEntityNotFound,
		Name:        "Entity Not Found",
		Description: "The requested entity does not exist",
		HTTPStatus:  404,
		Example:     `{"error":"Not Found","message":"entity with ID '123' and type 'products' not found","code":404,"db_code":"SY200"}`,
	},
	ErrCodeEntityAlreadyExists: {
		Code:        ErrCodeEntityAlreadyExists,
		Name:        "Entity Already Exists",
		Description: "An entity with this ID already exists",
		HTTPStatus:  409,
		Example:     `{"error":"Conflict","message":"entity with ID '123' already exists for entity type 'products'","code":409,"db_code":"SY201"}`,
	},
	ErrCodeInvalidEntity: {
		Code:        ErrCodeInvalidEntity,
		Name:        "Invalid Entity",
		Description: "The entity is invalid",
		HTTPStatus:  400,
		Example:     `{"error":"Bad Request","message":"Invalid entity data","code":400,"db_code":"SY202"}`,
	},
	ErrCodeEntityValidation: {
		Code:        ErrCodeEntityValidation,
		Name:        "Entity Validation Failed",
		Description: "The entity failed validation",
		HTTPStatus:  400,
		Example:     `{"error":"Bad Request","message":"Entity validation failed: invalid field value","code":400,"db_code":"SY203"}`,
	},
	ErrCodeInvalidID: {
		Code:        ErrCodeInvalidID,
		Name:        "Invalid ID",
		Description: "The entity ID is invalid",
		HTTPStatus:  400,
		Example:     `{"error":"Bad Request","message":"invalid ID format for entity type 'products': abc","code":400,"db_code":"SY204"}`,
	},
	ErrCodeIDGenerationFailed: {
		Code:        ErrCodeIDGenerationFailed,
		Name:        "ID Generation Failed",
		Description: "Failed to generate a unique ID for the entity",
		HTTPStatus:  500,
		Example:     `{"error":"Internal Server Error","message":"failed to generate ID","code":500,"db_code":"SY205"}`,
	},
	ErrCodeRequiredFieldMissing: {
		Code:        ErrCodeRequiredFieldMissing,
		Name:        "Required Field Missing",
		Description: "A required field is missing from the entity",
		HTTPStatus:  400,
		Example:     `{"error":"Bad Request","message":"required field 'name' is missing","code":400,"db_code":"SY206"}`,
	},
	ErrCodeFieldTypeMismatch: {
		Code:        ErrCodeFieldTypeMismatch,
		Name:        "Field Type Mismatch",
		Description: "A field value does not match the expected type",
		HTTPStatus:  400,
		Example:     `{"error":"Bad Request","message":"field 'price' has invalid type: value is not a number","code":400,"db_code":"SY207"}`,
	},
	ErrCodeNullableViolation: {
		Code:        ErrCodeNullableViolation,
		Name:        "Nullable Violation",
		Description: "A non-nullable field has a null value",
		HTTPStatus:  400,
		Example:     `{"error":"Bad Request","message":"field 'name' cannot be null","code":400,"db_code":"SY208"}`,
	},
	ErrCodeUniqueConstraint: {
		Code:        ErrCodeUniqueConstraint,
		Name:        "Unique Constraint Violation",
		Description: "A unique constraint has been violated",
		HTTPStatus:  409,
		Example:     `{"error":"Conflict","message":"unique constraint violation: field 'email' with value 'user@example.com' already exists in entity ID '123'","code":409,"db_code":"SY209"}`,
	},

	// Query errors (SY300-SY399)
	ErrCodeInvalidQuery: {
		Code:        ErrCodeInvalidQuery,
		Name:        "Invalid Query",
		Description: "The query is invalid",
		HTTPStatus:  400,
		Example:     `{"error":"Bad Request","message":"Invalid query format","code":400,"db_code":"SY300"}`,
	},
	ErrCodeInvalidFilter: {
		Code:        ErrCodeInvalidFilter,
		Name:        "Invalid Filter",
		Description: "A query filter is invalid",
		HTTPStatus:  400,
		Example:     `{"error":"Bad Request","message":"invalid filter operator 'invalidop'","code":400,"db_code":"SY301"}`,
	},
	ErrCodeInvalidJoin: {
		Code:        ErrCodeInvalidJoin,
		Name:        "Invalid Join",
		Description: "A query join is invalid",
		HTTPStatus:  400,
		Example:     `{"error":"Bad Request","message":"join entity type is required","code":400,"db_code":"SY302"}`,
	},
	ErrCodeJoinTargetNotFound: {
		Code:        ErrCodeJoinTargetNotFound,
		Name:        "Join Target Not Found",
		Description: "The target entity type for a join does not exist",
		HTTPStatus:  404,
		Example:     `{"error":"Not Found","message":"join target entity type 'orders' not found","code":404,"db_code":"SY303"}`,
	},
	ErrCodeInvalidSort: {
		Code:        ErrCodeInvalidSort,
		Name:        "Invalid Sort",
		Description: "The sort field or direction is invalid",
		HTTPStatus:  400,
		Example:     `{"error":"Bad Request","message":"sort field 'invalid_field' does not exist in entity type 'products'","code":400,"db_code":"SY304"}`,
	},
	ErrCodeQueryTimeout: {
		Code:        ErrCodeQueryTimeout,
		Name:        "Query Timeout",
		Description: "The query timed out",
		HTTPStatus:  504,
		Example:     `{"error":"Gateway Timeout","message":"Query execution timed out","code":504,"db_code":"SY305"}`,
	},
	ErrCodeQueryTooComplex: {
		Code:        ErrCodeQueryTooComplex,
		Name:        "Query Too Complex",
		Description: "The query is too complex to execute",
		HTTPStatus:  400,
		Example:     `{"error":"Bad Request","message":"Query is too complex, please simplify","code":400,"db_code":"SY306"}`,
	},

	// Persistence errors (SY400-SY499)
	ErrCodePersistenceFailed: {
		Code:        ErrCodePersistenceFailed,
		Name:        "Persistence Failed",
		Description: "Failed to persist data",
		HTTPStatus:  500,
		Example:     `{"error":"Internal Server Error","message":"persistence operation failed","code":500,"db_code":"SY400"}`,
	},
	ErrCodeSnapshotFailed: {
		Code:        ErrCodeSnapshotFailed,
		Name:        "Snapshot Failed",
		Description: "Failed to create a snapshot",
		HTTPStatus:  500,
		Example:     `{"error":"Internal Server Error","message":"failed to create snapshot","code":500,"db_code":"SY401"}`,
	},
	ErrCodeWALWriteFailed: {
		Code:        ErrCodeWALWriteFailed,
		Name:        "WAL Write Failed",
		Description: "Failed to write to the Write Ahead Log",
		HTTPStatus:  500,
		Example:     `{"error":"Internal Server Error","message":"failed to write to WAL","code":500,"db_code":"SY402"}`,
	},
	ErrCodeDatabaseCorruption: {
		Code:        ErrCodeDatabaseCorruption,
		Name:        "Database Corruption",
		Description: "The database is corrupted",
		HTTPStatus:  500,
		Example:     `{"error":"Internal Server Error","message":"Database corruption detected","code":500,"db_code":"SY403"}`,
	},
	ErrCodeBackupFailed: {
		Code:        ErrCodeBackupFailed,
		Name:        "Backup Failed",
		Description: "Failed to create a backup",
		HTTPStatus:  500,
		Example:     `{"error":"Internal Server Error","message":"Failed to create database backup","code":500,"db_code":"SY404"}`,
	},
	ErrCodeRestoreFailed: {
		Code:        ErrCodeRestoreFailed,
		Name:        "Restore Failed",
		Description: "Failed to restore from a backup",
		HTTPStatus:  500,
		Example:     `{"error":"Internal Server Error","message":"Failed to restore database from backup","code":500,"db_code":"SY405"}`,
	},
}

// GetHTTPStatusForErrorCode returns the appropriate HTTP status code for a SyncopateDB error code
func GetHTTPStatusForErrorCode(code ErrorCode) int {
	if doc, exists := ErrorCodeDocs[code]; exists {
		return doc.HTTPStatus
	}
	return 500 // Default to internal server error
}

// GetNameForErrorCode returns the human-readable name for a SyncopateDB error code
func GetNameForErrorCode(code ErrorCode) string {
	if doc, exists := ErrorCodeDocs[code]; exists {
		return doc.Name
	}
	return "Unknown Error"
}

// GetDescriptionForErrorCode returns the description for a SyncopateDB error code
func GetDescriptionForErrorCode(code ErrorCode) string {
	if doc, exists := ErrorCodeDocs[code]; exists {
		return doc.Description
	}
	return "An unknown error occurred"
}

// IsClientError returns true if the error code represents a client error (4xx)
func IsClientError(code ErrorCode) bool {
	status := GetHTTPStatusForErrorCode(code)
	return status >= 400 && status < 500
}

// IsServerError returns true if the error code represents a server error (5xx)
func IsServerError(code ErrorCode) bool {
	status := GetHTTPStatusForErrorCode(code)
	return status >= 500
}

// CategoryForErrorCode returns the category name for an error code
func CategoryForErrorCode(code ErrorCode) string {
	codeStr := string(code)
	if len(codeStr) < 3 {
		return "Unknown"
	}

	prefix := codeStr[0:3]
	switch {
	case prefix == "SY0":
		return "General"
	case prefix == "SY1":
		return "Entity Type"
	case prefix == "SY2":
		return "Entity"
	case prefix == "SY3":
		return "Query"
	case prefix == "SY4":
		return "Persistence"
	default:
		return "Unknown"
	}
}
