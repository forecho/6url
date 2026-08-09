package server

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
)

func contextWithTimeout(c *gin.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(c.Request.Context(), timeout)
}
