// Package common provides shared types for pagination and validation across
// domain packages.
package common

import "time"

// Pagination carries pagination parameters with a cursor-based alternative.
type Pagination struct {
	Limit  int
	Offset int
	Cursor string
}

// CursorPage holds a page of typed items with cursor-based pagination metadata.
type CursorPage[T any] struct {
	Items      []T
	NextCursor string
	HasMore    bool
}

// TimeRange specifies an inclusive time window for queries.
type TimeRange struct {
	From time.Time
	To   time.Time
}

// Normalized returns a copy of p with validated Limit and Offset values.
func (p Pagination) Normalized(defaultLimit, maxLimit int) Pagination {
	if defaultLimit <= 0 {
		defaultLimit = 20
	}
	if maxLimit <= 0 {
		maxLimit = 100
	}
	if p.Limit <= 0 {
		p.Limit = defaultLimit
	}
	if p.Limit > maxLimit {
		p.Limit = maxLimit
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
	return p
}
