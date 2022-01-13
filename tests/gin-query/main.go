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
		type queryParamsA struct {
			A string `form:"a"`
		}
		var queryA queryParamsA
		_ = c.ShouldBindQuery(&queryA)

		type queryParamsB struct {
			B string `form:"b"`
		}
		var queryB queryParamsB
		_ = c.BindQuery(&queryB)
	})

	// it should support query parameters via inline struct binding
	router.GET("/query3", func(c *gin.Context) {
		var queryA struct {
			A string `form:"a"`
		}
		_ = c.ShouldBindQuery(&queryA)

		var queryB struct {
			B string `form:"b"`
		}
		_ = c.BindQuery(&queryB)
	})

	if err := router.Run(":8080"); err != nil {
		panic(err)
	}
}
