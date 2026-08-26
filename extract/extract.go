// Package extract derives content edges — the relationships that live inside
// files rather than in the directory tree. Structural edges tell you where a
// file sits; content edges tell you what it actually depends on.
//
// Every extractor is pure Go. No cgo, no external parsers: Go files go through
// go/parser, everything else through a lightweight scan of the source text.
package extract

import (
	"os"
	"path/filepath"
	"strings"

	"grafux/scanner"
)

// Strategy says how a reference's literal specifier should be turned into a
// node ID. Extractors pick the strategy; the resolver applies it.
type Strategy int

const (
	// RelPath resolves the spec relative to the referencing file's directory.
	RelPath Strategy = iota
	// GoPkg resolves a Go import path against the module paths found in go.mod.
	GoPkg
	// PyModule resolves a dotted Python module path, honoring leading dots.
	PyModule
	// Basename matches the spec against file basenames anywhere in the tree.
	Basename
)

// Ref is one unresolved outbound reference found in a file.
type Ref struct {
	Spec     string   // the specifier exactly as written in the source
	Kind     string   // edge type: "import" or "reference"
	Line     int      // 1-indexed line the reference was found on
	Strategy Strategy // how to resolve Spec
	TryExts  []string // extensions to append when Spec has none
}

// FileContext describes the file an extractor is reading.
type FileContext struct {
	ID   string // node ID (slash-separated path relative to root)
	Name string // base name
	Dir  string // node ID of the containing directory ("." for root)
	Root string // absolute path of the scan root
}

// An Extractor pulls outbound references from one file's contents.
type Extractor interface {
	Exts() []string
	Refs(src []byte, fc FileContext) []Ref
}

var extractors = []Extractor{
	goExtractor{},
	markdownExtractor{},
	jsExtractor{},
	pythonExtractor{},
}

// byExt indexes the registered extractors by lowercase file extension.
var byExt = func() map[string]Extractor {
	m := map[string]Extractor{}
	for _, e := range extractors {
		for _, ext := range e.Exts() {
			m[ext] = e
		}
	}
	return m
}()

// maxFileSize caps how much of a file we will read. Anything larger is almost
// certainly generated or vendored, and parsing it is not worth the stall.
const maxFileSize = 2 << 20 // 2 MiB

// Stats reports what a content pass found.
type Stats struct {
	FilesRead  int `json:"filesRead"`
	Refs       int `json:"refs"`
	Resolved   int `json:"resolved"`
	Unresolved int `json:"unresolved"`
	Duplicate  int `json:"duplicate"` // resolved, but the edge already existed
}

// Run reads every file node the registered extractors understand, resolves the
// references it finds against the graph, and appends the resulting content
// edges to g. Duplicate edges collapse into one.
func Run(g *scanner.Graph) Stats {
	var st Stats
	idx := newIndex(g)

	// Existing edges, so a content edge never duplicates a structural one.
	seen := map[string]bool{}
	for _, e := range g.Edges {
		seen[e.Source+"\x00"+e.Target+"\x00"+e.Type] = true
	}

	for _, n := range g.Nodes {
		if n.Type != "file" {
			continue
		}
		ex, ok := byExt[strings.ToLower(n.Extension)]
		if !ok {
			continue
		}
		info, err := os.Stat(n.Path)
		if err != nil || info.Size() > maxFileSize {
			continue
		}
		src, err := os.ReadFile(n.Path)
		if err != nil {
			continue
		}
		st.FilesRead++

		fc := FileContext{
			ID:   n.ID,
			Name: n.Name,
			Dir:  parentID(n.ID),
			Root: g.Meta.Root,
		}

		for _, ref := range ex.Refs(src, fc) {
			st.Refs++
			target, conf, ok := idx.resolve(ref, fc)
			if !ok || target == n.ID {
				st.Unresolved++
				continue
			}
			key := n.ID + "\x00" + target + "\x00" + ref.Kind
			if seen[key] {
				st.Duplicate++
				continue
			}
			seen[key] = true
			st.Resolved++
			g.Edges = append(g.Edges, scanner.Edge{
				Source:     n.ID,
				Target:     target,
				Type:       ref.Kind,
				Confidence: conf,
				Line:       ref.Line,
			})
		}
	}

	g.Meta.ContentEdges = st.Resolved
	return st
}

// parentID returns the node ID of the directory containing id.
func parentID(id string) string {
	d := filepath.ToSlash(filepath.Dir(id))
	if d == "" {
		return "."
	}
	return d
}
