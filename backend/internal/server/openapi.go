package server

import (
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed openapi.admin.v1.json
var adminOpenAPI []byte

func serveAdminOpenAPI(c *gin.Context) {
	c.Header("Cache-Control", "private, max-age=300")
	c.Data(http.StatusOK, "application/json; charset=utf-8", adminOpenAPI)
}
