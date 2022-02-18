package reveal

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path"
	"regexp"
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
			description = fmt.Sprintf("Source: [%s](%s)", url, url)
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

	// Walk the ASTs to discover endpoints

	ev := NewEndpointsVisitor(pkgs)
	ev.Walk()

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

	for _, e := range ev.Endpoints() {
		item, ok := doc.Paths[e.Path]
		if !ok {
			item = &openapi3.PathItem{}
			doc.Paths[e.Path] = item
		}

		operation := &openapi3.Operation{
			Description: e.Description,
			Parameters:  e.Params,
			RequestBody: e.RequestBody,
			Responses:   e.Responses,
		}

		switch e.Method {
		case http.MethodConnect:
			item.Connect = operation
		case http.MethodDelete:
			item.Delete = operation
		case http.MethodGet:
			item.Get = operation
		case http.MethodHead:
			item.Head = operation
		case http.MethodOptions:
			item.Options = operation
		case http.MethodPatch:
			item.Patch = operation
		case http.MethodPost:
			item.Post = operation
		case http.MethodPut:
			item.Put = operation
		case http.MethodTrace:
			item.Trace = operation
		}
	}

	if err := doc.Validate(ctx); err != nil {
		return nil, err
	}

	return doc, nil
}
