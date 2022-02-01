package a

import (
	"github.com/gin-gonic/gin"
	"github.com/rekki/reveal/tests/gin-nested/a/b"
)

func Up(svc *gin.Engine) {
	group := svc.Group("/a")

	group.GET("endpoint", func(c *gin.Context) {})

	b.Up(&svc.RouterGroup)
}
