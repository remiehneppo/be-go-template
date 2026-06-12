package mongo

import "time"

const (
	userProfileCacheTTL    = 10 * time.Minute
	sessionActiveCacheTTL  = 2 * time.Minute
	sessionRefreshCacheTTL = 1 * time.Minute
	activeSessionsCacheTTL = 1 * time.Minute
)
