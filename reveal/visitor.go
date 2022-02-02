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

type Group struct {
	Path      string
	Endpoints []*Endpoint
}

type Endpoint struct {
	Method      string
	Path        string
	PathParams  openapi3.Parameters
	Description string
}

type Visitor struct {
	pkg   *packages.Package
	edges map[*ast.Object]ast.Expr // maps idents to exprs
	Root  *Group
}

func NewVisitor(pkg *packages.Package) *Visitor {
	return &Visitor{pkg, map[*ast.Object]ast.Expr{}, &Group{"/", nil}}
}

func (v *Visitor) Walk() {
	if len(v.pkg.Syntax) < 1 {
		return
	}
	file := v.pkg.Syntax[0]

	entrypoints := v.getEntrypoints(file)
	if len(entrypoints) < 1 {
		return
	}
	entrypoint := entrypoints[0]

	engines := v.getEngines(entrypoint)
	if len(engines) < 1 {
		return
	}
	engine := engines[0]

	v.collectEndpoints(v.Root, file, v.pkg.TypesInfo.Defs[engine])
}

func (v *Visitor) getEntrypoints(file *ast.File) []*ast.FuncDecl {
	// if we are in a main package we only want the main function
	if file.Name.Name == "main" {
		for _, decl := range file.Decls {
			if fdecl, ok := decl.(*ast.FuncDecl); ok {
				if fdecl.Name.Name == "main" {
					return []*ast.FuncDecl{fdecl}
				}
			}
		}
	}

	// if we are not in a main package, we want all the exported functions
	var out []*ast.FuncDecl
	for _, decl := range file.Decls {
		if fdecl, ok := decl.(*ast.FuncDecl); ok && fdecl.Name.IsExported() {
			out = append(out, fdecl)
		}
	}
	return out
}

func (v *Visitor) getEngines(fdecl *ast.FuncDecl) []*ast.Ident {
	var out []*ast.Ident

	ast.Inspect(fdecl, func(n ast.Node) bool {
		if assignstmt, ok := n.(*ast.AssignStmt); ok {
			for _, lhs := range assignstmt.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok {
					if def := v.pkg.TypesInfo.Defs[ident]; def != nil {
						if kind := resolveGinKind(def.Type()); kind == Engine {
							out = append(out, ident)
						}
					}
				}
			}
			return false
		}

		if valuespec, ok := n.(*ast.ValueSpec); ok {
			for _, ident := range valuespec.Names {
				if def := v.pkg.TypesInfo.Defs[ident]; def != nil {
					if kind := resolveGinKind(def.Type()); kind == Engine {
						out = append(out, ident)
					}
				}
			}
			return false
		}

		return true
	})

	return out
}

func (v *Visitor) collectEndpoints(group *Group, file *ast.File, engine types.Object) {
	ast.Inspect(file, func(n ast.Node) bool {
		if assignstmt, ok := n.(*ast.AssignStmt); ok {
			for i, lhs := range assignstmt.Lhs {
				if i >= len(assignstmt.Rhs) {
					break
				}
				if ident, ok := lhs.(*ast.Ident); ok {
					v.edges[ident.Obj] = assignstmt.Rhs[i]
				}
			}
			return false
		}

		if valuespec, ok := n.(*ast.ValueSpec); ok {
			for i, ident := range valuespec.Names {
				if i >= len(valuespec.Values) {
					break
				}
				v.edges[ident.Obj] = valuespec.Values[i]
			}
			return false
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

		if use, ok := v.resolveObject(x); ok && use == engine {
			switch selector.Sel.Name {
			case "Group":
				// TODO: gather nested nodes

			case "Handle":
				if len(callexpr.Args) > 1 {
					m := constant.StringVal(v.pkg.TypesInfo.Types[callexpr.Args[0]].Value)
					p, pp := inferPath(constant.StringVal(v.pkg.TypesInfo.Types[callexpr.Args[1]].Value))
					if len(m) > 0 && len(p) > 0 {
						group.Endpoints = append(group.Endpoints, &Endpoint{Method: m, Path: p, PathParams: pp})
					}
				}

			case "POST", "GET", "HEAD", "PUT", "PATCH", "DELETE", "OPTIONS":
				if len(callexpr.Args) > 0 {
					m := selector.Sel.Name
					p, pp := inferPath(constant.StringVal(v.pkg.TypesInfo.Types[callexpr.Args[0]].Value))
					if len(m) > 0 && len(p) > 0 {
						group.Endpoints = append(group.Endpoints, &Endpoint{Method: m, Path: p, PathParams: pp})
					}
				}
			}
		}

		return false
	})
}

func (v *Visitor) resolveObject(x *ast.Ident) (types.Object, bool) {
	if resolved, ok := v.edges[x.Obj]; ok {
		if ident, ok := resolved.(*ast.Ident); ok {
			return v.resolveObject(ident)
		}
	}

	object, ok := v.pkg.TypesInfo.Uses[x]
	return object, ok
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
