package astindex

import (
	"reflect"
	"testing"
)

func TestIndexEmptyInput(t *testing.T) {
	result, diags, err := Index(nil, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics, got %d", len(diags))
	}
	if result.SchemaVersion == "" {
		t.Fatal("expected schema version")
	}
}

func TestIndexValidation(t *testing.T) {
	_, _, err := Index([]SourceFile{{Path: "a.go"}}, Options{})
	if err == nil {
		t.Fatal("expected error for missing content")
	}

	_, _, err = Index([]SourceFile{{Content: "package p"}}, Options{})
	if err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestIndexGoExtraction(t *testing.T) {
	goFile := `package sample

import "fmt"

// Service processes values.
type Service[T any] struct {
	Name string
}

func (s *Service[T]) Run(v T) {
	fmt.Println(v)
	Helper()
}

func Helper() {
	_ = Service[int]{}
}
`

	result, diags, err := Index([]SourceFile{{Path: "sample.go", Content: goFile}}, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	if len(result.Files) != 1 {
		t.Fatalf("expected one file result, got %d", len(result.Files))
	}

	file := result.Files[0]
	assertHasKind(t, file.Nodes, NodeKindFileRoot)
	assertHasKind(t, file.Nodes, NodeKindPackage)
	assertHasKind(t, file.Nodes, NodeKindImport)
	assertHasKind(t, file.Nodes, NodeKindType)
	assertHasKind(t, file.Nodes, NodeKindMethod)
	assertHasKind(t, file.Nodes, NodeKindFunction)
	assertHasKind(t, file.Nodes, NodeKindField)
	assertHasKind(t, file.Nodes, NodeKindDocComment)
	assertHasKind(t, file.Nodes, NodeKindGenericParameter)

	assertHasRelation(t, file.Relations, RelationTypeDeclares)
	assertHasRelation(t, file.Relations, RelationTypeImports)
	assertHasRelation(t, file.Relations, RelationTypeCalls)
}

func TestIndexCSharpExtraction(t *testing.T) {
	csFile := `using System;

namespace Demo.App;

/// <summary>Greeter</summary>
public class Greeter<T>
{
	private string _name;
	public string Name { get; set; }

	public void Hello()
	{
		Console.WriteLine(Name);
	}
}
`

	result, diags, err := Index([]SourceFile{{Path: "greeter.cs", Content: csFile}}, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	if len(result.Files) != 1 {
		t.Fatalf("expected one file result, got %d", len(result.Files))
	}

	file := result.Files[0]
	assertHasKind(t, file.Nodes, NodeKindFileRoot)
	assertHasKind(t, file.Nodes, NodeKindNamespace)
	assertHasKind(t, file.Nodes, NodeKindUsing)
	assertHasKind(t, file.Nodes, NodeKindType)
	assertHasKind(t, file.Nodes, NodeKindMethod)
	assertHasKind(t, file.Nodes, NodeKindProperty)
	assertHasKind(t, file.Nodes, NodeKindField)
	assertHasKind(t, file.Nodes, NodeKindDocComment)
	assertHasKind(t, file.Nodes, NodeKindGenericParameter)

	assertHasRelation(t, file.Relations, RelationTypeDeclares)
	assertHasRelation(t, file.Relations, RelationTypeImports)
	assertHasRelation(t, file.Relations, RelationTypeCalls)
}

func TestIndexDeterministicOutput(t *testing.T) {
	files := []SourceFile{
		{Path: "b.go", Content: "package b\nfunc B() {}\n"},
		{Path: "a.go", Content: "package a\nfunc A() { B() }\n"},
	}

	r1, d1, err := Index(files, Options{})
	if err != nil {
		t.Fatalf("unexpected error on first run: %v", err)
	}
	r2, d2, err := Index(files, Options{})
	if err != nil {
		t.Fatalf("unexpected error on second run: %v", err)
	}

	if !reflect.DeepEqual(d1, d2) {
		t.Fatalf("diagnostics differ between runs\nfirst=%+v\nsecond=%+v", d1, d2)
	}
	if !reflect.DeepEqual(r1, r2) {
		t.Fatalf("results differ between runs\nfirst=%+v\nsecond=%+v", r1, r2)
	}
}

func assertHasKind(t *testing.T, nodes []Node, kind NodeKind) {
	t.Helper()
	for _, n := range nodes {
		if n.Kind == kind {
			return
		}
	}
	t.Fatalf("expected node kind %q", kind)
}

func assertHasRelation(t *testing.T, rels []Relation, kind RelationType) {
	t.Helper()
	for _, r := range rels {
		if r.Type == kind {
			return
		}
	}
	t.Fatalf("expected relation type %q", kind)
}
