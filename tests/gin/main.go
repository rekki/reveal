package main

import (
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

	router.Run(":8080")
}
