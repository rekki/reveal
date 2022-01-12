package reveal

import (
	"context"
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

	out := &openapi3.T{
		Paths: openapi3.Paths{},
	}

	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			ast.Inspect(file, func(n ast.Node) bool {
				if method, path, ok := extractMethodPath(n, pkg); ok {
					if _, ok := out.Paths[path]; !ok {
						out.Paths[path] = &openapi3.PathItem{}
					}

					op := &openapi3.Operation{}

					switch method {
					case http.MethodPost:
						out.Paths[path].Post = op
					case http.MethodGet:
						out.Paths[path].Get = op
					case http.MethodHead:
						out.Paths[path].Head = op
					case http.MethodPut:
						out.Paths[path].Put = op
					case http.MethodPatch:
						out.Paths[path].Patch = op
					case http.MethodDelete:
						out.Paths[path].Delete = op
					case http.MethodOptions:
						out.Paths[path].Options = op
					}
				}

				return true
			})
		}
	}

	return out, nil
}

func extractMethodPath(n ast.Node, pkg *packages.Package) (string, string, bool) {
	// Find call expressions
	if call, ok := n.(*ast.CallExpr); ok {
		if selector, ok := call.Fun.(*ast.SelectorExpr); ok {
			// Only keep calls on a gin Engine
			if x, ok := selector.X.(*ast.Ident); ok {
				if info, ok := pkg.TypesInfo.Uses[x]; ok && info.Type().String() == "*github.com/gin-gonic/gin.Engine" {
					// Only keep calls to HTTP handlers methods
					switch selector.Sel.Name {

					case "POST":
						if len(call.Args) > 1 {
							if path, ok := call.Args[0].(*ast.BasicLit); ok && path.Kind == token.STRING {
								p, _ := strconv.Unquote(path.Value)
								return http.MethodPost, p, true
							}
						}

					case "GET":
						if len(call.Args) > 1 {
							if path, ok := call.Args[0].(*ast.BasicLit); ok && path.Kind == token.STRING {
								p, _ := strconv.Unquote(path.Value)
								return http.MethodGet, p, true
							}
						}

					case "HEAD":
						if len(call.Args) > 1 {
							if path, ok := call.Args[0].(*ast.BasicLit); ok && path.Kind == token.STRING {
								p, _ := strconv.Unquote(path.Value)
								return http.MethodHead, p, true
							}
						}

					case "PUT":
						if len(call.Args) > 1 {
							if path, ok := call.Args[0].(*ast.BasicLit); ok && path.Kind == token.STRING {
								p, _ := strconv.Unquote(path.Value)
								return http.MethodPut, p, true
							}
						}

					case "PATCH":
						if len(call.Args) > 1 {
							if path, ok := call.Args[0].(*ast.BasicLit); ok && path.Kind == token.STRING {
								p, _ := strconv.Unquote(path.Value)
								return http.MethodPatch, p, true
							}
						}

					case "DELETE":
						if len(call.Args) > 1 {
							if path, ok := call.Args[0].(*ast.BasicLit); ok && path.Kind == token.STRING {
								p, _ := strconv.Unquote(path.Value)
								return http.MethodDelete, p, true
							}
						}

					case "OPTIONS":
						if len(call.Args) > 1 {
							if path, ok := call.Args[0].(*ast.BasicLit); ok && path.Kind == token.STRING {
								p, _ := strconv.Unquote(path.Value)
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
