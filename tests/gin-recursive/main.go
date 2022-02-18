package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Foo struct {
	Name string
	F    *Foo
	B    *Bar
}

type Bar struct {
	Name string
	F    *Foo
}

func main() {
	svc := gin.Default()

	svc.GET("/rec", func(c *gin.Context) {
		c.JSON(http.StatusOK, &Foo{
			Name: "root",
			F: &Foo{
				Name: "foo",
			},
			B: &Bar{
				F: &Foo{
					Name: "bar",
				},
			},
		})
	})

	if err := svc.Run(":8080"); err != nil {
		panic(err)
	}
}
