package common

type ValidationDetail struct {
	Field  string
	Reason string
	Meta   map[string]any
}

type ValidationError struct {
	Details []ValidationDetail
}

func (e ValidationError) Error() string {
	return "validation failed"
}
