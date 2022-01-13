package reveal

// TODO: support query parameters
// TODO: support json input
// TODO: support json output

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"net/http"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/mitchellh/go-homedir"
	"golang.org/x/tools/go/packages"
)

var githubRegexp = regexp.MustCompilePOSIX(`^git@github\.com:([^/]+/[^.]+)\.git$`)

func Reveal(ctx context.Context, dir string) (*openapi3.T, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	dir, err = homedir.Expand(dir)
	if err != nil {
		return nil, err
	}

	if !strings.HasPrefix(dir, "/") {
		dir = path.Clean(path.Join(wd, dir))
	}

	var gitRoot, githubUserRepo, gitHash string

	if repo, err := git.PlainOpenWithOptions(dir, &git.PlainOpenOptions{
		DetectDotGit: true,
	}); err == nil {
		if remote, err := repo.Remote("origin"); err == nil {
			if urls := remote.Config().URLs; len(urls) > 0 {
				if matches := githubRegexp.FindStringSubmatch(urls[0]); len(matches) == 2 {
					githubUserRepo = matches[1]
				}
			}
		}

		if storage, ok := repo.Storer.(*filesystem.Storage); ok {
			gitRoot = path.Dir(storage.Filesystem().Root())
		}

		if head, err := repo.Head(); err == nil {
			gitHash = head.Hash().String()
		}
	}

	pkgs, err := packages.Load(&packages.Config{
		Context: ctx,
		Dir:     dir,
		Mode:    packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports | packages.NeedDeps | packages.NeedExportsFile | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedTypesSizes | packages.NeedModule,
	}, ".")
	if err != nil {
		return nil, err
	}

	title := dir
	var description string

	if len(gitRoot) > 0 {
		title = path.Clean("." + strings.TrimPrefix(title, gitRoot))
		if len(githubUserRepo) > 0 {
			url := "https://github.com/" + githubUserRepo
			if len(gitHash) > 0 {
				url += "/tree/" + gitHash + "/" + title
			}
			description = fmt.Sprintf("Source: [%s]", url)
		}
	}

	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info: &openapi3.Info{
			Title:       title,
			Description: description,
			Version:     gitHash,
		},
		Paths: openapi3.Paths{},
	}

	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			ast.Inspect(file, func(n ast.Node) bool {
				if callexpr, ok := n.(*ast.CallExpr); ok {
					if method, httpPath, ok := intoMethodPath(callexpr, pkg); ok {
						httpPath, pathParams := expandPath(httpPath)
						operation := intoOperation(callexpr, pathParams, pkg.Fset, func(filename string, start, end int) string {
							if len(gitRoot) > 0 && len(gitHash) > 0 && len(githubUserRepo) > 0 {
								filename = path.Clean("." + strings.TrimPrefix(filename, gitRoot))
								return fmt.Sprintf("https://github.com/%s/blob/%s/%s#L%d-L%d", githubUserRepo, gitHash, filename, start, end)
							}
							return fmt.Sprintf("%s:%d", filename, start)
						})

						item, ok := doc.Paths[httpPath]
						if !ok {
							item = &openapi3.PathItem{}
							doc.Paths[httpPath] = item
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

					return false // don't go any deeper
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

func intoOperation(callexpr *ast.CallExpr, pathParameters openapi3.Parameters, fset *token.FileSet, formatDesc func(string, int, int) string) *openapi3.Operation {
	lastArg := callexpr.Args[len(callexpr.Args)-1]
	start := fset.Position(lastArg.Pos())
	end := fset.Position(lastArg.End())

	return &openapi3.Operation{
		Description: formatDesc(start.Filename, start.Line, end.Line),
		Parameters:  append(openapi3.Parameters{
			// TODO
		}, pathParameters...),
	}
}

var expandPathRegexp = regexp.MustCompilePOSIX(`\/[*:][^\/]+`)

func expandPath(path string) (string, openapi3.Parameters) {
	params := openapi3.Parameters{}

	path = expandPathRegexp.ReplaceAllStringFunc(path, func(match string) string {
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
