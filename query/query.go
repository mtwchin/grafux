// Package query answers questions about a scanned graph: find a node, list
// what it touches, and trace how two nodes connect. It is the read surface
// that the CLI and any agent driving it share.
package query

import (
	"fmt"
	"sort"
	"strings"

	"grafux/scanner"
)

// Graph wraps a scanned graph with the indexes traversal needs.
type Graph struct {
	*scanner.Graph
	byID map[string]int   // node ID -> index into Nodes
	out  map[string][]int // node ID -> indices into Edges, as source
	in   map[string][]int // node ID -> indices into Edges, as target
}

// New builds the lookup indexes. Cost is linear in the size of the graph.
func New(g *scanner.Graph) *Graph {
	q := &Graph{
		Graph: g,
		byID:  make(map[string]int, len(g.Nodes)),
		out:   make(map[string][]int),
		in:    make(map[string][]int),
	}
	for i, n := range g.Nodes {
		q.byID[n.ID] = i
	}
	for i, e := range g.Edges {
		q.out[e.Source] = append(q.out[e.Source], i)
		q.in[e.Target] = append(q.in[e.Target], i)
	}
	return q
}

// Node looks up a node by exact ID.
func (g *Graph) Node(id string) (scanner.Node, bool) {
	i, ok := g.byID[id]
	if !ok {
		return scanner.Node{}, false
	}
	return g.Nodes[i], true
}

// Degree counts every edge touching a node, in either direction.
func (g *Graph) Degree(id string) int {
	return len(g.out[id]) + len(g.in[id])
}

// IsContent reports whether an edge came from a file's contents rather than
// from the directory tree.
func IsContent(e scanner.Edge) bool { return e.Type != "structural" }

// ─── Resolving a user-supplied reference ─────────────────────────────────────

// AmbiguousError reports a reference that matched several nodes. It lists them
// so the caller can pick one instead of guessing.
type AmbiguousError struct {
	Ref        string
	Candidates []string
}

func (e *AmbiguousError) Error() string {
	return fmt.Sprintf("%q matches %d nodes: %s", e.Ref, len(e.Candidates), strings.Join(e.Candidates, ", "))
}

// NotFoundError reports a reference that matched nothing, with the nearest
// misses attached so the caller has somewhere to go next.
type NotFoundError struct {
	Ref     string
	Nearest []string
}

func (e *NotFoundError) Error() string {
	if len(e.Nearest) == 0 {
		return fmt.Sprintf("no node matches %q", e.Ref)
	}
	return fmt.Sprintf("no node matches %q; did you mean: %s", e.Ref, strings.Join(e.Nearest, ", "))
}

// Resolve turns a human- or agent-supplied reference into exactly one node.
// It tries progressively looser matches: exact ID, path suffix, base name,
// then a ranked search.
func (g *Graph) Resolve(ref string) (scanner.Node, error) {
	ref = strings.TrimSpace(strings.Trim(ref, `"'`))
	ref = strings.TrimPrefix(ref, "./")
	if ref == "" {
		return scanner.Node{}, &NotFoundError{Ref: ref}
	}

	if n, ok := g.Node(ref); ok {
		return n, nil
	}

	lower := strings.ToLower(ref)
	var suffix, base []scanner.Node
	for _, n := range g.Nodes {
		id := strings.ToLower(n.ID)
		if strings.HasSuffix(id, "/"+lower) {
			suffix = append(suffix, n)
		}
		if strings.ToLower(n.Name) == lower {
			base = append(base, n)
		}
	}
	for _, set := range [][]scanner.Node{suffix, base} {
		switch len(set) {
		case 1:
			return set[0], nil
		case 0:
			continue
		default:
			return scanner.Node{}, &AmbiguousError{Ref: ref, Candidates: ids(set)}
		}
	}

	// Fall back to search. A clear winner is good enough to act on.
	matches := Search(g, ref, 5)
	if len(matches) == 0 {
		return scanner.Node{}, &NotFoundError{Ref: ref}
	}
	if len(matches) == 1 || matches[0].Score > matches[1].Score*2 {
		return matches[0].Node, nil
	}
	return scanner.Node{}, &AmbiguousError{Ref: ref, Candidates: matchIDs(matches)}
}

func ids(ns []scanner.Node) []string {
	out := make([]string, len(ns))
	for i, n := range ns {
		out[i] = n.ID
	}
	return out
}

func matchIDs(ms []Match) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Node.ID
	}
	return out
}

// ─── Search ──────────────────────────────────────────────────────────────────

// Match is one search hit.
type Match struct {
	Node   scanner.Node `json:"node"`
	Score  int          `json:"score"`
	Degree int          `json:"degree"`
}

// Search ranks nodes against a free-text query. Every whitespace-separated
// term must match somewhere, so more words narrow the result rather than
// widening it.
func Search(g *Graph, q string, limit int) []Match {
	terms := strings.Fields(strings.ToLower(q))
	if len(terms) == 0 {
		return nil
	}

	var out []Match
	for _, n := range g.Nodes {
		total := 0
		for _, t := range terms {
			s := scoreNode(n, t)
			if s == 0 {
				total = 0
				break
			}
			total += s
		}
		if total == 0 {
			continue
		}
		deg := g.Degree(n.ID)
		// A well-connected node is more likely to be the one being asked about,
		// but connectivity must not outrank a direct name match.
		if deg > 20 {
			deg = 20
		}
		out = append(out, Match{Node: n, Score: total + deg, Degree: g.Degree(n.ID)})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if len(out[i].Node.ID) != len(out[j].Node.ID) {
			return len(out[i].Node.ID) < len(out[j].Node.ID)
		}
		return out[i].Node.ID < out[j].Node.ID
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func scoreNode(n scanner.Node, term string) int {
	id := strings.ToLower(n.ID)
	name := strings.ToLower(n.Name)
	ext := strings.ToLower(n.Extension)
	stem := strings.TrimSuffix(name, ext)

	switch {
	case id == term:
		return 1000
	case name == term, stem == term:
		return 900
	case ext != "" && ext == term:
		return 800
	case strings.HasPrefix(name, term):
		return 700
	case strings.Contains(name, term):
		return 500
	case strings.Contains(id, term):
		return 300
	case isSubsequence(name, term):
		return 120
	case isSubsequence(id, term):
		return 60
	}
	return 0
}

// isSubsequence reports whether every character of needle appears in haystack
// in order — the loose matching a fuzzy finder does.
func isSubsequence(haystack, needle string) bool {
	if needle == "" {
		return false
	}
	i := 0
	for j := 0; j < len(haystack) && i < len(needle); j++ {
		if haystack[j] == needle[i] {
			i++
		}
	}
	return i == len(needle)
}

// ─── Traversal ───────────────────────────────────────────────────────────────

// Neighbor is a node reached from a starting point, with the edge that got
// there.
type Neighbor struct {
	Node      scanner.Node `json:"node"`
	Hops      int          `json:"hops"`
	Via       scanner.Edge `json:"via"`
	Direction string       `json:"direction"` // "out" if the edge points away, "in" if toward
}

// Neighbors walks outward from a node up to hops away. Passing contentOnly
// skips the directory tree, which is what makes the result a dependency
// neighborhood rather than a folder listing.
func Neighbors(g *Graph, id string, hops int, contentOnly bool) []Neighbor {
	if hops < 1 {
		hops = 1
	}
	seen := map[string]bool{id: true}
	var out []Neighbor
	frontier := []string{id}

	for h := 1; h <= hops && len(frontier) > 0; h++ {
		var next []string
		for _, cur := range frontier {
			visit := func(edgeIdx []int, dir string) {
				for _, ei := range edgeIdx {
					e := g.Edges[ei]
					if contentOnly && !IsContent(e) {
						continue
					}
					other := e.Target
					if dir == "in" {
						other = e.Source
					}
					if seen[other] {
						continue
					}
					n, ok := g.Node(other)
					if !ok {
						continue
					}
					seen[other] = true
					out = append(out, Neighbor{Node: n, Hops: h, Via: e, Direction: dir})
					next = append(next, other)
				}
			}
			visit(g.out[cur], "out")
			visit(g.in[cur], "in")
		}
		frontier = next
	}
	return out
}

// Step is one hop along a path. Forward reports whether the edge was traversed
// in its own direction or against it.
type Step struct {
	From    string       `json:"from"`
	To      string       `json:"to"`
	Edge    scanner.Edge `json:"edge"`
	Forward bool         `json:"forward"`
}

// ShortestPath finds the fewest hops between two nodes, treating edges as
// undirected — how two files relate matters more than which one pointed first.
func ShortestPath(g *Graph, from, to string) ([]Step, bool) {
	if from == to {
		return nil, true
	}
	type crumb struct {
		prev    string
		edge    scanner.Edge
		forward bool
	}
	trail := map[string]crumb{from: {}}
	queue := []string{from}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		step := func(edgeIdx []int, forward bool) bool {
			for _, ei := range edgeIdx {
				e := g.Edges[ei]
				other := e.Target
				if !forward {
					other = e.Source
				}
				if _, ok := trail[other]; ok {
					continue
				}
				trail[other] = crumb{prev: cur, edge: e, forward: forward}
				if other == to {
					return true
				}
				queue = append(queue, other)
			}
			return false
		}
		if step(g.out[cur], true) || step(g.in[cur], false) {
			break
		}
	}

	if _, ok := trail[to]; !ok {
		return nil, false
	}

	var steps []Step
	for cur := to; cur != from; {
		c := trail[cur]
		steps = append(steps, Step{From: c.prev, To: cur, Edge: c.edge, Forward: c.forward})
		cur = c.prev
	}
	// Built backwards from the destination.
	for i, j := 0, len(steps)-1; i < j; i, j = i+1, j-1 {
		steps[i], steps[j] = steps[j], steps[i]
	}
	return steps, true
}

// ─── Explain ─────────────────────────────────────────────────────────────────

// Link is one connection listed in an explanation.
type Link struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Confidence string `json:"confidence,omitempty"`
	Line       int    `json:"line,omitempty"`
}

// Explanation is everything the graph knows about a single node.
type Explanation struct {
	Node       scanner.Node `json:"node"`
	Degree     int          `json:"degree"`
	Parent     string       `json:"parent,omitempty"`
	Children   []Link       `json:"children,omitempty"`
	DependsOn  []Link       `json:"dependsOn,omitempty"`
	DependedOn []Link       `json:"dependedOnBy,omitempty"`
}

// Explain gathers a node's structural position and its content relationships.
func Explain(g *Graph, id string) (Explanation, bool) {
	n, ok := g.Node(id)
	if !ok {
		return Explanation{}, false
	}
	ex := Explanation{Node: n, Degree: g.Degree(id)}

	for _, ei := range g.out[id] {
		e := g.Edges[ei]
		l := Link{ID: e.Target, Type: e.Type, Confidence: e.Confidence, Line: e.Line}
		if IsContent(e) {
			ex.DependsOn = append(ex.DependsOn, l)
		} else {
			ex.Children = append(ex.Children, l)
		}
	}
	for _, ei := range g.in[id] {
		e := g.Edges[ei]
		l := Link{ID: e.Source, Type: e.Type, Confidence: e.Confidence, Line: e.Line}
		if IsContent(e) {
			ex.DependedOn = append(ex.DependedOn, l)
		} else if ex.Parent == "" {
			ex.Parent = e.Source
		}
	}

	sortLinks(ex.Children)
	sortLinks(ex.DependsOn)
	sortLinks(ex.DependedOn)
	return ex, true
}

func sortLinks(ls []Link) {
	sort.Slice(ls, func(i, j int) bool { return ls[i].ID < ls[j].ID })
}
