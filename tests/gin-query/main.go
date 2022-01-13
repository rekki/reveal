package main

import (
	"github.com/gin-gonic/gin"
)

func main() {
	router := gin.Default()

	// it should support query parameters
	router.GET("/query1", func(c *gin.Context) {
		_ = c.DefaultQuery("firstname", "Guest")
		_ = c.Query("lastname")
	})

	// it should support query parameters via struct binding
	router.GET("/query2", func(c *gin.Context) {
	})

	// it should support query parameters via inline struct binding
	router.GET("/query3", func(c *gin.Context) {
	})

	if err := router.Run(":8080"); err != nil {
		panic(err)
	}
}
