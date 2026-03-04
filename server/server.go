package server

import (
	"bytes"
	"compress/gzip"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"strings"

	"grafux/config"
	"grafux/scanner"
)

//go:embed web
var webFS embed.FS

type clientConfig struct {
	Theme           string  `json:"theme"`
	FileRadius      float64 `json:"fileRadius"`
	FolderBase      float64 `json:"folderBase"`
	FolderScale     float64 `json:"folderScale"`
	EdgeWidth       float64 `json:"edgeWidth"`
	LabelZoom       float64 `json:"labelZoom"`
	ChargeStrength  float64 `json:"chargeStrength"`
	ChargeMax       float64 `json:"chargeMax"`
	LinkDistance    float64 `json:"linkDistance"`
	LinkStrength    float64 `json:"linkStrength"`
	CenterStrength  float64 `json:"centerStrength"`
	CollideStrength float64 `json:"collideStrength"`
	AlphaDecay      float64 `json:"alphaDecay"`
	VelocityDecay   float64 `json:"velocityDecay"`
	Layout          string  `json:"layout"`
}

func Start(port int, graph *scanner.Graph, cfg config.Config) (string, error) {
	mux := http.NewServeMux()

	// Serve embedded frontend
	webContent, err := fs.Sub(webFS, "web")
	if err != nil {
		return "", fmt.Errorf("embed sub: %w", err)
	}
	mux.Handle("/", http.FileServer(http.FS(webContent)))

	// Graph API endpoint — pre-compress for large graphs
	graphJSON, err := json.Marshal(graph)
	if err != nil {
		return "", fmt.Errorf("marshal graph: %w", err)
	}

	var graphGzip []byte
	var buf bytes.Buffer
	gz, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	if err != nil {
		return "", fmt.Errorf("gzip writer: %w", err)
	}
	if _, err := gz.Write(graphJSON); err != nil {
		return "", fmt.Errorf("gzip write: %w", err)
	}
	if err := gz.Close(); err != nil {
		return "", fmt.Errorf("gzip close: %w", err)
	}
	graphGzip = buf.Bytes()

	mux.HandleFunc("/api/graph", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			w.Header().Set("Content-Encoding", "gzip")
			w.Write(graphGzip)
		} else {
			w.Write(graphJSON)
		}
	})

	// Config API endpoint — exposes server-side settings to the frontend
	cfgJSON, err := json.Marshal(clientConfig{
		Theme:           cfg.Theme,
		FileRadius:      cfg.FileRadius,
		FolderBase:      cfg.FolderBase,
		FolderScale:     cfg.FolderScale,
		EdgeWidth:       cfg.EdgeWidth,
		LabelZoom:       cfg.LabelZoom,
		ChargeStrength:  cfg.ChargeStrength,
		ChargeMax:       cfg.ChargeMax,
		LinkDistance:     cfg.LinkDistance,
		LinkStrength:    cfg.LinkStrength,
		CenterStrength:  cfg.CenterStrength,
		CollideStrength: cfg.CollideStrength,
		AlphaDecay:      cfg.AlphaDecay,
		VelocityDecay:   cfg.VelocityDecay,
		Layout:          cfg.Layout,
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
