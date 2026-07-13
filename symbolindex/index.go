package symbolindex

import "errors"

// Index parses caller-provided files and returns a storage-agnostic symbol graph.
func Index(files []SourceFile, opts Options) (Result, []Diagnostic, error) {
	if len(files) == 0 {
		return Result{}, nil, nil
	}

	if opts.DefaultContextID == "" && !opts.IncludeGenerated && !opts.IncludeTests && len(opts.LanguageOverrides) == 0 {
		opts = defaultOptions()
	}
	if opts.DefaultContextID == "" {
		opts.DefaultContextID = "default"
	}

	for i := range files {
		if files[i].Path == "" {
			return Result{}, nil, errors.New("invalid input: file path is required")
		}
		if files[i].Content == "" {
			return Result{}, nil, errors.New("invalid input: file content is required")
		}
	}

	collector := newCollector()
	diagnostics := make([]Diagnostic, 0, 50)

	for _, f := range files {
		if !opts.IncludeTests && isTestPath(f.Path) {
			continue
		}
		if !opts.IncludeGenerated && isGenerated(f.Path, f.Content) {
			continue
		}

		contextID := f.ContextID
		if contextID == "" {
			contextID = opts.DefaultContextID
		}

		language := detectLanguage(f, opts)
		switch language {
		case LanguageGo:
			diags := analyzeGoFile(f, contextID, collector)
			diagnostics = append(diagnostics, diags...)
		case LanguageCSharp:
			diags := analyzeCSharpFile(f, contextID, collector)
			diagnostics = append(diagnostics, diags...)
		default:
			diagnostics = append(diagnostics, Diagnostic{
				Level:   DiagnosticLevelWarning,
				Code:    "UNSUPPORTED_LANGUAGE",
				Message: "skipped file with unsupported language",
				Path:    f.Path,
				Line:    1,
			})
		}
	}

	return collector.result(), diagnostics, nil
}
