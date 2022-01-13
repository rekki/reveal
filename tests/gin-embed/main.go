package main

import "github.com/gin-gonic/gin"

type Service struct {
	GinEngine
}

type GinEngine struct {
	*gin.Engine
}

func main() {
	svc := Service{
		GinEngine: GinEngine{
			Engine: gin.Default(),
		},
	}

	svc.GET("/user", func(c *gin.Context) {})

	svc.Run(":8080")
}
