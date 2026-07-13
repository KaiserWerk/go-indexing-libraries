package symbolindex

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
)

func makeStableKey(language Language, kind SymbolKind, fqn, signature string) string {
	normalized := fmt.Sprintf("%s|%s|%s|%s", language, kind, strings.TrimSpace(strings.ToLower(fqn)), canonicalSignature(signature))
	sum := sha1.Sum([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

func canonicalSignature(signature string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(strings.ToLower(signature))), " ")
}
