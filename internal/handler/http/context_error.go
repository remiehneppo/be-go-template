package http

import "github.com/gin-gonic/gin"

func reportContextError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	return c.Error(err) != nil
}
