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

type EndpointsVisitor struct {
	root         *Group            // root group
	entrypoint   *packages.Package // root package
	pkgsByID     map[string]*packages.Package
	groupsByExpr map[ast.Expr]*Group
	exprsByIdent map[ast.Object]ast.Expr
}

func NewEndpointsVisitor(pkgs []*packages.Package) *EndpointsVisitor {
	v := &EndpointsVisitor{
		root:         &Group{},
		entrypoint:   nil,
		pkgsByID:     map[string]*packages.Package{},
		groupsByExpr: map[ast.Expr]*Group{},
		exprsByIdent: map[ast.Object]ast.Expr{},
	}

	// entrypoint is always the last package
	v.entrypoint = pkgs[len(pkgs)-1]

	// indexing packages by id
	for _, pkg := range pkgs {
		v.pkgsByID[pkg.ID] = pkg
	}

	return v
}

func (v *EndpointsVisitor) Walk() {
	if v.entrypoint != nil {
		v.walk(v.entrypoint.Syntax[0], v.entrypoint)
	}
}

func (v *EndpointsVisitor) Endpoints() []*Endpoint {
	return v.root.all()
}

func (v *EndpointsVisitor) walk(node ast.Node, pkg *packages.Package) {
	ast.Inspect(node, func(n ast.Node) bool {
		// Gather and store assignements and var declarations as we find them to
		// make it possible to resolve identifiers chains
		{
			if assignstmt, ok := n.(*ast.AssignStmt); ok {
				for i, lhs := range assignstmt.Lhs {
					if i >= len(assignstmt.Rhs) {
						break
					}
					if ident, ok := lhs.(*ast.Ident); ok {
						v.exprsByIdent[*ident.Obj] = assignstmt.Rhs[i]
					}
				}
				return true
			}

			if valuespec, ok := n.(*ast.ValueSpec); ok {
				for i, ident := range valuespec.Names {
					if i >= len(valuespec.Values) {
						break
					}
					v.exprsByIdent[*ident.Obj] = valuespec.Values[i]
				}
				return true
			}
		}

		callexpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true // go deeper until we find a call expression
		}

		// If we are passing a gin engine/routergroup as an argument to the
		// function call, follow that function to its definition.
		{
			var follow bool
			for _, arg := range callexpr.Args {
				if kind := resolveGinKind(pkg.TypesInfo.Types[arg].Type); kind != Unknown {
					follow = true
					break
				}
			}

			if follow {
				if fdecl, fpkg := v.resolveFunctionDeclaration(callexpr, pkg); fdecl != nil {
					i := 0
					for _, param := range fdecl.Type.Params.List {
						for _, name := range param.Names {
							v.exprsByIdent[*name.Obj] = callexpr.Args[i]
							i++
						}
					}
					v.walk(fdecl, fpkg)
				}
			}
		}

		// If we are calling a method on a Gin engine/routergroup, then infer
		// endpoints/groups.
		{
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

			if kind := resolveGinKind(pkg.TypesInfo.Types[x].Type); kind != Unknown {
				var parent *Group
				if kind == Engine {
					parent = v.root
				} else if kind == RouterGroup {
					parent = v.groupsByExpr[v.resolveExpr(x)]
				}

				if parent == nil {
					return false
				}

				switch selector.Sel.Name {
				case "Group":
					if len(callexpr.Args) >= 1 {
						if arg0, ok := v.foldConstant(callexpr.Args[0], pkg); ok {
							path, pathParams := inferPath(arg0)
							g := &Group{Path: path, Params: pathParams}
							v.groupsByExpr[callexpr] = g
							parent.groups = append(parent.groups, g)
						}
					}

				case "Handle":
					if len(callexpr.Args) > 2 {
						if m, ok := v.foldConstant(callexpr.Args[0], pkg); ok {
							if arg1, ok := v.foldConstant(callexpr.Args[1], pkg); ok {
								path, pathParams := inferPath(arg1)
								handlerParams := v.inferHandler(callexpr.Args[len(callexpr.Args)-1], pkg)

								endpoint := &Endpoint{Method: m, Path: path}
								endpoint.Params = append(endpoint.Params, pathParams...)
								endpoint.Params = append(endpoint.Params, handlerParams...)

								parent.endpoints = append(parent.endpoints, endpoint)
							}
						}
					}

				case "POST", "GET", "HEAD", "PUT", "PATCH", "DELETE", "OPTIONS":
					if len(callexpr.Args) >= 2 {
						m := selector.Sel.Name
						if arg0, ok := v.foldConstant(callexpr.Args[0], pkg); ok {
							path, pathParams := inferPath(arg0)
							handlerParams := v.inferHandler(callexpr.Args[len(callexpr.Args)-1], pkg)

							endpoint := &Endpoint{Method: m, Path: path}
							endpoint.Params = append(endpoint.Params, pathParams...)
							endpoint.Params = append(endpoint.Params, handlerParams...)

							parent.endpoints = append(parent.endpoints, endpoint)
						}
					}
				}

				return false
			}
		}

		return false
	})
}

func (v *EndpointsVisitor) foldConstant(expr ast.Expr, pkg *packages.Package) (string, bool) {
	ty, ok := pkg.TypesInfo.Types[expr]
	if !ok {
		return "", false
	}

	folded := constant.StringVal(ty.Value)
	if len(folded) == 0 {
		return "", false
	}

	return folded, true
}

func (v *EndpointsVisitor) inferHandler(expr ast.Expr, pkg *packages.Package) openapi3.Parameters {
	var out openapi3.Parameters

	if lit, ok := expr.(*ast.FuncLit); ok {
		ast.Inspect(lit, func(n ast.Node) bool {
			if callexpr, ok := n.(*ast.CallExpr); ok {
				if selectorexpr, ok := callexpr.Fun.(*ast.SelectorExpr); ok {
					if isGinContext(pkg.TypesInfo.Types[selectorexpr.X].Type) {

						switch selectorexpr.Sel.Name {
						case "Query":
							if len(callexpr.Args) > 0 {
								if query, ok := v.foldConstant(callexpr.Args[0], pkg); ok {
									out = append(out, &openapi3.ParameterRef{
										Value: &openapi3.Parameter{
											In:       openapi3.ParameterInQuery,
											Name:     query,
											Required: true,
											Schema: &openapi3.SchemaRef{
												Value: &openapi3.Schema{
													Type: openapi3.TypeString,
												},
											},
										},
									})
								}
							}

						case "DefaultQuery":
							if len(callexpr.Args) > 1 {
								if query, ok := v.foldConstant(callexpr.Args[0], pkg); ok {
									if defaultValue, ok := v.foldConstant(callexpr.Args[1], pkg); ok {
										out = append(out, &openapi3.ParameterRef{
											Value: &openapi3.Parameter{
												In:       openapi3.ParameterInQuery,
												Name:     query,
												Required: false,
												Schema: &openapi3.SchemaRef{
													Value: &openapi3.Schema{
														Type:    openapi3.TypeString,
														Default: defaultValue,
													},
												},
											},
										})
									}
								}
							}
						}

					}
				}
				return false
			}

			return true
		})
	}

	return out
}

func (v *EndpointsVisitor) resolveFunctionDeclaration(callexpr *ast.CallExpr, pkg *packages.Package) (*ast.FuncDecl, *packages.Package) {
	if selectorexpr, ok := callexpr.Fun.(*ast.SelectorExpr); ok {
		if ident, ok := selectorexpr.X.(*ast.Ident); ok {
			if fpkgName, ok := pkg.TypesInfo.Uses[ident].(*types.PkgName); ok && fpkgName != nil {
				if fpkg := v.pkgsByID[fpkgName.Imported().Path()]; fpkg != nil {
					for _, decl := range fpkg.Syntax[0].Decls {
						if fdecl, ok := decl.(*ast.FuncDecl); ok {
							if fdecl.Name.IsExported() && fdecl.Name.Name == selectorexpr.Sel.Name {
								return fdecl, fpkg
							}
						}
					}
				}
			}
		}
	}
	return nil, nil
}

func (v *EndpointsVisitor) resolveExpr(x *ast.Ident) ast.Expr {
	if x.Obj != nil {
		if resolved, ok := v.exprsByIdent[*x.Obj]; ok {
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
	Unknown GinKind = iota
	Engine
	RouterGroup
)

func resolveGinKind(ty types.Type) GinKind {
	if ty == nil {
		return Unknown
	}

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
				if kind := resolveGinKind(f.Type()); kind != Unknown {
					return kind
				}
			}
		}
	}

	return Unknown
}

func isGinContext(ty types.Type) bool {
	return ty != nil && ty.String() == "*github.com/gin-gonic/gin.Context"
}

type Group struct {
	Path      string
	Params    openapi3.Parameters
	groups    []*Group
	endpoints []*Endpoint
}

func (g *Group) all() []*Endpoint {
	out := append([]*Endpoint{}, g.endpoints...)

	for _, group := range g.groups {
		for _, endpoint := range group.all() {
			endpoint.Path = group.Path + endpoint.Path
			endpoint.Params = append(endpoint.Params, group.Params...)
			out = append(out, endpoint)
		}
	}

	return out
}

type Endpoint struct {
	Path        string
	Params      openapi3.Parameters
	Method      string
	Description string
}
