package scanner

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Node struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"` // "file" or "folder"
	Path      string `json:"path"`
	Depth     int    `json:"depth"`
	Children  int    `json:"children,omitempty"`
	Extension string `json:"extension,omitempty"`
	Size      int64  `json:"size,omitempty"`
}

// Edge connects two nodes. Type is "structural" for the directory tree, or
// "import"/"reference" for a relationship read out of a file's contents.
// Confidence records whether the source named its target unambiguously
// ("extracted") or the resolver had to choose between candidates ("inferred").
type Edge struct {
	Source     string `json:"source"`
	Target     string `json:"target"`
	Type       string `json:"type"`
	Confidence string `json:"confidence,omitempty"`
	Line       int    `json:"line,omitempty"`
}

type Meta struct {
	Root         string    `json:"root"`
	TotalFiles   int       `json:"totalFiles"`
	TotalFolders int       `json:"totalFolders"`
	ScanDepth    int       `json:"scanDepth"`
	ContentEdges int       `json:"contentEdges"`
	ScannedAt    time.Time `json:"scannedAt"`
}

type Graph struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
	Meta  Meta   `json:"meta"`
}

type Options struct {
	MaxDepth   int
	ShowHidden bool
	Include    []string // if non-empty, only include files with these extensions
	Exclude    []string // exclude files with these extensions
}

var defaultIgnore = map[string]bool{
	".git":          true,
	".grafux":       true,
	"node_modules":  true,
	"__pycache__":   true,
	".DS_Store":     true,
	"thumbs.db":     true,
	"Thumbs.db":     true,
	".idea":         true,
	".vscode":       true,
	"vendor":        true,
	"dist":          false, // not ignored by default
	"build":         false,
}

func Scan(root string, opts Options) (*Graph, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	graph := &Graph{
		Meta: Meta{
			Root:      absRoot,
			ScanDepth: opts.MaxDepth,
			ScannedAt: time.Now().UTC(),
		},
	}

	// pathToID maps absolute path -> node ID (relative path from root)
	pathToID := map[string]string{}
	// pathToIdx maps absolute path -> index in graph.Nodes
	pathToIdx := map[string]int{}

	// Add root node
	rootInfo, err := os.Stat(absRoot)
	if err != nil {
		return nil, err
	}

	rootNode := Node{
		ID:    ".",
		Name:  rootInfo.Name(),
		Type:  "folder",
		Path:  absRoot,
		Depth: 0,
	}
	graph.Nodes = append(graph.Nodes, rootNode)
	pathToID[absRoot] = "."
	pathToIdx[absRoot] = 0
	graph.Meta.TotalFolders++

	err = filepath.Walk(absRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable paths
		}
		if path == absRoot {
			return nil // root already added
		}

		name := info.Name()

		// Skip hidden files/folders unless requested
		if !opts.ShowHidden && strings.HasPrefix(name, ".") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip default ignores
		if defaultIgnore[name] {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Calculate depth
		rel, _ := filepath.Rel(absRoot, path)
		parts := strings.Split(rel, string(filepath.Separator))
		depth := len(parts)

		if opts.MaxDepth > 0 && depth > opts.MaxDepth {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		nodeID := filepath.ToSlash(rel) // use forward slashes for consistency

		// Extension-based filtering (files only)
		if !info.IsDir() {
			ext := filepath.Ext(name)
			if len(opts.Include) > 0 && !extInList(ext, opts.Include) {
				return nil
			}
			if len(opts.Exclude) > 0 && extInList(ext, opts.Exclude) {
				return nil
			}
		}

		node := Node{
			ID:    nodeID,
			Name:  name,
			Path:  path,
			Depth: depth,
		}

		if info.IsDir() {
			node.Type = "folder"
			graph.Meta.TotalFolders++
		} else {
			node.Type = "file"
			node.Extension = filepath.Ext(name)
			node.Size = info.Size()
			graph.Meta.TotalFiles++
		}

		idx := len(graph.Nodes)
		graph.Nodes = append(graph.Nodes, node)
		pathToID[path] = nodeID
		pathToIdx[path] = idx

		// Connect to parent
		parentPath := filepath.Dir(path)
		if parentID, ok := pathToID[parentPath]; ok {
			if parentIdx, ok2 := pathToIdx[parentPath]; ok2 {
				graph.Edges = append(graph.Edges, Edge{
					Source: parentID,
					Target: nodeID,
					Type:   "structural",
				})
				graph.Nodes[parentIdx].Children++
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return graph, nil
}

func extInList(ext string, list []string) bool {
	for _, e := range list {
		if strings.EqualFold(ext, e) {
			return true
		}
	}
	return false
}
