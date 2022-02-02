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
	out := append([]*Endpoint{}, g.endpoints...)

	for _, group := range g.groups {
		for _, endpoint := range group.Endpoints() {
			endpoint.Path = group.Path + endpoint.Path
			endpoint.PathParams = append(endpoint.PathParams, group.PathParams...)
			out = append(out, endpoint)
		}
	}

	return out
}

type Endpoint struct {
	Path       string
	PathParams openapi3.Parameters
	//
	Method      string
	Description string
}
