package apperrors

type Code string

const (
	CodeInternal        Code = "INTERNAL_ERROR"
	CodeValidation      Code = "VALIDATION_ERROR"
	CodeUnauthorized    Code = "UNAUTHORIZED"
	CodeForbidden       Code = "FORBIDDEN"
	CodeNotFound        Code = "NOT_FOUND"
	CodeConflict        Code = "CONFLICT"
	CodeTokenExpired    Code = "TOKEN_EXPIRED"
	CodeTokenRevoked    Code = "TOKEN_REVOKED"
	CodeDependency      Code = "DEPENDENCY_ERROR"
	CodeRateLimited     Code = "RATE_LIMITED"
	CodeRequestTooLarge Code = "REQUEST_TOO_LARGE"
	CodeTimeout         Code = "TIMEOUT"
)
