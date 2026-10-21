package symbolindex

import (
	"errors"
	"sort"
	"strconv"
	"strings"
)

var errEmptySearchQuery = errors.New("invalid input: search query is required")

// SearchSummary captures a compact, human-readable view of relevant files and symbol relationships.
type SearchSummary struct {
	Query         string
	RelevantFiles []RelevantFile
	Relationships []SymbolRelationship
}

// RelevantFile describes one file in a search summary.
type RelevantFile struct {
	Path        string
	Symbols     []string
	TextMatches int
}

// SymbolRelationship captures one directed relationship between symbols.
type SymbolRelationship struct {
	From string
	To   string
}

// BuildSearchSummary builds a report-friendly summary for query-driven symbol exploration.
func BuildSearchSummary(query string, files []SourceFile, index Result) (SearchSummary, error) {
	needle := strings.TrimSpace(strings.ToLower(query))
	if needle == "" {
		return SearchSummary{}, errEmptySearchQuery
	}

	type fileMeta struct {
		textMatches int
		symbols     map[string]int
		relevant    bool
	}

	metaByPath := make(map[string]*fileMeta, len(files))
	for _, file := range files {
		matches := countTextMatches(file.Content, needle)
		metaByPath[file.Path] = &fileMeta{
			textMatches: matches,
			symbols:     make(map[string]int, 8),
			relevant:    matches > 0 || strings.Contains(strings.ToLower(file.Path), needle),
		}
	}

	symbolsByID := make(map[string]Symbol, len(index.Symbols))
	symbolFileByID := make(map[string]string, len(index.Symbols))

	for _, symbol := range index.Symbols {
		symbolsByID[symbol.ID] = symbol
		path, line := symbolPrimaryLocation(symbol)
		if path == "" {
			continue
		}
		symbolFileByID[symbol.ID] = path
		meta, ok := metaByPath[path]
		if !ok {
			meta = &fileMeta{symbols: make(map[string]int, 8)}
			metaByPath[path] = meta
		}
		if symbol.Kind == SymbolKindPackage || symbol.Kind == SymbolKindNamespace {
			continue
		}
		label := displaySymbolName(symbol)
		if _, exists := meta.symbols[label]; !exists {
			meta.symbols[label] = line
		} else if line < meta.symbols[label] {
			meta.symbols[label] = line
		}
		if !meta.relevant && symbolMatchesQuery(symbol, needle) {
			meta.relevant = true
		}
	}

	relevantPaths := make(map[string]struct{}, len(metaByPath))
	relevantFiles := make([]RelevantFile, 0, len(metaByPath))
	for path, meta := range metaByPath {
		if !meta.relevant {
			continue
		}
		relevantPaths[path] = struct{}{}

		symbols := make([]string, 0, len(meta.symbols))
		for symbol := range meta.symbols {
			symbols = append(symbols, symbol)
		}
		sort.Slice(symbols, func(i, j int) bool {
			leftLine := meta.symbols[symbols[i]]
			rightLine := meta.symbols[symbols[j]]
			if leftLine == rightLine {
				return symbols[i] < symbols[j]
			}
			return leftLine < rightLine
		})

		relevantFiles = append(relevantFiles, RelevantFile{
			Path:        path,
			Symbols:     symbols,
			TextMatches: meta.textMatches,
		})
	}

	sort.Slice(relevantFiles, func(i, j int) bool {
		if relevantFiles[i].TextMatches == relevantFiles[j].TextMatches {
			return relevantFiles[i].Path < relevantFiles[j].Path
		}
		return relevantFiles[i].TextMatches > relevantFiles[j].TextMatches
	})

	relationshipSeen := make(map[string]struct{}, len(index.Edges))
	relationships := make([]SymbolRelationship, 0, len(index.Edges))
	for _, edge := range index.Edges {
		if edge.Type != EdgeTypeCalls {
			continue
		}

		fromPath := symbolFileByID[edge.FromID]
		toPath := symbolFileByID[edge.ToID]
		if fromPath == "" || toPath == "" {
			continue
		}
		if _, ok := relevantPaths[fromPath]; !ok {
			continue
		}
		if _, ok := relevantPaths[toPath]; !ok {
			continue
		}

		fromSymbol, okFrom := symbolsByID[edge.FromID]
		toSymbol, okTo := symbolsByID[edge.ToID]
		if !okFrom || !okTo {
			continue
		}
		from := displaySymbolName(fromSymbol)
		to := displaySymbolName(toSymbol)
		key := from + "|" + to
		if _, seen := relationshipSeen[key]; seen {
			continue
		}
		relationshipSeen[key] = struct{}{}
		relationships = append(relationships, SymbolRelationship{From: from, To: to})
	}

	sort.Slice(relationships, func(i, j int) bool {
		if relationships[i].From == relationships[j].From {
			return relationships[i].To < relationships[j].To
		}
		return relationships[i].From < relationships[j].From
	})

	return SearchSummary{
		Query:         query,
		RelevantFiles: relevantFiles,
		Relationships: relationships,
	}, nil
}

// FormatSearchSummary renders a SearchSummary in a plain-text format suitable for CLI output.
func FormatSearchSummary(summary SearchSummary) string {
	var b strings.Builder
	b.WriteString("Search: \"")
	b.WriteString(summary.Query)
	b.WriteString("\"\n\n")
	b.WriteString("Relevant files:\n\n")

	for i, file := range summary.RelevantFiles {
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteString(". ")
		b.WriteString(file.Path)
		b.WriteString("\n")
		b.WriteString("   Symbols: ")
		if len(file.Symbols) == 0 {
			b.WriteString("(none)")
		} else {
			b.WriteString(strings.Join(file.Symbols, ", "))
		}
		b.WriteString("\n")
		b.WriteString("   Text matches: ")
		b.WriteString(strconv.Itoa(file.TextMatches))
		b.WriteString("\n\n")
	}

	b.WriteString("Relationships:\n")
	if len(summary.Relationships) == 0 {
		b.WriteString("  (none)\n")
		return b.String()
	}

	currentFrom := ""
	for _, rel := range summary.Relationships {
		if rel.From != currentFrom {
			if currentFrom != "" {
				b.WriteString("\n")
			}
			currentFrom = rel.From
			b.WriteString("  ")
			b.WriteString(rel.From)
			b.WriteString("\n")
		}
		b.WriteString("    -> ")
		b.WriteString(rel.To)
		b.WriteString("\n")
	}

	return b.String()
}

func symbolPrimaryLocation(symbol Symbol) (path string, line int) {
	if len(symbol.Declarations) == 0 {
		return "", 0
	}
	decl := symbol.Declarations[0]
	return decl.Path, decl.LineStart
}

func displaySymbolName(symbol Symbol) string {
	if symbol.Kind == SymbolKindMethod && symbol.ContainerFQN != "" && symbol.Name != "" {
		return lastSegment(symbol.ContainerFQN) + "." + symbol.Name
	}
	if symbol.Name != "" {
		return symbol.Name
	}
	return symbol.FQN
}

func lastSegment(fqn string) string {
	if fqn == "" {
		return ""
	}
	parts := strings.Split(fqn, ".")
	return parts[len(parts)-1]
}

func symbolMatchesQuery(symbol Symbol, queryLower string) bool {
	name := strings.ToLower(symbol.Name)
	fqn := strings.ToLower(symbol.FQN)
	container := strings.ToLower(symbol.ContainerFQN)
	return strings.Contains(name, queryLower) || strings.Contains(fqn, queryLower) || strings.Contains(container, queryLower)
}

func countTextMatches(content string, queryLower string) int {
	if queryLower == "" {
		return 0
	}
	return strings.Count(strings.ToLower(content), queryLower)
}
