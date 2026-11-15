package main

import (
	"bytes"
	"fmt"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
)

// Static literals fail the contract check before a new producer can ship.
// Dynamic producers still pass through the same finite writer catalog at run
// time; their exact call sites are included in the generated integration ledger.
func producerCatalog() ([]map[string]any, error) {
	out := []map[string]any{}
	e := filepath.WalkDir("packages/go/platform", func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		set := token.NewFileSet()
		file, e := parser.ParseFile(set, path, nil, 0)
		if e != nil {
			return e
		}
		aliases := map[string]string{}
		for _, i := range file.Imports {
			p, _ := strconv.Unquote(i.Path.Value)
			name := filepath.Base(p)
			if i.Name != nil {
				name = i.Name.Name
			}
			aliases[name] = p
		}
		var failure error
		owner := func(pos token.Pos) string {
			for _, declaration := range file.Decls {
				if fn, ok := declaration.(*ast.FuncDecl); ok && pos >= fn.Pos() && pos <= fn.End() {
					return fn.Name.Name
				}
			}
			return "package"
		}
		record := func(expr ast.Expr) {
			if failure != nil {
				return
			}
			var b bytes.Buffer
			_ = printer.Fprint(&b, set, expr)
			value := b.String()
			literal := ""
			if l, ok := expr.(*ast.BasicLit); ok && l.Kind == token.STRING {
				literal, _ = strconv.Unquote(l.Value)
				if _, ok = ar.Lookup(literal); !ok {
					failure = fmt.Errorf("unregistered audit event %q at %s:%d", literal, path, set.Position(expr.Pos()).Line)
					return
				}
			}
			out = append(out, map[string]any{"path": filepath.ToSlash(path), "line": set.Position(expr.Pos()).Line, "function": owner(expr.Pos()), "kind": "producer", "expression": value, "literal": literal})
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.CallExpr:
				if name, ok := v.Fun.(*ast.Ident); ok && (file.Name.Name == "identity" && name.Name == "audit" || file.Name.Name == "access" && name.Name == "Audit") && len(v.Args) > 2 {
					record(v.Args[2])
					return true
				}
				sel, ok := v.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				alias, ok := sel.X.(*ast.Ident)
				if !ok {
					return true
				}
				if strings.HasSuffix(aliases[alias.Name], "/access") && sel.Sel.Name == "Audit" && len(v.Args) > 2 {
					record(v.Args[2])
				}
				if strings.HasSuffix(aliases[alias.Name], "/devicekit") && (sel.Sel.Name == "Audit" || sel.Sel.Name == "AuditSnapshots") && len(v.Args) > 4 {
					record(v.Args[4])
				}
			case *ast.BasicLit:
				if v.Kind == token.STRING {
					value, _ := strconv.Unquote(v.Value)
					upper := strings.ToUpper(value)
					if strings.Contains(upper, "INSERT INTO ") && (strings.Contains(upper, "_AUDIT") || strings.Contains(upper, "_CHANGES") || strings.Contains(upper, "_EVENTS") || strings.Contains(upper, "_HANDOFFS")) {
						statement := strings.Fields(value)
						table := ""
						for i, word := range statement {
							if strings.EqualFold(word, "INTO") && i+1 < len(statement) {
								table = strings.Split(statement[i+1], "(")[0]
								break
							}
						}
						out = append(out, map[string]any{"path": filepath.ToSlash(path), "line": set.Position(v.Pos()).Line, "function": owner(v.Pos()), "kind": "domain_journal", "expression": table, "literal": ""})
					}
				}
			case *ast.CompositeLit:
				if name, ok := v.Type.(*ast.Ident); ok && file.Name.Name == "auditrecord" && (name.Name == "Fact" || name.Name == "LegacyInput") {
					for _, e := range v.Elts {
						if kv, ok := e.(*ast.KeyValueExpr); ok {
							if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "Event" {
								record(kv.Value)
							}
						}
					}
				}
				sel, ok := v.Type.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				alias, ok := sel.X.(*ast.Ident)
				if !ok {
					return true
				}
				if strings.HasSuffix(aliases[alias.Name], "/auditrecord") && (sel.Sel.Name == "Fact" || sel.Sel.Name == "LegacyInput") {
					for _, e := range v.Elts {
						kv, ok := e.(*ast.KeyValueExpr)
						if !ok {
							continue
						}
						key, ok := kv.Key.(*ast.Ident)
						if ok && key.Name == "Event" {
							record(kv.Value)
						}
					}
				}
			}
			return true
		})
		return failure
	})
	return out, e
}
