package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func main() {
	router := gin.Default()

	// it should support http methods
	router.DELETE("/", func(c *gin.Context) {})
	router.GET("/", func(c *gin.Context) {})
	router.HEAD("/", func(c *gin.Context) {})
	router.OPTIONS("/", func(c *gin.Context) {})
	router.PATCH("/", func(c *gin.Context) {})
	router.POST("/", func(c *gin.Context) {})
	router.PUT("/", func(c *gin.Context) {})

	// it should support custom http methods
	router.Handle(http.MethodConnect, "/", func(c *gin.Context) {})

	if err := router.Run(":8080"); err != nil {
		panic(err)
	}
}
