package http

import "testing"

func TestStableETagIsStableForSamePayload(t *testing.T) {
	payload := []map[string]string{{"session_id": "s1", "device_id": "d1"}}

	first := StableETag(payload)
	second := StableETag(payload)

	if first == "" || first != second {
		t.Fatalf("etag = %q, %q", first, second)
	}
}

func TestIfNoneMatchSupportsMultipleValues(t *testing.T) {
	if !ifNoneMatch(`"other", "abc"`, `"abc"`) {
		t.Fatal("ifNoneMatch() = false")
	}
}
