'use strict';

// ─── Themes ───────────────────────────────────────────────────────────────────
const THEMES = {
  gruvbox: {
    bg:            '#282828',
    root:          '#fabd2f',
    folder:        '#fe8019',
    file:          '#8ec07c',
    edge:          'rgba(146,131,116,0.25)',
    edgeHover:     'rgba(235,219,178,0.75)',
    edgeDim:       'rgba(146,131,116,0.05)',
    hoverRing:     'rgba(250,189,47,0.5)',
    label:         'rgba(235,219,178,1)',
    labelNeighbor: 'rgba(235,219,178,0.65)',
    accent:        '#fabd2f',
  },
  obsidian: {
    bg:            '#13131f',
    root:          '#f4a261',
    folder:        '#e76f51',
    file:          '#4db8ff',
    edge:          'rgba(255,255,255,0.18)',
    edgeHover:     'rgba(255,255,255,0.65)',
    edgeDim:       'rgba(255,255,255,0.05)',
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
  chargeStrength:  -100,
  chargeMax:       400,
  linkDistance:     55,
  linkStrength:    0.4,
  centerStrength:  0.05,
  collideStrength: 0.8,
  alphaDecay:      0.02,
  velocityDecay:   0.35,
  // Layout
  layout:          'force',
};

// Snapshot of initial defaults for Reset button
const defaultSettings = Object.assign({}, settings);

function applyTheme(name) {
  activeTheme = THEMES[name] || THEMES.gruvbox;
  document.documentElement.style.setProperty('--bg', activeTheme.bg);
  document.documentElement.style.setProperty('--accent', activeTheme.accent);
  document.body.style.background = activeTheme.bg;
  document.querySelectorAll('.theme-dot').forEach(d => {
    d.classList.toggle('active', d.dataset.theme === name);
  });
  try { sessionStorage.setItem('grafux-theme', name); } catch (_) {}
  scheduleRender();
}

function setupThemeSwitcher() {
  document.querySelectorAll('.theme-dot').forEach(dot => {
    dot.addEventListener('click', () => applyTheme(dot.dataset.theme));
  });
}

// ─── State ────────────────────────────────────────────────────────────────────
let graphData = { nodes: [], edges: [] };
let simulation = null;
let transform = d3.zoomIdentity;
let mouse = { x: -9999, y: -9999 };
let hoveredNode = null;
let dragNode = null;
let rafPending = false;
const adjacency = new Map();
let nodeQuadtree = null;

// ─── Canvas ───────────────────────────────────────────────────────────────────
const canvas = document.getElementById('graph');
const ctx = canvas.getContext('2d');

function resize() {
  canvas.width = window.innerWidth;
  canvas.height = window.innerHeight;
  if (simulation) {
    simulation.force('center', d3.forceCenter(canvas.width / 2, canvas.height / 2));
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

function isSpanningTreeEdge(e) {
  if (!spanningTreeEdges) return false;
  const sid = typeof e.source === 'object' ? e.source.id : e.source;
  const tid = typeof e.target === 'object' ? e.target.id : e.target;
  return spanningTreeEdges.has(sid + '|' + tid);
}

// ─── Layout algorithms ───────────────────────────────────────────────────────
function layoutRadial() {
  const cx = canvas.width / 2;
  const cy = canvas.height / 2;
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
  const cx = canvas.width / 2;
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
    const totalWidth = Math.max(count * 30, canvas.width * 0.8);
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
  // Update layout pills UI
  document.querySelectorAll('.layout-pill').forEach(p => {
    p.classList.toggle('active', p.dataset.layout === name);
  });
}

// ─── Simulation ───────────────────────────────────────────────────────────────
function setupSimulation() {
  const cx = canvas.width / 2;
  const cy = canvas.height / 2;

  for (const n of graphData.nodes) {
    n.x = cx + (Math.random() - 0.5) * 30;
    n.y = cy + (Math.random() - 0.5) * 30;
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
  const W = canvas.width;
  const H = canvas.height;
  ctx.clearRect(0, 0, W, H);

  ctx.save();
  ctx.translate(transform.x, transform.y);
  ctx.scale(transform.k, transform.k);

  const k = transform.k;
  const ew = settings.edgeWidth;
  const hasHover = hoveredNode !== null;
  const nodeCount = graphData.nodes.length;
  const showSpanning = !!spanningTreeEdges;

  // ── Viewport culling bounds (in simulation space) ─────────────────────────
  const pad = 50 / k;
  const vx0 = -transform.x / k - pad;
  const vy0 = -transform.y / k - pad;
  const vx1 = (W - transform.x) / k + pad;
  const vy1 = (H - transform.y) / k + pad;

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
      if (!edgeInView(src.x, src.y, tgt.x, tgt.y)) continue;
      if (src === hoveredNode || tgt === hoveredNode) continue;
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
      if (src !== hoveredNode && tgt !== hoveredNode) continue;
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
      if (!isSpanningTreeEdge(e)) continue;
      ctx.moveTo(src.x, src.y);
      ctx.lineTo(tgt.x, tgt.y);
    }
    ctx.stroke();
  } else {
    // Single batch — all edges same style
    ctx.beginPath();
    ctx.strokeStyle = activeTheme.edge;
    ctx.lineWidth = (0.8 * ew) / k;
    for (const e of graphData.edges) {
      const src = e.source, tgt = e.target;
      if (src.x === undefined || tgt.x === undefined) continue;
      if (!edgeInView(src.x, src.y, tgt.x, tgt.y)) continue;
      ctx.moveTo(src.x, src.y);
      ctx.lineTo(tgt.x, tgt.y);
    }
    ctx.stroke();
  }

  // ── Nodes ──────────────────────────────────────────────────────────────────
  // Level-of-detail: skip shadowBlur when zoomed out or large graph
  const skipGlow = k < 0.5 || nodeCount > 2000;

  for (const n of graphData.nodes) {
    if (n.x === undefined) continue;
    if (!inView(n.x, n.y)) continue;

    const r = n._r || nodeRadius(n);
    const color = nodeColor(n);
    const isHover = n === hoveredNode;
    const isNeighbor = hasHover && !isHover && isConnected(n, hoveredNode);
    const isDim = hasHover && !isHover && !isNeighbor;

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
  if (hasHover || showAllLabels) {
    const fontSize = Math.max(9, 13 / k);
    ctx.font = `${fontSize}px -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif`;
    ctx.textBaseline = 'middle';

    for (const n of graphData.nodes) {
      if (n.x === undefined) continue;
      if (!inView(n.x, n.y)) continue;
      const isHover = n === hoveredNode;
      const isNeighbor = hasHover && isConnected(n, hoveredNode);
      if (!showAllLabels && !isHover && !isNeighbor) continue;
      const r = n._r || nodeRadius(n);
      const isDimmed = hasHover && !isHover && !isNeighbor;
      ctx.fillStyle = isHover ? activeTheme.label : activeTheme.labelNeighbor;
      ctx.globalAlpha = isDimmed ? 0.2 : 1;
      ctx.fillText(n.name, n.x + (r + 5) / k, n.y);
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

  const zoomBehavior = d3.zoom()
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
      scheduleRender();
    });

  d3.select(canvas).call(zoomBehavior);

  canvas.addEventListener('dblclick', (e) => {
    const node = findNodeAt(
      e.clientX - canvas.getBoundingClientRect().left,
      e.clientY - canvas.getBoundingClientRect().top
    );
    if (!node) {
      d3.select(canvas)
        .transition()
        .duration(400)
        .call(zoomBehavior.transform, d3.zoomIdentity);
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
    if (simulation) simulation.alphaTarget(0.3).restart();
  } else {
    const prev = hoveredNode;
    hoveredNode = findNodeAt(mouse.x, mouse.y);
    canvas.style.cursor = hoveredNode ? 'grab' : 'default';
    if (hoveredNode !== prev) scheduleRender();
  }
}

function onMouseDown(e) {
  if (e.button !== 0) return;
  const rect = canvas.getBoundingClientRect();
  const node = findNodeAt(e.clientX - rect.left, e.clientY - rect.top);
  if (node) {
    e.stopImmediatePropagation();
    dragNode = node;
    node.fx = node.x;
    node.fy = node.y;
    canvas.style.cursor = 'grabbing';
    if (simulation) simulation.alphaTarget(0.1).restart();
  }
}

function onMouseUp() {
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

  toggle.addEventListener('click', () => {
    panel.classList.toggle('open');
    toggle.classList.toggle('open');
  });

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

  // Spanning tree checkbox
  const stCheck = document.getElementById('spanning-tree-toggle');
  if (stCheck) {
    stCheck.addEventListener('change', () => {
      spanningTreeEdges = stCheck.checked ? computeSpanningTree() : null;
      scheduleRender();
    });
  }
}

// ─── Init ─────────────────────────────────────────────────────────────────────
async function init() {
  const statsEl = document.getElementById('stats');

  setupThemeSwitcher();

  try {
    // Fetch config and graph in parallel
    const [cfgRes, graphRes] = await Promise.all([
      fetch('/api/config'),
      fetch('/api/graph'),
    ]);

    // Determine initial theme: server config → sessionStorage (within-session switch) → default
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

    // sessionStorage overrides server theme only if user switched themes this session
    try { themeName = sessionStorage.getItem('grafux-theme') || themeName; } catch (_) {}
    applyTheme(themeName);

    if (!graphRes.ok) throw new Error(`HTTP ${graphRes.status}`);
    graphData = await graphRes.json();

    if (!graphData.nodes || graphData.nodes.length === 0) {
      statsEl.textContent = 'No files found';
      return;
    }

    const m = graphData.meta || {};
    const depthStr = m.scanDepth > 0 ? `depth ${m.scanDepth}` : 'unlimited depth';
    statsEl.textContent = `${m.totalFiles || 0} files · ${m.totalFolders || 0} folders · ${depthStr}`;

    setupSimulation();
    setupInteractions();
    setupSettingsPanel();

    // Apply initial layout if not force
    if (settings.layout !== 'force') {
      applyLayout(settings.layout);
    }

    scheduleRender();
  } catch (err) {
    console.error('Grafux error:', err);
    statsEl.textContent = `Error: ${err.message}`;
  }
}

init();
