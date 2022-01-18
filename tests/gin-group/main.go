package main

import (
	"github.com/gin-gonic/gin"
)

func main() {
	router := gin.Default()

	r := router
	var rr *gin.Engine
	rr = r
	var rrr *gin.Engine = rr

	rrr.GET("/root", func(c *gin.Context) {})

	a := rrr.Group("/:a")
	var aa *gin.RouterGroup
	aa = a
	var aaa *gin.RouterGroup = aa

	{
		aaa.GET("/under-a", func(c *gin.Context) {})

		b := aaa.Group("/b")
		var bb *gin.RouterGroup
		bb = b
		var bbb *gin.RouterGroup = bb

		{
			bbb.GET("/under-a-b", func(c *gin.Context) {})

			c := bbb.Group("/c")
			var cc *gin.RouterGroup
			cc = c
			var ccc *gin.RouterGroup = cc
			{
				ccc.GET("/under-a-b-c", func(c *gin.Context) {})
			}
		}
	}

	if err := router.Run(":8080"); err != nil {
		panic(err)
	}
}
