package main

import "github.com/gin-gonic/gin"

type Service struct {
	GinEngine
}

type GinEngine struct {
	*gin.Engine
}

func main() {
	// it should support embedded gin engines
	svc := Service{
		GinEngine: GinEngine{
			Engine: gin.Default(),
		},
	}

	svc.GET("/main", func(c *gin.Context) {})

	if err := svc.Run(":8080"); err != nil {
		panic(err)
	}
}

func main2() {
	svc := struct {
		service Service
	}{
		service: Service{
			GinEngine: GinEngine{
				Engine: gin.Default(),
			},
		},
	}

	svc.service.GET("/main2", func(c *gin.Context) {})

	if err := svc.service.Run(":8080"); err != nil {
		panic(err)
	}
}
