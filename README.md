# grafux

**Obsidian's graph view, for any directory on your filesystem — readable by you and by your agent.**

Run `grf` in any folder → it scans the file tree, reads the imports and links *inside* your files, starts a localhost server, and opens an interactive force-directed graph in your browser. Files are nodes. Folders are hubs. Everything is physics.

The same graph is written to `.grafux/graph.json` and queryable from the terminal, so an agent can traverse your project's dependencies instead of grepping for them.

---

## Install

```bash
go install github.com/balnc9/grafux@latest
```

Or build from source:

```bash
git clone https://github.com/balnc9/grafux
cd grafux
go build -o grf .
```

Requires Go 1.21+. No other runtime dependencies.

---

## Usage

```bash
# Visualize the current directory
grf

# Visualize a specific path
grf /path/to/project

# Limit scan depth (recommended for large repos)
grf --depth 3
```

### Querying the graph

Every run writes `.grafux/graph.json`. These commands read it — no server, no browser:

```bash
grf scan                              # scan and save the graph
grf query auth token                  # find nodes; every term must match
grf explain scanner/scanner.go        # one node and everything it connects to
grf neighbors main.go --content       # what it reaches, ignoring the folder tree
grf path docs/design.md policy.py     # shortest connection between two things
```

Any command takes `--json` for machine-readable output, and node arguments are
fuzzy: `grf explain token.go` resolves to `auth/token.go`, and an ambiguous name
returns the candidates rather than guessing.

```
$ grf path docs/design.md policy.py
docs/design.md → pkg/auth/policy.py (2 hop(s))

  docs/design.md
    --[reference extracted L3]-->  docs/auth-notes.md
    --[reference extracted L2]-->  pkg/auth/policy.py
```

### Flags

| Flag            | Default | Description                                   |
| --------------- | ------- | --------------------------------------------- |
| `--depth N`     | `5`     | Max directory depth to scan (`0` = unlimited) |
| `--port N`      | random  | Port to serve on                              |
| `--no-open`     | false   | Don't auto-open the browser                   |
| `--show-hidden` | false   | Include dotfiles and hidden folders           |
| `--include`     | —       | Comma-separated extensions to include only    |
| `--exclude`     | —       | Comma-separated extensions to drop            |
| `--theme NAME`  | gruvbox | `gruvbox`, `obsidian`, `forest`, `aurora`, `mono` |

---

## What it does

```
grf invoked
  → scans directory tree (respects --depth, ignores .git / node_modules / etc.)
  → builds a graph: files + folders as nodes, parent relationships as edges
  → reads each parseable file and adds content edges (imports, links)
  → writes .grafux/graph.json
  → starts a local HTTP server
  → opens your default browser
  → renders an interactive force-directed graph on HTML5 Canvas
```

**Edge types:**

| Type         | Meaning                                                     | Drawn as              |
| ------------ | ----------------------------------------------------------- | --------------------- |
| `structural` | The directory tree — a file's parent folder                 | Faint, underneath     |
| `import`     | A Go/JS/TS/Python import that resolves inside the project    | Bright, on top        |
| `reference`  | A markdown `[[wikilink]]` or `[text](relative/path)`         | Bright, on top        |

Every content edge carries a confidence: **extracted** when the source named its
target unambiguously, **inferred** when several candidates matched and the
resolver had to pick one. Toggle content edges on and off in the settings panel.

**What gets parsed:**

| Language        | Extensions                                              | How                             |
| --------------- | ------------------------------------------------------- | ------------------------------- |
| Go              | `.go`                                                    | `go/parser`, resolved via `go.mod` |
| JavaScript / TS | `.js .jsx .mjs .cjs .ts .tsx .mts .cts .vue .svelte .astro` | relative specifiers only     |
| Python          | `.py .pyi`                                               | absolute and relative imports   |
| Markdown        | `.md .mdx .markdown`                                     | wikilinks and inline links      |

Only references that resolve to a file in the scan become edges — third-party
packages and external URLs are left out. Code inside fenced blocks and
commented-out imports are ignored.

**Node types:**

| Type        | Color      | Size                           |
| ----------- | ---------- | ------------------------------ |
| Root folder | Amber      | Largest — center of graph      |
| Subfolder   | Orange-red | Scales with number of children |
| File        | Blue       | Small, uniform                 |

**Interactions:**

| Input                   | Effect                                                                   |
| ----------------------- | ------------------------------------------------------------------------ |
| Hover node              | Highlights node + direct connections, dims everything else, shows labels |
| Drag node               | Pick up and reposition; releases back into physics                       |
| Scroll                  | Zoom in/out                                                              |
| Click + drag background | Pan                                                                      |
| Double-click background | Reset zoom                                                               |

---

## Ignored by default

`.git` · `node_modules` · `__pycache__` · `.DS_Store` · `.idea` · `.vscode` · hidden files/folders (`.` prefix)

---

## Roadmap

- [x] **v0.1** — Directory scan, force-directed graph, dark theme, drag/zoom/pan, hover highlights, auto-open browser
- [x] **v0.2** — Content edges from Go/JS/TS/Python imports and markdown links, persisted `graph.json`, `query`/`explain`/`neighbors`/`path` commands
- [ ] **v0.3** — MCP server, token-budgeted context packs, hub and community detection, symbol-level nodes
- [ ] **v0.4** — Spanning tree overlays, layout algorithm selection, physics settings panel
- [ ] **v0.5** — WebGL rendering for 10k+ node graphs, watch mode (live filesystem updates), PNG/SVG export

---

## Development

```bash
# Run from source
go run . [flags] [path]

# Project layout
grafux/
├── main.go                  # CLI: verb dispatch, flags, scanner, server, browser open
├── cli/cli.go               # scan / query / explain / neighbors / path
├── scanner/scanner.go       # filepath.Walk → nodes + structural edges
├── extract/                 # content edges: imports and links inside files
│   ├── extract.go           # extractor interface + the content pass
│   ├── resolve.go           # specifier → node ID, with confidence
│   └── golang.go, jsts.go, python.go, markdown.go
├── query/query.go           # search, traversal, shortest path, explain
├── store/store.go           # .grafux/graph.json read + atomic write
├── server/
│   ├── server.go            # HTTP server + go:embed
│   └── web/
│       ├── index.html       # Single-page shell
│       ├── style.css        # Dark theme
│       └── graph.js         # d3-force simulation + canvas renderer
└── go.mod
```

Frontend is embedded into the Go binary at build time via `go:embed`. No build step, no bundler — vanilla JS served directly.
