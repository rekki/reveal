package main

import (
	"github.com/gin-gonic/gin"
)

func main() {
	router := gin.Default()

	// it should support route parameters
	router.GET("/users/:id", func(c *gin.Context) {})
	router.GET("/shops/:a/users", func(c *gin.Context) {})

	// it should support optional route parameters
	router.GET("/trucks/*id", func(c *gin.Context) {})

	// it should also support both in the same path
	router.GET("/orders/:a/*b", func(c *gin.Context) {})

	if err := router.Run(":8080"); err != nil {
		panic(err)
	}
}
