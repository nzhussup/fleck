package golang

import (
	"context"
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/nzhussup/fleck/internal/model"
	"golang.org/x/tools/go/packages"
)

type Result struct {
	EnvReads    []model.EnvRead
	ListenPorts []model.ListenPort
}

type assignedRead struct {
	call                       *ast.CallExpr
	name, variable, okVariable string
	resolved                   bool
}
type valueExpr struct {
	literal, envName string
	defaultPort      int
}

func Extract(ctx context.Context, root, applicationPath, applicationID string) (Result, error) {
	fset := token.NewFileSet()
	cfg := &packages.Config{Context: ctx, Dir: applicationPath, Fset: fset, Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedDeps}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		return Result{}, fmt.Errorf("load Go packages: %w", err)
	}
	if n := packages.PrintErrors(pkgs); n > 0 {
		return Result{}, fmt.Errorf("load Go packages: %d package errors", n)
	}
	sourceRoot := nearestModuleRoot(applicationPath)
	if sourceRoot == "" {
		sourceRoot = root
	}
	rootAbs, _ := filepath.Abs(sourceRoot)
	var out Result
	seenFiles := map[string]bool{}
	var visit func(*packages.Package)
	visit = func(pkg *packages.Package) {
		for _, imp := range pkg.Imports {
			visit(imp)
		}
		for i, file := range pkg.Syntax {
			filename := pkg.CompiledGoFiles[i]
			abs, _ := filepath.Abs(filename)
			if seenFiles[abs] || !within(rootAbs, abs) || strings.Contains(abs, string(filepath.Separator)+"vendor"+string(filepath.Separator)) {
				continue
			}
			seenFiles[abs] = true
			extractFile(file, pkg.TypesInfo, fset, rootAbs, applicationID, &out)
		}
	}
	for _, pkg := range pkgs {
		visit(pkg)
	}
	sort.Slice(out.EnvReads, func(i, j int) bool { return out.EnvReads[i].ID < out.EnvReads[j].ID })
	sort.Slice(out.ListenPorts, func(i, j int) bool { return out.ListenPorts[i].ID < out.ListenPorts[j].ID })
	return out, nil
}

func nearestModuleRoot(start string) string {
	current, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	for {
		if info, statErr := os.Stat(filepath.Join(current, "go.mod")); statErr == nil && !info.IsDir() {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func extractFile(file *ast.File, info *types.Info, fset *token.FileSet, root, app string, out *Result) {
	parents := map[ast.Node]ast.Node{}
	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			return true
		}
		for _, c := range children(n) {
			parents[c] = n
		}
		return true
	})
	vars := map[string]valueExpr{}
	reads := []assignedRead{}
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		kind, ok := callKind(call, info)
		if !ok || (kind != "getenv" && kind != "lookupenv") {
			return true
		}
		name, resolved := stringConst(call.Args[0], info)
		r := assignedRead{call: call, name: name, resolved: resolved}
		if a, ok := parents[call].(*ast.AssignStmt); ok {
			for idx, rhs := range a.Rhs {
				if rhs == call && idx < len(a.Lhs) {
					if id, ok := a.Lhs[idx].(*ast.Ident); ok {
						r.variable = id.Name
					}
					if kind == "lookupenv" && len(a.Lhs) > idx+1 {
						if id, ok := a.Lhs[idx+1].(*ast.Ident); ok {
							r.okVariable = id.Name
						}
					}
				}
			}
		}
		reads = append(reads, r)
		if r.variable != "" {
			vars[r.variable] = valueExpr{envName: name}
		}
		return true
	})
	// Collect literal variables and supported fallback assignments.
	ast.Inspect(file, func(n ast.Node) bool {
		a, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, rhs := range a.Rhs {
			if i >= len(a.Lhs) {
				continue
			}
			id, ok := a.Lhs[i].(*ast.Ident)
			if !ok {
				continue
			}
			if s, ok := literalString(rhs, info); ok {
				v := vars[id.Name]
				v.literal = s
				vars[id.Name] = v
			}
		}
		return true
	})
	for _, r := range reads {
		req, ev := classify(file, r, info, fset, root)
		loc := location(fset, r.call.Pos(), root)
		name := r.name
		if !r.resolved {
			name = ""
		}
		out.EnvReads = append(out.EnvReads, model.EnvRead{ID: fmt.Sprintf("envread:%s:%d:%d", loc.File, loc.Line, loc.Column), Application: app, Name: name, Resolved: r.resolved, Requirement: req, Evidence: ev, Source: loc})
		if r.variable != "" && req == model.RequirementOptional {
			if p := fallbackPort(file, r.variable, info); p > 0 {
				v := vars[r.variable]
				v.defaultPort = p
				vars[r.variable] = v
			}
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		kind, ok := callKind(call, info)
		if !ok || kind != "listen" || len(call.Args) == 0 {
			return true
		}
		loc := location(fset, call.Pos(), root)
		lp := model.ListenPort{ID: fmt.Sprintf("listen:%s:%d:%d", loc.File, loc.Line, loc.Column), Application: app, Source: loc, Expression: types.ExprString(call.Args[0])}
		if addr, ok := literalString(call.Args[0], info); ok {
			lp.Port, lp.Resolved = parseAddress(addr)
		} else if b, ok := call.Args[0].(*ast.BinaryExpr); ok && b.Op == token.ADD {
			if prefix, ok := literalString(b.X, info); ok && prefix == ":" {
				if id, ok := b.Y.(*ast.Ident); ok {
					v := vars[id.Name]
					if v.envName != "" {
						lp.EnvName = v.envName
						lp.DefaultPort = v.defaultPort
					} else {
						lp.Port, lp.Resolved = parseAddress(":" + v.literal)
					}
				}
			}
		}
		out.ListenPorts = append(out.ListenPorts, lp)
		return true
	})
}

func children(n ast.Node) []ast.Node {
	var a []ast.Node
	ast.Inspect(n, func(c ast.Node) bool {
		if c != nil && c != n {
			a = append(a, c)
			return false
		}
		return true
	})
	return a
}

func callKind(c *ast.CallExpr, info *types.Info) (string, bool) {
	s, ok := c.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	obj := info.Uses[s.Sel]
	if obj == nil || obj.Pkg() == nil {
		return "", false
	}
	p := obj.Pkg().Path()
	switch {
	case p == "os" && s.Sel.Name == "Getenv" && len(c.Args) == 1:
		return "getenv", true
	case p == "os" && s.Sel.Name == "LookupEnv" && len(c.Args) == 1:
		return "lookupenv", true
	case p == "net/http" && s.Sel.Name == "ListenAndServe":
		return "listen", true
	}
	return "", false
}

func stringConst(e ast.Expr, info *types.Info) (string, bool) {
	if tv, ok := info.Types[e]; ok && tv.Value != nil && tv.Value.Kind() == constant.String {
		return constant.StringVal(tv.Value), true
	}
	return "", false
}
func literalString(e ast.Expr, info *types.Info) (string, bool) { return stringConst(e, info) }

func classify(file *ast.File, r assignedRead, info *types.Info, fset *token.FileSet, root string) (model.Requirement, []model.Evidence) {
	if r.variable == "" { // Direct boolean feature flag comparison.
		optional := false
		ast.Inspect(file, func(n ast.Node) bool {
			b, ok := n.(*ast.BinaryExpr)
			if !ok || (b.Op != token.EQL && b.Op != token.NEQ) {
				return true
			}
			other := ast.Expr(nil)
			if b.X == r.call {
				other = b.Y
			} else if b.Y == r.call {
				other = b.X
			}
			if other != nil {
				if value, ok := literalString(other, info); ok && (value == "true" || value == "false") {
					optional = true
				}
			}
			return !optional
		})
		if optional {
			return model.RequirementOptional, []model.Evidence{{Kind: "boolean-comparison", Description: "environment value is used directly as a feature flag", Location: location(fset, r.call.Pos(), root)}}
		}
		return model.RequirementUnknown, nil
	}
	result := model.RequirementUnknown
	var evidence []model.Evidence
	ast.Inspect(file, func(n ast.Node) bool {
		is, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}
		empty := conditionMatches(is.Cond, r.variable, r.okVariable, info)
		if !empty {
			return true
		}
		loc := location(fset, is.Pos(), root)
		if assignsNonEmpty(is.Body, r.variable, info) {
			result = model.RequirementOptional
			evidence = []model.Evidence{{Kind: "fallback", Description: "an empty value is replaced with a static fallback", Location: loc}}
			return false
		}
		if terminates(is.Body, info) {
			result = model.RequirementRequired
			evidence = []model.Evidence{{Kind: "termination", Description: "application initialization terminates when the value is absent", Location: loc}}
			return false
		}
		if returnsError(is.Body, info) {
			if fn := enclosingFunction(file, r.call); fn != nil && callerTerminatesOnError(file, fn.Name.Name, info) {
				result = model.RequirementRequired
				evidence = []model.Evidence{{Kind: "propagated-error", Description: "the read returns an error that the application entrypoint handles by terminating", Location: loc}}
				return false
			}
		}
		return true
	})
	return result, evidence
}

func conditionMatches(e ast.Expr, v, okv string, info *types.Info) bool {
	if u, ok := e.(*ast.UnaryExpr); ok && u.Op == token.NOT {
		id, _ := u.X.(*ast.Ident)
		return id != nil && id.Name == okv && okv != ""
	}
	b, ok := e.(*ast.BinaryExpr)
	if !ok || b.Op != token.EQL {
		return false
	}
	id, _ := b.X.(*ast.Ident)
	if id == nil || id.Name != v {
		return false
	}
	s, yes := literalString(b.Y, info)
	return yes && s == ""
}
func assignsNonEmpty(b *ast.BlockStmt, v string, info *types.Info) bool {
	found := false
	ast.Inspect(b, func(n ast.Node) bool {
		a, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, l := range a.Lhs {
			if id, ok := l.(*ast.Ident); ok && id.Name == v && i < len(a.Rhs) {
				s, yes := literalString(a.Rhs[i], info)
				if yes && s != "" {
					found = true
				}
			}
		}
		return true
	})
	return found
}
func terminates(b *ast.BlockStmt, info *types.Info) bool {
	found := false
	ast.Inspect(b, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.ReturnStmt:
			// A returned error is not necessarily fatal unless propagation to
			// initialization is proven. Fleck deliberately leaves it unknown.
			_ = x
		case *ast.CallExpr:
			switch f := x.Fun.(type) {
			case *ast.Ident:
				if builtin, ok := info.Uses[f].(*types.Builtin); ok && builtin.Name() == "panic" {
					found = true
				}
			case *ast.SelectorExpr:
				obj := info.Uses[f.Sel]
				pkg := ""
				if obj != nil && obj.Pkg() != nil {
					pkg = obj.Pkg().Path()
				}
				if (pkg == "log" && (f.Sel.Name == "Fatal" || f.Sel.Name == "Fatalf" || f.Sel.Name == "Fatalln")) || (pkg == "os" && f.Sel.Name == "Exit") {
					found = true
				}
			}
		}
		return !found
	})
	return found
}

func returnsError(b *ast.BlockStmt, info *types.Info) bool {
	found := false
	ast.Inspect(b, func(n ast.Node) bool {
		r, ok := n.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		for _, result := range r.Results {
			if id, ok := result.(*ast.Ident); ok && id.Name == "nil" {
				continue
			}
			if isErrorType(info.TypeOf(result)) {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func enclosingFunction(file *ast.File, node ast.Node) *ast.FuncDecl {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Pos() <= node.Pos() && node.End() <= fn.End() {
			return fn
		}
	}
	return nil
}

func callerTerminatesOnError(file *ast.File, functionName string, info *types.Info) bool {
	for _, decl := range file.Decls {
		mainFn, ok := decl.(*ast.FuncDecl)
		if !ok || mainFn.Name.Name != "main" || mainFn.Body == nil {
			continue
		}
		errorVars := map[string]bool{}
		ast.Inspect(mainFn.Body, func(n ast.Node) bool {
			a, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for _, rhs := range a.Rhs {
				call, ok := rhs.(*ast.CallExpr)
				if !ok {
					continue
				}
				id, ok := call.Fun.(*ast.Ident)
				if !ok || id.Name != functionName {
					continue
				}
				sig, _ := info.TypeOf(call.Fun).(*types.Signature)
				for index, lhs := range a.Lhs {
					if sig == nil || sig.Results() == nil || index >= sig.Results().Len() || !isErrorType(sig.Results().At(index).Type()) {
						continue
					}
					if name, ok := lhs.(*ast.Ident); ok && name.Name != "_" {
						errorVars[name.Name] = true
					}
				}
			}
			return true
		})
		proven := false
		ast.Inspect(mainFn.Body, func(n ast.Node) bool {
			if proven {
				return false
			}
			is, ok := n.(*ast.IfStmt)
			if !ok || !conditionNonNilVariable(is.Cond, errorVars) {
				return true
			}
			if terminates(is.Body, info) {
				proven = true
				return false
			}
			return true
		})
		if proven {
			return true
		}
	}
	return false
}

func isErrorType(value types.Type) bool {
	if value == nil {
		return false
	}
	errorType := types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
	return types.Implements(value, errorType) || types.Implements(types.NewPointer(value), errorType)
}

func conditionNonNilVariable(expr ast.Expr, names map[string]bool) bool {
	b, ok := expr.(*ast.BinaryExpr)
	if !ok || b.Op != token.NEQ {
		return false
	}
	left, lok := b.X.(*ast.Ident)
	right, rok := b.Y.(*ast.Ident)
	return lok && rok && ((names[left.Name] && right.Name == "nil") || (names[right.Name] && left.Name == "nil"))
}
func fallbackPort(file *ast.File, v string, info *types.Info) int {
	p := 0
	ast.Inspect(file, func(n ast.Node) bool {
		is, ok := n.(*ast.IfStmt)
		if !ok || !conditionMatches(is.Cond, v, "", info) {
			return true
		}
		ast.Inspect(is.Body, func(n ast.Node) bool {
			a, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for i, l := range a.Lhs {
				if id, ok := l.(*ast.Ident); ok && id.Name == v && i < len(a.Rhs) {
					s, _ := literalString(a.Rhs[i], info)
					p, _ = strconv.Atoi(s)
				}
			}
			return true
		})
		return true
	})
	return p
}
func parseAddress(s string) (int, bool) {
	if !strings.HasPrefix(s, ":") {
		return 0, false
	}
	p, err := strconv.Atoi(strings.TrimPrefix(s, ":"))
	return p, err == nil && p > 0 && p <= 65535
}
func location(fset *token.FileSet, p token.Pos, root string) model.SourceLocation {
	x := fset.Position(p)
	rel, err := filepath.Rel(root, x.Filename)
	if err != nil {
		rel = x.Filename
	}
	return model.SourceLocation{File: filepath.ToSlash(rel), Line: x.Line, Column: x.Column}
}
