// canvas.js — iframe interaction lifecycle for V2
import { buildMarkerIndex, visualRectForInstance } from "./markers.js";
import { Overlay } from "./overlay.js";
import { state, displayNameForBlock } from "./state.js";

function normalizePath(p) {
  if (!p || p === "") return "/";
  let s = String(p).trim();
  if (!s.startsWith("/")) s = "/" + s;
  s = s.replace(/\/+$/, "");
  if (s === "") s = "/";
  return s;
}

function getCurrentResourceInfo() {
  const origin = state.publicOrigin || window.location.origin;
  const pathname = normalizePath(state.publicPath || "/");
  const search = state.publicSearch || "";
  return { origin, pathname, search };
}

function isSameResourceFragment(rawHref) {
  if (!rawHref) return false;
  const trimmed = String(rawHref).trim();
  if (trimmed === "") return false;
  if (/^[a-z][a-z0-9+.-]*:/i.test(trimmed) && !/^https?:/i.test(trimmed)) return false;
  if (!trimmed.startsWith("#") && !trimmed.startsWith("/") && !/^https?:\/\//i.test(trimmed)) return false;
  if (trimmed === "#") return true;
  if (trimmed.startsWith("#")) {
    const frag = trimmed.slice(1);
    return frag.trim().length > 0;
  }
  const hashIndex = trimmed.indexOf("#");
  if (hashIndex === -1) return false;
  try {
    const current = getCurrentResourceInfo();
    const baseForResolve = current.origin + current.pathname + current.search;
    const url = new URL(trimmed, baseForResolve);
    if (url.origin !== current.origin) return false;
    const linkPathname = normalizePath(url.pathname);
    const linkSearch = url.search || "";
    if (linkPathname !== current.pathname) return false;
    if (linkSearch !== current.search) return false;
    return url.hash.length > 0;
  } catch (_) {
    return false;
  }
}

function findAnchorTarget(doc, hash) {
  if (!doc || !hash) return null;
  let fragment = hash.startsWith("#") ? hash.slice(1) : hash;
  try {
    fragment = decodeURIComponent(fragment);
  } catch (_) {}
  fragment = fragment.trim();
  if (!fragment) return null;
  let el = null;
  try { el = doc.getElementById(fragment); } catch (_) { el = null; }
  if (el) return el;
  try {
    const byName = doc.getElementsByName(fragment);
    if (byName && byName.length) return byName[0];
  } catch (_) {}
  return null;
}

function handleSamePageAnchor(doc, rawHref) {
  if (!doc) return;
  const trimmed = String(rawHref).trim();
  if (trimmed === "#") {
    const scroller = doc.scrollingElement || doc.documentElement || doc.body;
    if (scroller && typeof scroller.scrollTo === "function") {
      try { scroller.scrollTo({ top: 0, behavior: "smooth" }); } catch (_) {
        scroller.scrollTop = 0;
        if (doc.body) doc.body.scrollTop = 0;
      }
    } else {
      if (doc.documentElement) doc.documentElement.scrollTop = 0;
      if (doc.body) doc.body.scrollTop = 0;
    }
    return;
  }
  try {
    const current = getCurrentResourceInfo();
    const baseForResolve = current.origin + current.pathname + current.search;
    const url = new URL(trimmed, baseForResolve);
    const hash = url.hash || (trimmed.startsWith("#") ? trimmed : "");
    if (!hash || hash === "#") {
      const scroller = doc.scrollingElement || doc.documentElement || doc.body;
      if (scroller && scroller.scrollTo) scroller.scrollTo({ top: 0, behavior: "smooth" });
      return;
    }
    const target = findAnchorTarget(doc, hash);
    if (target && typeof target.scrollIntoView === "function") {
      try { target.scrollIntoView({ behavior: "smooth", block: "start" }); } catch (_) { target.scrollIntoView(); }
    }
  } catch (_) {
    if (trimmed.startsWith("#")) {
      const target = findAnchorTarget(doc, trimmed);
      if (target && target.scrollIntoView) {
        try { target.scrollIntoView({ behavior: "smooth", block: "start" }); } catch (_) { target.scrollIntoView(); }
      }
    }
  }
}

// Helper to get blockName for displayName — we need to map nodeId -> block.
// Bootstrap definitions + catalog give us block names, but we need nodeId mapping.
// We can extract from state.document SDT? For Collection repeated nodes, nodeId still maps to same block.
// So build map nodeId -> block once.
let nodeIdToBlockCache = null;
let nodeIdToBlockCacheDoc = null;
function buildNodeIdToBlock() {
  const curDoc = state.document;
  if (nodeIdToBlockCache && nodeIdToBlockCacheDoc === curDoc) return nodeIdToBlockCache;
  const map = new Map();
  function walk(nodes) {
    if (!Array.isArray(nodes)) return;
    for (const n of nodes) {
      if (n && n.id && n.block) map.set(n.id, n.block);
      if (n && n.children) walk(n.children);
    }
  }
  if (curDoc && Array.isArray(curDoc.nodes)) walk(curDoc.nodes);
  nodeIdToBlockCache = map;
  nodeIdToBlockCacheDoc = curDoc;
  return map;
}

function labelForInstance(instance) {
  if (!instance) return "Block";
  const map = buildNodeIdToBlock();
  const block = map.get(instance.nodeId);
  const isExternal = !instance.editable;
  // Try registry displayName
  let display = "";
  if (block) {
    display = displayNameForBlock(block);
  } else {
    // No block mapping — could be external/template node not in Page SDT
    // Do NOT show nodeId technical ID
    if (isExternal) {
      // For external, try to infer from owner: if site-part, use owner friendly if available and not technical
      if (instance.ownerType === "site-part" && instance.ownerLabel && !isTechnicalId(instance.ownerLabel)) {
        display = instance.ownerLabel;
      } else {
        // Unknown external → generic
        return "Template element";
      }
    } else {
      return "Block";
    }
  }

  // For external/read-only, append context
  if (isExternal) {
    if (instance.ownerType === "site-part") {
      // Prefer friendly site part name if available and not technical
      if (instance.ownerLabel && !isTechnicalId(instance.ownerLabel)) {
        // e.g., "Main Header" → show as is, no duplication
        // But if display is same as owner, don't duplicate
        if (display.toLowerCase() === instance.ownerLabel.toLowerCase()) return display;
        return `${display} · Site Part`;
      }
      return `${display} · Site Part`;
    }
    // Template external (layout template, etc)
    // Spec examples: "Entry Title · Template"
    return `${display} · Template`;
  }
  return display;
}

function isTechnicalId(s) {
  if (!s || typeof s !== "string") return true;
  const t = s.trim();
  if (t.length < 3) return true;
  // Heuristic: UUID, blk_..., random base64-like without spaces
  if (/^[a-z0-9_-]{8,}$/i.test(t) && !t.includes(" ") && !t.includes("/")) {
    // If it looks like ID and not human words (no space, short)
    // But "Main Header" has space, so not technical
    // "AaGwQYWccYC7wM3" matches this → technical
    // Also check if it contains only alnum and is long
    if (t.length >= 8 && /^[A-Za-z0-9_-]+$/.test(t) && !/[aeiou]/i.test(t.slice(0,4))) {
      // Very rough: but treat as technical if no displayName-like
    }
    // Simpler: if string has no space and length>6 and not in known display names, treat as technical
    // For now, if it has no space and is not humanized block, assume technical
    if (!t.includes(" ") && t.length > 6) {
      // Check if it's known block displayName? Those are short words with spaces maybe
      // For safety, treat UUID-like as technical
      if (/^(blk_|entry|site|page)_/i.test(t) || /^[A-Fa-f0-9-]{10,}$/.test(t) || /^[A-Za-z0-9]{10,}$/.test(t)) return true;
    }
  }
  // If string looks like UUID
  if (/^[0-9a-f]{8}-[0-9a-f]{4}-/i.test(t)) return true;
  return false;
}

export class CanvasController {
  constructor(iframe, stage) {
    this.iframe = iframe;
    this.stage = stage || null;
    this.doc = null;
    this.win = null;
    this.overlay = null;
    this.index = new Map();
    this.elementToNode = new WeakMap();
    this.nodeToKeys = new Map();
    this.hoverInst = null;
    this.selected = null; // {nodeId, instanceKey, editable, owner...}
    this._rafPending = false;
    this._onMove = this.onMove.bind(this);
    this._onClick = this.onClick.bind(this);
    this._onScroll = this.onScroll.bind(this);
    this._onResize = this.onResize.bind(this);
    this._onKey = this.onKey.bind(this);
    this._onAux = this.onAuxClick.bind(this);
    this._onSubmitBlock = this.onSubmitBlock.bind(this);
    this._boundWinEvents = false;
  }

  onSubmitBlock(e) {
    try { e.preventDefault(); e.stopPropagation(); } catch (_) {}
    const formEl = e.target;
    const hit = formEl ? this.hitForTarget(formEl) : null;
    if (hit) this.selectInstance(hit);
  }

  attach(doc) {
    this.destroy();
    if (!doc) {
      try { doc = this.iframe.contentDocument; } catch (_) { return; }
    }
    if (!doc) return;
    this.doc = doc;
    try { this.win = doc.defaultView || this.iframe.contentWindow; } catch (_) { this.win = null; }

    // Build marker index
    const built = buildMarkerIndex(doc);
    this.index = built.index;
    this.elementToNode = built.elementToNode;
    this.nodeToKeys = built.nodeToKeys;

    // Create overlay
    this.overlay = new Overlay(doc);
    this.overlay.attach();

    // Bind events inside iframe
    try {
      doc.addEventListener("pointermove", this._onMove, true);
      doc.addEventListener("mousemove", this._onMove, true);
      doc.addEventListener("click", this._onClick, true);
      doc.addEventListener("auxclick", this._onAux, true);
      doc.addEventListener("submit", this._onSubmitBlock, true);
      doc.addEventListener("keydown", this._onKey, true);
    } catch (_) {}

    if (this.win) {
      try {
        this.win.addEventListener("scroll", this._onScroll, { passive: true });
        this.win.addEventListener("resize", this._onResize);
        this._boundWinEvents = true;
      } catch (_) {}
    } else {
      try { doc.addEventListener("scroll", this._onScroll, { passive: true }); } catch (_) {}
    }

    // Parent key handler for Escape (also bind inside iframe above)
    try { document.addEventListener("keydown", this._onKey); } catch (_) {}

    // Initial sync
    this.syncGeometry();
    // Restore selection if state already has one (e.g., after viewport switch without reload)
    if (state.selection && this.index.has(state.selection.instanceKey)) {
      this.selected = state.selection;
      this.syncGeometry();
    }
  }

  destroy() {
    try {
      if (this.doc) {
        this.doc.removeEventListener("pointermove", this._onMove, true);
        this.doc.removeEventListener("mousemove", this._onMove, true);
        this.doc.removeEventListener("click", this._onClick, true);
        this.doc.removeEventListener("auxclick", this._onAux, true);
        this.doc.removeEventListener("submit", this._onSubmitBlock, true);
        this.doc.removeEventListener("keydown", this._onKey, true);
      }
    } catch (_) {}
    try {
      if (this.win && this._boundWinEvents) {
        this.win.removeEventListener("scroll", this._onScroll);
        this.win.removeEventListener("resize", this._onResize);
      }
    } catch (_) {}
    try { document.removeEventListener("keydown", this._onKey); } catch (_) {}
    if (this.overlay) {
      try { this.overlay.destroy(); } catch (_) {}
    }
    this.overlay = null;
    this.doc = null;
    this.win = null;
    this.index = new Map();
    this.elementToNode = new WeakMap();
    this.nodeToKeys = new Map();
    this.hoverInst = null;
    // keep this.selected? we keep state.selection, but clear local reference
    // Do not clear state.selection on destroy — it persists across reloads if same node still exists
    this._rafPending = false;
    this._boundWinEvents = false;
  }

  rebuildIndex() {
    if (!this.doc) return;
    const built = buildMarkerIndex(this.doc);
    this.index = built.index;
    this.elementToNode = built.elementToNode;
    this.nodeToKeys = built.nodeToKeys;
    this.syncGeometry();
  }

  hitForTarget(target) {
    if (!target || !this.elementToNode) return null;
    let el = target;
    if (el.nodeType === 3) el = el.parentElement; // TEXT_NODE
    while (el) {
      if (this.elementToNode.has(el)) return this.elementToNode.get(el);
      if (el === this.doc.documentElement) break;
      el = el.parentElement;
    }
    return null;
  }

  visualRect(instance) {
    if (!instance) return null;
    return visualRectForInstance(instance, this.doc);
  }

  onMove(e) {
    if (!this.doc || !this.overlay) return;
    const hit = this.hitForTarget(e.target);
    if (!hit) {
      if (this.hoverInst) {
        this.hoverInst = null;
        state.hoveredKey = null;
        this.overlay.clearHover();
      }
      return;
    }
    // If hover is same as selected, don't show hover
    if (this.selected && hit.instanceKey === this.selected.instanceKey) {
      if (this.hoverInst) {
        this.hoverInst = null;
        state.hoveredKey = null;
        this.overlay.clearHover();
      }
      return;
    }
    if (this.hoverInst && this.hoverInst.instanceKey === hit.instanceKey) {
      // Already hovered, but need to ensure geometry current (maybe scroll)
      return;
    }
    this.hoverInst = hit;
    state.hoveredKey = hit.instanceKey;
    const rect = this.visualRect(hit);
    if (!rect) {
      this.overlay.clearHover();
      return;
    }
    // Check viewport visibility — if rect is outside, still show but it will be clipped
    const isExternal = !hit.editable;
    this.overlay.setHover(rect, isExternal);
  }

  onClick(e) {
    if (!this.doc || !this.overlay) return;
    // Handle submit separately: block but also select
    if (e.type === "submit") {
      try { e.preventDefault(); e.stopPropagation(); } catch (_) {}
      // Try to find form's block
      const formEl = e.target;
      const hit = formEl ? this.hitForTarget(formEl) : null;
      if (hit) {
        this.selectInstance(hit);
      }
      return;
    }

    const target = e.target;
    // Auxclick already handled? but click path handles anchors

    // Check anchor first for navigation logic
    let anchor = null;
    try { anchor = target.closest ? target.closest("a[href]") : null; } catch (_) { anchor = null; }
    if (anchor) {
      const href = anchor.getAttribute("href") || "";
      const tgt = (anchor.getAttribute("target") || "").trim().toLowerCase();
      if (tgt === "_blank") {
        e.preventDefault();
        // no stopPropagation before selection — let selection happen
        const hit = this.hitForTarget(target);
        if (hit) this.selectInstance(hit);
        else {
          // Even if no hit, block navigation
          try { e.stopPropagation(); } catch (_) {}
        }
        return;
      }
      if (isSameResourceFragment(href)) {
        e.preventDefault();
        const hit = this.hitForTarget(target);
        if (hit) this.selectInstance(hit);
        try { e.stopPropagation(); } catch (_) {}
        return;
      }
      // Cross-page: block navigation, but still allow selection
      e.preventDefault();
      const hit = this.hitForTarget(target);
      if (hit) {
        this.selectInstance(hit);
      }
      try { e.stopPropagation(); } catch (_) {}
      return;
    }

    // Check interactive elements (button, input submit, form)
    let interactive = null;
    try { interactive = target.closest ? target.closest("button, input[type='submit'], input[type='button'], [role='link'], [role='button'], form") : null; } catch (_) {}
    if (interactive) {
      const tag = interactive.tagName;
      const role = interactive.getAttribute ? interactive.getAttribute("role") : "";
      if (tag === "BUTTON" || tag === "INPUT" || role === "button" || role === "link") {
        e.preventDefault();
        const hit = this.hitForTarget(target);
        if (hit) this.selectInstance(hit);
        try { e.stopPropagation(); } catch (_) {}
        return;
      }
      if (tag === "FORM") {
        e.preventDefault();
        const hit = this.hitForTarget(target);
        if (hit) this.selectInstance(hit);
        try { e.stopPropagation(); } catch (_) {}
        return;
      }
    }

    // Normal block click
    const hit = this.hitForTarget(target);
    if (hit) {
      e.preventDefault();
      try { e.stopPropagation(); } catch (_) {}
      this.selectInstance(hit);
      return;
    }

    // Click in empty area (mapped? hit null) → clear selection
    // But if clicked element is mapped to Section (Section padding), hit would be Section, we already handled.
    // Only clear if truly no mapped node
    e.preventDefault();
    try { e.stopPropagation(); } catch (_) {}
    this.clearSelection();
  }

  onAuxClick(e) {
    try {
      const a = e.target.closest ? e.target.closest("a[href]") : null;
      if (a) {
        e.preventDefault();
        e.stopPropagation();
      }
    } catch (_) {}
  }

  selectInstance(instance) {
    if (!instance) return;
    const sel = {
      nodeId: instance.nodeId,
      instanceKey: instance.instanceKey,
      editable: instance.editable,
      ownerType: instance.ownerType,
      ownerId: instance.ownerId,
      ownerLabel: instance.ownerLabel,
    };
    this.selected = sel;
    state.selection = sel;
    // Hover should not show for selected
    if (this.hoverInst && this.hoverInst.instanceKey === sel.instanceKey) {
      this.hoverInst = null;
      state.hoveredKey = null;
      if (this.overlay) this.overlay.clearHover();
    }
    this.syncGeometry();
  }

  clearSelection() {
    this.selected = null;
    state.selection = null;
    if (this.hoverInst) {
      // keep hover? will be recomputed on next move
    }
    if (this.overlay) this.overlay.clearSelection();
  }

  onScroll() {
    this.requestSync();
  }

  onResize() {
    this.requestSync();
  }

  requestSync() {
    if (this._rafPending) return;
    this._rafPending = true;
    const raf = (this.win && this.win.requestAnimationFrame) ? this.win.requestAnimationFrame.bind(this.win) : requestAnimationFrame;
    raf(() => {
      this._rafPending = false;
      this.syncGeometry();
    });
  }

  syncGeometry() {
    if (!this.overlay || !this.doc) return;
    // Update scope boundary (primary editable region)
    this.updateScope();
    // Hover
    if (this.hoverInst) {
      const inst = this.index.get(this.hoverInst.instanceKey) || this.hoverInst;
      const rect = this.visualRect(inst);
      if (!rect) {
        this.overlay.clearHover();
      } else {
        // If hover is selected, hide hover
        if (this.selected && this.selected.instanceKey === inst.instanceKey) {
          this.overlay.clearHover();
        } else {
          const isExternal = !inst.editable;
          this.overlay.setHover(rect, isExternal);
        }
      }
    } else {
      // Clear hover if no hoverInst
      if (this.overlay) this.overlay.clearHover();
    }
    // Selection
    if (this.selected) {
      // Ensure still in index (after reload, instanceKey may be stale if node removed)
      let inst = this.index.get(this.selected.instanceKey);
      if (!inst) {
        this.overlay.clearSelection();
        return;
      }
      const rect = this.visualRect(inst);
      if (!rect) {
        this.overlay.clearSelection();
        return;
      }
      const label = labelForInstance(inst);
      const isExternal = !inst.editable;
      this.overlay.setSelected(rect, label, { external: isExternal });
    } else {
      this.overlay.clearSelection();
    }
  }

  updateScope() {
    if (!this.overlay || !this.doc) return;
    // Compute union of all primary editable instances
    const primaryInstances = [];
    for (const inst of this.index.values()) {
      if (inst.editable) primaryInstances.push(inst);
    }
    if (primaryInstances.length === 0) {
      this.overlay.clearScope();
      return;
    }
    const rects = [];
    for (const inst of primaryInstances) {
      const r = this.visualRect(inst);
      if (r && r.width > 0 && r.height > 0) rects.push(r);
    }
    if (rects.length === 0) {
      this.overlay.clearScope();
      return;
    }
    // Union all rects
    let union = rects[0];
    for (let i=1;i<rects.length;i++) {
      const r = rects[i];
      const left = Math.min(union.left, r.left);
      const top = Math.min(union.top, r.top);
      const right = Math.max(union.left + union.width, r.left + r.width);
      const bottom = Math.max(union.top + union.height, r.top + r.height);
      union = { left, top, width: right-left, height: bottom-top, right, bottom };
    }
    // If union is huge and includes gaps (e.g., header/footer gaps) the box may be misleading,
    // but for typical Page with header/footer external, union will be middle contiguous region.
    // Add small padding for visibility? Keep tight.
    this.overlay.setScope(union);
  }

  onKey(e) {
    if (!e) return;
    if (e.key === "Escape") {
      // If selection exists, clear it. Do not prevent overflow menu's own Escape if no selection
      if (this.selected) {
        e.preventDefault();
        // Don't stopPropagation? Let overflow menu also handle? But spec: if overflow is open, its Escape still works.
        // We clear selection and let event пузырь continue for overflow.
        this.clearSelection();
        return;
      }
    }
  }

  // For viewport switch to re-sync
  notifyViewportChanged() {
    // Marker index stays, geometry needs refresh after layout reflow
    // Use double RAF to wait for layout
    this.requestSync();
    const raf = (this.win && this.win.requestAnimationFrame) ? this.win.requestAnimationFrame.bind(this.win) : requestAnimationFrame;
    raf(() => this.requestSync());
  }
}
