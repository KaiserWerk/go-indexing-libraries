package astindex

import (
	"errors"
	"sort"
)

const schemaVersionV1 = "1.0"

// Index parses caller-provided files and returns per-file normalized AST data.
func Index(files []SourceFile, opts Options) (Result, []Diagnostic, error) {
	if len(files) == 0 {
		return Result{SchemaVersion: schemaVersionV1, Files: nil}, nil, nil
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

	out := make([]FileAST, 0, len(files))
	diagnostics := make([]Diagnostic, 0, 64)

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
			fileResult, diags := analyzeGoFile(f, contextID)
			out = append(out, fileResult)
			diagnostics = append(diagnostics, diags...)
		case LanguageCSharp:
			fileResult, diags := analyzeCSharpFile(f, contextID)
			out = append(out, fileResult)
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

	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })

	return Result{SchemaVersion: schemaVersionV1, Files: out}, diagnostics, nil
}
