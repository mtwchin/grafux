package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"

	"grafux/config"
	"grafux/scanner"
)

//go:embed web
var webFS embed.FS

type clientConfig struct {
	Theme       string  `json:"theme"`
	FileRadius  float64 `json:"fileRadius"`
	FolderBase  float64 `json:"folderBase"`
	FolderScale float64 `json:"folderScale"`
	EdgeWidth   float64 `json:"edgeWidth"`
	LabelZoom   float64 `json:"labelZoom"`
}

func Start(port int, graph *scanner.Graph, cfg config.Config) (string, error) {
	mux := http.NewServeMux()

	// Serve embedded frontend
	webContent, err := fs.Sub(webFS, "web")
	if err != nil {
		return "", fmt.Errorf("embed sub: %w", err)
	}
	mux.Handle("/", http.FileServer(http.FS(webContent)))

	// Graph API endpoint
	graphJSON, err := json.Marshal(graph)
	if err != nil {
		return "", fmt.Errorf("marshal graph: %w", err)
	}
	mux.HandleFunc("/api/graph", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Write(graphJSON)
	})

	// Config API endpoint — exposes server-side settings to the frontend
	cfgJSON, err := json.Marshal(clientConfig{
		Theme:       cfg.Theme,
		FileRadius:  cfg.FileRadius,
		FolderBase:  cfg.FolderBase,
		FolderScale: cfg.FolderScale,
		EdgeWidth:   cfg.EdgeWidth,
		LabelZoom:   cfg.LabelZoom,
	})
	if err != nil {
		return "", fmt.Errorf("marshal config: %w", err)
	}
	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Write(cfgJSON)
	})

	// Find an available port
	if port == 0 {
		ln, err := net.Listen("tcp", "localhost:0")
		if err != nil {
			return "", fmt.Errorf("find port: %w", err)
		}
		port = ln.Addr().(*net.TCPAddr).Port
		ln.Close()
	}

	addr := fmt.Sprintf("localhost:%d", port)
	go func() {
		if err := http.ListenAndServe(addr, mux); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		}
	}()

	return addr, nil
}
