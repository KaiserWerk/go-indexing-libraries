package symbolindex

import (
	"path/filepath"
	"strings"
)

func detectLanguage(file SourceFile, opts Options) Language {
	if file.Language != "" && file.Language != LanguageUnknown {
		return file.Language
	}
	if len(opts.LanguageOverrides) > 0 {
		if l, ok := opts.LanguageOverrides[file.Path]; ok {
			return l
		}
	}
	ext := strings.ToLower(filepath.Ext(file.Path))
	switch ext {
	case ".go":
		return LanguageGo
	case ".cs":
		return LanguageCSharp
	default:
		return LanguageUnknown
	}
}
