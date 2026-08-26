package query

import (
	"errors"
	"testing"

	"grafux/scanner"
)

// testGraph is a small project: two markdown notes linking to each other and a
// Go file that main.go imports through its package folder.
func testGraph() *Graph {
	g := &scanner.Graph{
		Nodes: []scanner.Node{
			{ID: ".", Name: "proj", Type: "folder"},
			{ID: "main.go", Name: "main.go", Type: "file", Extension: ".go"},
			{ID: "auth", Name: "auth", Type: "folder"},
			{ID: "auth/token.go", Name: "token.go", Type: "file", Extension: ".go"},
			{ID: "docs", Name: "docs", Type: "folder"},
			{ID: "docs/design.md", Name: "design.md", Type: "file", Extension: ".md"},
			{ID: "notes/design.md", Name: "design.md", Type: "file", Extension: ".md"},
		},
		Edges: []scanner.Edge{
			{Source: ".", Target: "main.go", Type: "structural"},
			{Source: ".", Target: "auth", Type: "structural"},
			{Source: "auth", Target: "auth/token.go", Type: "structural"},
			{Source: ".", Target: "docs", Type: "structural"},
			{Source: "docs", Target: "docs/design.md", Type: "structural"},
			{Source: "main.go", Target: "auth", Type: "import", Confidence: "extracted", Line: 5},
			{Source: "docs/design.md", Target: "main.go", Type: "reference", Confidence: "extracted", Line: 3},
		},
	}
	return New(g)
}

func TestResolveExactAndSuffix(t *testing.T) {
	g := testGraph()

	if n, err := g.Resolve("auth/token.go"); err != nil || n.ID != "auth/token.go" {
		t.Errorf("exact ID: got %q, %v", n.ID, err)
	}
	if n, err := g.Resolve("token.go"); err != nil || n.ID != "auth/token.go" {
		t.Errorf("base name: got %q, %v", n.ID, err)
	}
	if n, err := g.Resolve("./main.go"); err != nil || n.ID != "main.go" {
		t.Errorf("leading ./ should be tolerated: got %q, %v", n.ID, err)
	}
}

func TestResolveAmbiguousListsCandidates(t *testing.T) {
	g := testGraph()

	_, err := g.Resolve("design.md")
	var amb *AmbiguousError
	if !errors.As(err, &amb) {
		t.Fatalf("want AmbiguousError, got %v", err)
	}
	if len(amb.Candidates) != 2 {
		t.Errorf("want both notes listed, got %v", amb.Candidates)
	}
	// The full path disambiguates.
	if n, err := g.Resolve("docs/design.md"); err != nil || n.ID != "docs/design.md" {
		t.Errorf("full path: got %q, %v", n.ID, err)
	}
}

func TestResolveNotFound(t *testing.T) {
	g := testGraph()
	if _, err := g.Resolve("nothing-like-this-exists"); err == nil {
		t.Fatal("want an error for an unmatched reference")
	}
}

func TestShortestPathTraversesEdgesInEitherDirection(t *testing.T) {
	g := testGraph()

	steps, ok := ShortestPath(g, "docs/design.md", "auth/token.go")
	if !ok {
		t.Fatal("nodes should be connected")
	}
	// design.md -> main.go -> auth -> auth/token.go
	if len(steps) != 3 {
		t.Fatalf("want 3 hops, got %d: %+v", len(steps), steps)
	}
	if steps[0].To != "main.go" || steps[1].To != "auth" || steps[2].To != "auth/token.go" {
		t.Errorf("unexpected route: %+v", steps)
	}
	if !steps[0].Forward {
		t.Error("the reference edge points from the note to main.go")
	}
}

func TestShortestPathReportsDisconnected(t *testing.T) {
	g := testGraph()
	if _, ok := ShortestPath(g, "main.go", "notes/design.md"); ok {
		t.Error("an unlinked node should not be reachable")
	}
}

func TestNeighborsContentOnlySkipsTheDirectoryTree(t *testing.T) {
	g := testGraph()

	all := Neighbors(g, "main.go", 1, false)
	content := Neighbors(g, "main.go", 1, true)

	if len(all) <= len(content) {
		t.Errorf("structural edges should add neighbors: all=%d content=%d", len(all), len(content))
	}
	for _, n := range content {
		if !IsContent(n.Via) {
			t.Errorf("content-only walk returned a %q edge", n.Via.Type)
		}
	}
	if len(content) != 2 {
		t.Errorf("want the import out and the reference in, got %d", len(content))
	}
}

func TestExplainSeparatesStructureFromDependencies(t *testing.T) {
	g := testGraph()

	ex, ok := Explain(g, "main.go")
	if !ok {
		t.Fatal("main.go should exist")
	}
	if ex.Parent != "." {
		t.Errorf("want parent '.', got %q", ex.Parent)
	}
	if len(ex.DependsOn) != 1 || ex.DependsOn[0].ID != "auth" {
		t.Errorf("want one dependency on auth, got %+v", ex.DependsOn)
	}
	if len(ex.DependedOn) != 1 || ex.DependedOn[0].ID != "docs/design.md" {
		t.Errorf("want the doc reference back, got %+v", ex.DependedOn)
	}
}

func TestSearchRequiresEveryTerm(t *testing.T) {
	g := testGraph()

	if got := Search(g, "token", 10); len(got) == 0 || got[0].Node.ID != "auth/token.go" {
		t.Errorf("single term: got %+v", got)
	}
	if got := Search(g, "auth token", 10); len(got) != 1 || got[0].Node.ID != "auth/token.go" {
		t.Errorf("both terms must match the same node: got %+v", got)
	}
	if got := Search(g, "token nonexistentterm", 10); len(got) != 0 {
		t.Errorf("an unmatched term should eliminate the node: got %+v", got)
	}
}
