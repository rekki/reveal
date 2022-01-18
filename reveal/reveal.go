package reveal

// TODO: support router.Group
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

	var graph Graph

	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			ast.Inspect(file, func(n ast.Node) bool {
				if node, ok := extractRoute(n, pkg, gitRoot, githubUserRepo, gitHash); ok {
					graph.Nodes = append(graph.Nodes, node)
					return false
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

	for _, node := range graph.Nodes {
		rootedPath := node.RootedPath()

		item, ok := doc.Paths[rootedPath]
		if !ok {
			item = &openapi3.PathItem{}
			doc.Paths[rootedPath] = item
		}

		switch node.Method {
		case http.MethodPost:
			item.Post = node.Operation
		case http.MethodGet:
			item.Get = node.Operation
		case http.MethodHead:
			item.Head = node.Operation
		case http.MethodPut:
			item.Put = node.Operation
		case http.MethodPatch:
			item.Patch = node.Operation
		case http.MethodDelete:
			item.Delete = node.Operation
		case http.MethodOptions:
			item.Options = node.Operation
		default:
			return nil, fmt.Errorf("unsupported http method: %v", node.Method)
		}
	}

	return doc, nil
}

func extractRoute(n ast.Node, pkg *packages.Package, gitRoot, githubUserRepo, gitHash string) (*Node, bool) {
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

	return &Node{
		Method: httpMethod,
		Path:   httpPath,
		Operation: &openapi3.Operation{
			Description: description,
			Parameters:  append(openapi3.Parameters{
				// TODO
			}, httpPathParams...),
		},
	}, true
}

var pathParamsRegexp = regexp.MustCompilePOSIX(`\/[*:][^\/]+`)

func extractPathAndPathParams(lit *ast.BasicLit) (string, openapi3.Parameters, error) {
	unquoted, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", nil, err
	}

	params := openapi3.Parameters{}

	out := pathParamsRegexp.ReplaceAllStringFunc(unquoted, func(match string) string {
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

	return out, params, nil
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
