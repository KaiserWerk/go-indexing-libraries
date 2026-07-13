package astindex

import (
	"regexp"
	"strings"
)

var (
	csNamespaceRe = regexp.MustCompile(`\bnamespace\s+([A-Za-z_][A-Za-z0-9_\.]*)`)
	csUsingRe     = regexp.MustCompile(`^\s*using\s+([A-Za-z_][A-Za-z0-9_\.]*)\s*;`)
	csTypeRe      = regexp.MustCompile(`\b(class|interface|record|enum|delegate|struct)\s+([A-Za-z_][A-Za-z0-9_]*)(<[^>]+>)?`)
	csMethodRe    = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	csPropertyRe  = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_<>\[\]]*)\s+([A-Za-z_][A-Za-z0-9_]*)\s*\{\s*(get|set|init)`) // heuristic
	csFieldRe     = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_<>\[\]]*)\s+([A-Za-z_][A-Za-z0-9_]*)\s*(=|;)`)               // heuristic
	csCallRe      = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
)

func analyzeCSharpFile(file SourceFile, contextID string) (FileAST, []Diagnostic) {
	lines := strings.Split(file.Content, "\n")
	collector := newFileCollector(file.Path, LanguageCSharp, contextID)

	root := makeNode(LanguageCSharp, NodeKindFileRoot, "file", file.Path, file.Path, "", "", contextID, Span{
		Path:      file.Path,
		LineStart: 1,
		LineEnd:   max(1, len(lines)),
	})
	root = collector.addNode(root)

	namespace := "global"
	if m := csNamespaceRe.FindStringSubmatch(file.Content); len(m) == 2 {
		namespace = m[1]
	}

	ns := makeNode(LanguageCSharp, NodeKindNamespace, "namespace", namespace, namespace, "", root.ID, contextID, Span{
		Path:      file.Path,
		LineStart: 1,
		LineEnd:   max(1, len(lines)),
	})
	ns = collector.addNode(ns)
	collector.addRelation(Relation{Type: RelationTypeParent, FromID: root.ID, ToID: ns.ID, ContextID: contextID, Location: ns.Span, Confidence: 1.0})

	knownMethods := make(map[string][]Node)
	knownTypes := make(map[string]Node)
	currentType := Node{}
	currentMethod := Node{}
	braceDepth := 0
	typeDepth := -1
	methodDepth := -1

	for idx, raw := range lines {
		lineNo := idx + 1
		trimmed := strings.TrimSpace(raw)

		if m := csUsingRe.FindStringSubmatch(raw); len(m) == 2 {
			usingName := m[1]
			usingNode := makeNode(LanguageCSharp, NodeKindUsing, "using", usingName, usingName, "", root.ID, contextID, Span{
				Path:      file.Path,
				LineStart: lineNo,
				LineEnd:   lineNo,
			})
			usingNode = collector.addNode(usingNode)
			collector.addRelation(Relation{Type: RelationTypeParent, FromID: root.ID, ToID: usingNode.ID, ContextID: contextID, Location: usingNode.Span, Confidence: 1.0})
			collector.addRelation(Relation{Type: RelationTypeImports, FromID: ns.ID, ToID: usingNode.ID, ContextID: contextID, Location: usingNode.Span, Confidence: 1.0})
		}

		if strings.HasPrefix(trimmed, "///") || strings.HasPrefix(trimmed, "//") {
			kind := NodeKindComment
			langKind := "comment"
			if strings.HasPrefix(trimmed, "///") {
				kind = NodeKindDocComment
				langKind = "doc_comment"
			}
			commentNode := makeNode(LanguageCSharp, kind, langKind, truncate(trimmed, 64), "", "", root.ID, contextID, Span{
				Path:      file.Path,
				LineStart: lineNo,
				LineEnd:   lineNo,
			})
			commentNode = collector.addNode(commentNode)
			collector.addRelation(Relation{Type: RelationTypeParent, FromID: root.ID, ToID: commentNode.ID, ContextID: contextID, Location: commentNode.Span, Confidence: 1.0})
		}

		if m := csTypeRe.FindStringSubmatch(trimmed); len(m) >= 3 {
			languageKind := m[1]
			typeName := m[2]
			fqn := namespace + "." + typeName
			typeNode := makeNode(LanguageCSharp, NodeKindType, languageKind, typeName, fqn, "", ns.ID, contextID, Span{
				Path:      file.Path,
				LineStart: lineNo,
				LineEnd:   lineNo,
			})
			typeNode = collector.addNode(typeNode)
			knownTypes[typeName] = typeNode
			collector.addRelation(Relation{Type: RelationTypeParent, FromID: ns.ID, ToID: typeNode.ID, ContextID: contextID, Location: typeNode.Span, Confidence: 1.0})
			collector.addRelation(Relation{Type: RelationTypeDeclares, FromID: ns.ID, ToID: typeNode.ID, ContextID: contextID, Location: typeNode.Span, Confidence: 1.0})
			currentType = typeNode
			typeDepth = braceDepth + strings.Count(raw, "{")

			genericRaw := ""
			if len(m) >= 4 {
				genericRaw = strings.TrimSpace(m[3])
			}
			for _, gp := range parseCSharpGenericParams(genericRaw) {
				gpNode := makeNode(LanguageCSharp, NodeKindGenericParameter, "type_parameter", gp, fqn+"."+gp, "", typeNode.ID, contextID, Span{
					Path:      file.Path,
					LineStart: lineNo,
					LineEnd:   lineNo,
				})
				gpNode = collector.addNode(gpNode)
				collector.addRelation(Relation{Type: RelationTypeParent, FromID: typeNode.ID, ToID: gpNode.ID, ContextID: contextID, Location: gpNode.Span, Confidence: 1.0})
				collector.addRelation(Relation{Type: RelationTypeDeclares, FromID: typeNode.ID, ToID: gpNode.ID, ContextID: contextID, Location: gpNode.Span, Confidence: 1.0})
			}
		}

		if currentType.ID != "" {
			if methodLike(trimmed) {
				if m := csMethodRe.FindStringSubmatch(trimmed); len(m) == 2 {
					methodName := m[1]
					if !isControlKeyword(methodName) {
						fqn := currentType.FQN + "." + methodName
						methodNode := makeNode(LanguageCSharp, NodeKindMethod, "method", methodName, fqn, "()", currentType.ID, contextID, Span{
							Path:      file.Path,
							LineStart: lineNo,
							LineEnd:   lineNo,
						})
						methodNode = collector.addNode(methodNode)
						collector.addRelation(Relation{Type: RelationTypeParent, FromID: currentType.ID, ToID: methodNode.ID, ContextID: contextID, Location: methodNode.Span, Confidence: 1.0})
						collector.addRelation(Relation{Type: RelationTypeDeclares, FromID: currentType.ID, ToID: methodNode.ID, ContextID: contextID, Location: methodNode.Span, Confidence: 1.0})
						knownMethods[methodName] = append(knownMethods[methodName], methodNode)
						currentMethod = methodNode
						methodDepth = braceDepth + strings.Count(raw, "{")
					}
				}
			}

			if m := csPropertyRe.FindStringSubmatch(trimmed); len(m) >= 3 {
				propertyType := m[1]
				propertyName := m[2]
				propertyNode := makeNode(LanguageCSharp, NodeKindProperty, "property", propertyName, currentType.FQN+"."+propertyName, propertyType, currentType.ID, contextID, Span{
					Path:      file.Path,
					LineStart: lineNo,
					LineEnd:   lineNo,
				})
				propertyNode = collector.addNode(propertyNode)
				collector.addRelation(Relation{Type: RelationTypeParent, FromID: currentType.ID, ToID: propertyNode.ID, ContextID: contextID, Location: propertyNode.Span, Confidence: 1.0})
				collector.addRelation(Relation{Type: RelationTypeDeclares, FromID: currentType.ID, ToID: propertyNode.ID, ContextID: contextID, Location: propertyNode.Span, Confidence: 1.0})
			}

			if m := csFieldRe.FindStringSubmatch(trimmed); len(m) >= 3 {
				fieldType := m[1]
				fieldName := m[2]
				if !isModifierOrKeyword(fieldType) {
					fieldNode := makeNode(LanguageCSharp, NodeKindField, "field", fieldName, currentType.FQN+"."+fieldName, fieldType, currentType.ID, contextID, Span{
						Path:      file.Path,
						LineStart: lineNo,
						LineEnd:   lineNo,
					})
					fieldNode = collector.addNode(fieldNode)
					collector.addRelation(Relation{Type: RelationTypeParent, FromID: currentType.ID, ToID: fieldNode.ID, ContextID: contextID, Location: fieldNode.Span, Confidence: 1.0})
					collector.addRelation(Relation{Type: RelationTypeDeclares, FromID: currentType.ID, ToID: fieldNode.ID, ContextID: contextID, Location: fieldNode.Span, Confidence: 1.0})
				}
			}
		}

		if currentMethod.ID != "" {
			for _, call := range csCallRe.FindAllStringSubmatch(trimmed, -1) {
				if len(call) != 2 {
					continue
				}
				name := call[1]
				if isControlKeyword(name) {
					continue
				}
				targets := knownMethods[name]
				if len(targets) == 0 {
					continue
				}
				confidence := 0.6
				if len(targets) == 1 {
					confidence = 0.9
				}
				for _, target := range targets {
					collector.addRelation(Relation{Type: RelationTypeCalls, FromID: currentMethod.ID, ToID: target.ID, ContextID: contextID, Location: Span{Path: file.Path, LineStart: lineNo, LineEnd: lineNo}, Confidence: confidence})
				}
			}

			for typeName, typeNode := range knownTypes {
				if strings.Contains(trimmed, typeName) {
					collector.addRelation(Relation{Type: RelationTypeReference, FromID: currentMethod.ID, ToID: typeNode.ID, ContextID: contextID, Location: Span{Path: file.Path, LineStart: lineNo, LineEnd: lineNo}, Confidence: 0.55})
				}
			}
		}

		braceDepth += strings.Count(raw, "{")
		braceDepth -= strings.Count(raw, "}")

		if methodDepth >= 0 && braceDepth < methodDepth {
			currentMethod = Node{}
			methodDepth = -1
		}
		if typeDepth >= 0 && braceDepth < typeDepth {
			currentType = Node{}
			typeDepth = -1
		}
	}

	return collector.result(), nil
}

func parseCSharpGenericParams(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	raw = strings.TrimPrefix(raw, "<")
	raw = strings.TrimSuffix(raw, ">")
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		name := strings.TrimSpace(p)
		if name == "" {
			continue
		}
		out = append(out, name)
	}
	return out
}

func methodLike(line string) bool {
	if !strings.Contains(line, "(") || !strings.Contains(line, ")") {
		return false
	}
	if strings.Contains(line, "=>") {
		return true
	}
	if strings.HasPrefix(line, "if ") || strings.HasPrefix(line, "for ") || strings.HasPrefix(line, "while ") || strings.HasPrefix(line, "switch ") {
		return false
	}
	if strings.HasPrefix(line, "return ") {
		return false
	}
	return true
}

func isControlKeyword(name string) bool {
	switch name {
	case "if", "for", "foreach", "while", "switch", "catch", "lock", "using", "return", "new", "nameof":
		return true
	default:
		return false
	}
}

func isModifierOrKeyword(token string) bool {
	switch token {
	case "public", "private", "protected", "internal", "static", "readonly", "volatile", "const", "new":
		return true
	default:
		return false
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
