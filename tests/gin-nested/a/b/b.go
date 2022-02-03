package b

import (
	"github.com/gin-gonic/gin"
)

func Up(svc *gin.RouterGroup) {
	group := svc.Group("/b")

	group.GET("endpoint", func(c *gin.Context) {})
}
