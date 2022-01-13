package reveal

// TODO: follow embedded types
// TODO: support path parameters
// TODO: infer input types
// TODO: infer input parameters
// TODO: infer output types
// TODO: add link to source code

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"net/http"
	"strconv"

	"github.com/getkin/kin-openapi/openapi3"
	"golang.org/x/tools/go/packages"
)

func Reveal(ctx context.Context, rootPkg string) (*openapi3.T, error) {
	cfg := &packages.Config{
		Context: ctx,
		Mode:    packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports | packages.NeedDeps | packages.NeedExportsFile | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedTypesSizes | packages.NeedModule,
	}
	pkgs, err := packages.Load(cfg, "pattern=./"+rootPkg)
	if err != nil {
		return nil, err
	}

	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info: &openapi3.Info{
			Title:   "service-xxx",
			Version: "git hash",
		},
		Servers: openapi3.Servers{
			&openapi3.Server{URL: "https://rekki.com"},
		},
		Paths: openapi3.Paths{},
	}

	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			ast.Inspect(file, func(n ast.Node) bool {
				if callexpr, ok := n.(*ast.CallExpr); ok {
					if method, path, ok := intoMethodPath(callexpr, pkg); ok {
						operation := intoOperation(callexpr)

						item, ok := doc.Paths[path]
						if !ok {
							item = &openapi3.PathItem{}
							doc.Paths[path] = item
						}

						switch method {
						case http.MethodPost:
							item.Post = operation
						case http.MethodGet:
							item.Get = operation
						case http.MethodHead:
							item.Head = operation
						case http.MethodPut:
							item.Put = operation
						case http.MethodPatch:
							item.Patch = operation
						case http.MethodDelete:
							item.Delete = operation
						case http.MethodOptions:
							item.Options = operation
						}
					}
				}

				return true
			})
		}
	}

	return doc, nil
}

func intoMethodPath(callexpr *ast.CallExpr, pkg *packages.Package) (string, string, bool) {
	if selector, ok := callexpr.Fun.(*ast.SelectorExpr); ok {
		if x, ok := selector.X.(*ast.Ident); ok {
			if info, ok := pkg.TypesInfo.Uses[x]; ok && info.Type().String() == "*github.com/gin-gonic/gin.Engine" {
				if len(callexpr.Args) > 1 {
					if path, ok := callexpr.Args[0].(*ast.BasicLit); ok && path.Kind == token.STRING {
						if p, err := strconv.Unquote(path.Value); err != nil {
							switch selector.Sel.Name {
							case "POST":
								return http.MethodPost, p, true
							case "GET":
								return http.MethodGet, p, true
							case "HEAD":
								return http.MethodHead, p, true
							case "PUT":
								return http.MethodPut, p, true
							case "PATCH":
								return http.MethodPatch, p, true
							case "DELETE":
								return http.MethodDelete, p, true
							case "OPTIONS":
								return http.MethodOptions, p, true
							}
						}
					}
				}
			}
		}
	}

	return "", "", false
}

func intoOperation(callexpr *ast.CallExpr) *openapi3.Operation {
	return &openapi3.Operation{
		Description: fmt.Sprintf("%#v", callexpr.Fun.Pos()),
	}
}
