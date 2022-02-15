package main

import (
	"github.com/gin-gonic/gin"
)

func main() {
	router := gin.Default()

	// it should support json parameters via struct binding
	router.GET("/json2", func(c *gin.Context) {
		type jsonParamsA struct {
			A string `json:"a"`
		}
		var jsonA jsonParamsA
		_ = c.ShouldBindJSON(&jsonA)

		type jsonParamsB struct {
			B string `json:"b"`
		}
		var jsonB jsonParamsB
		_ = c.BindJSON(&jsonB)
	})

	// it should support json parameters via inline struct binding
	router.GET("/json3", func(c *gin.Context) {
		var jsonA struct {
			A string `json:"a"`
		}
		_ = c.ShouldBindJSON(&jsonA)

		var jsonB struct {
			B string `json:"b"`
		}
		_ = c.BindJSON(&jsonB)
	})

	if err := router.Run(":8080"); err != nil {
		panic(err)
	}
}
