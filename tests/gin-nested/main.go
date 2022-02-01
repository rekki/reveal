package main

import (
	"github.com/gin-gonic/gin"
	"github.com/rekki/reveal/tests/gin-nested/a"
)

func main() {
	router := gin.Default()

	a.Up(router)

	if err := router.Run(":8080"); err != nil {
		panic(err)
	}
}
