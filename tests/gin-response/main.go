package main

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
)

func main() {
	router := gin.Default()

	router.GET("/responses", func(c *gin.Context) {
		_ = c.AbortWithError(400, fmt.Errorf("c.AbortWithError"))

		c.AbortWithStatus(401)

		c.AbortWithStatusJSON(402, &struct{ A string }{A: "c.AbortWithStatusJSON"})

		c.AsciiJSON(403, &struct{ B string }{B: "c.AsciiJSON"})

		c.Data(404, "plain/text", []byte("c.Data"))

		c.DataFromReader(405, 0, "text/plain", strings.NewReader(""), nil)

		c.HTML(406, "template", nil)

		c.IndentedJSON(407, &struct{ C string }{C: "c.IndentedJSON"})

		c.JSON(408, &struct{ D string }{D: "c.JSON"})

		c.JSONP(409, &struct{ E string }{E: "c.JSONP"})

		//c.Negotiate(410, gin.Negotiate{})

		c.PureJSON(411, &struct{ F string }{F: "c.PureJSON"})

		//c.ProtoBuf(412, &struct{ G string }{G: "c.Protobuf"})

		c.Redirect(413, "/foobar")

		c.Render(414, nil)

		c.SecureJSON(415, &struct{ H string }{H: "c.SecureJSON"})

		c.Status(416)

		c.String(417, "c.String")

		c.XML(418, &struct{ I string }{I: "c.XML"})

		c.YAML(419, &struct{ J string }{J: "c.YAML"})
	})

	router.GET("/headers", func(c *gin.Context) {
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

	if err := router.Run(":8080"); err != nil {
		panic(err)
	}
}
