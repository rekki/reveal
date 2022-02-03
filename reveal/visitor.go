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

type Visitor struct {
	Root         *Group            // root group
	pkg          *packages.Package // root package
	pkgsByID     map[string]*packages.Package
	groupsByExpr map[ast.Expr]*Group
	exprsByIdent map[ast.Object]ast.Expr
}

func NewVisitor(pkgs []*packages.Package) *Visitor {
	v := &Visitor{
		Root:         &Group{},
		pkg:          nil,
		pkgsByID:     map[string]*packages.Package{},
		groupsByExpr: map[ast.Expr]*Group{},
		exprsByIdent: map[ast.Object]ast.Expr{},
	}

	for _, pkg := range pkgs {
		v.pkgsByID[pkg.ID] = pkg
		if v.pkg == nil || len(pkg.ID) < len(v.pkg.ID) {
			v.pkg = pkg
		}
	}

	return v
}

func (v *Visitor) Walk() {
	if v.pkg != nil {
		v.walk(v.pkg.Syntax[0])
	}
}

func (v *Visitor) walk(file *ast.File) {
	ast.Inspect(file, func(n ast.Node) bool {
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

		if kind := resolveGinKind(v.pkg.TypesInfo.Types[x].Type); kind != Unknown {
			var parent *Group
			if kind == Engine {
				parent = v.Root
			} else if kind == RouterGroup {
				parent = v.groupsByExpr[v.resolveExpr(x)]
			}

			if parent == nil {
				return false
			}

			switch selector.Sel.Name {
			case "Group":
				if len(callexpr.Args) > 0 {
					if arg0, ok := v.foldConstant(callexpr.Args[0]); ok {
						p, pp := inferPath(arg0)
						g := &Group{Path: p, PathParams: pp}
						v.groupsByExpr[callexpr] = g
						parent.groups = append(parent.groups, g)
					}
				}

			case "Handle":
				if len(callexpr.Args) > 1 {
					if m, ok := v.foldConstant(callexpr.Args[0]); ok {
						if arg1, ok := v.foldConstant(callexpr.Args[1]); ok {
							p, pp := inferPath(arg1)
							if len(m) > 0 && len(p) > 0 {
								parent.endpoints = append(parent.endpoints, &Endpoint{Method: m, Path: p, PathParams: pp})
							}
						}
					}
				}

			case "POST", "GET", "HEAD", "PUT", "PATCH", "DELETE", "OPTIONS":
				if len(callexpr.Args) > 0 {
					m := selector.Sel.Name
					p, pp := inferPath(constant.StringVal(v.pkg.TypesInfo.Types[callexpr.Args[0]].Value))
					if len(m) > 0 && len(p) > 0 {
						parent.endpoints = append(parent.endpoints, &Endpoint{Method: m, Path: p, PathParams: pp})
					}
				}
			}

			return false
		}

		for _, arg := range callexpr.Args {
			if resolveGinKind(v.pkg.TypesInfo.Types[arg].Type) != Unknown {
				v.followCallExpr(callexpr)
				return false
			}
		}

		return false
	})
}

func (v *Visitor) foldConstant(expr ast.Expr) (string, bool) {
	ty, ok := v.pkg.TypesInfo.Types[expr]
	if !ok {
		return "", false
	}

	folded := constant.StringVal(ty.Value)
	if len(folded) == 0 {
		return "", false
	}

	return folded, true
}

func (v *Visitor) followCallExpr(callexpr *ast.CallExpr) {
	if selectorexpr, ok := callexpr.Fun.(*ast.SelectorExpr); ok {
		if ident, ok := selectorexpr.X.(*ast.Ident); ok {
			if pkgName, ok := v.pkg.TypesInfo.Uses[ident].(*types.PkgName); ok && pkgName != nil {
				pkg := pkgName.Imported()

				obj := pkg.Scope().Lookup(selectorexpr.Sel.Name)
				if _, ok := obj.(*types.Func); ok {
					// TODO: find function body + collect endpoints from there
				}
			}
		}
	}
}

func (v *Visitor) resolveExpr(x *ast.Ident) ast.Expr {
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
