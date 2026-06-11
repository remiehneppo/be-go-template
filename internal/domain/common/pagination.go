package common

import "time"

type Pagination struct {
	Limit  int
	Offset int
	Cursor string
}

type CursorPage[T any] struct {
	Items      []T
	NextCursor string
	HasMore    bool
}

type TimeRange struct {
	From time.Time
	To   time.Time
}

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
