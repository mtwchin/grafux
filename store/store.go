// Package store persists a scanned graph to disk so that anything which is not
// a browser — an agent, a script, another grafux command — can read it without
// a running server.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"grafux/scanner"
)

// Dir is the per-project directory holding grafux's artifacts.
const Dir = ".grafux"

// FileName is the graph artifact inside Dir.
const FileName = "graph.json"

// Path returns where the graph artifact lives for a given project root.
func Path(root string) string {
	return filepath.Join(root, Dir, FileName)
}

// Save writes the graph atomically and returns the path it wrote to.
func Save(root string, g *scanner.Graph) (string, error) {
	dir := filepath.Join(root, Dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal graph: %w", err)
	}

	dest := filepath.Join(dir, FileName)
	tmp, err := os.CreateTemp(dir, FileName+".tmp*")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "", fmt.Errorf("write graph: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close graph: %w", err)
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return "", fmt.Errorf("replace %s: %w", dest, err)
	}
	return dest, nil
}

// Load reads a graph artifact from an explicit path.
func Load(path string) (*scanner.Graph, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var g scanner.Graph
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(g.Nodes) == 0 {
		return nil, fmt.Errorf("%s contains no nodes", path)
	}
	return &g, nil
}

// ErrNotFound reports that no graph artifact exists in or above a directory.
var ErrNotFound = errors.New("no .grafux/graph.json found")

// Find walks up from start looking for a graph artifact, the way git looks for
// .git. It returns the path to the artifact and the project root holding it.
func Find(start string) (graphPath, root string, err error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", "", err
	}
	for {
		candidate := filepath.Join(dir, Dir, FileName)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", ErrNotFound
		}
		dir = parent
	}
}
