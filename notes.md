---
Tech Stack

┌────────────────┬────────────────────────────┬────────────────────────────────────────────────────────────────────────────┐
│ Layer │ Technology │ Why │
├────────────────┼────────────────────────────┼────────────────────────────────────────────────────────────────────────────┤
│ CLI + Server │ Go │ Single binary, fast startup, great stdlib for HTTP and filesystem ops │
├────────────────┼────────────────────────────┼────────────────────────────────────────────────────────────────────────────┤
│ Frontend │ Vanilla JS + HTML5 Canvas │ No build step, no framework, embedded directly into the binary │
├────────────────┼────────────────────────────┼────────────────────────────────────────────────────────────────────────────┤
│ Graph physics │ d3-force (loaded from CDN) │ Industry-standard force simulation, handles repulsion/attraction/collision │
├────────────────┼────────────────────────────┼────────────────────────────────────────────────────────────────────────────┤
│ Asset delivery │ Go embed package │ Compiles HTML/CSS/JS into the binary — no external files needed │
└────────────────┴────────────────────────────┴────────────────────────────────────────────────────────────────────────────┘
---

Architecture in one sentence: Grafux is a Go binary that scans a directory, spins up a local HTTP server with the frontend baked in, and
serves an interactive Canvas visualization powered by d3-force.

---

Data flow (30-second version):

1. User runs grf → flags + config parsed
2. Scanner walks the filesystem → builds nodes + edges JSON
3. Go HTTP server starts → frontend HTML/JS served from embedded memory, graph JSON served at /api/graph
4. Browser opens → fetches graph data → d3-force simulation runs → Canvas renders the graph
5. User interacts → JS handles hover, drag, zoom, layout switching in real-time
