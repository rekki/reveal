package reveal

import (
	"go/ast"

	"github.com/getkin/kin-openapi/openapi3"
)

type Graph struct {
	Nodes []*Node                 // vertices
	Edges map[*ast.Ident]ast.Node // edges
}

func NewGraph() *Graph {
	return &Graph{
		Edges: map[*ast.Ident]ast.Node{},
	}
}

type Node struct {
	// Common to endpoints and groups
	ASTNode    ast.Node
	Path       string
	PathParams openapi3.Parameters
	// Only for endpoints
	Method    string
	Operation *openapi3.Operation
}

func (n *Node) RootedPath(graph *Graph) string {
	current := n.ASTNode
	path := n.Path

	for {
		if current == nil {
			return path
		}

		if callexpr, ok := current.(*ast.CallExpr); ok {
			if selectorexpr, ok := callexpr.Fun.(*ast.SelectorExpr); ok {
				if selectorexpr.Sel.Name == "Default" {
					return path
				}
				current = graph.Edges[selectorexpr.Sel]
			}
		}

		break
	}

	return path + "___"
}
