package symbolindex

import (
	"regexp"
	"strings"
)

var (
	namespaceRe = regexp.MustCompile(`\bnamespace\s+([A-Za-z_][A-Za-z0-9_\.]*)`)
	typeRe      = regexp.MustCompile(`\b(class|interface|record|enum|delegate|struct)\s+([A-Za-z_][A-Za-z0-9_]*)`)
	methodRe    = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
)

func analyzeCSharpFile(file SourceFile, contextID string, c *collector) []Diagnostic {
	lines := strings.Split(file.Content, "\n")
	namespace := "global"

	if m := namespaceRe.FindStringSubmatch(file.Content); len(m) == 2 {
		namespace = m[1]
	}

	ns := makeSymbol(LanguageCSharp, SymbolKindNamespace, "namespace", namespace, namespace, "", "", contextID, Span{
		Path:      file.Path,
		LineStart: 1,
		LineEnd:   max(1, len(lines)),
	})
	ns = c.upsertSymbol(ns)

	currentTypeFQN := ""
	currentTypeID := ""
	braceDepth := 0
	typeDepth := -1

	for idx, raw := range lines {
		lineNo := idx + 1
		line := strings.TrimSpace(raw)
		if line == "" {
			braceDepth += strings.Count(raw, "{")
			braceDepth -= strings.Count(raw, "}")
			if typeDepth >= 0 && braceDepth < typeDepth {
				currentTypeFQN = ""
				currentTypeID = ""
				typeDepth = -1
			}
			continue
		}

		if m := typeRe.FindStringSubmatch(line); len(m) == 3 {
			languageKind := m[1]
			name := m[2]
			fqn := namespace + "." + name
			s := makeSymbol(LanguageCSharp, SymbolKindType, languageKind, name, fqn, "", namespace, contextID, Span{
				Path:      file.Path,
				LineStart: lineNo,
				LineEnd:   lineNo,
			})
			s = c.upsertSymbol(s)
			c.addEdge(Edge{Type: EdgeTypeDeclares, FromID: ns.ID, ToID: s.ID, ContextID: contextID})
			currentTypeFQN = fqn
			currentTypeID = s.ID
			typeDepth = braceDepth + strings.Count(raw, "{")
		}

		if currentTypeFQN != "" {
			if methodLike(line) {
				if m := methodRe.FindStringSubmatch(line); len(m) == 2 {
					methodName := m[1]
					if isControlKeyword(methodName) {
						goto depthUpdate
					}
					fqn := currentTypeFQN + "." + methodName
					sig := "()"
					s := makeSymbol(LanguageCSharp, SymbolKindMethod, "method", methodName, fqn, sig, currentTypeFQN, contextID, Span{
						Path:      file.Path,
						LineStart: lineNo,
						LineEnd:   lineNo,
					})
					s = c.upsertSymbol(s)
					c.addEdge(Edge{Type: EdgeTypeDeclares, FromID: currentTypeID, ToID: s.ID, ContextID: contextID})
				}
			}
		}

	depthUpdate:
		braceDepth += strings.Count(raw, "{")
		braceDepth -= strings.Count(raw, "}")
		if typeDepth >= 0 && braceDepth < typeDepth {
			currentTypeFQN = ""
			currentTypeID = ""
			typeDepth = -1
		}
	}

	return nil
}

func methodLike(line string) bool {
	if !strings.Contains(line, "(") || !strings.Contains(line, ")") {
		return false
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
	case "if", "for", "foreach", "while", "switch", "catch", "lock", "using", "return", "new":
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
