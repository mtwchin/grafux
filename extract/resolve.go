package extract

import (
	"bufio"
	"bytes"
	"os"
	"path"
	"sort"
	"strings"

	"grafux/scanner"
)

// Confidence values mirror the distinction the graph makes to its callers:
// extracted means the source named the target unambiguously, inferred means
// several candidates matched and the resolver chose one.
const (
	Extracted = "extracted"
	Inferred  = "inferred"
)

// index is a lookup table over the scanned graph, built once per content pass.
type index struct {
	kind    map[string]string   // node ID -> "file" | "folder"
	byBase  map[string][]string // lowercase basename -> node IDs
	byStem  map[string][]string // lowercase basename minus extension -> node IDs
	modules []goModule          // Go modules found in the tree, longest path first
}

type goModule struct {
	path string // module path declared in go.mod
	dir  string // node ID of the directory holding that go.mod
}

func newIndex(g *scanner.Graph) *index {
	idx := &index{
		kind:   make(map[string]string, len(g.Nodes)),
		byBase: map[string][]string{},
		byStem: map[string][]string{},
	}
	for _, n := range g.Nodes {
		idx.kind[n.ID] = n.Type
		if n.Type != "file" {
			continue
		}
		base := strings.ToLower(n.Name)
		idx.byBase[base] = append(idx.byBase[base], n.ID)
		stem := strings.TrimSuffix(base, strings.ToLower(n.Extension))
		idx.byStem[stem] = append(idx.byStem[stem], n.ID)
		if n.Name == "go.mod" {
			if mp := readModulePath(n.Path); mp != "" {
				idx.modules = append(idx.modules, goModule{path: mp, dir: parentID(n.ID)})
			}
		}
	}
	// Longest module path first, so a nested module wins over its parent.
	sort.Slice(idx.modules, func(i, j int) bool {
		return len(idx.modules[i].path) > len(idx.modules[j].path)
	})
	return idx
}

// readModulePath pulls the module path out of a go.mod file.
func readModulePath(absPath string) string {
	f, err := os.Open(absPath)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if rest, ok := strings.CutPrefix(line, "module "); ok {
			return strings.Trim(strings.TrimSpace(rest), `"`)
		}
	}
	return ""
}

// resolve turns a reference into a node ID, reporting how sure it is.
func (idx *index) resolve(ref Ref, fc FileContext) (string, string, bool) {
	switch ref.Strategy {
	case RelPath:
		return idx.pick(idx.relPathCandidates(ref, fc))
	case GoPkg:
		return idx.resolveGoPkg(ref.Spec)
	case PyModule:
		return idx.pick(idx.pyCandidates(ref, fc))
	case Basename:
		return idx.resolveBasename(ref, fc)
	}
	return "", "", false
}

// pick chooses among the candidates a specifier could name. Files win over
// directories, so `require("./mod")` lands on mod/index.js rather than on the
// mod folder; a directory is only the answer when nothing inside it matched.
// One surviving candidate means the reference named its target, several mean
// the resolver had to choose.
func (idx *index) pick(candidates []string) (string, string, bool) {
	var files, dirs []string
	for _, c := range candidates {
		switch idx.kind[c] {
		case "file":
			files = append(files, c)
		case "folder":
			dirs = append(dirs, c)
		}
	}
	hits := files
	if len(hits) == 0 {
		hits = dirs
	}
	switch len(hits) {
	case 0:
		return "", "", false
	case 1:
		return hits[0], Extracted, true
	default:
		return hits[0], Inferred, true
	}
}

// relPathCandidates expands a filesystem-style specifier into the node IDs it
// could name, in precedence order: the literal path, then each try extension,
// then a directory index file.
func (idx *index) relPathCandidates(ref Ref, fc FileContext) []string {
	spec := cleanSpec(ref.Spec)
	if spec == "" {
		return nil
	}
	base := fc.Dir
	if strings.HasPrefix(spec, "/") {
		base, spec = ".", strings.TrimPrefix(spec, "/")
	}
	joined := path.Join(base, spec)
	if joined == ".." || strings.HasPrefix(joined, "../") {
		return nil // escapes the scan root
	}

	out := []string{joined}
	for _, ext := range ref.TryExts {
		out = append(out, joined+ext)
	}
	for _, ext := range ref.TryExts {
		out = append(out, path.Join(joined, "index"+ext))
	}
	return out
}

// pyCandidates maps a dotted module path to the files that could define it.
// Leading dots are relative-import markers: one dot is the current package,
// each additional dot climbs one level.
func (idx *index) pyCandidates(ref Ref, fc FileContext) []string {
	spec := ref.Spec
	dots := 0
	for dots < len(spec) && spec[dots] == '.' {
		dots++
	}
	rest := strings.Trim(spec[dots:], ".")
	parts := strings.Split(rest, ".")
	if rest == "" {
		parts = nil
	}

	var bases []string
	if dots == 0 {
		// Absolute import: could be rooted at the scan root or beside the file.
		bases = []string{".", fc.Dir}
	} else {
		b := fc.Dir
		for i := 1; i < dots; i++ {
			b = path.Dir(b)
			if b == "." || b == "/" {
				b = "."
				break
			}
		}
		bases = []string{b}
	}

	var out []string
	for _, b := range bases {
		joined := path.Join(append([]string{b}, parts...)...)
		if joined == ".." || strings.HasPrefix(joined, "../") {
			continue
		}
		out = append(out, joined+".py", path.Join(joined, "__init__.py"))
	}
	return out
}

// resolveGoPkg maps a Go import path onto a package directory in the tree.
// Imports that belong to no local module are external and stay unresolved.
func (idx *index) resolveGoPkg(spec string) (string, string, bool) {
	for _, m := range idx.modules {
		var rel string
		switch {
		case spec == m.path:
			rel = ""
		case strings.HasPrefix(spec, m.path+"/"):
			rel = strings.TrimPrefix(spec, m.path+"/")
		default:
			continue
		}
		dir := m.dir
		if rel != "" {
			dir = path.Join(m.dir, rel)
		}
		if idx.kind[dir] == "folder" {
			return dir, Extracted, true
		}
	}
	return "", "", false
}

// resolveBasename backs wiki-style links, which name a note rather than a path.
func (idx *index) resolveBasename(ref Ref, fc FileContext) (string, string, bool) {
	spec := cleanSpec(ref.Spec)
	if spec == "" {
		return "", "", false
	}
	// A wikilink may still carry a path, e.g. [[notes/design]] — try that first.
	if strings.Contains(spec, "/") {
		if id, conf, ok := idx.pick(idx.relPathCandidates(Ref{Spec: spec, TryExts: ref.TryExts}, FileContext{Dir: "."})); ok {
			return id, conf, true
		}
		spec = path.Base(spec)
	}

	key := strings.ToLower(spec)
	hits := idx.byBase[key]
	if len(hits) == 0 {
		hits = idx.byStem[key]
	}
	switch len(hits) {
	case 0:
		return "", "", false
	case 1:
		return hits[0], Extracted, true
	default:
		// Ambiguous note name: prefer one in the same folder, else the shallowest.
		best, bestDepth := "", 1<<30
		for _, h := range hits {
			if parentID(h) == fc.Dir {
				return h, Inferred, true
			}
			if d := strings.Count(h, "/"); d < bestDepth {
				best, bestDepth = h, d
			}
		}
		return best, Inferred, true
	}
}

// cleanSpec strips the parts of a link that never name a file: URL fragments,
// query strings, surrounding quotes, and percent-encoded spaces.
func cleanSpec(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"'`)
	if i := strings.IndexAny(s, "#?"); i >= 0 {
		s = s[:i]
	}
	s = strings.ReplaceAll(s, "%20", " ")
	return strings.TrimSpace(s)
}

// isRelativeSpec reports whether a module specifier points inside the project
// rather than at a package registry.
func isRelativeSpec(s string) bool {
	return strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") || s == "." || s == ".."
}

// stripComments blanks out // and /* */ comments so that a commented-out
// import never becomes an edge. Bytes are replaced in place, so offsets and
// line numbers stay accurate.
func stripComments(src []byte) []byte {
	out := make([]byte, len(src))
	copy(out, src)
	var inLine, inBlock bool
	for i := 0; i < len(out); i++ {
		switch {
		case inLine:
			if out[i] == '\n' {
				inLine = false
			} else {
				out[i] = ' '
			}
		case inBlock:
			if out[i] == '*' && i+1 < len(out) && out[i+1] == '/' {
				out[i], out[i+1] = ' ', ' '
				i++
				inBlock = false
			} else if out[i] != '\n' {
				out[i] = ' '
			}
		case out[i] == '/' && i+1 < len(out) && out[i+1] == '/':
			out[i], out[i+1] = ' ', ' '
			i++
			inLine = true
		case out[i] == '/' && i+1 < len(out) && out[i+1] == '*':
			out[i], out[i+1] = ' ', ' '
			i++
			inBlock = true
		}
	}
	return out
}

// lineOf reports the 1-indexed line containing byte offset off.
func lineOf(src []byte, off int) int {
	if off > len(src) {
		off = len(src)
	}
	return bytes.Count(src[:off], []byte("\n")) + 1
}
