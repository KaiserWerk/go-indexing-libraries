package symbolindex

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

func analyzeGoFile(file SourceFile, contextID string, c *collector) []Diagnostic {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, file.Path, file.Content, parser.AllErrors)
	if err != nil {
		return []Diagnostic{{
			Level:   DiagnosticLevelError,
			Code:    "GO_PARSE_ERROR",
			Message: err.Error(),
			Path:    file.Path,
			Line:    1,
		}}
	}

	pkgName := node.Name.Name
	pkgFQN := pkgName
	pkg := makeSymbol(LanguageGo, SymbolKindPackage, "package", pkgName, pkgFQN, "", "", contextID, Span{
		Path:      file.Path,
		LineStart: posLine(fset, node.Pos()),
		LineEnd:   posLine(fset, node.End()),
	})
	pkg = c.upsertSymbol(pkg)

	funcSymbols := make(map[*ast.FuncDecl]Symbol)

	for _, decl := range node.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			if d.Tok != token.TYPE {
				continue
			}
			for _, spec := range d.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				kind := SymbolKindType
				languageKind := "type"
				switch ts.Type.(type) {
				case *ast.StructType:
					languageKind = "struct"
				case *ast.InterfaceType:
					languageKind = "interface"
				}
				fqn := pkgFQN + "." + ts.Name.Name
				s := makeSymbol(LanguageGo, kind, languageKind, ts.Name.Name, fqn, "", pkgFQN, contextID, Span{
					Path:      file.Path,
					LineStart: posLine(fset, ts.Pos()),
					LineEnd:   posLine(fset, ts.End()),
				})
				s = c.upsertSymbol(s)
				c.addEdge(Edge{Type: EdgeTypeDeclares, FromID: pkg.ID, ToID: s.ID, ContextID: contextID})
			}
		case *ast.FuncDecl:
			name := d.Name.Name
			line := Span{Path: file.Path, LineStart: posLine(fset, d.Pos()), LineEnd: posLine(fset, d.End())}
			sig := goFuncSignature(d)
			if d.Recv == nil {
				fqn := pkgFQN + "." + name
				s := makeSymbol(LanguageGo, SymbolKindFunction, "function", name, fqn, sig, pkgFQN, contextID, line)
				s = c.upsertSymbol(s)
				funcSymbols[d] = s
				c.addEdge(Edge{Type: EdgeTypeDeclares, FromID: pkg.ID, ToID: s.ID, ContextID: contextID})
				continue
			}

			recv := goReceiverName(d.Recv)
			containerFQN := pkgFQN + "." + recv
			fqn := containerFQN + "." + name
			s := makeSymbol(LanguageGo, SymbolKindMethod, "method", name, fqn, sig, containerFQN, contextID, line)
			s = c.upsertSymbol(s)
			funcSymbols[d] = s

			recvCandidates := c.findSymbolsByName(recv, contextID)
			for _, candidate := range recvCandidates {
				if candidate.FQN == containerFQN {
					c.addEdge(Edge{Type: EdgeTypeDeclares, FromID: candidate.ID, ToID: s.ID, ContextID: contextID})
					break
				}
			}
		}
	}

	for fd, caller := range funcSymbols {
		if fd.Body == nil {
			continue
		}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			callExpr, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			calleeName := goCallName(callExpr.Fun)
			if calleeName == "" {
				return true
			}
			line := Span{Path: file.Path, LineStart: posLine(fset, callExpr.Pos()), LineEnd: posLine(fset, callExpr.End())}
			candidates := c.findSymbolsByName(calleeName, contextID)
			cand := make([]Candidate, 0, len(candidates))
			for _, target := range candidates {
				conf := 0.5
				if len(candidates) == 1 {
					conf = 0.9
				}
				cand = append(cand, Candidate{SymbolID: target.ID, Confidence: conf})
			}

			call := Call{CallerID: caller.ID, ContextID: contextID, Location: line, Candidates: cand, IsAmbiguous: len(cand) > 1}
			if len(cand) == 1 {
				call.CalleeID = cand[0].SymbolID
				c.addEdge(Edge{Type: EdgeTypeCalls, FromID: caller.ID, ToID: call.CalleeID, ContextID: contextID})
			}
			c.addCall(call)
			return true
		})
	}

	return nil
}

func goReceiverName(fl *ast.FieldList) string {
	if fl == nil || len(fl.List) == 0 {
		return ""
	}
	t := fl.List[0].Type
	switch x := t.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.StarExpr:
		if id, ok := x.X.(*ast.Ident); ok {
			return id.Name
		}
	}
	return ""
}

func goCallName(expr ast.Expr) string {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return x.Sel.Name
	default:
		return ""
	}
}

func goFuncSignature(fd *ast.FuncDecl) string {
	params := make([]string, 0)
	if fd.Type.Params != nil {
		for _, p := range fd.Type.Params.List {
			ptype := exprString(p.Type)
			if len(p.Names) == 0 {
				params = append(params, ptype)
				continue
			}
			for range p.Names {
				params = append(params, ptype)
			}
		}
	}
	results := ""
	if fd.Type.Results != nil {
		parts := make([]string, 0)
		for _, r := range fd.Type.Results.List {
			rt := exprString(r.Type)
			if len(r.Names) == 0 {
				parts = append(parts, rt)
				continue
			}
			for range r.Names {
				parts = append(parts, rt)
			}
		}
		results = strings.Join(parts, ",")
	}
	if results == "" {
		return fmt.Sprintf("(%s)", strings.Join(params, ","))
	}
	return fmt.Sprintf("(%s)->(%s)", strings.Join(params, ","), results)
}

func exprString(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		left := exprString(x.X)
		if left == "" {
			return x.Sel.Name
		}
		return left + "." + x.Sel.Name
	case *ast.StarExpr:
		return "*" + exprString(x.X)
	case *ast.ArrayType:
		return "[]" + exprString(x.Elt)
	default:
		return ""
	}
}

func posLine(fset *token.FileSet, p token.Pos) int {
	if !p.IsValid() {
		return 1
	}
	return fset.Position(p).Line
}

func makeSymbol(language Language, kind SymbolKind, languageKind, name, fqn, signature, containerFQN, contextID string, decl Span) Symbol {
	stable := makeStableKey(language, kind, fqn, signature)
	return Symbol{
		ID:                   stable,
		StableKey:            stable,
		Language:             language,
		Kind:                 kind,
		LanguageSpecificKind: languageKind,
		Name:                 name,
		FQN:                  fqn,
		Signature:            canonicalSignature(signature),
		ContainerFQN:         containerFQN,
		ContextID:            contextID,
		Declarations:         []Span{decl},
	}
}
