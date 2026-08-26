package extract

import (
	"os"
	"path/filepath"
	"testing"

	"grafux/scanner"
)

// write lays out a temporary project from a path -> contents map.
func write(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		full := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// contentEdges runs a full scan plus extraction and returns "src -> tgt" keys.
func contentEdges(t *testing.T, root string) map[string]scanner.Edge {
	t.Helper()
	g, err := scanner.Scan(root, scanner.Options{})
	if err != nil {
		t.Fatal(err)
	}
	Run(g)
	out := map[string]scanner.Edge{}
	for _, e := range g.Edges {
		if e.Type != "structural" {
			out[e.Source+" -> "+e.Target] = e
		}
	}
	return out
}

func TestGoImportsResolveToPackageDirs(t *testing.T) {
	root := write(t, map[string]string{
		"go.mod":        "module example.com/demo\n\ngo 1.21\n",
		"main.go":       "package main\n\nimport (\n\t\"fmt\"\n\t\"example.com/demo/pkg/auth\"\n)\n\nfunc main() { fmt.Println(auth.X) }\n",
		"pkg/auth/a.go": "package auth\n\nvar X = 1\n",
	})
	edges := contentEdges(t, root)

	e, ok := edges["main.go -> pkg/auth"]
	if !ok {
		t.Fatalf("missing edge to package dir; got %v", keys(edges))
	}
	if e.Type != "import" || e.Confidence != Extracted {
		t.Errorf("got type=%q confidence=%q, want import/extracted", e.Type, e.Confidence)
	}
	if e.Line != 5 {
		t.Errorf("got line %d, want 5", e.Line)
	}
	// "fmt" is external and must not become an edge.
	if len(edges) != 1 {
		t.Errorf("expected only the local import, got %v", keys(edges))
	}
}

func TestJSResolutionSkipsCommentsAndPackages(t *testing.T) {
	root := write(t, map[string]string{
		"a.ts":        "import x from './lib/util';\n// import dead from './gone';\nimport 'react';\nconst y = require('./lib/util');\n",
		"lib/util.ts": "export default 1;\n",
	})
	edges := contentEdges(t, root)

	if _, ok := edges["a.ts -> lib/util.ts"]; !ok {
		t.Fatalf("missing relative import; got %v", keys(edges))
	}
	if len(edges) != 1 {
		t.Errorf("commented-out and package imports must not become edges; got %v", keys(edges))
	}
}

func TestJSIndexFallback(t *testing.T) {
	root := write(t, map[string]string{
		"a.js":         "const m = require('./mod');\n",
		"mod/index.js": "module.exports = {};\n",
	})
	if _, ok := contentEdges(t, root)["a.js -> mod/index.js"]; !ok {
		t.Fatal("a directory import should resolve to its index file")
	}
}

func TestPythonRelativeAndAbsoluteImports(t *testing.T) {
	root := write(t, map[string]string{
		"api/handlers/h.py":      "import os\nfrom ..store import cache\nfrom .helper import shape\nimport pkg.policy as p\n",
		"api/store.py":           "cache = {}\n",
		"api/handlers/helper.py": "def shape(x): return x\n",
		"pkg/policy.py":          "ALLOW = True\n",
	})
	edges := contentEdges(t, root)

	for _, want := range []string{
		"api/handlers/h.py -> api/store.py",
		"api/handlers/h.py -> api/handlers/helper.py",
		"api/handlers/h.py -> pkg/policy.py",
	} {
		if _, ok := edges[want]; !ok {
			t.Errorf("missing %q; got %v", want, keys(edges))
		}
	}
	if len(edges) != 3 {
		t.Errorf("stdlib `os` must not become an edge; got %v", keys(edges))
	}
}

func TestMarkdownLinksAndFencedCodeIsIgnored(t *testing.T) {
	root := write(t, map[string]string{
		"docs/a.md":   "See [[b]] and [code](../src/main.go).\n\n```\n[[never]] and [x](./nope.md)\n```\n[web](https://example.com)\n",
		"docs/b.md":   "# B\n",
		"src/main.go": "package main\n",
	})
	edges := contentEdges(t, root)

	if e, ok := edges["docs/a.md -> docs/b.md"]; !ok {
		t.Errorf("wikilink did not resolve; got %v", keys(edges))
	} else if e.Type != "reference" {
		t.Errorf("got type %q, want reference", e.Type)
	}
	if _, ok := edges["docs/a.md -> src/main.go"]; !ok {
		t.Errorf("relative markdown link did not resolve; got %v", keys(edges))
	}
	if len(edges) != 2 {
		t.Errorf("fenced code and external URLs must not become edges; got %v", keys(edges))
	}
}

func TestAmbiguousWikilinkIsMarkedInferred(t *testing.T) {
	root := write(t, map[string]string{
		"a/note.md": "# A\n",
		"b/note.md": "# B\n",
		"index.md":  "See [[note]].\n",
	})
	edges := contentEdges(t, root)
	if len(edges) != 1 {
		t.Fatalf("expected one edge, got %v", keys(edges))
	}
	for _, e := range edges {
		if e.Confidence != Inferred {
			t.Errorf("a name matching two files should be inferred, got %q", e.Confidence)
		}
	}
}

func keys(m map[string]scanner.Edge) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
