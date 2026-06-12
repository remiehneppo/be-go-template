package apperrors

import "net/http"

func StatusForCode(code Code) int {
	switch code {
	case CodeValidation:
		return http.StatusBadRequest
	case CodeUnauthorized:
		return http.StatusUnauthorized
	case CodeForbidden:
		return http.StatusForbidden
	case CodeNotFound:
		return http.StatusNotFound
	case CodeConflict:
		return http.StatusConflict
	case CodeTokenExpired:
		return http.StatusUnauthorized
	case CodeTokenRevoked:
		return http.StatusUnauthorized
	case CodeDependency:
		return http.StatusServiceUnavailable
	case CodeRateLimited:
		return http.StatusTooManyRequests
	case CodeRequestTooLarge:
		return http.StatusRequestEntityTooLarge
	case CodeTimeout:
		return http.StatusGatewayTimeout
	default:
		return http.StatusInternalServerError
	}
}
