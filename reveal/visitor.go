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
	pkg  *packages.Package
	Root *Group
}

func NewVisitor(pkg *packages.Package) *Visitor {
	return &Visitor{pkg, &Group{"/", nil}}
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
					if kind := resolveGinKind(def.Type()); kind != Engine {
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

		if obj, ok := v.pkg.TypesInfo.Uses[x]; !ok || obj != engine {
			return false
		}

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

		return false
	})
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

//func extractGroup(n ast.Node, pkg *packages.Package) (*Group, bool) {
//callexpr, ok := n.(*ast.CallExpr)
//if !ok {
//return nil, false
//}

//selector, ok := callexpr.Fun.(*ast.SelectorExpr)
//if !ok {
//return nil, false
//}

//var x *ast.Ident
//if ident, ok := selector.X.(*ast.Ident); ok {
//x = ident
//} else if selectorexpr, ok := selector.X.(*ast.SelectorExpr); ok {
//x = selectorexpr.Sel
//} else {
//return nil, false
//}

//kind := resolveGinKind(pkg.TypesInfo.Uses[x].Type())
//if kind == Unsupported {
//return nil, false
//}

//if selector.Sel.Name != "Group" {
//return nil, false
//}

//if len(callexpr.Args) < 1 {
//return nil, false
//}

//httpPath, httpPathParams, err := extractPathAndPathParams(callexpr.Args[0], pkg)
//if err != nil {
//return nil, false
//}

//return &Group{
//ASTNode:    n,
//Path:       httpPath,
//PathParams: httpPathParams,
//}, true
//}

//func extractEndpoint(n ast.Node, pkg *packages.Package, gitRoot, githubUserRepo, gitHash string) (*Endpoint, bool) {
//callexpr, ok := n.(*ast.CallExpr)
//if !ok {
//return nil, false
//}

//selector, ok := callexpr.Fun.(*ast.SelectorExpr)
//if !ok {
//return nil, false
//}

//var x *ast.Ident
//if ident, ok := selector.X.(*ast.Ident); ok {
//x = ident
//} else if selectorexpr, ok := selector.X.(*ast.SelectorExpr); ok {
//x = selectorexpr.Sel
//} else {
//return nil, false
//}

//kind := resolveGinKind(pkg.TypesInfo.Uses[x].Type())
//if kind == Unsupported {
//return nil, false
//}

//// get first arg (the http path) / last arg (the handler)
//if len(callexpr.Args) < 2 {
//return nil, false
//}
//handlerArg := callexpr.Args[len(callexpr.Args)-1]
//handlerArgStartPos := pkg.Fset.Position(handlerArg.Pos())
//handlerArgEndPos := pkg.Fset.Position(handlerArg.End())

//// get the http method and define which parameter is to be the route
//var httpMethod string
//pathArg := callexpr.Args[0]
//switch selector.Sel.Name {
//case "POST":
//httpMethod = http.MethodPost
//case "GET":
//httpMethod = http.MethodGet
//case "HEAD":
//httpMethod = http.MethodHead
//case "PUT":
//httpMethod = http.MethodPut
//case "PATCH":
//httpMethod = http.MethodPatch
//case "DELETE":
//httpMethod = http.MethodDelete
//case "OPTIONS":
//httpMethod = http.MethodOptions
//case "Handle":
//httpMethod = constant.StringVal(pkg.TypesInfo.Types[callexpr.Args[0]].Value)
//pathArg = callexpr.Args[1]
//default:
//return nil, false
//}

//// extract the http path + path parameters
//httpPath, httpPathParams, err := extractPathAndPathParams(pathArg, pkg)
//if err != nil {
//return nil, false
//}

//// description
//var description string
//if len(gitRoot) > 0 && len(gitHash) > 0 && len(githubUserRepo) > 0 {
//description = fmt.Sprintf(
//"Source: [https://github.com/%s/blob/%s/%s#L%d-L%d]",
//githubUserRepo,
//gitHash,
//path.Clean("."+strings.TrimPrefix(handlerArgEndPos.Filename, gitRoot)),
//handlerArgStartPos.Line,
//handlerArgEndPos.Line,
//)
//} else {
//description = fmt.Sprintf("%s:%d", handlerArgStartPos.Filename, handlerArgStartPos.Line)
//}

//return &Endpoint{
//Group: Group{
//ASTNode:    n,
//Path:       httpPath,
//PathParams: httpPathParams,
//},
//Method:      httpMethod,
//Description: description,
//}, true
//}

//var pathAndPathParamsRegexp = regexp.MustCompilePOSIX(`\/[*:][^\/]+`)

//func extractPathAndPathParams(expr ast.Expr, pkg *packages.Package) (string, openapi3.Parameters, error) {
//params := openapi3.Parameters{}

//path := constant.StringVal(pkg.TypesInfo.Types[expr].Value)
//path = pathAndPathParamsRegexp.ReplaceAllStringFunc(path, func(match string) string {
//required := match[1] == ':'
//name := match[2:]

//params = append(params, &openapi3.ParameterRef{
//Value: &openapi3.Parameter{
//In:       openapi3.ParameterInPath,
//Name:     name,
//Required: required,
//Schema: &openapi3.SchemaRef{
//Value: &openapi3.Schema{
//Type: openapi3.TypeString,
//},
//},
//},
//})

//return "/{" + name + "}"
//})

//return "/" + strings.TrimLeft(strings.TrimRight(path, "/"), "/"), params, nil
//}

//func extractEdge(n ast.Node, pkg *packages.Package) (*ast.Ident, ast.Expr, bool) {
//if assignstmt, ok := n.(*ast.AssignStmt); ok {
//for i, lhs := range assignstmt.Lhs {
//if len(assignstmt.Rhs) > i {
//if ident, ok := lhs.(*ast.Ident); ok {
//if def := pkg.TypesInfo.Defs[ident]; def != nil {
//if kind := resolveGinKind(def.Type()); kind != Unsupported {
//return ident, assignstmt.Rhs[i], true
//}
//}

//if use := pkg.TypesInfo.Uses[ident]; use != nil {
//if kind := resolveGinKind(use.Type()); kind != Unsupported {
//return ident, assignstmt.Rhs[i], true
//}
//}
//}
//}
//}
//}

//if valuespec, ok := n.(*ast.ValueSpec); ok {
//for i, ident := range valuespec.Names {
//if len(valuespec.Values) > i {
//if def := pkg.TypesInfo.Defs[ident]; def != nil {
//if kind := resolveGinKind(def.Type()); kind != Unsupported {
//return ident, valuespec.Values[i], true
//}
//}

//if use := pkg.TypesInfo.Uses[ident]; use != nil {
//if kind := resolveGinKind(use.Type()); kind != Unsupported {
//return ident, valuespec.Values[i], true
//}
//}
//}
//}
//}

//return nil, nil, false
//}
