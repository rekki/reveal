package main

import (
	"github.com/gin-gonic/gin"
	"github.com/rekki/reveal/tests/gin-nested/a"
	"github.com/rekki/reveal/tests/gin-nested/a/b"
)

func main() {
	router := gin.Default()

	router.GET("endpoint", func(c *gin.Context) {})

	a.Up(router)
	b.Up(&router.RouterGroup)

	if err := router.Run(":8080"); err != nil {
		panic(err)
	}
}
