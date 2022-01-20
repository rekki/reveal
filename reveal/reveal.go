package reveal

// TODO: support router.Handle
// TODO: support in/out headers
// TODO: support in/out json body
// TODO: support in query parameters

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
	// Resolve the root path to the directory

	dir, err := homedir.Expand(dir)
	if err != nil {
		return nil, err
	}

	if !path.IsAbs(dir) {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		dir = path.Join(wd, dir)
	}

	dir = path.Clean(dir)

	// Try to acquire git infos

	var gitRoot, githubUserRepo, gitHash string

	if repo, err := git.PlainOpenWithOptions(dir, &git.PlainOpenOptions{
		DetectDotGit: true,
	}); err == nil {
		if storage, ok := repo.Storer.(*filesystem.Storage); ok {
			gitRoot = path.Dir(storage.Filesystem().Root())
		}

		if remote, err := repo.Remote("origin"); err == nil {
			if urls := remote.Config().URLs; len(urls) > 0 {
				if matches := githubRegexp.FindStringSubmatch(urls[0]); len(matches) == 2 {
					githubUserRepo = matches[1]
				}
			}
		}

		if head, err := repo.Head(); err == nil {
			gitHash = head.Hash().String()
		}
	}

	// Prepare the title/description

	var title = dir
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

	// Parse code and resolve types

	pkgs, err := packages.Load(&packages.Config{
		Context: ctx,
		Dir:     dir,
		Mode:    packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports | packages.NeedDeps | packages.NeedExportsFile | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedTypesSizes | packages.NeedModule,
	}, "./...")
	if err != nil {
		return nil, err
	}

	// Browse the ASTs

	graph := NewGraph()

	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			ast.Inspect(file, func(n ast.Node) bool {
				if group, ok := extractGroup(n, pkg); ok {
					graph.Groups[n] = group
				} else if endpoint, ok := extractEndpoint(n, pkg, gitRoot, githubUserRepo, gitHash); ok {
					graph.Endpoints[n] = endpoint
				} else if ident, expr, ok := extractEdge(n, pkg); ok {
					graph.Idents[ident.Name] = expr
				}

				return true
			})
		}
	}

	// Build the OpenAPI schema

	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info: &openapi3.Info{
			Title:       title,
			Description: description,
			Version:     gitHash,
		},
		Paths: openapi3.Paths{},
	}

	for _, endpoint := range graph.Endpoints {
		rootedPath, rootedParams := graph.RootedPathAndParams(endpoint)
		item, ok := doc.Paths[rootedPath]
		if !ok {
			item = &openapi3.PathItem{}
			doc.Paths[rootedPath] = item
		}

		operation := &openapi3.Operation{
			Parameters:  rootedParams,
			Description: endpoint.Description,
		}

		switch endpoint.Method {
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

	return doc, nil
}

func extractGroup(n ast.Node, pkg *packages.Package) (*Group, bool) {
	callexpr, ok := n.(*ast.CallExpr)
	if !ok {
		return nil, false
	}

	selector, ok := callexpr.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil, false
	}

	var x *ast.Ident
	if ident, ok := selector.X.(*ast.Ident); ok {
		x = ident
	} else if selectorexpr, ok := selector.X.(*ast.SelectorExpr); ok {
		x = selectorexpr.Sel
	} else {
		return nil, false
	}

	kind := resolveGinKind(pkg.TypesInfo.Uses[x].Type())
	if kind == Unsupported {
		return nil, false
	}

	if selector.Sel.Name != "Group" {
		return nil, false
	}

	if len(callexpr.Args) < 1 {
		return nil, false
	}

	httpPath, httpPathParams, err := extractPathAndPathParams(callexpr.Args[0].(*ast.BasicLit))
	if err != nil {
		return nil, false
	}

	return &Group{
		ASTNode:    n,
		Path:       httpPath,
		PathParams: httpPathParams,
	}, true
}

func extractEndpoint(n ast.Node, pkg *packages.Package, gitRoot, githubUserRepo, gitHash string) (*Endpoint, bool) {
	callexpr, ok := n.(*ast.CallExpr)
	if !ok {
		return nil, false
	}

	selector, ok := callexpr.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil, false
	}

	var x *ast.Ident
	if ident, ok := selector.X.(*ast.Ident); ok {
		x = ident
	} else if selectorexpr, ok := selector.X.(*ast.SelectorExpr); ok {
		x = selectorexpr.Sel
	} else {
		return nil, false
	}

	kind := resolveGinKind(pkg.TypesInfo.Uses[x].Type())
	if kind == Unsupported {
		return nil, false
	}

	// get first arg (the http path) / last arg (the handler)
	if len(callexpr.Args) < 2 {
		return nil, false
	}
	firstArg, ok := callexpr.Args[0].(*ast.BasicLit)
	if !(ok && firstArg.Kind == token.STRING) {
		return nil, false
	}
	lastArg := callexpr.Args[len(callexpr.Args)-1]
	lastArgStartPos := pkg.Fset.Position(lastArg.Pos())
	lastArgEndPos := pkg.Fset.Position(lastArg.End())

	// get the http method
	var httpMethod string
	switch selector.Sel.Name {
	case "POST":
		httpMethod = http.MethodPost
	case "GET":
		httpMethod = http.MethodGet
	case "HEAD":
		httpMethod = http.MethodHead
	case "PUT":
		httpMethod = http.MethodPut
	case "PATCH":
		httpMethod = http.MethodPatch
	case "DELETE":
		httpMethod = http.MethodDelete
	case "OPTIONS":
		httpMethod = http.MethodOptions
	default:
		return nil, false
	}

	// get the http path + path parameters
	httpPath, httpPathParams, err := extractPathAndPathParams(firstArg)
	if err != nil {
		return nil, false
	}

	// description
	var description string
	if len(gitRoot) > 0 && len(gitHash) > 0 && len(githubUserRepo) > 0 {
		description = fmt.Sprintf(
			"Source: [https://github.com/%s/blob/%s/%s#L%d-L%d]",
			githubUserRepo,
			gitHash,
			path.Clean("."+strings.TrimPrefix(lastArgEndPos.Filename, gitRoot)),
			lastArgStartPos.Line,
			lastArgEndPos.Line,
		)
	} else {
		description = fmt.Sprintf("%s:%d", lastArgStartPos.Filename, lastArgStartPos.Line)
	}

	return &Endpoint{
		Group: Group{
			ASTNode:    n,
			Path:       httpPath,
			PathParams: httpPathParams,
		},
		Method:      httpMethod,
		Description: description,
	}, true
}

var pathAndPathParamsRegexp = regexp.MustCompilePOSIX(`\/[*:][^\/]+`)

func extractPathAndPathParams(lit *ast.BasicLit) (string, openapi3.Parameters, error) {
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", nil, err
	}

	params := openapi3.Parameters{}

	path := pathAndPathParamsRegexp.ReplaceAllStringFunc(value, func(match string) string {
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

	return "/" + strings.TrimLeft(strings.TrimRight(path, "/"), "/"), params, nil
}

func extractEdge(n ast.Node, pkg *packages.Package) (*ast.Ident, ast.Expr, bool) {
	if assignstmt, ok := n.(*ast.AssignStmt); ok {
		for i, lhs := range assignstmt.Lhs {
			if len(assignstmt.Rhs) > i {
				if ident, ok := lhs.(*ast.Ident); ok {
					if def := pkg.TypesInfo.Defs[ident]; def != nil {
						if kind := resolveGinKind(def.Type()); kind != Unsupported {
							return ident, assignstmt.Rhs[i], true
						}
					}

					if use := pkg.TypesInfo.Uses[ident]; use != nil {
						if kind := resolveGinKind(use.Type()); kind != Unsupported {
							return ident, assignstmt.Rhs[i], true
						}
					}
				}
			}
		}
	}

	if valuespec, ok := n.(*ast.ValueSpec); ok {
		for i, ident := range valuespec.Names {
			if len(valuespec.Values) > i {
				if def := pkg.TypesInfo.Defs[ident]; def != nil {
					if kind := resolveGinKind(def.Type()); kind != Unsupported {
						return ident, valuespec.Values[i], true
					}
				}

				if use := pkg.TypesInfo.Uses[ident]; use != nil {
					if kind := resolveGinKind(use.Type()); kind != Unsupported {
						return ident, valuespec.Values[i], true
					}
				}
			}
		}
	}

	return nil, nil, false
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
