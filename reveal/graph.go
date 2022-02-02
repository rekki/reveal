package reveal

import "github.com/getkin/kin-openapi/openapi3"

type Group struct {
	Path       string
	PathParams openapi3.Parameters
	//
	groups    []*Group
	endpoints []*Endpoint
}

func (g *Group) Endpoints() []*Endpoint {
	return nil
}

type Endpoint struct {
	Path       string
	PathParams openapi3.Parameters
	//
	Method      string
	Description string
}
