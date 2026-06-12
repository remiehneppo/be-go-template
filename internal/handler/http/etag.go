package http

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/gin-gonic/gin"
)

var etagEnabled atomic.Bool

func init() {
	etagEnabled.Store(true)
}

func SetETagEnabled(enabled bool) {
	etagEnabled.Store(enabled)
}

func WriteETagOrNotModified(c *gin.Context, payload any) bool {
	if !etagEnabled.Load() {
		return false
	}
	etag := StableETag(payload)
	c.Header("ETag", etag)
	if ifNoneMatch(c.GetHeader("If-None-Match"), etag) {
		c.Status(http.StatusNotModified)
		return true
	}
	return false
}

func StableETag(payload any) string {
	data, err := json.Marshal(payload)
	if err != nil {
		data = []byte("null")
	}
	sum := sha256.Sum256(data)
	return `"` + hex.EncodeToString(sum[:]) + `"`
}

func ifNoneMatch(header string, etag string) bool {
	for _, part := range strings.Split(header, ",") {
		if strings.TrimSpace(part) == etag {
			return true
		}
	}
	return false
}
