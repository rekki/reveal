package reveal

// TODO: support path parameters
// TODO: support query parameters
// TODO: support json input
// TODO: support json output
// TODO: add link to source code in endpoint description

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"net/http"
	"regexp"
	"strconv"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-git/go-git/v5"
	"golang.org/x/tools/go/packages"
)

func Reveal(ctx context.Context, rootDir string) (*openapi3.T, error) {
	var version string

	if repo, err := git.PlainOpenWithOptions(rootDir, &git.PlainOpenOptions{
		DetectDotGit: true,
	}); err == nil {
		if head, err := repo.Head(); err == nil {
			version = head.Hash().String()
		}
	}

	cfg := &packages.Config{
		Context: ctx,
		Dir:     rootDir,
		Mode:    packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports | packages.NeedDeps | packages.NeedExportsFile | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedTypesSizes | packages.NeedModule,
	}

	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		return nil, err
	}

	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info: &openapi3.Info{
			Title:   rootDir,
			Version: version,
		},
		Paths: openapi3.Paths{},
	}

	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			ast.Inspect(file, func(n ast.Node) bool {
				if callexpr, ok := n.(*ast.CallExpr); ok {
					if method, path, ok := intoMethodPath(callexpr, pkg); ok {
						path, pathParams := expandPath(path)
						operation := intoOperation(callexpr, pathParams)

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

		var x *ast.Ident
		if ident, ok := selector.X.(*ast.Ident); ok {
			x = ident
		} else if selectorexpr, ok := selector.X.(*ast.SelectorExpr); ok {
			x = selectorexpr.Sel
		} else {
			return "", "", false
		}

		if isGinEngine(pkg.TypesInfo.Uses[x].Type()) {
			if len(callexpr.Args) > 1 {
				if path, ok := callexpr.Args[0].(*ast.BasicLit); ok && path.Kind == token.STRING {
					if p, err := strconv.Unquote(path.Value); err == nil {
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

	return "", "", false
}

func intoOperation(callexpr *ast.CallExpr, pathParameters openapi3.Parameters) *openapi3.Operation {
	return &openapi3.Operation{
		Description: fmt.Sprintf("%#v", callexpr.Fun.Pos()),
		Parameters:  append(openapi3.Parameters{
			// TODO
		}, pathParameters...),
	}
}

var expandPathRegexp = regexp.MustCompilePOSIX(`\/:[^\/]+`)

func expandPath(path string) (string, openapi3.Parameters) {
	params := openapi3.Parameters{}

	path = expandPathRegexp.ReplaceAllStringFunc(path, func(match string) string {
		name := match[2:]

		params = append(params, &openapi3.ParameterRef{
			Value: &openapi3.Parameter{
				In:       openapi3.ParameterInPath,
				Name:     name,
				Required: true,
				Schema: &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: openapi3.TypeString,
					},
				},
			},
		})

		return "/{" + name + "}"
	})

	return path, params
}

// isGinEngine checks whether a type is a gin engine, or embeds a gin engine.
func isGinEngine(ty types.Type) bool {
	for {
		if ty.String() == "github.com/gin-gonic/gin.Engine" {
			return true
		} else if ty.String() == "github.com/gin-gonic/gin.RouterGroup" {
			// TODO: this is not correct, we should walk up to compute the correct absolute path
			return true
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
			if f := strct.Field(i); f.Embedded() && isGinEngine(f.Type()) {
				return true
			}
		}
	}

	return false
}
