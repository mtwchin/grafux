// Package cli implements grafux's non-visual commands — the surface an agent
// or a script uses. Running `grf` with no verb still opens the browser; the
// verbs here read the same graph without one.
package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"grafux/config"
	"grafux/extract"
	"grafux/query"
	"grafux/scanner"
	"grafux/store"
)

// commands maps a verb to its implementation.
var commands = map[string]func([]string) error{
	"scan":      cmdScan,
	"query":     cmdQuery,
	"path":      cmdPath,
	"neighbors": cmdNeighbors,
	"explain":   cmdExplain,
}

// IsCommand reports whether name is a verb rather than a path to visualize.
func IsCommand(name string) bool {
	_, ok := commands[name]
	return ok
}

// Run dispatches a verb.
func Run(name string, args []string) error {
	fn, ok := commands[name]
	if !ok {
		return fmt.Errorf("unknown command %q", name)
	}
	return fn(args)
}

// Usage describes the verbs, for the top-level help text.
func Usage() string {
	return `Commands:
  grf scan [path]              Scan and write .grafux/graph.json
  grf query <terms...>         Find nodes by name, path, or extension
  grf explain <node>           Show one node and everything it connects to
  grf neighbors <node>         List what a node reaches, with --hops N
  grf path <from> <to>         Trace the shortest connection between two nodes

Run a command with -h for its own flags.`
}

// BuildGraph scans a directory and enriches it with content edges. It is the
// one place that defines what a complete grafux graph is.
func BuildGraph(root string, cfg config.Config) (*scanner.Graph, extract.Stats, error) {
	g, err := scanner.Scan(root, scanner.Options{
		MaxDepth:   cfg.Depth,
		ShowHidden: cfg.ShowHidden,
		Include:    cfg.Include,
		Exclude:    cfg.Exclude,
	})
	if err != nil {
		return nil, extract.Stats{}, err
	}
	return g, extract.Run(g), nil
}

// ─── Shared flags and loading ────────────────────────────────────────────────

type commonFlags struct {
	graphPath string
	rescan    bool
	asJSON    bool
}

func (c *commonFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&c.graphPath, "graph", "", "Path to a graph.json (default: nearest .grafux/graph.json)")
	fs.BoolVar(&c.rescan, "rescan", false, "Scan the current directory instead of reading a saved graph")
	fs.BoolVar(&c.asJSON, "json", false, "Emit JSON instead of text")
}

// load returns a query-ready graph, preferring a saved artifact and falling
// back to scanning the working directory so the commands work before anyone
// has run `grf scan`.
func (c *commonFlags) load() (*query.Graph, error) {
	if c.rescan {
		return c.scanHere()
	}
	if c.graphPath != "" {
		g, err := store.Load(c.graphPath)
		if err != nil {
			return nil, err
		}
		return query.New(g), nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	path, _, err := store.Find(cwd)
	if err == nil {
		g, loadErr := store.Load(path)
		if loadErr != nil {
			return nil, loadErr
		}
		return query.New(g), nil
	}
	if err != store.ErrNotFound {
		return nil, err
	}
	fmt.Fprintln(os.Stderr, "note: no .grafux/graph.json found — scanning this directory. Run `grf scan` to save one.")
	return c.scanHere()
}

func (c *commonFlags) scanHere() (*query.Graph, error) {
	cfg, err := config.Load(".")
	if err != nil {
		return nil, err
	}
	g, _, err := BuildGraph(".", cfg)
	if err != nil {
		return nil, err
	}
	return query.New(g), nil
}

// parseArgs parses flags wherever they appear — before, after, or between
// positional arguments. Go's flag package stops at the first non-flag, which
// would silently swallow `grf neighbors main.go --content` as one long name.
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return positional, nil
		}
		positional = append(positional, rest[0])
		args = rest[1:]
	}
}

func emitJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// ─── scan ────────────────────────────────────────────────────────────────────

func cmdScan(args []string) error {
	fs := flag.NewFlagSet("scan", flag.ExitOnError)
	depth := fs.Int("depth", 0, "Max directory depth (0 = use config default)")
	showHidden := fs.Bool("show-hidden", false, "Include hidden files and folders")
	include := fs.String("include", "", "Comma-separated extensions to include")
	exclude := fs.String("exclude", "", "Comma-separated extensions to exclude")
	out := fs.String("out", "", "Write the graph here instead of <path>/.grafux/graph.json")
	quiet := fs.Bool("quiet", false, "Only print the path that was written")
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}

	root := "."
	if len(rest) > 0 {
		root = rest[0]
	}

	cfg, cfgErr := config.Load(root)
	if cfgErr != nil {
		fmt.Fprintf(os.Stderr, "warning: could not load .grafux.yml: %v\n", cfgErr)
	}
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "depth":
			cfg.Depth = *depth
		case "show-hidden":
			cfg.ShowHidden = *showHidden
		case "include":
			cfg.Include = splitList(*include)
		case "exclude":
			cfg.Exclude = splitList(*exclude)
		}
	})

	g, stats, err := BuildGraph(root, cfg)
	if err != nil {
		return err
	}

	written, err := saveTo(root, *out, g)
	if err != nil {
		return err
	}

	if *quiet {
		fmt.Println(written)
		return nil
	}
	fmt.Printf("%s\n", g.Meta.Root)
	fmt.Printf("  %d files · %d folders · %d structural edges · %d content edges\n",
		g.Meta.TotalFiles, g.Meta.TotalFolders, len(g.Edges)-stats.Resolved, stats.Resolved)
	fmt.Printf("  read %d parseable files, resolved %d of %d references",
		stats.FilesRead, stats.Resolved+stats.Duplicate, stats.Refs)
	if stats.Duplicate > 0 {
		fmt.Printf(" (%d duplicate)", stats.Duplicate)
	}
	fmt.Println()
	fmt.Printf("  wrote %s\n", written)
	return nil
}

func saveTo(root, out string, g *scanner.Graph) (string, error) {
	if out == "" {
		return store.Save(root, g)
	}
	data, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return "", err
	}
	if out == "-" {
		_, err = os.Stdout.Write(append(data, '\n'))
		return "(stdout)", err
	}
	return out, os.WriteFile(out, data, 0o644)
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// ─── query ───────────────────────────────────────────────────────────────────

func cmdQuery(args []string) error {
	fs := flag.NewFlagSet("query", flag.ExitOnError)
	var cf commonFlags
	cf.register(fs)
	limit := fs.Int("limit", 20, "Maximum results")
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		return fmt.Errorf("usage: grf query <terms...>")
	}
	term := strings.Join(rest, " ")

	g, err := cf.load()
	if err != nil {
		return err
	}
	matches := query.Search(g, term, *limit)

	if cf.asJSON {
		return emitJSON(map[string]any{
			"query":   term,
			"root":    g.Meta.Root,
			"matches": matches,
		})
	}

	w := os.Stdout
	writeHeader(w, g)
	if len(matches) == 0 {
		fmt.Fprintf(w, "\nno match for %q\n", term)
		return nil
	}
	fmt.Fprintf(w, "\n%d match(es) for %q:\n\n", len(matches), term)
	width := 0
	for _, m := range matches {
		if len(m.Node.ID) > width {
			width = len(m.Node.ID)
		}
	}
	for _, m := range matches {
		kind := m.Node.Type
		if m.Node.Extension != "" {
			kind = m.Node.Extension
		}
		fmt.Fprintf(w, "  %-*s  %-8s  %d link(s)\n", width, m.Node.ID, kind, m.Degree)
	}
	return nil
}

// ─── explain ─────────────────────────────────────────────────────────────────

func cmdExplain(args []string) error {
	fs := flag.NewFlagSet("explain", flag.ExitOnError)
	var cf commonFlags
	cf.register(fs)
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return fmt.Errorf("usage: grf explain <node>")
	}

	g, err := cf.load()
	if err != nil {
		return err
	}
	node, err := g.Resolve(rest[0])
	if err != nil {
		return err
	}
	ex, _ := query.Explain(g, node.ID)

	if cf.asJSON {
		return emitJSON(ex)
	}

	w := os.Stdout
	fmt.Fprintf(w, "%s\n", ex.Node.ID)
	fmt.Fprintf(w, "  type      %s%s\n", ex.Node.Type, extNote(ex.Node))
	fmt.Fprintf(w, "  path      %s\n", ex.Node.Path)
	if ex.Parent != "" {
		fmt.Fprintf(w, "  parent    %s\n", ex.Parent)
	}
	fmt.Fprintf(w, "  links     %d\n", ex.Degree)

	writeLinks(w, "depends on", ex.DependsOn)
	writeLinks(w, "depended on by", ex.DependedOn)
	if len(ex.Children) > 0 {
		writeLinks(w, "contains", ex.Children)
	}
	return nil
}

func extNote(n scanner.Node) string {
	if n.Extension == "" {
		return ""
	}
	return fmt.Sprintf(" (%s, %s)", n.Extension, humanSize(n.Size))
}

func humanSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGT"[exp])
}

func writeLinks(w io.Writer, title string, links []query.Link) {
	fmt.Fprintf(w, "\n%s (%d)\n", title, len(links))
	if len(links) == 0 {
		return
	}
	width := 0
	for _, l := range links {
		if len(l.ID) > width {
			width = len(l.ID)
		}
	}
	for _, l := range links {
		fmt.Fprintf(w, "  %-*s  %s", width, l.ID, l.Type)
		if l.Confidence != "" {
			fmt.Fprintf(w, "  [%s]", l.Confidence)
		}
		if l.Line > 0 {
			fmt.Fprintf(w, "  L%d", l.Line)
		}
		fmt.Fprintln(w)
	}
}

// ─── neighbors ───────────────────────────────────────────────────────────────

func cmdNeighbors(args []string) error {
	fs := flag.NewFlagSet("neighbors", flag.ExitOnError)
	var cf commonFlags
	cf.register(fs)
	hops := fs.Int("hops", 1, "How many hops to walk outward")
	contentOnly := fs.Bool("content", false, "Follow only content edges, ignoring the directory tree")
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return fmt.Errorf("usage: grf neighbors <node> [--hops N] [--content]")
	}

	g, err := cf.load()
	if err != nil {
		return err
	}
	node, err := g.Resolve(rest[0])
	if err != nil {
		return err
	}
	found := query.Neighbors(g, node.ID, *hops, *contentOnly)

	if cf.asJSON {
		return emitJSON(map[string]any{
			"node":      node,
			"hops":      *hops,
			"content":   *contentOnly,
			"neighbors": found,
		})
	}

	// Group by distance so the output reads as rings around the start.
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].Hops != found[j].Hops {
			return found[i].Hops < found[j].Hops
		}
		return found[i].Node.ID < found[j].Node.ID
	})

	w := os.Stdout
	scope := "all edges"
	if *contentOnly {
		scope = "content edges only"
	}
	fmt.Fprintf(w, "%s — %d hop(s), %s\n", node.ID, *hops, scope)
	if len(found) == 0 {
		fmt.Fprintln(w, "\nnothing reachable")
		return nil
	}
	current := 0
	for _, nb := range found {
		if nb.Hops != current {
			current = nb.Hops
			fmt.Fprintf(w, "\nhop %d\n", current)
		}
		arrow := "-->"
		if nb.Direction == "in" {
			arrow = "<--"
		}
		fmt.Fprintf(w, "  %s %s  [%s]\n", arrow, nb.Node.ID, nb.Via.Type)
	}
	return nil
}

// ─── path ────────────────────────────────────────────────────────────────────

func cmdPath(args []string) error {
	fs := flag.NewFlagSet("path", flag.ExitOnError)
	var cf commonFlags
	cf.register(fs)
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(rest) != 2 {
		return fmt.Errorf("usage: grf path <from> <to>")
	}

	g, err := cf.load()
	if err != nil {
		return err
	}
	from, err := g.Resolve(rest[0])
	if err != nil {
		return err
	}
	to, err := g.Resolve(rest[1])
	if err != nil {
		return err
	}

	steps, ok := query.ShortestPath(g, from.ID, to.ID)
	if cf.asJSON {
		return emitJSON(map[string]any{
			"from":      from.ID,
			"to":        to.ID,
			"connected": ok,
			"hops":      len(steps),
			"steps":     steps,
		})
	}

	w := os.Stdout
	if !ok {
		fmt.Fprintf(w, "%s and %s are not connected\n", from.ID, to.ID)
		return nil
	}
	fmt.Fprintf(w, "%s → %s (%d hop(s))\n\n", from.ID, to.ID, len(steps))
	fmt.Fprintf(w, "  %s\n", from.ID)
	for _, s := range steps {
		label := s.Edge.Type
		if s.Edge.Confidence != "" {
			label += " " + s.Edge.Confidence
		}
		if s.Edge.Line > 0 {
			label += fmt.Sprintf(" L%d", s.Edge.Line)
		}
		if s.Forward {
			fmt.Fprintf(w, "    --[%s]-->  %s\n", label, s.To)
		} else {
			fmt.Fprintf(w, "    <--[%s]--  %s\n", label, s.To)
		}
	}
	return nil
}

func writeHeader(w io.Writer, g *query.Graph) {
	fmt.Fprintf(w, "%s — %d files, %d folders, %d content edges\n",
		g.Meta.Root, g.Meta.TotalFiles, g.Meta.TotalFolders, g.Meta.ContentEdges)
}
