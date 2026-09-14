'use strict';

// ─── Themes ───────────────────────────────────────────────────────────────────
const THEMES = {
  gruvbox: {
    bg: '#141618', root: '#f0c78d', folder: '#dfaa69', file: '#87b9bd',
    edge: 'rgba(173,184,191,0.22)', edgeHover: 'rgba(234,199,153,0.75)',
    edgeDim: 'rgba(173,184,191,0.045)', contentEdge: 'rgba(154,145,188,0.35)',
    hoverRing: 'rgba(240,199,141,0.65)', label: '#e9e9e3',
    labelNeighbor: 'rgba(206,213,213,0.75)', accent: '#eab775',
  },
  obsidian: {
    bg:            '#13131f',
    root:          '#f4a261',
    folder:        '#e76f51',
    file:          '#4db8ff',
    edge:          'rgba(255,255,255,0.18)',
    edgeHover:     'rgba(255,255,255,0.65)',
    edgeDim:       'rgba(255,255,255,0.05)',
    contentEdge:   'rgba(122,162,247,0.5)',
    hoverRing:     'rgba(255,255,255,0.55)',
    label:         'rgba(255,255,255,1)',
    labelNeighbor: 'rgba(255,255,255,0.6)',
    accent:        '#e76f51',
  },
  forest: {
    bg:            '#0c180c',
    root:          '#95d5b2',
    folder:        '#52b788',
    file:          '#40916c',
    edge:          'rgba(149,213,178,0.2)',
    edgeHover:     'rgba(149,213,178,0.7)',
    edgeDim:       'rgba(149,213,178,0.04)',
    contentEdge:   'rgba(244,211,94,0.5)',
    hoverRing:     'rgba(149,213,178,0.6)',
    label:         'rgba(210,240,220,1)',
    labelNeighbor: 'rgba(210,240,220,0.65)',
    accent:        '#52b788',
  },
  aurora: {
    bg:            '#0b0b18',
    root:          '#c4b5fd',
    folder:        '#818cf8',
    file:          '#38bdf8',
    edge:          'rgba(196,181,253,0.18)',
    edgeHover:     'rgba(196,181,253,0.7)',
    edgeDim:       'rgba(196,181,253,0.04)',
    contentEdge:   'rgba(244,114,182,0.5)',
    hoverRing:     'rgba(196,181,253,0.6)',
    label:         'rgba(230,225,255,1)',
    labelNeighbor: 'rgba(230,225,255,0.65)',
    accent:        '#818cf8',
  },
  mono: {
    bg:            '#111111',
    root:          '#ffffff',
    folder:        '#bbbbbb',
    file:          '#666666',
    edge:          'rgba(255,255,255,0.14)',
    edgeHover:     'rgba(255,255,255,0.6)',
    edgeDim:       'rgba(255,255,255,0.03)',
    contentEdge:   'rgba(255,255,255,0.42)',
    hoverRing:     'rgba(255,255,255,0.5)',
    label:         'rgba(255,255,255,1)',
    labelNeighbor: 'rgba(255,255,255,0.55)',
    accent:        '#bbbbbb',
  },
};

let activeTheme = THEMES.gruvbox;

// Visual settings — defaults match config.Defaults(); overridden by /api/config response
const settings = {
  fileRadius:      5,
  folderBase:      8,
  folderScale:     2.5,
  edgeWidth:       1.0,
  labelZoom:       2.0,
  // Physics defaults — overridden by /api/config
  chargeStrength:  -150,
  chargeMax:       600,
  linkDistance:     80,
  linkStrength:    0.25,
  centerStrength:  0.02,
  collideStrength: 0.8,
  alphaDecay:      0.015,
  velocityDecay:   0.25,
  // Layout
  layout:          'force',
};

// Snapshot of initial defaults for Reset button
const defaultSettings = Object.assign({}, settings);

function applyTheme(name) {
  activeTheme = THEMES[name] || THEMES.gruvbox;
  document.documentElement.style.setProperty('--bg', activeTheme.bg);
  document.documentElement.style.setProperty('--accent', activeTheme.accent);
  document.documentElement.style.setProperty('--folder', activeTheme.folder);
  document.documentElement.style.setProperty('--file', activeTheme.file);
  document.documentElement.style.setProperty('--content', activeTheme.contentEdge);
  document.getElementById('theme-select').value = THEMES[name] ? name : 'gruvbox';
  if (graphData.nodes.length) updateExplorer();
  scheduleRender();
}

// ─── State ────────────────────────────────────────────────────────────────────
let graphData = { nodes: [], edges: [] };
let simulation = null;
let transform = d3.zoomIdentity;
let mouse = { x: -9999, y: -9999 };
let hoveredNode = null;
let dragNode = null;
let selectedNode = null;
let pointerStart = null;
let zoomBehavior = null;
let searchMatches = null;
const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
let rafPending = false;
const adjacency = new Map();
let nodeQuadtree = null;

// ─── Canvas ───────────────────────────────────────────────────────────────────
const canvas = document.getElementById('graph');
const ctx = canvas.getContext('2d');

function resize() {
  const dpr = window.devicePixelRatio || 1;
  const W = window.innerWidth;
  const H = window.innerHeight;
  canvas.width  = W * dpr;
  canvas.height = H * dpr;
  canvas.style.width  = W + 'px';
  canvas.style.height = H + 'px';
  if (simulation) {
    const area = graphArea();
    simulation.force('center', d3.forceCenter(area.cx, area.cy).strength(settings.centerStrength));
    simulation.force('radial').x(area.cx).y(area.cy);
    fitGraph(false);
    scheduleRender();
  }
}

window.addEventListener('resize', resize);
resize();

// ─── Node visuals ─────────────────────────────────────────────────────────────
function nodeRadius(n) {
  if (n._r !== undefined) return n._r;
  if (n.type === 'folder') {
    const base = settings.folderBase;
    return Math.max(base + 2, base + Math.sqrt(n.children || 0) * settings.folderScale);
  }
  return settings.fileRadius;
}

function cacheRadii() {
  for (const n of graphData.nodes) {
    n._r = undefined; // clear so nodeRadius computes fresh
    n._r = nodeRadius(n);
  }
}

function nodeColor(n) {
  if (n.depth === 0)       return activeTheme.root;
  if (n.type === 'folder') return activeTheme.folder;
  if (activeTheme === THEMES.mono) return activeTheme.file;
  if (['.md', '.mdx', '.txt', '.rst'].includes(n.extension)) return '#b1a0cc';
  if (['.json', '.yml', '.yaml', '.toml', '.mod', '.sum'].includes(n.extension)) return '#b0b78c';
  return activeTheme.file;
}

// ─── Coordinate helpers ───────────────────────────────────────────────────────
function screenToSim(sx, sy) {
  return transform.invert([sx, sy]);
}

function findNodeAt(sx, sy) {
  const [wx, wy] = screenToSim(sx, sy);
  if (!nodeQuadtree) return null;
  // d3.quadtree.find with a search radius
  const maxR = settings.folderBase + 20; // generous search radius
  const found = nodeQuadtree.find(wx, wy, maxR);
  if (!found) return null;
  const r = (found._r || nodeRadius(found)) + 4;
  const dx = found.x - wx;
  const dy = found.y - wy;
  if (dx * dx + dy * dy < r * r) return found;
  return null;
}

function rebuildQuadtree() {
  nodeQuadtree = d3.quadtree()
    .x(d => d.x)
    .y(d => d.y)
    .addAll(graphData.nodes.filter(n => n.x !== undefined));
}

// ─── Adjacency ────────────────────────────────────────────────────────────────
function buildAdjacency() {
  adjacency.clear();
  for (const e of graphData.edges) {
    const s = typeof e.source === 'object' ? e.source.id : e.source;
    const t = typeof e.target === 'object' ? e.target.id : e.target;
    if (!adjacency.has(s)) adjacency.set(s, new Set());
    if (!adjacency.has(t)) adjacency.set(t, new Set());
    adjacency.get(s).add(t);
    adjacency.get(t).add(s);
  }
}

function isConnected(a, b) {
  const nbrs = adjacency.get(a.id);
  return nbrs ? nbrs.has(b.id) : false;
}

// ─── Spanning Tree (Kruskal's) ────────────────────────────────────────────────
let spanningTreeEdges = null; // Set of "srcId|tgtId" keys when active
let showContentEdges = true;  // content edges are on by default

function computeSpanningTree() {
  // Union-Find
  const parent = new Map();
  const rank = new Map();
  function find(x) {
    if (parent.get(x) !== x) parent.set(x, find(parent.get(x)));
    return parent.get(x);
  }
  function union(a, b) {
    const ra = find(a), rb = find(b);
    if (ra === rb) return false;
    if (rank.get(ra) < rank.get(rb)) parent.set(ra, rb);
    else if (rank.get(ra) > rank.get(rb)) parent.set(rb, ra);
    else { parent.set(rb, ra); rank.set(ra, rank.get(ra) + 1); }
    return true;
  }
  for (const n of graphData.nodes) {
    parent.set(n.id, n.id);
    rank.set(n.id, 0);
  }

  const treeEdges = new Set();
  for (const e of graphData.edges) {
    const sid = typeof e.source === 'object' ? e.source.id : e.source;
    const tid = typeof e.target === 'object' ? e.target.id : e.target;
    if (union(sid, tid)) {
      treeEdges.add(sid + '|' + tid);
      treeEdges.add(tid + '|' + sid);
    }
  }
  return treeEdges;
}

// Content edges come from inside files (imports, links); structural edges are
// the directory tree. They are drawn differently because they mean different
// things.
function isContentEdge(e) {
  return e.type !== undefined && e.type !== 'structural';
}

function skipEdge(e) {
  return !showContentEdges && isContentEdge(e);
}

function isSpanningTreeEdge(e) {
  if (!spanningTreeEdges) return false;
  const sid = typeof e.source === 'object' ? e.source.id : e.source;
  const tid = typeof e.target === 'object' ? e.target.id : e.target;
  return spanningTreeEdges.has(sid + '|' + tid);
}

// ─── Layout algorithms ───────────────────────────────────────────────────────
function layoutRadial() {
  const { cx, cy } = graphArea();
  const nodeMap = new Map();
  for (const n of graphData.nodes) nodeMap.set(n.id, n);

  // BFS from root (depth 0 node)
  const root = graphData.nodes.find(n => n.depth === 0) || graphData.nodes[0];
  const visited = new Set();
  const levels = []; // levels[depth] = [nodes...]
  const queue = [root];
  visited.add(root.id);

  while (queue.length > 0) {
    const node = queue.shift();
    const d = node.depth || 0;
    if (!levels[d]) levels[d] = [];
    levels[d].push(node);
    const nbrs = adjacency.get(node.id);
    if (nbrs) {
      for (const nid of nbrs) {
        if (!visited.has(nid)) {
          visited.add(nid);
          const child = nodeMap.get(nid);
          if (child) queue.push(child);
        }
      }
    }
  }
  // Add any unvisited nodes to their depth level
  for (const n of graphData.nodes) {
    if (!visited.has(n.id)) {
      const d = n.depth || 0;
      if (!levels[d]) levels[d] = [];
      levels[d].push(n);
    }
  }

  const ringSpacing = 80;
  for (let d = 0; d < levels.length; d++) {
    if (!levels[d]) continue;
    if (d === 0) {
      for (const n of levels[d]) { n.fx = cx; n.fy = cy; }
      continue;
    }
    const r = d * ringSpacing;
    const count = levels[d].length;
    for (let i = 0; i < count; i++) {
      const angle = (2 * Math.PI * i) / count - Math.PI / 2;
      levels[d][i].fx = cx + r * Math.cos(angle);
      levels[d][i].fy = cy + r * Math.sin(angle);
    }
  }
}

function layoutTree() {
  const cx = window.innerWidth / 2;
  const topY = 80;
  const levelHeight = 70;
  const nodeMap = new Map();
  for (const n of graphData.nodes) nodeMap.set(n.id, n);

  const root = graphData.nodes.find(n => n.depth === 0) || graphData.nodes[0];
  const visited = new Set();
  const levels = [];
  const queue = [root];
  visited.add(root.id);

  while (queue.length > 0) {
    const node = queue.shift();
    const d = node.depth || 0;
    if (!levels[d]) levels[d] = [];
    levels[d].push(node);
    const nbrs = adjacency.get(node.id);
    if (nbrs) {
      for (const nid of nbrs) {
        if (!visited.has(nid)) {
          visited.add(nid);
          const child = nodeMap.get(nid);
          if (child) queue.push(child);
        }
      }
    }
  }
  for (const n of graphData.nodes) {
    if (!visited.has(n.id)) {
      const d = n.depth || 0;
      if (!levels[d]) levels[d] = [];
      levels[d].push(n);
    }
  }

  for (let d = 0; d < levels.length; d++) {
    if (!levels[d]) continue;
    const count = levels[d].length;
    const totalWidth = Math.max(count * 30, window.innerWidth * 0.8);
    const startX = cx - totalWidth / 2;
    const spacing = count > 1 ? totalWidth / (count - 1) : 0;
    for (let i = 0; i < count; i++) {
      levels[d][i].fx = count > 1 ? startX + spacing * i : cx;
      levels[d][i].fy = topY + d * levelHeight;
    }
  }
}

function clearPinnedPositions() {
  for (const n of graphData.nodes) {
    n.fx = null;
    n.fy = null;
  }
}

function applyLayout(name) {
  settings.layout = name;
  if (name === 'radial') {
    if (simulation) simulation.stop();
    layoutRadial();
    // Copy pinned positions to actual positions for immediate render
    for (const n of graphData.nodes) {
      if (n.fx != null) n.x = n.fx;
      if (n.fy != null) n.y = n.fy;
    }
    rebuildQuadtree();
    scheduleRender();
  } else if (name === 'tree') {
    if (simulation) simulation.stop();
    layoutTree();
    for (const n of graphData.nodes) {
      if (n.fx != null) n.x = n.fx;
      if (n.fy != null) n.y = n.fy;
    }
    rebuildQuadtree();
    scheduleRender();
  } else {
    // force layout — clear pins, restart simulation
    clearPinnedPositions();
    if (simulation) simulation.alpha(0.5).restart();
  }
  document.getElementById('layout-caption').textContent = name.toUpperCase() + ' LAYOUT';
  if (name !== 'force') fitGraph();
  // Update layout pills UI
  document.querySelectorAll('.layout-pill').forEach(p => {
    p.classList.toggle('active', p.dataset.layout === name);
    p.setAttribute('aria-pressed', p.dataset.layout === name);
  });
}

// ─── Simulation ───────────────────────────────────────────────────────────────
function setupSimulation() {
  const { cx, cy } = graphArea();

  // Spread initial positions outward by depth so nodes don't clump at center
  const maxDepth = graphData.nodes.reduce((m, n) => Math.max(m, n.depth || 0), 1);
  for (const n of graphData.nodes) {
    const d = n.depth || 0;
    const angle = Math.random() * Math.PI * 2;
    const spread = d * 60 + Math.random() * 40;
    n.x = cx + Math.cos(angle) * spread;
    n.y = cy + Math.sin(angle) * spread;
  }

  cacheRadii();

  simulation = d3.forceSimulation(graphData.nodes)
    .force('link', d3.forceLink(graphData.edges)
      .id(d => d.id)
      .distance(d => settings.linkDistance + (d.source._r || 0) + (d.target._r || 0))
      .strength(settings.linkStrength))
    .force('charge', d3.forceManyBody()
      .strength(d => settings.chargeStrength - (d._r || 0) * 14)
      .distanceMax(settings.chargeMax))
    .force('center', d3.forceCenter(cx, cy).strength(settings.centerStrength))
    .force('collide', d3.forceCollide()
      .radius(d => (d._r || 0) + 5)
      .strength(settings.collideStrength))
    .force('radial', d3.forceRadial(
      d => (d.depth || 0) * 70,
      cx, cy
    ).strength(0.03))
    .alphaDecay(settings.alphaDecay)
    .velocityDecay(settings.velocityDecay)
    .on('tick', () => {
      rebuildQuadtree();
      scheduleRender();
    });

  buildAdjacency();
}

function updateSimulationParams() {
  if (!simulation) return;
  const linkForce = simulation.force('link');
  if (linkForce) {
    linkForce.distance(d => settings.linkDistance + (d.source._r || 0) + (d.target._r || 0));
    linkForce.strength(settings.linkStrength);
  }
  const chargeForce = simulation.force('charge');
  if (chargeForce) {
    chargeForce.strength(d => settings.chargeStrength - (d._r || 0) * 14);
    chargeForce.distanceMax(settings.chargeMax);
  }
  const centerForce = simulation.force('center');
  if (centerForce) {
    centerForce.strength(settings.centerStrength);
  }
  const collideForce = simulation.force('collide');
  if (collideForce) {
    collideForce.strength(settings.collideStrength);
  }
  const radialForce = simulation.force('radial');
  if (radialForce) {
    radialForce.radius(d => (d.depth || 0) * 70);
  }
  simulation.alphaDecay(settings.alphaDecay);
  simulation.velocityDecay(settings.velocityDecay);
  simulation.alpha(0.3).restart();
}

// ─── Render loop ──────────────────────────────────────────────────────────────
function scheduleRender() {
  if (rafPending) return;
  rafPending = true;
  requestAnimationFrame(() => {
    rafPending = false;
    render();
  });
}

function render() {
  const dpr = window.devicePixelRatio || 1;
  const W = canvas.width;
  const H = canvas.height;
  const Wl = W / dpr;  // logical width
  const Hl = H / dpr;  // logical height
  ctx.clearRect(0, 0, W, H);

  ctx.save();
  ctx.scale(dpr, dpr);
  ctx.translate(transform.x, transform.y);
  ctx.scale(transform.k, transform.k);

  const k = transform.k;
  const ew = settings.edgeWidth;
  const focusNode = hoveredNode || selectedNode;
  const hasHover = focusNode !== null;
  const nodeCount = graphData.nodes.length;
  const showSpanning = !!spanningTreeEdges;

  // ── Viewport culling bounds (in simulation space) ─────────────────────────
  const pad = 50 / k;
  const vx0 = -transform.x / k - pad;
  const vy0 = -transform.y / k - pad;
  const vx1 = (Wl - transform.x) / k + pad;
  const vy1 = (Hl - transform.y) / k + pad;

  function inView(x, y) {
    return x >= vx0 && x <= vx1 && y >= vy0 && y <= vy1;
  }

  function edgeInView(sx, sy, tx, ty) {
    // Check if either endpoint or the bounding box overlaps viewport
    if (inView(sx, sy) || inView(tx, ty)) return true;
    const minx = Math.min(sx, tx), maxx = Math.max(sx, tx);
    const miny = Math.min(sy, ty), maxy = Math.max(sy, ty);
    return minx <= vx1 && maxx >= vx0 && miny <= vy1 && maxy >= vy0;
  }

  // ── Edges (batched) ───────────────────────────────────────────────────────
  if (hasHover) {
    // Two passes: dim edges first, highlighted on top
    ctx.beginPath();
    ctx.strokeStyle = activeTheme.edgeDim;
    ctx.lineWidth = (0.5 * ew) / k;
    for (const e of graphData.edges) {
      const src = e.source, tgt = e.target;
      if (src.x === undefined || tgt.x === undefined) continue;
      if (skipEdge(e)) continue;
      if (!edgeInView(src.x, src.y, tgt.x, tgt.y)) continue;
      if (src === focusNode || tgt === focusNode) continue;
      ctx.moveTo(src.x, src.y);
      ctx.lineTo(tgt.x, tgt.y);
    }
    ctx.stroke();

    ctx.beginPath();
    ctx.strokeStyle = activeTheme.edgeHover;
    ctx.lineWidth = (1.5 * ew) / k;
    for (const e of graphData.edges) {
      const src = e.source, tgt = e.target;
      if (src.x === undefined || tgt.x === undefined) continue;
      if (skipEdge(e)) continue;
      if (src !== focusNode && tgt !== focusNode) continue;
      ctx.moveTo(src.x, src.y);
      ctx.lineTo(tgt.x, tgt.y);
    }
    ctx.stroke();
  } else if (showSpanning) {
    // Non-tree edges (dim)
    ctx.beginPath();
    ctx.strokeStyle = activeTheme.edgeDim;
    ctx.lineWidth = (0.5 * ew) / k;
    for (const e of graphData.edges) {
      const src = e.source, tgt = e.target;
      if (src.x === undefined || tgt.x === undefined) continue;
      if (skipEdge(e)) continue;
      if (!edgeInView(src.x, src.y, tgt.x, tgt.y)) continue;
      if (isSpanningTreeEdge(e)) continue;
      ctx.moveTo(src.x, src.y);
      ctx.lineTo(tgt.x, tgt.y);
    }
    ctx.stroke();

    // Tree edges (accent, thicker)
    ctx.beginPath();
    ctx.strokeStyle = activeTheme.accent;
    ctx.lineWidth = (2.0 * ew) / k;
    for (const e of graphData.edges) {
      const src = e.source, tgt = e.target;
      if (src.x === undefined || tgt.x === undefined) continue;
      if (!edgeInView(src.x, src.y, tgt.x, tgt.y)) continue;
      if (skipEdge(e) || !isSpanningTreeEdge(e)) continue;
      ctx.moveTo(src.x, src.y);
      ctx.lineTo(tgt.x, tgt.y);
    }
    ctx.stroke();
  } else {
    // Structural edges — the directory tree, drawn quietly underneath.
    ctx.beginPath();
    ctx.strokeStyle = activeTheme.edge;
    ctx.lineWidth = (0.8 * ew) / k;
    for (const e of graphData.edges) {
      const src = e.source, tgt = e.target;
      if (src.x === undefined || tgt.x === undefined) continue;
      if (isContentEdge(e)) continue;
      if (!edgeInView(src.x, src.y, tgt.x, tgt.y)) continue;
      ctx.moveTo(src.x, src.y);
      ctx.lineTo(tgt.x, tgt.y);
    }
    ctx.stroke();

    // Content edges — imports and links read out of the files themselves.
    if (showContentEdges) {
      ctx.beginPath();
      ctx.strokeStyle = activeTheme.contentEdge || activeTheme.edgeHover;
      ctx.lineWidth = (1.3 * ew) / k;
      for (const e of graphData.edges) {
        const src = e.source, tgt = e.target;
        if (src.x === undefined || tgt.x === undefined) continue;
        if (!isContentEdge(e)) continue;
        if (!edgeInView(src.x, src.y, tgt.x, tgt.y)) continue;
        ctx.moveTo(src.x, src.y);
        ctx.lineTo(tgt.x, tgt.y);
      }
      ctx.stroke();
    }
  }

  // ── Nodes ──────────────────────────────────────────────────────────────────
  // Level-of-detail: skip shadowBlur when zoomed out or large graph
  const skipGlow = k < 0.5 || nodeCount > 2000;

  for (const n of graphData.nodes) {
    if (n.x === undefined) continue;
    if (!inView(n.x, n.y)) continue;

    const r = n._r || nodeRadius(n);
    const color = nodeColor(n);
    const isHover = n === focusNode;
    const isNeighbor = hasHover && !isHover && isConnected(n, focusNode);
    const isDim = (hasHover && !isHover && !isNeighbor) || (searchMatches && !searchMatches.has(n.id));

    if (!isDim && n.type === 'folder' && !skipGlow) {
      ctx.beginPath();
      ctx.arc(n.x, n.y, r + 5 / k, 0, Math.PI * 2);
      ctx.fillStyle = color + '0c';
      ctx.fill();
      ctx.strokeStyle = color + '30';
      ctx.lineWidth = 1 / k;
      ctx.stroke();
    }
    ctx.beginPath();
    ctx.arc(n.x, n.y, r, 0, Math.PI * 2);

    if (isHover && !skipGlow) {
      ctx.shadowBlur = 20;
      ctx.shadowColor = color;
      ctx.fillStyle = color;
    } else if (isHover) {
      ctx.shadowBlur = 0;
      ctx.fillStyle = color;
    } else if (isDim) {
      ctx.shadowBlur = 0;
      ctx.fillStyle = color + '28'; // ~16% opacity
    } else {
      ctx.shadowBlur = 0;
      ctx.fillStyle = color;
    }

    ctx.fill();
    ctx.shadowBlur = 0;

    if (isHover) {
      ctx.beginPath();
      ctx.arc(n.x, n.y, r + 2.5 / k, 0, Math.PI * 2);
      ctx.strokeStyle = activeTheme.hoverRing;
      ctx.lineWidth = 1.5 / k;
      ctx.stroke();
    }
  }

  // ── Labels ─────────────────────────────────────────────────────────────────
  // Level-of-detail: cap "show all labels" for very large graphs
  const showAllLabels = k >= settings.labelZoom && nodeCount < 5000;
  if (nodeCount) {
    const fontSize = 11 / k;
    ctx.font = `${fontSize}px -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif`;
    ctx.textBaseline = 'middle';

    for (const n of graphData.nodes) {
      if (n.x === undefined) continue;
      if (!inView(n.x, n.y)) continue;
      const isHover = n === focusNode;
      const isNeighbor = hasHover && isConnected(n, focusNode);
      if (!showAllLabels && !isHover && !isNeighbor && !(n.type === 'folder' && k >= 0.35 && nodeCount < 1000) && !(searchMatches && searchMatches.has(n.id))) continue;
      const r = n._r || nodeRadius(n);
      const isDimmed = hasHover && !isHover && !isNeighbor;
      ctx.fillStyle = isHover ? activeTheme.label : activeTheme.labelNeighbor;
      ctx.globalAlpha = isDimmed ? 0.2 : 1;
      ctx.strokeStyle = activeTheme.bg;
      ctx.lineWidth = 4 / k;
      ctx.lineJoin = 'round';
      const label = n.name.length > 32 ? n.name.slice(0, 29) + '…' : n.name;
      ctx.strokeText(label, n.x + r + 8 / k, n.y);
      ctx.fillText(label, n.x + r + 8 / k, n.y);
    }
    ctx.globalAlpha = 1;
  }

  ctx.restore();
}

// ─── Interactions ─────────────────────────────────────────────────────────────
function setupInteractions() {
  canvas.addEventListener('mousedown', onMouseDown, { capture: true });
  canvas.addEventListener('mousemove', onMouseMove);
  window.addEventListener('mouseup', onMouseUp);
  canvas.addEventListener('mouseleave', onMouseLeave);

  zoomBehavior = d3.zoom()
    .scaleExtent([0.04, 14])
    .filter(event => {
      if (event.type === 'mousedown' && event.button === 0) {
        const rect = canvas.getBoundingClientRect();
        return !findNodeAt(event.clientX - rect.left, event.clientY - rect.top);
      }
      return !event.button;
    })
    .on('zoom', event => {
      transform = event.transform;
      document.getElementById('zoom-level').textContent = Math.round(transform.k * 100) + '%';
      scheduleRender();
    });

  d3.select(canvas).call(zoomBehavior).on('dblclick.zoom', null);

  canvas.addEventListener('dblclick', (e) => {
    const node = findNodeAt(
      e.clientX - canvas.getBoundingClientRect().left,
      e.clientY - canvas.getBoundingClientRect().top
    );
    if (!node) {
      fitGraph();
    }
  });
}

function onMouseMove(e) {
  const rect = canvas.getBoundingClientRect();
  mouse.x = e.clientX - rect.left;
  mouse.y = e.clientY - rect.top;

  if (dragNode) {
    const [wx, wy] = screenToSim(mouse.x, mouse.y);
    dragNode.fx = wx;
    dragNode.fy = wy;
    if (settings.layout !== 'force') {
      dragNode.x = wx;
      dragNode.y = wy;
      rebuildQuadtree();
    }
    scheduleRender();
  } else {
    const prev = hoveredNode;
    hoveredNode = findNodeAt(mouse.x, mouse.y);
    canvas.style.cursor = hoveredNode ? 'grab' : 'default';
    if (hoveredNode !== prev) scheduleRender();
  }
}

function onMouseDown(e) {
  if (e.button !== 0) return;
  pointerStart = { x: e.clientX, y: e.clientY };
  const rect = canvas.getBoundingClientRect();
  const node = findNodeAt(e.clientX - rect.left, e.clientY - rect.top);
  if (node) {
    e.stopImmediatePropagation();
    dragNode = node;
    node.fx = node.x;
    node.fy = node.y;
    canvas.style.cursor = 'grabbing';
    if (simulation && settings.layout === 'force') simulation.alphaTarget(0.02).restart();
  }
}

function onMouseUp(e) {
  const clicked = pointerStart && Math.hypot(e.clientX - pointerStart.x, e.clientY - pointerStart.y) < 5;
  if (clicked) selectNode(dragNode);
  pointerStart = null;
  if (dragNode) {
    // For static layouts, keep the pin; for force, release it
    if (settings.layout === 'force') {
      dragNode.fx = null;
      dragNode.fy = null;
    }
    if (simulation) simulation.alphaTarget(0);
    dragNode = null;
  }
  canvas.style.cursor = hoveredNode ? 'grab' : 'default';
}

function onMouseLeave() {
  mouse = { x: -9999, y: -9999 };
  if (!dragNode) {
    hoveredNode = null;
    scheduleRender();
  }
}

// ─── Settings Panel ──────────────────────────────────────────────────────────
function setupSettingsPanel() {
  const toggle = document.getElementById('settings-toggle');
  const panel = document.getElementById('settings-panel');
  if (!toggle || !panel) return;

  function setOpen(open) {
    panel.classList.toggle('open', open);
    toggle.classList.toggle('open', open);
    toggle.setAttribute('aria-expanded', open);
    panel.inert = !open;
    if (open) document.getElementById('settings-close').focus();
    else toggle.focus();
  }
  toggle.addEventListener('click', () => setOpen(!panel.classList.contains('open')));
  document.getElementById('settings-close').addEventListener('click', () => setOpen(false));
  document.addEventListener('keydown', e => {
    if (e.key === 'Escape' && panel.classList.contains('open')) setOpen(false);
  });
  document.getElementById('theme-select').addEventListener('change', e => applyTheme(e.target.value));

  // Map slider IDs to settings keys
  const sliderMap = {
    'slider-charge':   'chargeStrength',
    'slider-chargemax':'chargeMax',
    'slider-linkdist': 'linkDistance',
    'slider-linkstr':  'linkStrength',
    'slider-center':   'centerStrength',
    'slider-collide':  'collideStrength',
    'slider-alpha':    'alphaDecay',
    'slider-velocity': 'velocityDecay',
  };

  for (const [id, key] of Object.entries(sliderMap)) {
    const slider = document.getElementById(id);
    const display = document.getElementById(id + '-val');
    if (!slider) continue;
    // Set initial value from settings
    slider.value = settings[key];
    if (display) display.textContent = settings[key];

    slider.addEventListener('input', () => {
      settings[key] = parseFloat(slider.value);
      if (display) display.textContent = slider.value;
      if (settings.layout === 'force') updateSimulationParams();
    });
  }

  // Reset button
  const resetBtn = document.getElementById('settings-reset');
  if (resetBtn) {
    resetBtn.addEventListener('click', () => {
      for (const [id, key] of Object.entries(sliderMap)) {
        settings[key] = defaultSettings[key];
        const slider = document.getElementById(id);
        const display = document.getElementById(id + '-val');
        if (slider) slider.value = settings[key];
        if (display) display.textContent = settings[key];
      }
      if (settings.layout === 'force') updateSimulationParams();
    });
  }

  // Layout pills
  document.querySelectorAll('.layout-pill').forEach(pill => {
    pill.addEventListener('click', () => applyLayout(pill.dataset.layout));
  });

  // Content edge checkbox
  const ceCheck = document.getElementById('content-edge-toggle');
  if (ceCheck) {
    ceCheck.addEventListener('change', () => {
      showContentEdges = ceCheck.checked;
      scheduleRender();
    });
  }

  // Spanning tree checkbox
  const stCheck = document.getElementById('spanning-tree-toggle');
  if (stCheck) {
    stCheck.addEventListener('change', () => {
      spanningTreeEdges = stCheck.checked ? computeSpanningTree() : null;
      scheduleRender();
    });
  }
}

// ─── Workspace controls ──────────────────────────────────────────────────────
function graphArea() {
  const sidebar = parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--sidebar')) || 0;
  const left = sidebar + 45;
  const top = sidebar ? 160 : 185;
  const width = Math.max(100, window.innerWidth - left - 65);
  const height = Math.max(100, window.innerHeight - top - 115);
  return { left, top, width, height, cx: left + width / 2, cy: top + height / 2 };
}

function moveView(next, animate = true) {
  if (!zoomBehavior) return;
  const selection = d3.select(canvas);
  selection.interrupt();
  if (animate && !reducedMotion) selection.transition().duration(350).call(zoomBehavior.transform, next);
  else selection.call(zoomBehavior.transform, next);
}

function fitGraph(animate = true) {
  const nodes = graphData.nodes.filter(n => Number.isFinite(n.x) && Number.isFinite(n.y));
  if (!nodes.length) return;
  const area = graphArea();
  const x0 = d3.min(nodes, n => n.x - n._r - 15);
  const x1 = d3.max(nodes, n => n.x + n._r + 55);
  const y0 = d3.min(nodes, n => n.y - n._r - 15);
  const y1 = d3.max(nodes, n => n.y + n._r + 15);
  const k = Math.max(0.04, Math.min(1.5, area.width / (x1 - x0 || 1), area.height / (y1 - y0 || 1)));
  moveView(d3.zoomIdentity.translate(area.cx - (x0 + x1) * k / 2, area.cy - (y0 + y1) * k / 2).scale(k), animate);
}

function selectNode(node, center = false) {
  selectedNode = node;
  const details = document.getElementById('node-details');
  details.hidden = !node;
  if (node) {
    document.getElementById('detail-kind').textContent = node.type === 'folder' ? 'FOLDER DETAILS' : 'FILE DETAILS';
    document.getElementById('detail-name').textContent = node.name;
    document.getElementById('detail-path').textContent = node.path || node.id;
    const size = node.type === 'folder' ? `${node.children || 0} children` : node.size < 1024 ? `${node.size || 0} B` : `${(node.size / 1024).toFixed(1)} KB`;
    const degree = adjacency.get(node.id)?.size || 0;
    document.getElementById('detail-meta').textContent = `${size} · ${degree} ${degree === 1 ? 'connection' : 'connections'}`;
    if (center) {
      const area = graphArea();
      const k = Math.max(transform.k, 1.2);
      moveView(d3.zoomIdentity.translate(area.cx - node.x * k, area.cy - node.y * k).scale(k));
    }
  }
  document.querySelectorAll('.file-row').forEach(row => {
    const active = row.dataset.nodeId === node?.id;
    row.classList.toggle('active', active);
    row.setAttribute('aria-pressed', active);
  });
  scheduleRender();
}

function updateExplorer() {
  const term = document.getElementById('search').value.trim().toLowerCase();
  const matches = graphData.nodes.filter(n => n.id.toLowerCase().includes(term) || n.name.toLowerCase().includes(term));
  searchMatches = term ? new Set(matches.map(n => n.id)) : null;
  const sorted = matches.slice().sort((a, b) => (a.type === 'folder' ? 0 : 1) - (b.type === 'folder' ? 0 : 1) || a.depth - b.depth || a.name.localeCompare(b.name));
  const results = document.getElementById('search-results');
  results.replaceChildren();
  document.getElementById('results-heading').textContent = term ? 'SEARCH RESULTS' : 'PROJECT FILES';
  document.getElementById('result-count').textContent = sorted.length > 80 ? `80 / ${sorted.length}` : sorted.length;
  if (!sorted.length) {
    const empty = document.createElement('p');
    empty.className = 'no-results';
    empty.textContent = 'No matches. Try a file name, path, or extension.';
    results.append(empty);
  }
  for (const n of sorted.slice(0, 80)) {
    const row = document.createElement('button');
    row.className = 'file-row' + (selectedNode === n ? ' active' : '');
    row.setAttribute('aria-pressed', selectedNode === n);
    row.title = n.id;
    row.dataset.nodeId = n.id;
    const icon = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    icon.setAttribute('viewBox', '0 0 20 20');
    icon.setAttribute('aria-hidden', 'true');
    icon.style.color = nodeColor(n);
    const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
    path.setAttribute('d', n.type === 'folder' ? 'M2 5h6l2 2h8v9H2Z' : 'M5 2h6l4 4v12H5ZM11 2v5h4');
    icon.append(path);
    const copy = document.createElement('span');
    copy.className = 'row-copy';
    const name = document.createElement('span');
    name.className = 'row-name';
    name.textContent = n.name;
    copy.append(name);
    if (term && n.id !== n.name) {
      const pathLabel = document.createElement('span');
      pathLabel.className = 'row-path';
      pathLabel.textContent = n.id;
      copy.append(pathLabel);
    }
    const degree = document.createElement('span');
    degree.className = 'row-degree';
    degree.textContent = adjacency.get(n.id)?.size || '—';
    row.append(icon, copy, degree);
    row.addEventListener('click', () => {
      selectNode(n, true);
      if (window.innerWidth <= 700) row.blur();
    });
    results.append(row);
  }
  scheduleRender();
}

function setupExplorer() {
  document.getElementById('search').addEventListener('input', () => {
    selectedNode = null;
    document.getElementById('node-details').hidden = true;
    updateExplorer();
  });
  document.getElementById('search').addEventListener('keydown', e => {
    if (e.key === 'Enter') document.querySelector('.file-row')?.click();
    if (e.key === 'Escape') { e.target.value = ''; e.target.blur(); updateExplorer(); }
  });
  document.getElementById('clear-selection').addEventListener('click', () => selectNode(null));
  document.getElementById('fit-view').addEventListener('click', () => fitGraph());
  for (const [id, factor] of [['zoom-in', 1.3], ['zoom-out', 1 / 1.3]]) {
    document.getElementById(id).addEventListener('click', () => {
      const { cx, cy } = graphArea();
      d3.select(canvas).call(zoomBehavior.scaleBy, factor, [cx, cy]);
    });
  }
  document.addEventListener('keydown', e => {
    if (e.target.matches('input, select, textarea') || e.ctrlKey || e.metaKey || e.altKey) return;
    if (e.key === '/') { e.preventDefault(); document.getElementById('search').focus(); }
    if (e.key.toLowerCase() === 'f') fitGraph();
    if (e.key === 'Escape') selectNode(null);
  });
  updateExplorer();
}

// ─── Init ─────────────────────────────────────────────────────────────────────
async function init() {
  const statsEl = document.getElementById('stats');


  try {
    // Fetch config and graph in parallel
    const [cfgRes, graphRes] = await Promise.all([
      fetch('/api/config'),
      fetch('/api/graph'),
    ]);

    // Respect the server's configured palette.
    let themeName = 'gruvbox';
    if (cfgRes.ok) {
      const cfg = await cfgRes.json();
      // Apply visual settings from server config
      if (cfg.fileRadius      != null) settings.fileRadius      = cfg.fileRadius;
      if (cfg.folderBase      != null) settings.folderBase      = cfg.folderBase;
      if (cfg.folderScale     != null) settings.folderScale     = cfg.folderScale;
      if (cfg.edgeWidth       != null) settings.edgeWidth       = cfg.edgeWidth;
      if (cfg.labelZoom       != null) settings.labelZoom       = cfg.labelZoom;
      // Physics settings
      if (cfg.chargeStrength  != null) settings.chargeStrength  = cfg.chargeStrength;
      if (cfg.chargeMax       != null) settings.chargeMax       = cfg.chargeMax;
      if (cfg.linkDistance    != null) settings.linkDistance     = cfg.linkDistance;
      if (cfg.linkStrength    != null) settings.linkStrength    = cfg.linkStrength;
      if (cfg.centerStrength  != null) settings.centerStrength  = cfg.centerStrength;
      if (cfg.collideStrength != null) settings.collideStrength = cfg.collideStrength;
      if (cfg.alphaDecay      != null) settings.alphaDecay      = cfg.alphaDecay;
      if (cfg.velocityDecay   != null) settings.velocityDecay   = cfg.velocityDecay;
      if (cfg.layout          != null) settings.layout          = cfg.layout;
      if (cfg.theme) themeName = cfg.theme;
    }
    // Update defaultSettings snapshot after server config applied
    Object.assign(defaultSettings, settings);

    applyTheme(themeName);

    if (!graphRes.ok) throw new Error(`HTTP ${graphRes.status}`);
    graphData = await graphRes.json();

    if (!graphData.nodes || graphData.nodes.length === 0) {
      statsEl.textContent = 'No files found';
      document.getElementById('empty-state').hidden = false;
      return;
    }

    const m = graphData.meta || {};
    const root = graphData.nodes.find(n => n.depth === 0);
    document.getElementById('project-name').textContent = root ? root.name : 'Project';
    document.title = `${root ? root.name : 'Project'} · Grafux`;
    document.getElementById('project-path').textContent = m.root || '';
    document.getElementById('project-path').title = m.root || '';
    document.getElementById('file-count').textContent = (m.totalFiles || 0).toLocaleString();
    document.getElementById('folder-count').textContent = (m.totalFolders || 0).toLocaleString();
    document.getElementById('link-count').textContent = (m.contentEdges || 0).toLocaleString();
    statsEl.textContent = `${graphData.nodes.length.toLocaleString()} nodes · ${m.scanDepth > 0 ? 'depth ' + m.scanDepth : 'all depths'}`;

    setupSimulation();
    setupInteractions();
    setupSettingsPanel();
    setupExplorer();
    // Settle the first frame without blocking startup on very large graphs.
    simulation.tick(Math.min(100, Math.max(5, Math.floor(50000 / graphData.nodes.length))));
    rebuildQuadtree();
    fitGraph(false);

    // Apply initial layout if not force
    if (settings.layout !== 'force') {
      applyLayout(settings.layout);
    }

    scheduleRender();
  } catch (err) {
    console.error('Grafux error:', err);
    statsEl.textContent = 'Graph unavailable';
    document.getElementById('empty-state').hidden = false;
    document.getElementById('empty-title').textContent = 'Couldn’t load this graph';
    document.getElementById('empty-message').textContent = `Refresh to try again. ${err.message}`;
  }
}

init();

// Heartbeat — tells the server this tab is still open (every 3s)
setInterval(() => fetch('/api/ping').catch(() => {}), 3000);
