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
	call       *ast.CallExpr
	name       string
	variable   types.Object
	okVariable types.Object
	function   *ast.FuncDecl
	resolved   bool
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
	vars := map[types.Object]valueExpr{}
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
		r := assignedRead{call: call, name: name, resolved: resolved, function: enclosingFunction(file, call)}
		if a, ok := parents[call].(*ast.AssignStmt); ok {
			for idx, rhs := range a.Rhs {
				if rhs == call && idx < len(a.Lhs) {
					if id, ok := a.Lhs[idx].(*ast.Ident); ok {
						r.variable = objectOf(info, id)
					}
					if kind == "lookupenv" && len(a.Lhs) > idx+1 {
						if id, ok := a.Lhs[idx+1].(*ast.Ident); ok {
							r.okVariable = objectOf(info, id)
						}
					}
				}
			}
		} else if spec, ok := parents[call].(*ast.ValueSpec); ok {
			for idx, value := range spec.Values {
				if value != call || idx >= len(spec.Names) {
					continue
				}
				r.variable = objectOf(info, spec.Names[idx])
				if kind == "lookupenv" && len(spec.Names) > idx+1 {
					r.okVariable = objectOf(info, spec.Names[idx+1])
				}
			}
		}
		reads = append(reads, r)
		if r.variable != nil {
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
			if !ok || objectOf(info, id) == nil {
				continue
			}
			if s, ok := literalString(rhs, info); ok {
				object := objectOf(info, id)
				v := vars[object]
				v.literal = s
				vars[object] = v
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
		if r.variable != nil && req == model.RequirementOptional {
			if p := fallbackPort(r.function, r.variable, info); p > 0 {
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
					v := vars[objectOf(info, id)]
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

func objectOf(info *types.Info, id *ast.Ident) types.Object {
	if id == nil {
		return nil
	}
	if object := info.Defs[id]; object != nil {
		return object
	}
	return info.Uses[id]
}

func classify(file *ast.File, r assignedRead, info *types.Info, fset *token.FileSet, root string) (model.Requirement, []model.Evidence) {
	scope := ast.Node(file)
	if r.function != nil && r.function.Body != nil {
		scope = r.function.Body
	}
	if r.variable == nil { // Direct boolean feature flag comparison.
		optional := false
		ast.Inspect(scope, func(n ast.Node) bool {
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
	if r.function == nil || r.function.Body == nil {
		return model.RequirementUnknown, nil
	}
	result := model.RequirementUnknown
	var evidence []model.Evidence
	for _, statement := range r.function.Body.List {
		is, ok := statement.(*ast.IfStmt)
		if !ok {
			continue
		}
		empty := conditionMatches(is.Cond, r.variable, r.okVariable, info)
		if !empty {
			continue
		}
		loc := location(fset, is.Pos(), root)
		if assignsNonEmpty(is.Body, r.variable, info) {
			result = model.RequirementOptional
			evidence = []model.Evidence{{Kind: "fallback", Description: "an empty value is replaced with a static fallback", Location: loc}}
			break
		}
		if definitelyTerminates(is.Body, info) {
			result = model.RequirementRequired
			evidence = []model.Evidence{{Kind: "termination", Description: "application initialization terminates when the value is absent", Location: loc}}
			break
		}
		if returnsError(is.Body, info) {
			if r.function != nil && callerTerminatesOnError(file, objectOf(info, r.function.Name), info) {
				result = model.RequirementRequired
				evidence = []model.Evidence{{Kind: "propagated-error", Description: "the read returns an error that the application entrypoint handles by terminating", Location: loc}}
				break
			}
		}
	}
	return result, evidence
}

func conditionMatches(e ast.Expr, variable, okVariable types.Object, info *types.Info) bool {
	if u, ok := e.(*ast.UnaryExpr); ok && u.Op == token.NOT {
		id, _ := u.X.(*ast.Ident)
		return id != nil && okVariable != nil && objectOf(info, id) == okVariable
	}
	b, ok := e.(*ast.BinaryExpr)
	if !ok || b.Op != token.EQL {
		return false
	}
	for _, pair := range [][2]ast.Expr{{b.X, b.Y}, {b.Y, b.X}} {
		id, _ := pair[0].(*ast.Ident)
		if id == nil || objectOf(info, id) != variable {
			continue
		}
		s, yes := literalString(pair[1], info)
		return yes && s == ""
	}
	return false
}
func assignsNonEmpty(block *ast.BlockStmt, variable types.Object, info *types.Info) bool {
	if len(block.List) != 1 {
		return false
	}
	assignment, ok := block.List[0].(*ast.AssignStmt)
	if !ok {
		return false
	}
	for index, lhs := range assignment.Lhs {
		id, ok := lhs.(*ast.Ident)
		if !ok || objectOf(info, id) != variable || index >= len(assignment.Rhs) {
			continue
		}
		value, constantValue := literalString(assignment.Rhs[index], info)
		return constantValue && value != ""
	}
	return false
}
func definitelyTerminates(block *ast.BlockStmt, info *types.Info) bool {
	if len(block.List) != 1 {
		return false
	}
	expression, ok := block.List[0].(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := expression.X.(*ast.CallExpr)
	return ok && isTerminatingCall(call, info)
}

func isTerminatingCall(call *ast.CallExpr, info *types.Info) bool {
	switch function := call.Fun.(type) {
	case *ast.Ident:
		builtin, ok := info.Uses[function].(*types.Builtin)
		return ok && builtin.Name() == "panic"
	case *ast.SelectorExpr:
		object := info.Uses[function.Sel]
		if object == nil || object.Pkg() == nil {
			return false
		}
		path := object.Pkg().Path()
		return (path == "log" && (function.Sel.Name == "Fatal" || function.Sel.Name == "Fatalf" || function.Sel.Name == "Fatalln")) || (path == "os" && function.Sel.Name == "Exit")
	default:
		return false
	}
}

func returnsError(b *ast.BlockStmt, info *types.Info) bool {
	if len(b.List) != 1 {
		return false
	}
	returned, ok := b.List[0].(*ast.ReturnStmt)
	if !ok {
		return false
	}
	for _, result := range returned.Results {
		if id, ok := result.(*ast.Ident); ok && id.Name == "nil" {
			continue
		}
		if isErrorType(info.TypeOf(result)) {
			return true
		}
	}
	return false
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

func callerTerminatesOnError(file *ast.File, function types.Object, info *types.Info) bool {
	if function == nil {
		return false
	}
	for _, decl := range file.Decls {
		mainFn, ok := decl.(*ast.FuncDecl)
		if !ok || mainFn.Name.Name != "main" || mainFn.Body == nil {
			continue
		}
		errorVars := map[types.Object]bool{}
		for _, statement := range mainFn.Body.List {
			a, ok := statement.(*ast.AssignStmt)
			if !ok {
				continue
			}
			for _, rhs := range a.Rhs {
				call, ok := rhs.(*ast.CallExpr)
				if !ok {
					continue
				}
				id, ok := call.Fun.(*ast.Ident)
				if !ok || objectOf(info, id) != function {
					continue
				}
				sig, _ := info.TypeOf(call.Fun).(*types.Signature)
				for index, lhs := range a.Lhs {
					if sig == nil || sig.Results() == nil || index >= sig.Results().Len() || !isErrorType(sig.Results().At(index).Type()) {
						continue
					}
					if name, ok := lhs.(*ast.Ident); ok && name.Name != "_" {
						errorVars[objectOf(info, name)] = true
					}
				}
			}
		}
		for _, statement := range mainFn.Body.List {
			is, ok := statement.(*ast.IfStmt)
			if !ok || !conditionNonNilVariable(is.Cond, errorVars, info) {
				continue
			}
			if definitelyTerminates(is.Body, info) {
				return true
			}
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

func conditionNonNilVariable(expr ast.Expr, objects map[types.Object]bool, info *types.Info) bool {
	b, ok := expr.(*ast.BinaryExpr)
	if !ok || b.Op != token.NEQ {
		return false
	}
	left, lok := b.X.(*ast.Ident)
	right, rok := b.Y.(*ast.Ident)
	return lok && rok && ((objects[objectOf(info, left)] && right.Name == "nil") || (objects[objectOf(info, right)] && left.Name == "nil"))
}
func fallbackPort(function *ast.FuncDecl, variable types.Object, info *types.Info) int {
	if function == nil || function.Body == nil {
		return 0
	}
	p := 0
	for _, statement := range function.Body.List {
		is, ok := statement.(*ast.IfStmt)
		if !ok || !conditionMatches(is.Cond, variable, nil, info) || len(is.Body.List) != 1 {
			continue
		}
		for _, bodyStatement := range is.Body.List {
			a, ok := bodyStatement.(*ast.AssignStmt)
			if !ok {
				continue
			}
			for i, l := range a.Lhs {
				if id, ok := l.(*ast.Ident); ok && objectOf(info, id) == variable && i < len(a.Rhs) {
					s, _ := literalString(a.Rhs[i], info)
					p, _ = strconv.Atoi(s)
				}
			}
		}
	}
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
