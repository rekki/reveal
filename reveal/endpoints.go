package reveal

import (
	"go/ast"
	"go/constant"
	"go/types"
	"regexp"
	"strconv"
	"strings"

	"github.com/fatih/structtag"
	"github.com/getkin/kin-openapi/openapi3"
	"golang.org/x/tools/go/packages"
)

type EndpointsVisitor struct {
	root         *Group            // root group
	schemas      *SchemaRegistry   // hoisted schemas
	entrypoint   *packages.Package // root package
	pkgsByID     map[string]*packages.Package
	groupsByExpr map[ast.Expr]*Group
	exprsByIdent map[ast.Object]ast.Expr
}

func NewEndpointsVisitor(pkgs []*packages.Package) *EndpointsVisitor {
	v := &EndpointsVisitor{
		root:         &Group{},
		schemas:      NewSchemaRegistry(),
		entrypoint:   nil,
		pkgsByID:     map[string]*packages.Package{},
		groupsByExpr: map[ast.Expr]*Group{},
		exprsByIdent: map[ast.Object]ast.Expr{},
	}

	// entrypoint is always the last package
	if l := len(pkgs); l > 0 {
		v.entrypoint = pkgs[l-1]
	}

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
					if ident, ok := lhs.(*ast.Ident); ok && ident.Obj != nil {
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
						if arg0, ok := v.foldStringConstant(callexpr.Args[0], pkg); ok {
							path, pathParams := inferPath(arg0)
							g := &Group{Path: path, Params: pathParams}
							v.groupsByExpr[callexpr] = g
							parent.groups = append(parent.groups, g)
						}
					}

				case "Handle":
					if len(callexpr.Args) > 2 {
						if m, ok := v.foldStringConstant(callexpr.Args[0], pkg); ok {
							if arg1, ok := v.foldStringConstant(callexpr.Args[1], pkg); ok {
								path, pathParams := inferPath(arg1)
								reqBody, handlerParams, res := v.inferHandler(callexpr.Args[len(callexpr.Args)-1], pkg)

								endpoint := &Endpoint{
									Method:      m,
									Path:        path,
									RequestBody: reqBody,
									Responses:   res,
								}
								endpoint.Params = append(endpoint.Params, pathParams...)
								endpoint.Params = append(endpoint.Params, handlerParams...)

								parent.endpoints = append(parent.endpoints, endpoint)
							}
						}
					}

				case "POST", "GET", "HEAD", "PUT", "PATCH", "DELETE", "OPTIONS":
					if len(callexpr.Args) >= 2 {
						m := selector.Sel.Name
						if arg0, ok := v.foldStringConstant(callexpr.Args[0], pkg); ok {
							path, pathParams := inferPath(arg0)
							reqBody, handlerParams, res := v.inferHandler(callexpr.Args[len(callexpr.Args)-1], pkg)

							endpoint := &Endpoint{
								Method:      m,
								Path:        path,
								RequestBody: reqBody,
								Responses:   res,
							}
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

func (v *EndpointsVisitor) foldIntConstant(expr ast.Expr, pkg *packages.Package) (int, bool) {
	if expr == nil {
		return 0, false
	}

	ty, ok := pkg.TypesInfo.Types[expr]
	if !ok {
		return 0, false
	}

	if ty.Value == nil {
		return 0, false
	}

	folded, ok := constant.Int64Val(ty.Value)
	if !ok {
		return 0, false
	}

	return int(folded), true
}

func (v *EndpointsVisitor) foldStringConstant(expr ast.Expr, pkg *packages.Package) (string, bool) {
	if expr == nil {
		return "", false
	}

	ty, ok := pkg.TypesInfo.Types[expr]
	if !ok {
		return "", false
	}

	if ty.Value == nil {
		return "", false
	}

	if basic, ok := ty.Type.(*types.Basic); !ok || basic.Kind() != types.String {
		return "", false
	}

	folded := constant.StringVal(ty.Value)
	if len(folded) == 0 {
		return "", false
	}

	return folded, true
}

func (v *EndpointsVisitor) inferHandler(expr ast.Expr, pkg *packages.Package) (*openapi3.RequestBodyRef, openapi3.Parameters, openapi3.Responses) {
	var requestBody *openapi3.RequestBodyRef
	var params openapi3.Parameters
	responses := openapi3.Responses{}

	if lit, ok := expr.(*ast.FuncLit); ok {
		ast.Inspect(lit, func(n ast.Node) bool {
			if callexpr, ok := n.(*ast.CallExpr); ok {
				if selectorexpr, ok := callexpr.Fun.(*ast.SelectorExpr); ok {
					if isGinContext(pkg.TypesInfo.Types[selectorexpr.X].Type) {
						switch selectorexpr.Sel.Name {
						case "Query":
							if len(callexpr.Args) > 0 {
								if name, ok := v.foldStringConstant(callexpr.Args[0], pkg); ok {
									params = append(params, &openapi3.ParameterRef{
										Value: &openapi3.Parameter{
											In:   openapi3.ParameterInQuery,
											Name: name,
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
								if name, ok := v.foldStringConstant(callexpr.Args[0], pkg); ok {
									if defaultValue, ok := v.foldStringConstant(callexpr.Args[1], pkg); ok {
										params = append(params, &openapi3.ParameterRef{
											Value: &openapi3.Parameter{
												In:   openapi3.ParameterInQuery,
												Name: name,
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

						case "ShouldBindQuery", "BindQuery":
							if len(callexpr.Args) > 0 {
								arg0 := pkg.TypesInfo.Types[callexpr.Args[0]].Type
								p := paramsFromStructFields(arg0, "form", openapi3.ParameterInQuery)
								params = append(params, p...)
							}

						case "GetHeader":
							if len(callexpr.Args) > 0 {
								if name, ok := v.foldStringConstant(callexpr.Args[0], pkg); ok {
									params = append(params, &openapi3.ParameterRef{
										Value: &openapi3.Parameter{
											In:   openapi3.ParameterInHeader,
											Name: name,
											Schema: &openapi3.SchemaRef{
												Value: &openapi3.Schema{
													Type: openapi3.TypeString,
												},
											},
										},
									})
								}
							}

						case "ShouldBindHeader", "BindHeader":
							if len(callexpr.Args) > 0 {
								arg0 := pkg.TypesInfo.Types[callexpr.Args[0]].Type
								p := paramsFromStructFields(arg0, "header", openapi3.ParameterInHeader)
								params = append(params, p...)
							}

						case "ShouldBindJSON", "BindJSON":
							if len(callexpr.Args) > 0 {
								arg0 := pkg.TypesInfo.Types[callexpr.Args[0]].Type
								requestSchema := v.schemas.ToSchemaRef(arg0, "json")
								if requestBody == nil {
									requestBody = &openapi3.RequestBodyRef{
										Value: &openapi3.RequestBody{
											Content: map[string]*openapi3.MediaType{
												"application/json": {
													Schema: &openapi3.SchemaRef{
														Value: &openapi3.Schema{
															OneOf: []*openapi3.SchemaRef{requestSchema},
														},
													},
												},
											},
										},
									}
								} else {
									v := requestBody.Value.Content.Get("application/json").Schema.Value
									v.OneOf = append(v.OneOf, requestSchema)
								}
							}

						case "AbortWithError", "AbortWithStatus":
							if len(callexpr.Args) > 0 {
								if status, ok := v.foldIntConstant(callexpr.Args[0], pkg); ok {
									d := "description"
									responses[strconv.Itoa(status)] = &openapi3.ResponseRef{
										Value: &openapi3.Response{
											Description: &d,
										},
									}
								}
							}

						case "AbortWithStatusJSON", "AsciiJSON", "IndentedJSON", "JSON", "PureJSON", "SecureJSON":
							if len(callexpr.Args) > 1 {
								if status, ok := v.foldIntConstant(callexpr.Args[0], pkg); ok {
									arg1 := pkg.TypesInfo.Types[callexpr.Args[1]].Type
									d := "description"
									responses[strconv.Itoa(status)] = &openapi3.ResponseRef{
										Value: &openapi3.Response{
											Description: &d,
											Content: openapi3.Content{
												"application/json": &openapi3.MediaType{
													Schema: v.schemas.ToSchemaRef(arg1, "json"),
												},
											},
										},
									}
								}
							}

						case "Data":
							if len(callexpr.Args) > 1 {
								if status, ok := v.foldIntConstant(callexpr.Args[0], pkg); ok {
									if contentType, ok := v.foldStringConstant(callexpr.Args[1], pkg); ok {
										d := "description"
										responses[strconv.Itoa(status)] = &openapi3.ResponseRef{
											Value: &openapi3.Response{
												Description: &d,
												Content: openapi3.Content{
													contentType: &openapi3.MediaType{},
												},
											},
										}
									}
								}
							}

						case "DataFromReader":
							if len(callexpr.Args) > 2 {
								if status, ok := v.foldIntConstant(callexpr.Args[0], pkg); ok {
									if contentType, ok := v.foldStringConstant(callexpr.Args[2], pkg); ok {
										d := "description"
										responses[strconv.Itoa(status)] = &openapi3.ResponseRef{
											Value: &openapi3.Response{
												Description: &d,
												Content: openapi3.Content{
													contentType: &openapi3.MediaType{},
												},
											},
										}
									}
								}
							}

						case "HTML", "Render":
							if len(callexpr.Args) > 0 {
								if status, ok := v.foldIntConstant(callexpr.Args[0], pkg); ok {
									d := "description"
									responses[strconv.Itoa(status)] = &openapi3.ResponseRef{
										Value: &openapi3.Response{
											Description: &d,
											Content: openapi3.Content{
												"text/html": &openapi3.MediaType{},
											},
										},
									}
								}
							}

						case "JSONP":
							if len(callexpr.Args) > 1 {
								if status, ok := v.foldIntConstant(callexpr.Args[0], pkg); ok {
									arg1 := pkg.TypesInfo.Types[callexpr.Args[1]].Type
									d := "description"
									responses[strconv.Itoa(status)] = &openapi3.ResponseRef{
										Value: &openapi3.Response{
											Description: &d,
											Content: openapi3.Content{
												"application/javascript": &openapi3.MediaType{
													Schema: v.schemas.ToSchemaRef(arg1, "json"),
												},
											},
										},
									}
								}
							}

						case "Redirect", "Status", "String":
							if len(callexpr.Args) > 0 {
								if status, ok := v.foldIntConstant(callexpr.Args[0], pkg); ok {
									d := "description"
									responses[strconv.Itoa(status)] = &openapi3.ResponseRef{
										Value: &openapi3.Response{
											Description: &d,
										},
									}
								}
							}

						case "XML":
							if len(callexpr.Args) > 1 {
								if status, ok := v.foldIntConstant(callexpr.Args[0], pkg); ok {
									arg1 := pkg.TypesInfo.Types[callexpr.Args[1]].Type
									d := "description"
									responses[strconv.Itoa(status)] = &openapi3.ResponseRef{
										Value: &openapi3.Response{
											Description: &d,
											Content: openapi3.Content{
												"text/xml": &openapi3.MediaType{
													Schema: v.schemas.ToSchemaRef(arg1, "xml"),
												},
											},
										},
									}
								}
							}

						case "YAML":
							if len(callexpr.Args) > 1 {
								if status, ok := v.foldIntConstant(callexpr.Args[0], pkg); ok {
									arg1 := pkg.TypesInfo.Types[callexpr.Args[1]].Type
									d := "description"
									responses[strconv.Itoa(status)] = &openapi3.ResponseRef{
										Value: &openapi3.Response{
											Description: &d,
											Content: openapi3.Content{
												"text/yaml": &openapi3.MediaType{
													Schema: v.schemas.ToSchemaRef(arg1, "yaml"),
												},
											},
										},
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

	// for each content, flatten if there is only one possible type
	if requestBody != nil && requestBody.Value != nil && requestBody.Value.Content != nil {
		for _, content := range requestBody.Value.Content {
			if content != nil && content.Schema != nil && content.Schema.Value != nil && len(content.Schema.Value.OneOf) == 1 {
				content.Schema = content.Schema.Value.OneOf[0]
			}
		}
	}

	if len(responses) == 0 {
		responses = openapi3.NewResponses()
	}

	return requestBody, params, responses
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

func paramsFromStructFields(ty types.Type, tag string, in string) openapi3.Parameters {
	var out openapi3.Parameters

	if ty == nil {
		return out
	}

	for {
		if ptr, ok := ty.(*types.Pointer); ok {
			ty = ptr.Elem()
		} else if named, ok := ty.(*types.Named); ok {
			ty = named.Underlying()
		} else {
			break
		}
	}

	if strct, ok := ty.(*types.Struct); ok {
		for i, max := 0, strct.NumFields(); i < max; i++ {
			if f := strct.Field(i); f.Exported() {
				tags, err := structtag.Parse(strct.Tag(i))
				if err != nil {
					continue
				}

				for _, key := range tags.Keys() {
					if key == tag {
						if value, err := tags.Get(tag); err == nil {
							out = append(out, &openapi3.ParameterRef{
								Value: &openapi3.Parameter{
									In:       in,
									Name:     value.Name,
									Required: false,
									Schema: &openapi3.SchemaRef{
										Value: &openapi3.Schema{
											Type: openapi3.TypeString,
										},
									},
								},
							})
						}
						continue
					}
				}
			}
		}
	}

	return out
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
			endpoint.Path = "/" + strings.TrimLeft(group.Path+endpoint.Path, "/")
			endpoint.Params = append(endpoint.Params, group.Params...)
			out = append(out, endpoint)
		}
	}

	return out
}

type Endpoint struct {
	Path        string
	Params      openapi3.Parameters
	RequestBody *openapi3.RequestBodyRef
	Responses   openapi3.Responses
	Method      string
	Description string
}
