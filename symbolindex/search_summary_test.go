package symbolindex

import (
	"strings"
	"testing"
)

func TestBuildSearchSummaryAndFormat(t *testing.T) {
	files := []SourceFile{
		{
			Path: "internal/app/auth/service.go",
			Content: `package auth
type Service struct{}
func (s *Service) AuthenticateRequest() {}
func (s *Service) LoginWithPassword() {}
`,
		},
		{
			Path: "internal/adapters/web/auth_handlers.go",
			Content: `package auth
func DoPasswordLogin(s *Service) {
	s.LoginWithPassword()
}
`,
		},
		{
			Path: "internal/middleware/auth.go",
			Content: `package auth
func AuthMiddleware(s *Service) {
	s.AuthenticateRequest()
}
`,
		},
	}

	index, diagnostics, err := Index(files, Options{})
	if err != nil {
		t.Fatalf("unexpected index error: %v", err)
	}
	if len(diagnostics) > 0 {
		t.Fatalf("expected no diagnostics, got %+v", diagnostics)
	}

	summary, err := BuildSearchSummary("auth", files, index)
	if err != nil {
		t.Fatalf("unexpected search summary error: %v", err)
	}
	if summary.Query != "auth" {
		t.Fatalf("expected query to be preserved, got %q", summary.Query)
	}
	if len(summary.RelevantFiles) != 3 {
		t.Fatalf("expected 3 relevant files, got %d", len(summary.RelevantFiles))
	}

	hasRelationship := false
	for _, rel := range summary.Relationships {
		if rel.From == "DoPasswordLogin" && rel.To == "Service.LoginWithPassword" {
			hasRelationship = true
			break
		}
	}
	if !hasRelationship {
		t.Fatalf("expected relationship DoPasswordLogin -> Service.LoginWithPassword, got %+v", summary.Relationships)
	}

	rendered := FormatSearchSummary(summary)
	if !strings.Contains(rendered, `Search: "auth"`) {
		t.Fatalf("rendered output missing query:\n%s", rendered)
	}
	if !strings.Contains(rendered, "internal/app/auth/service.go") {
		t.Fatalf("rendered output missing service file:\n%s", rendered)
	}
	if !strings.Contains(rendered, "DoPasswordLogin") {
		t.Fatalf("rendered output missing caller symbol:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Service.LoginWithPassword") {
		t.Fatalf("rendered output missing callee symbol:\n%s", rendered)
	}
}

func TestBuildSearchSummaryRequiresQuery(t *testing.T) {
	_, err := BuildSearchSummary("   ", nil, Result{})
	if err == nil {
		t.Fatal("expected error for empty query")
	}
}
