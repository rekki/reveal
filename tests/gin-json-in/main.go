package main

import (
	"github.com/gin-gonic/gin"
)

func main() {
	router := gin.Default()

	// it should support bind and should bind
	router.GET("/json0", func(c *gin.Context) {
		type jsonParamsA struct {
			A string `json:"a__"`
		}
		var jsonA jsonParamsA
		_ = c.ShouldBindJSON(&jsonA)

		type jsonParamsB struct {
			B string `json:"b__"`
		}
		var jsonB jsonParamsB
		_ = c.BindJSON(&jsonB)
	})

	// it should support json fields via struct binding
	router.GET("/json1", func(c *gin.Context) {
		type jsonParamsA struct {
			A string `json:"a__"`
		}
		var jsonA jsonParamsA
		_ = c.ShouldBindJSON(&jsonA)
	})

	// it should support json fields via inline struct binding
	router.GET("/json2", func(c *gin.Context) {
		var jsonA struct {
			A string `json:"a__"`
		}
		_ = c.ShouldBindJSON(&jsonA)
	})

	// it should support json fields with no tags
	router.GET("/json3", func(c *gin.Context) {
		var jsonA struct {
			A string
		}
		_ = c.ShouldBindJSON(&jsonA)
	})

	// it should ignore lower-cased fields
	router.GET("/json4", func(c *gin.Context) {
		var jsonA struct {
			a string
		}
		_ = c.ShouldBindJSON(&jsonA)
	})

	// it should ignore fields with json:"-"
	router.GET("/json5", func(c *gin.Context) {
		var jsonA struct {
			A string `json:"-"`
		}
		_ = c.ShouldBindJSON(&jsonA)
	})

	if err := router.Run(":8080"); err != nil {
		panic(err)
	}

	// it should support all go types
	router.GET("/json6", func(c *gin.Context) {
		var jsonA struct {
			Bool bool

			String string

			Int     int
			Int8    int8
			Int16   int16
			Int32   int32
			Int64   int64
			Uint    uint
			Uint8   uint8
			Uint16  uint16
			Uint32  uint32
			Uint64  uint64
			Uintptr uintptr
			Byte    byte
			Rune    rune

			Float32 float32
			Float64 float64

			Array []string

			Map map[string]bool

			Struct struct{}

			//Complex64  complex64
			//Complex128 complex128
		}
		_ = c.ShouldBindJSON(&jsonA)
	})

	if err := router.Run(":8080"); err != nil {
		panic(err)
	}
}
