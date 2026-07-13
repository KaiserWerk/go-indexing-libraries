package astindex

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
)

func analyzeGoFile(file SourceFile, contextID string) (FileAST, []Diagnostic) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, file.Path, file.Content, parser.ParseComments|parser.AllErrors)
	collector := newFileCollector(file.Path, LanguageGo, contextID)

	root := makeNode(LanguageGo, NodeKindFileRoot, "file", file.Path, file.Path, "", "", contextID, Span{
		Path:      file.Path,
		LineStart: 1,
		LineEnd:   lineCount(file.Content),
	})
	root = collector.addNode(root)

	if err != nil {
		return collector.result(), []Diagnostic{{
			Level:   DiagnosticLevelError,
			Code:    "GO_PARSE_ERROR",
			Message: err.Error(),
			Path:    file.Path,
			Line:    1,
		}}
	}

	pkgName := ""
	if node.Name != nil {
		pkgName = node.Name.Name
	}
	pkgFQN := pkgName
	pkg := makeNode(LanguageGo, NodeKindPackage, "package", pkgName, pkgFQN, "", root.ID, contextID, Span{
		Path:      file.Path,
		LineStart: posLine(fset, node.Pos()),
		LineEnd:   posLine(fset, node.End()),
	})
	pkg = collector.addNode(pkg)
	collector.addRelation(Relation{Type: RelationTypeParent, FromID: root.ID, ToID: pkg.ID, ContextID: contextID, Location: pkg.Span, Confidence: 1.0})

	knownFunctions := make(map[string][]Node)
	knownTypes := make(map[string]Node)
	funcDeclNodes := make(map[*ast.FuncDecl]Node)

	for _, imp := range node.Imports {
		pathValue, _ := strconv.Unquote(imp.Path.Value)
		importNode := makeNode(LanguageGo, NodeKindImport, "import", pathValue, pathValue, "", root.ID, contextID, Span{
			Path:      file.Path,
			LineStart: posLine(fset, imp.Pos()),
			LineEnd:   posLine(fset, imp.End()),
		})
		importNode = collector.addNode(importNode)
		collector.addRelation(Relation{Type: RelationTypeParent, FromID: root.ID, ToID: importNode.ID, ContextID: contextID, Location: importNode.Span, Confidence: 1.0})
		collector.addRelation(Relation{Type: RelationTypeImports, FromID: pkg.ID, ToID: importNode.ID, ContextID: contextID, Location: importNode.Span, Confidence: 1.0})
	}

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
				languageKind := "type"
				switch ts.Type.(type) {
				case *ast.StructType:
					languageKind = "struct"
				case *ast.InterfaceType:
					languageKind = "interface"
				}

				fqn := pkgFQN + "." + ts.Name.Name
				typeNode := makeNode(LanguageGo, NodeKindType, languageKind, ts.Name.Name, fqn, "", pkg.ID, contextID, Span{
					Path:      file.Path,
					LineStart: posLine(fset, ts.Pos()),
					LineEnd:   posLine(fset, ts.End()),
				})
				typeNode = collector.addNode(typeNode)
				knownTypes[ts.Name.Name] = typeNode
				collector.addRelation(Relation{Type: RelationTypeParent, FromID: pkg.ID, ToID: typeNode.ID, ContextID: contextID, Location: typeNode.Span, Confidence: 1.0})
				collector.addRelation(Relation{Type: RelationTypeDeclares, FromID: pkg.ID, ToID: typeNode.ID, ContextID: contextID, Location: typeNode.Span, Confidence: 1.0})

				if ts.TypeParams != nil {
					for _, p := range ts.TypeParams.List {
						for _, n := range p.Names {
							paramNode := makeNode(LanguageGo, NodeKindGenericParameter, "type_parameter", n.Name, fqn+"."+n.Name, exprString(p.Type), typeNode.ID, contextID, Span{
								Path:      file.Path,
								LineStart: posLine(fset, n.Pos()),
								LineEnd:   posLine(fset, n.End()),
							})
							paramNode = collector.addNode(paramNode)
							collector.addRelation(Relation{Type: RelationTypeParent, FromID: typeNode.ID, ToID: paramNode.ID, ContextID: contextID, Location: paramNode.Span, Confidence: 1.0})
							collector.addRelation(Relation{Type: RelationTypeDeclares, FromID: typeNode.ID, ToID: paramNode.ID, ContextID: contextID, Location: paramNode.Span, Confidence: 1.0})
						}
					}
				}

				if st, ok := ts.Type.(*ast.StructType); ok && st.Fields != nil {
					for _, field := range st.Fields.List {
						fieldType := exprString(field.Type)
						if len(field.Names) == 0 {
							embedded := makeNode(LanguageGo, NodeKindField, "embedded_field", fieldType, fqn+"."+fieldType, fieldType, typeNode.ID, contextID, Span{
								Path:      file.Path,
								LineStart: posLine(fset, field.Pos()),
								LineEnd:   posLine(fset, field.End()),
							})
							embedded = collector.addNode(embedded)
							collector.addRelation(Relation{Type: RelationTypeParent, FromID: typeNode.ID, ToID: embedded.ID, ContextID: contextID, Location: embedded.Span, Confidence: 1.0})
							collector.addRelation(Relation{Type: RelationTypeDeclares, FromID: typeNode.ID, ToID: embedded.ID, ContextID: contextID, Location: embedded.Span, Confidence: 1.0})
							continue
						}
						for _, name := range field.Names {
							fieldNode := makeNode(LanguageGo, NodeKindField, "field", name.Name, fqn+"."+name.Name, fieldType, typeNode.ID, contextID, Span{
								Path:      file.Path,
								LineStart: posLine(fset, name.Pos()),
								LineEnd:   posLine(fset, field.End()),
							})
							fieldNode = collector.addNode(fieldNode)
							collector.addRelation(Relation{Type: RelationTypeParent, FromID: typeNode.ID, ToID: fieldNode.ID, ContextID: contextID, Location: fieldNode.Span, Confidence: 1.0})
							collector.addRelation(Relation{Type: RelationTypeDeclares, FromID: typeNode.ID, ToID: fieldNode.ID, ContextID: contextID, Location: fieldNode.Span, Confidence: 1.0})
						}
					}
				}
			}
		case *ast.FuncDecl:
			name := d.Name.Name
			sig := goFuncSignature(d)
			span := Span{Path: file.Path, LineStart: posLine(fset, d.Pos()), LineEnd: posLine(fset, d.End())}
			if d.Recv == nil {
				fqn := pkgFQN + "." + name
				fnNode := makeNode(LanguageGo, NodeKindFunction, "function", name, fqn, sig, pkg.ID, contextID, span)
				fnNode = collector.addNode(fnNode)
				collector.addRelation(Relation{Type: RelationTypeParent, FromID: pkg.ID, ToID: fnNode.ID, ContextID: contextID, Location: span, Confidence: 1.0})
				collector.addRelation(Relation{Type: RelationTypeDeclares, FromID: pkg.ID, ToID: fnNode.ID, ContextID: contextID, Location: span, Confidence: 1.0})
				knownFunctions[name] = append(knownFunctions[name], fnNode)
				funcDeclNodes[d] = fnNode
			} else {
				recv := goReceiverName(d.Recv)
				container := pkgFQN + "." + recv
				fqn := container + "." + name
				methodNode := makeNode(LanguageGo, NodeKindMethod, "method", name, fqn, sig, pkg.ID, contextID, span)
				methodNode = collector.addNode(methodNode)
				collector.addRelation(Relation{Type: RelationTypeParent, FromID: pkg.ID, ToID: methodNode.ID, ContextID: contextID, Location: span, Confidence: 1.0})
				collector.addRelation(Relation{Type: RelationTypeDeclares, FromID: pkg.ID, ToID: methodNode.ID, ContextID: contextID, Location: span, Confidence: 1.0})
				knownFunctions[name] = append(knownFunctions[name], methodNode)
				funcDeclNodes[d] = methodNode
			}

			if d.Type.TypeParams != nil {
				parent := funcDeclNodes[d]
				for _, p := range d.Type.TypeParams.List {
					for _, n := range p.Names {
						paramNode := makeNode(LanguageGo, NodeKindGenericParameter, "type_parameter", n.Name, parent.FQN+"."+n.Name, exprString(p.Type), parent.ID, contextID, Span{
							Path:      file.Path,
							LineStart: posLine(fset, n.Pos()),
							LineEnd:   posLine(fset, n.End()),
						})
						paramNode = collector.addNode(paramNode)
						collector.addRelation(Relation{Type: RelationTypeParent, FromID: parent.ID, ToID: paramNode.ID, ContextID: contextID, Location: paramNode.Span, Confidence: 1.0})
						collector.addRelation(Relation{Type: RelationTypeDeclares, FromID: parent.ID, ToID: paramNode.ID, ContextID: contextID, Location: paramNode.Span, Confidence: 1.0})
					}
				}
			}
		}
	}

	docGroups := make(map[*ast.CommentGroup]struct{})
	for _, decl := range node.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			if d.Doc != nil {
				docGroups[d.Doc] = struct{}{}
			}
		case *ast.FuncDecl:
			if d.Doc != nil {
				docGroups[d.Doc] = struct{}{}
			}
		}
	}

	for _, cg := range node.Comments {
		kind := NodeKindComment
		langKind := "comment"
		if _, ok := docGroups[cg]; ok {
			kind = NodeKindDocComment
			langKind = "doc_comment"
		}
		text := strings.TrimSpace(cg.Text())
		commentNode := makeNode(LanguageGo, kind, langKind, truncate(text, 64), "", "", root.ID, contextID, Span{
			Path:      file.Path,
			LineStart: posLine(fset, cg.Pos()),
			LineEnd:   posLine(fset, cg.End()),
		})
		commentNode = collector.addNode(commentNode)
		collector.addRelation(Relation{Type: RelationTypeParent, FromID: root.ID, ToID: commentNode.ID, ContextID: contextID, Location: commentNode.Span, Confidence: 1.0})
	}

	for fd, caller := range funcDeclNodes {
		if fd.Body == nil {
			continue
		}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			if callExpr, ok := n.(*ast.CallExpr); ok {
				calleeName := goCallName(callExpr.Fun)
				if calleeName != "" {
					targets := knownFunctions[calleeName]
					if len(targets) > 0 {
						confidence := 0.6
						if len(targets) == 1 {
							confidence = 0.9
						}
						for _, target := range targets {
							collector.addRelation(Relation{Type: RelationTypeCalls, FromID: caller.ID, ToID: target.ID, ContextID: contextID, Location: Span{Path: file.Path, LineStart: posLine(fset, callExpr.Pos()), LineEnd: posLine(fset, callExpr.End())}, Confidence: confidence})
						}
					}
				}
			}

			if ident, ok := n.(*ast.Ident); ok {
				if target, found := knownTypes[ident.Name]; found {
					collector.addRelation(Relation{Type: RelationTypeReference, FromID: caller.ID, ToID: target.ID, ContextID: contextID, Location: Span{Path: file.Path, LineStart: posLine(fset, ident.Pos()), LineEnd: posLine(fset, ident.End())}, Confidence: 0.55})
				}
			}
			return true
		})
	}

	return collector.result(), nil
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
		return "(" + strings.Join(params, ",") + ")"
	}
	return "(" + strings.Join(params, ",") + ")->(" + results + ")"
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
	case *ast.IndexExpr:
		return exprString(x.X) + "[" + exprString(x.Index) + "]"
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

func lineCount(content string) int {
	if content == "" {
		return 1
	}
	return strings.Count(content, "\n") + 1
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return strings.TrimSpace(s[:max])
}
