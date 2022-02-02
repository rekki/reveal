package reveal

import (
	"go/ast"
	"go/constant"
	"go/types"
	"regexp"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"golang.org/x/tools/go/packages"
)

type Visitor struct {
	pkg    *packages.Package
	groups map[ast.Expr]*Group     // maps engines/routergroup to groups
	edges  map[ast.Object]ast.Expr // maps idents to exprs
	Root   *Group
}

func NewVisitor(pkg *packages.Package) *Visitor {
	return &Visitor{
		pkg,
		map[ast.Expr]*Group{},
		map[ast.Object]ast.Expr{},
		&Group{},
	}
}

func (v *Visitor) Walk() {
	if len(v.pkg.Syntax) < 1 {
		return
	}
	v.walk(v.pkg.Syntax[0])
}

func (v *Visitor) walk(file *ast.File) {
	ast.Inspect(file, func(n ast.Node) bool {
		if assignstmt, ok := n.(*ast.AssignStmt); ok {
			for i, lhs := range assignstmt.Lhs {
				if i >= len(assignstmt.Rhs) {
					break
				}

				if ident, ok := lhs.(*ast.Ident); ok {
					v.edges[*ident.Obj] = assignstmt.Rhs[i]
				}
			}
			return true
		}

		if valuespec, ok := n.(*ast.ValueSpec); ok {
			for i, ident := range valuespec.Names {
				if i >= len(valuespec.Values) {
					break
				}
				v.edges[*ident.Obj] = valuespec.Values[i]
			}
			return true
		}

		callexpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true // go deeper until we find a call expression
		}

		selector, ok := callexpr.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}

		var x *ast.Ident
		if ident, ok := selector.X.(*ast.Ident); ok {
			x = ident
		} else if selectorexpr, ok := selector.X.(*ast.SelectorExpr); ok {
			x = selectorexpr.Sel
		} else {
			return false
		}

		expr := v.resolveExpr(x) // expr defining the parent group
		parent, ok := v.groups[expr]
		if !ok {
			parent = v.Root
		}

		switch selector.Sel.Name {
		case "Group":
			if len(callexpr.Args) > 0 {
				p, pp := inferPath(constant.StringVal(v.pkg.TypesInfo.Types[callexpr.Args[0]].Value))
				g := &Group{Path: p, PathParams: pp}
				v.groups[callexpr] = g
				parent.groups = append(parent.groups, g)
			}

		case "Handle":
			if len(callexpr.Args) > 1 {
				m := constant.StringVal(v.pkg.TypesInfo.Types[callexpr.Args[0]].Value)
				p, pp := inferPath(constant.StringVal(v.pkg.TypesInfo.Types[callexpr.Args[1]].Value))
				if len(m) > 0 && len(p) > 0 {
					parent.endpoints = append(parent.endpoints, &Endpoint{Method: m, Path: p, PathParams: pp})
				}
			}

		case "POST", "GET", "HEAD", "PUT", "PATCH", "DELETE", "OPTIONS":
			if len(callexpr.Args) > 0 {
				m := selector.Sel.Name
				p, pp := inferPath(constant.StringVal(v.pkg.TypesInfo.Types[callexpr.Args[0]].Value))
				if len(m) > 0 && len(p) > 0 {
					parent.endpoints = append(parent.endpoints, &Endpoint{Method: m, Path: p, PathParams: pp})
				}
			}
		}

		return false
	})
}

func (v *Visitor) resolveExpr(x *ast.Ident) ast.Expr {
	if x.Obj != nil {
		if resolved, ok := v.edges[*x.Obj]; ok {
			if ident, ok := resolved.(*ast.Ident); ok {
				return v.resolveExpr(ident)
			}
			return resolved
		}
	}
	return x
}

var inferPathRegexp = regexp.MustCompilePOSIX(`\/[*:][^\/]+`)

func inferPath(input string) (string, openapi3.Parameters) {
	params := openapi3.Parameters{}

	path := inferPathRegexp.ReplaceAllStringFunc(input, func(match string) string {
		required := match[1] == ':'
		name := match[2:]

		params = append(params, &openapi3.ParameterRef{
			Value: &openapi3.Parameter{
				In:       openapi3.ParameterInPath,
				Name:     name,
				Required: required,
				Schema: &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: openapi3.TypeString,
					},
				},
			},
		})

		return "/{" + name + "}"
	})

	return "/" + strings.TrimLeft(strings.TrimRight(path, "/"), "/"), params
}

type GinKind int

const (
	Unsupported GinKind = iota
	Engine
	RouterGroup
)

func resolveGinKind(ty types.Type) GinKind {
	for {
		if ty.String() == "github.com/gin-gonic/gin.Engine" {
			return Engine
		} else if ty.String() == "github.com/gin-gonic/gin.RouterGroup" {
			return RouterGroup
		} else if ptr, ok := ty.(*types.Pointer); ok {
			ty = ptr.Elem()
		} else if named, ok := ty.(*types.Named); ok {
			ty = named.Underlying()
		} else {
			break
		}
	}

	if strct, ok := ty.(*types.Struct); ok {
		for i, max := 0, strct.NumFields(); i < max; i++ {
			if f := strct.Field(i); f.Embedded() {
				if kind := resolveGinKind(f.Type()); kind != Unsupported {
					return kind
				}
			}
		}
	}

	return Unsupported
}
