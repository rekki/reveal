package reveal

import (
	"go/ast"

	"github.com/getkin/kin-openapi/openapi3"
)

type Graph struct {
	Groups    map[ast.Node]*Group
	Endpoints map[ast.Node]*Endpoint
	Idents    map[string]ast.Node
}

func NewGraph() *Graph {
	return &Graph{
		Groups:    map[ast.Node]*Group{},
		Endpoints: map[ast.Node]*Endpoint{},
		Idents:    map[string]ast.Node{},
	}
}

func (g *Graph) RootedPathAndParams(e *Endpoint) (string, openapi3.Parameters) {
	current := e.ASTNode

	path := e.Path
	params := append(openapi3.Parameters{}, e.PathParams...)

	for {
		if current == nil {
			return path, params
		}

		if group, ok := g.Groups[current]; ok {
			path = group.Path + path
			params = append(params, group.PathParams...)
		}

		if callexpr, ok := current.(*ast.CallExpr); ok {
			if selectorexpr, ok := callexpr.Fun.(*ast.SelectorExpr); ok {
				current = selectorexpr.X.(*ast.Ident)
			} else {
				current = nil
			}
		} else if ident, ok := current.(*ast.Ident); ok {
			current = g.Idents[ident.Name]
		}
	}
}

type Group struct {
	ASTNode    ast.Node
	Path       string
	PathParams openapi3.Parameters
}

type Endpoint struct {
	Group
	Method      string
	Description string
}
