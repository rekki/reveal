package main

import (
	"github.com/gin-gonic/gin"
)

func main() {
	router := gin.Default()

	// it should support inbound header parameters
	router.GET("/header-inbound-1", func(c *gin.Context) {
		_ = c.GetHeader("Authorization")
		//_ = c.Request.Header.Get("ETag") // TODO
	})

	// it should support inbound header parameters via struct binding
	router.GET("/header-inbound-2", func(c *gin.Context) {
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

	// it should support inbound header parameters via inline struct binding
	router.GET("/header-inbound-3", func(c *gin.Context) {
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

	// it should support outbounds headers
	router.GET("/header-outbound-1", func(c *gin.Context) {
		// replace any existing header at key "A"
		c.Header("A", "foo")

		// delete any existing header at key "B"
		c.Header("B", "__nope__")
		c.Header("B", "")

		// this should result in an empty header for "C"
		c.Request.Header.Add("C", "__nope__")
		c.Request.Header.Del("C")

		// this should result in a 2-value header for "D"
		c.Request.Header.Add("D", "__nope__")
		c.Request.Header.Add("D", "__nope__")
		c.Request.Header.Set("D", "foobar")
		c.Request.Header.Add("D", "foobar")
	})
}
