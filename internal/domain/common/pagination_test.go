package common

import "testing"

func TestPaginationNormalized(t *testing.T) {
	got := (Pagination{Limit: 500, Offset: -1}).Normalized(20, 100)
	if got.Limit != 100 {
		t.Fatalf("Limit = %d", got.Limit)
	}
	if got.Offset != 0 {
		t.Fatalf("Offset = %d", got.Offset)
	}
}
