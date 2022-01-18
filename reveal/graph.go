package reveal

import "github.com/getkin/kin-openapi/openapi3"

type Graph struct {
	Nodes []*Node
}

type Node struct {
	Parent   *Node // nil for root nodes
	Children []*Node

	Method    string
	Path      string
	Operation *openapi3.Operation
}

func (n *Node) RootedPath() string {
	return n.Path // TODO: resolve a clean rooted path
}
