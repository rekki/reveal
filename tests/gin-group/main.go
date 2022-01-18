package main

import (
	"github.com/gin-gonic/gin"
)

func main() {
	router := gin.Default()

	router.GET("/root", func(c *gin.Context) {})

	// it should support (nested) groups
	a := router.Group("/a")
	{
		a.GET("/under-a", func(c *gin.Context) {})

		b := a.Group("/b")
		{
			b.GET("/under-a-b", func(c *gin.Context) {})

			c := b.Group("/c")
			{
				c.GET("/under-a-b-c", func(c *gin.Context) {})
			}
		}
	}

	if err := router.Run(":8080"); err != nil {
		panic(err)
	}
}
