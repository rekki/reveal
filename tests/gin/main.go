package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func main() {
	router := gin.Default()

	router.DELETE("/", func(c *gin.Context) {})
	router.GET("/", func(c *gin.Context) {})
	router.HEAD("/", func(c *gin.Context) {})
	router.OPTIONS("/", func(c *gin.Context) {})
	router.PATCH("/", func(c *gin.Context) {})
	router.POST("/", func(c *gin.Context) {})
	router.PUT("/", func(c *gin.Context) {})
	router.Handle(http.MethodConnect, "/", func(c *gin.Context) {})

	a := router.Group("/a")
	{
		a.GET("/", func(c *gin.Context) {})
		a.Handle(http.MethodConnect, "/", func(c *gin.Context) {})

		b := a.Group("/b")
		{
			b.GET("/", func(c *gin.Context) {})
			b.Handle(http.MethodConnect, "/", func(c *gin.Context) {})

			c := b.Group("/c")
			{
				c.GET("/", func(c *gin.Context) {})
				c.Handle(http.MethodConnect, "/", func(c *gin.Context) {})
			}
		}
	}

	router.Run(":8080")
}
