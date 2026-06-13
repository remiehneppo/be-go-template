package apperrors

// Code is a stable string identifier for application errors. Codes are used
// in the JSON error envelope and should remain stable across releases.
type Code string

const (
	// CodeInternal indicates an unexpected server error.
	CodeInternal Code = "INTERNAL_ERROR"
	// CodeValidation indicates the request failed validation.
	CodeValidation Code = "VALIDATION_ERROR"
	// CodeUnauthorized indicates missing or invalid credentials.
	CodeUnauthorized Code = "UNAUTHORIZED"
	// CodeForbidden indicates the user lacks the required role or permission.
	CodeForbidden Code = "FORBIDDEN"
	// CodeNotFound indicates the requested resource does not exist.
	CodeNotFound Code = "NOT_FOUND"
	// CodeConflict indicates a uniqueness constraint violation.
	CodeConflict Code = "CONFLICT"
	// CodeTokenExpired indicates the token has expired.
	CodeTokenExpired Code = "TOKEN_EXPIRED"
	// CodeTokenRevoked indicates the token has been revoked.
	CodeTokenRevoked Code = "TOKEN_REVOKED"
	// CodeDependency indicates a downstream dependency failure.
	CodeDependency Code = "DEPENDENCY_ERROR"
	// CodeRateLimited indicates the request was rate-limited.
	CodeRateLimited Code = "RATE_LIMITED"
	// CodeRequestTooLarge indicates the request body exceeds the limit.
	CodeRequestTooLarge Code = "REQUEST_TOO_LARGE"
	// CodeTimeout indicates the request exceeded the timeout.
	CodeTimeout Code = "TIMEOUT"
)
