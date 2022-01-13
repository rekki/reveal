package main

import (
	"github.com/gin-gonic/gin"
)

func main() {
	router := gin.Default()

	// it should support (nested) groups
	a := router.Group("/a")
	{
		a.GET("/", func(c *gin.Context) {})

		b := a.Group("/b")
		{
			b.GET("/", func(c *gin.Context) {})

			c := b.Group("/c")
			{
				c.GET("/", func(c *gin.Context) {})
			}
		}
	}

	if err := router.Run(":8080"); err != nil {
		panic(err)
	}
}
