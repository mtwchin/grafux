package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"grafux/cli"
	"grafux/config"
	"grafux/server"
	"grafux/store"
)

func main() {
	// A verb in the first position means the caller wants data, not a browser.
	if len(os.Args) > 1 && cli.IsCommand(os.Args[1]) {
		if err := cli.Run(os.Args[1], os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "grf %s: %v\n", os.Args[1], err)
			os.Exit(1)
		}
		return
	}

	flag.Usage = usage

	depthFlag := flag.Int("depth", 5, "Max directory depth (0 = unlimited)")
	portFlag := flag.Int("port", 0, "Port to serve on (0 = random available port)")
	noOpenFlag := flag.Bool("no-open", false, "Don't auto-open browser")
	showHiddenFlag := flag.Bool("show-hidden", false, "Include hidden files and folders")
	themeFlag := flag.String("theme", "", "UI theme: gruvbox, obsidian, forest, aurora, mono")
	includeFlag := flag.String("include", "", "Comma-separated extensions to include (e.g. .go,.md)")
	excludeFlag := flag.String("exclude", "", "Comma-separated extensions to exclude (e.g. .log,.tmp)")
	flag.Parse()

	root := "."
	if flag.NArg() > 0 {
		root = flag.Arg(0)
	}

	if _, err := os.Stat(root); err != nil {
		log.Fatalf("Cannot access directory: %v", err)
	}

	// Load config file; CLI flags override via flag.Visit below.
	cfg, err := config.Load(root)
	if err != nil {
		log.Printf("Warning: could not load .grafux.yml: %v", err)
	}

	// Determine which flags were explicitly provided on the CLI.
	explicit := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { explicit[f.Name] = true })

	depth := cfg.Depth
	if explicit["depth"] {
		depth = *depthFlag
	}
	port := cfg.Port
	if explicit["port"] {
		port = *portFlag
	}
	noOpen := cfg.NoOpen
	if explicit["no-open"] {
		noOpen = *noOpenFlag
	}
	showHidden := cfg.ShowHidden
	if explicit["show-hidden"] {
		showHidden = *showHiddenFlag
	}
	theme := cfg.Theme
	if explicit["theme"] {
		theme = *themeFlag
	}
	if explicit["include"] {
		cfg.Include = splitExts(*includeFlag)
	}
	if explicit["exclude"] {
		cfg.Exclude = splitExts(*excludeFlag)
	}
	cfg.Theme = theme

	cfg.Depth = depth
	cfg.ShowHidden = showHidden

	graph, stats, scanErr := cli.BuildGraph(root, cfg)
	if scanErr != nil {
		log.Fatalf("Scan failed: %v", scanErr)
	}

	// Leave the graph on disk so `grf query` and any agent can read it without
	// this server running.
	savedTo, saveErr := store.Save(root, graph)
	if saveErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not save graph: %v\n", saveErr)
	}

	addr, tabClosed, startErr := server.Start(port, graph, cfg)
	if startErr != nil {
		log.Fatalf("Server failed to start: %v", startErr)
	}

	url := fmt.Sprintf("http://%s", addr)
	fmt.Printf("Grafux: %s\n", url)
	fmt.Printf("  %d files · %d folders · %d content edges · depth %d · theme: %s\n",
		graph.Meta.TotalFiles, graph.Meta.TotalFolders, stats.Resolved, depth, cfg.Theme)
	if savedTo != "" {
		fmt.Printf("  graph saved to %s\n", savedTo)
	}
	fmt.Println("  Press Ctrl+C to stop")

	if !noOpen {
		openBrowser(url)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	select {
	case <-quit:
		fmt.Println("\nStopped.")
	case <-tabClosed:
		fmt.Println("\nBrowser tab closed. Stopped.")
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `grf — visualize and query a directory as a graph.

Usage:
  grf [flags] [path]           Scan a directory and open the graph in a browser
  grf <command> [args]         Read the graph without a browser

%s

Flags:
`, cli.Usage())
	flag.PrintDefaults()
}

func splitExts(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func openBrowser(url string) {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "linux":
		cmd = "xdg-open"
		args = []string{url}
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	default:
		return
	}

	if err := exec.Command(cmd, args...).Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Could not open browser: %v\n", err)
	}
}
