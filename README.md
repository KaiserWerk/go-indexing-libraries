# Go Indexing Libraries

This repository contains two small Go libraries for indexing source files into structured metadata:

- `symbolindex`: extracts a symbol graph from source files (`package`, `type`, `function`, `method`, references, calls, edges)
- `astindex`: produces a per-file normalized AST model suitable for later storage, querying, and LLM-friendly consumption

Both libraries follow the same core principle:

- They accept source text as input
- They do not scan folders or files on disk
- They return structured data, not a filesystem or database layer
- They are designed to be kept simple, deterministic, and easy to persist later

---

## Package overview

### `symbolindex`

`symbolindex` is a symbol-level indexing library. It analyzes caller-provided source text and returns a storage-agnostic graph representation of declarations and relationships.

Typical use cases:

- identify exported and internal symbols
- track type declarations
- detect method ownership
- collect call relationships
- prepare symbol metadata for downstream storage or search

### `astindex`

`astindex` is a file-level AST indexing library. It parses provided source content and returns normalized AST nodes and relationships for each file.

Typical use cases:

- capture structure of Go or C# files
- store AST metadata in a DB later
- build AI-friendly file summaries
- support downstream semantic indexing without raw token consumption

---

## Installation

This repository is a Go module:

```bash
go get github.com/KaiserWerk/go-indexing-libraries
```

Then import the packages as needed:

```go
import (
    "github.com/KaiserWerk/go-indexing-libraries/astindex"
    "github.com/KaiserWerk/go-indexing-libraries/symbolindex"
)
```

---

## `symbolindex` usage

### Input model

```go
file := symbolindex.SourceFile{
    Path:     "example.go",
    Content:  "package demo\n\nfunc Hello(name string) string { return \"hi \" + name }\n",
    Language: symbolindex.LanguageGo,
}
```

### Basic example

```go
package main

import (
    "fmt"
    "github.com/KaiserWerk/go-indexing-libraries/symbolindex"
)

func main() {
    files := []symbolindex.SourceFile{
        {
            Path:     "example.go",
            Content:  "package demo\n\nfunc Hello(name string) string { return \"hi \" + name }\n",
            Language: symbolindex.LanguageGo,
        },
    }

    result, diagnostics, err := symbolindex.Index(files, symbolindex.Options{
        DefaultContextID: "demo",
    })
    if err != nil {
        panic(err)
    }

    fmt.Println("symbols:", len(result.Symbols))
    fmt.Println("edges:", len(result.Edges))
    fmt.Printf("diagnostics: %#v\n", diagnostics)
}
```

### Output shape

`symbolindex.Index(...)` returns:

```go
type Result struct {
    Symbols    []Symbol
    Edges      []Edge
    References []Reference
    Calls      []Call
}
```

Important fields include:

- `Symbol`: name, FQN, kind, signature, declarations, context
- `Edge`: typed relation between symbols
- `Reference`: symbol usage site
- `Call`: call location and candidate targets

### Notes

- `Path` and `Content` are required for each file
- `Language` is optional if the file extension is recognized
- unsupported languages are skipped with a warning diagnostic
- test and generated files can be filtered using `Options`

---

## `astindex` usage

### Input model

```go
file := astindex.SourceFile{
    Path:     "example.go",
    Content:  "package demo\n\ntype Greeter struct{}\nfunc (g Greeter) Say() { }\n",
    Language: astindex.LanguageGo,
}
```

### Basic example

```go
package main

import (
    "fmt"
    "github.com/KaiserWerk/go-indexing-libraries/astindex"
)

func main() {
    files := []astindex.SourceFile{
        {
            Path:     "example.go",
            Content:  "package demo\n\ntype Greeter struct{}\nfunc (g Greeter) Say() { }\n",
            Language: astindex.LanguageGo,
        },
    }

    result, diagnostics, err := astindex.Index(files, astindex.Options{
        DefaultContextID: "demo",
    })
    if err != nil {
        panic(err)
    }

    fmt.Println("schema:", result.SchemaVersion)
    fmt.Println("file count:", len(result.Files))
    fmt.Printf("diagnostics: %#v\n", diagnostics)
}
```

### Output shape

`astindex.Index(...)` returns:

```go
type Result struct {
    SchemaVersion string
    Files         []FileAST
}
```

Each `FileAST` contains:

```go
type FileAST struct {
    Path       string
    Language   Language
    ContextID  string
    RootNodeID string
    Nodes      []Node
    Relations  []Relation
}
```

The AST is normalized and flat by design:

- `Node` = declarative unit, e.g. package, type, method, field, comment, generic parameter
- `Relation` = relationship between nodes, e.g. `parent`, `declares`, `imports`, `calls`
- `Node.ID` is a stable identifier for later persistence and deduplication

### Notes

- `astindex` is intentionally file-oriented and does not read the filesystem
- it works with raw file text supplied by the caller
- the goal is to normalize source structure for storage, queries, or AI consumption
- it is designed to be later extended without changing the bulk of the external contract

---

## Language support and intent

Supported languages in the current implementation:

- Go
- C#

The libraries are intentionally conservative and storage-agnostic. They do not implement:

- directory discovery
- repository traversal
- DB persistence
- LLM wrappers
- token counting or runtime execution logic

Instead, they focus on one job: turn input source into structured, readable, and persistable metadata.

---

## Recommended usage pattern

Use the libraries as a pipeline:

1. read files into memory
2. pass them as `SourceFile` values
3. run `symbolindex` for symbol graph extraction or `astindex` for AST extraction
4. persist the result in your own database or application layer
5. query or summarize the result downstream

This keeps the parsing layer decoupled from storage and file-system concerns.

---

## Example: both libraries together

```go
package main

import (
    "fmt"
    "github.com/KaiserWerk/go-indexing-libraries/astindex"
    "github.com/KaiserWerk/go-indexing-libraries/symbolindex"
)

func main() {
    files := []symbolindex.SourceFile{{
        Path:     "demo.go",
        Content:  "package demo\n\ntype Greeter struct{}\nfunc (g Greeter) Say() { }\n",
        Language: symbolindex.LanguageGo,
    }}

    symRes, _, _ := symbolindex.Index(files, symbolindex.Options{DefaultContextID: "demo"})
    astRes, _, _ := astindex.Index([]astindex.SourceFile{{
        Path:     "demo.go",
        Content:  "package demo\n\ntype Greeter struct{}\nfunc (g Greeter) Say() { }\n",
        Language: astindex.LanguageGo,
    }}, astindex.Options{DefaultContextID: "demo"})

    fmt.Println("symbol count:", len(symRes.Symbols))
    fmt.Println("ast file count:", len(astRes.Files))
}
```

---

## Best practices

- Prefer passing `Path` and `Content` explicitly for deterministic indexing
- Keep `Language` set when the extension is ambiguous or custom
- Use `ContextID` to separate different repository or project scopes
- Filter test/generated files early through `Options`
- Persist the results in your own storage layer rather than in the parser itself

---

## Summary

These libraries are intentionally small and focused:

- `symbolindex` answers: “What symbols and relationships exist in this source?”
- `astindex` answers: “What AST-like structure exists in these files?”

Both are designed to produce structured output that is easy to store, query, and later reason about in a larger indexing or AI pipeline.
