package astindex

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
)

func makeNodeStableKey(language Language, kind NodeKind, fqn, name, signature string, span Span) string {
	normalized := fmt.Sprintf(
		"%s|%s|%s|%s|%s|%d|%d",
		language,
		kind,
		strings.TrimSpace(strings.ToLower(fqn)),
		strings.TrimSpace(strings.ToLower(name)),
		canonicalSignature(signature),
		span.LineStart,
		span.LineEnd,
	)
	sum := sha1.Sum([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

func canonicalSignature(signature string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(strings.ToLower(signature))), " ")
}
