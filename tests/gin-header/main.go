package main

import (
	"github.com/gin-gonic/gin"
)

func main() {
	router := gin.Default()

	// it should support header parameters
	router.GET("/header1", func(c *gin.Context) {
		_ = c.GetHeader("Authorization")
		// TODO: _ = c.Request.Header.Get("ETag")
	})

	// it should support header parameters via struct binding
	router.GET("/header2", func(c *gin.Context) {
		type headerParamsA struct {
			A string `header:"a"`
		}
		var headerA headerParamsA
		_ = c.ShouldBindHeader(&headerA)

		type headerParamsB struct {
			B string `header:"b"`
		}
		var headerB headerParamsB
		_ = c.BindHeader(&headerB)
	})

	// it should support header parameters via inline struct binding
	router.GET("/header3", func(c *gin.Context) {
		var headerA struct {
			A string `header:"a"`
		}
		_ = c.ShouldBindHeader(&headerA)

		var headerB struct {
			B string `header:"b"`
		}
		_ = c.BindHeader(&headerB)
	})

	if err := router.Run(":8080"); err != nil {
		panic(err)
	}
}
