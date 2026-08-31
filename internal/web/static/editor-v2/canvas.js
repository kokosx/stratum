// canvas.js — iframe interaction lifecycle for V2
import { buildMarkerIndex, visualRectForInstance } from "./markers.js";
import { Overlay } from "./overlay.js";
import { QuickInserter } from "./quick-inserter.js";
import { state, bootstrap, displayNameForBlock, findDocumentNode, findDocumentParent, isContainerNode, definitionForBlock, subscribeDocument } from "./state.js";
import { hasLegalInsertion, getInsertionTarget, subscribeInsertionTarget } from "./insertion.js";

function labelForInstance(instance) {
  if (!instance) return "Block";
  const block = instance.block || "";
  const isExternal = !instance.editable;
  let display = "";
  if (block && block.includes("/")) {
    display = displayNameForBlock(block);
  }
  if (!display || display === "Block") {
    // No block mapping — could be external/template node with unknown definition
    if (isExternal) {
      if (instance.ownerType === "site-part" && instance.ownerLabel && !isTechnicalId(instance.ownerLabel)) {
        display = instance.ownerLabel;
      } else if (block && block.includes("/")) {
        // block present but no displayName — humanized fallback already handled; use it
        // display already set via displayNameForBlock humanization
      } else {
        return "Template element";
      }
    } else {
      return "Block";
    }
  }

  if (isExternal) {
    if (instance.ownerType === "site-part") {
      return `${display} · Site Part`;
    }
    return `${display} · Template`;
  }
  return display;
}

function isTechnicalId(s) {
  if (!s || typeof s !== "string") return true;
  const t = s.trim();
  if (t.length < 3) return true;
  if (/^(blk|entry|site|page)[-_]/i.test(t)) return true;
  if (/^[0-9a-f]{8}-[0-9a-f]{4}-/i.test(t)) return true;
  if (/^[A-Za-z0-9_-]{10,}$/.test(t) && !t.includes(" ") && !t.includes("/")) return true;
  return false;
}

export function isEditorUIEvent(event) {
  if (!event) return false;
  try {
    const path = event.composedPath ? event.composedPath() : [];
    for (const n of path) {
      if (!n) continue;
      if (typeof n.getAttribute === "function" && n.getAttribute("data-stratum-editor-ui") === "true") return true;
      if (n.matches && typeof n.matches === "function") {
        try { if (n.matches('[data-stratum-editor-ui="true"]')) return true; } catch (_) {}
      }
      if (n.closest && typeof n.closest === "function") {
        try { if (n.closest('[data-stratum-editor-ui="true"]')) return true; } catch (_) {}
      }
    }
    const t = event.target;
    if (t && t.closest && typeof t.closest === "function") {
      try { if (t.closest('[data-stratum-editor-ui="true"]')) return true; } catch (_) {}
    }
  } catch (_) {}
  return false;
}

function isScopedEditorUIEvent(event, shadow) {
  if (!event || !shadow) return isEditorUIEvent(event);
  try {
    const path = event.composedPath ? event.composedPath() : [];
    for (const n of path) {
      if (!n) continue;
      let candidate = null;
      if (typeof n.getAttribute === "function" && n.getAttribute("data-stratum-editor-ui") === "true") candidate = n;
      else if (n.matches && typeof n.matches === "function") {
        try { if (n.matches('[data-stratum-editor-ui="true"]')) candidate = n; } catch (_) {}
      }
      if (candidate) {
        try {
          if (candidate.getRootNode && candidate.getRootNode() === shadow) return true;
          if (shadow.contains && shadow.contains(candidate)) return true;
          // If shadow check not available, fall back to generic
          if (!candidate.getRootNode && !shadow.contains) return true;
        } catch (_) { return true; }
      }
      if (n.closest && typeof n.closest === "function") {
        let closest = null;
        try { closest = n.closest('[data-stratum-editor-ui="true"]'); } catch (_) {}
        if (closest) {
          try {
            if (closest.getRootNode && closest.getRootNode() === shadow) return true;
            if (shadow.contains && shadow.contains(closest)) return true;
            if (!closest.getRootNode && !shadow.contains) return true;
          } catch (_) { return true; }
        }
      }
    }
    const t = event.target;
    if (t && t.closest && typeof t.closest === "function") {
      let closest = null;
      try { closest = t.closest('[data-stratum-editor-ui="true"]'); } catch (_) {}
      if (closest) {
        try {
          if (closest.getRootNode && closest.getRootNode() === shadow) return true;
          if (shadow.contains && shadow.contains(closest)) return true;
          if (!closest.getRootNode && !shadow.contains) return true;
        } catch (_) { return true; }
      }
    }
  } catch (_) {}
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
    this.onEscape = null;
    // insertion affordance (runtime only, never SDT)
    this.insertionHint = null; // {parentId,index,rect,contextInstanceKey}
    this._insertionRaf = 0;
    this._lastMoveEvent = null;
    this.quickInserter = null;
    // invalidate stale hint immediately after SDT mutation (max reached etc.)
    try {
      subscribeDocument(() => {
        if (this.insertionHint && this.overlay) {
          const parentNode = this.insertionHint.parentId == null ? null : findDocumentNode(this.insertionHint.parentId);
          if (!hasLegalInsertion(parentNode, this.insertionHint.index)) {
            this.insertionHint = null;
            this.overlay.clearInsertion();
          }
        }
        // persistent Blocks target must recompute after document change (new children count shifts geometry)
        this.requestSync();
      });
      subscribeInsertionTarget(() => {
        // show/hide persistent Blocks target indicator (§22) — one geometry refresh path
        this.requestSync();
      });
    } catch (_) {}
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
    this.quickInserter = new QuickInserter(this);

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

    // Initial sync
    this.syncGeometry();
    // Restore selection if state already has one (e.g., after viewport switch without reload)
    if (state.selection) {
      if (state.selection.instanceKey && this.index.has(state.selection.instanceKey)) {
        this.selected = state.selection;
        this.syncGeometry();
      } else if (!state.selection.logical) {
        state.selection = null;
      }
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
    if (this.overlay) {
      try { this.overlay.destroy(); } catch (_) {}
    }
    if (this.quickInserter) {
      try { this.quickInserter.close(); } catch (_) {}
    }
    this.overlay = null;
    this.quickInserter = null;
    this.doc = null;
    this.win = null;
    this.index = new Map();
    this.elementToNode = new WeakMap();
    this.nodeToKeys = new Map();
    this.hoverInst = null;
    this.selected = null;
    this.insertionHint = null;
    // Do not clear state.selection here — attach restores it when the same
    // rendered occurrence still exists in the reloaded preview.
    this._rafPending = false;
    this._boundWinEvents = false;
  }

  isEditorUIEvent(event) {
    if (!this.overlay || !this.overlay.shadow) return isEditorUIEvent(event);
    return isScopedEditorUIEvent(event, this.overlay.shadow);
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

  // SDT parent lookup for editable node id (null for root)
  findSDTParent(nodeId) {
    const info = findDocumentParent(nodeId);
    if (!info) return { parentId: null, index: 0, parentNode: null, parent: null };
    const parentId = info.parent ? info.parent.id : null;
    const parentNode = info.parent || null;
    return { parentId, parentNode, parent: parentNode, index: info.index, siblings: info.siblings, node: info.node };
  }

  visualRect(instance) {
    if (!instance) return null;
    return visualRectForInstance(instance, this.doc);
  }

  onMove(e) {
    if (!this.doc || !this.overlay) return;
    if (this.isEditorUIEvent(e)) return;
    // Throttle insertion hint derivation via rAF, but hover is immediate
    const hit = this.hitForTarget(e.target);
    // Always update hover outline (M2 behavior)
    if (!hit) {
      if (this.hoverInst) {
        this.hoverInst = null;
        state.hoveredKey = null;
        this.overlay.clearHover();
      }
      // also consider root insertion at document edges when no hit but inside Page content union
      this.scheduleInsertionHint(e, null);
      return;
    }
    // If hover is same as selected, don't show hover
    if (this.selected && hit.instanceKey === this.selected.instanceKey) {
      if (this.hoverInst) {
        this.hoverInst = null;
        state.hoveredKey = null;
        this.overlay.clearHover();
      }
    } else if (!this.hoverInst || this.hoverInst.instanceKey !== hit.instanceKey) {
      this.hoverInst = hit;
      state.hoveredKey = hit.instanceKey;
      const rect = this.visualRect(hit);
      if (!rect) {
        this.overlay.clearHover();
      } else {
        const isExternal = !hit.editable;
        this.overlay.setHover(rect, isExternal);
      }
    }
    this.scheduleInsertionHint(e, hit);
  }

  scheduleInsertionHint(e, hit) {
    this._lastMoveEvent = e;
    if (this._insertionRaf) return;
    const raf = this.win && this.win.requestAnimationFrame ? this.win.requestAnimationFrame.bind(this.win) : requestAnimationFrame;
    this._insertionRaf = raf(() => {
      this._insertionRaf = 0;
      const ev = this._lastMoveEvent;
      // if quick inserter open, freeze hint
      if (this.quickInserter && this.quickInserter.isOpen()) return;
      this.updateInsertionHint(ev, hit || (ev ? this.hitForTarget(ev.target) : null));
    });
  }

  updateInsertionHint(e, hit) {
    if (!this.doc || !this.overlay) return;
    if (this.quickInserter && this.quickInserter.isOpen()) {
      // keep hint but not derive new one while inserter open
      return;
    }
    // Only one active insertion target (§29) — while Blocks has explicit target, hide hover boundary
    if (getInsertionTarget()) {
      if (this.insertionHint) { this.insertionHint = null; this.overlay.clearInsertion(); }
      return;
    }
    // External template content has no insertion controls (§22)
    if (hit && !hit.editable) {
      if (this.insertionHint) { this.insertionHint = null; this.overlay.clearInsertion(); }
      return;
    }
    // Derive local candidate boundaries only (§20)
    const candidates = this.deriveInsertionCandidates(hit, e);
    if (!candidates || !candidates.length) {
      if (this.insertionHint) { this.insertionHint = null; this.overlay.clearInsertion(); }
      return;
    }
    // Pick closest candidate to pointer Y
    const clientY = e ? e.clientY : 0;
    let best = candidates[0];
    let bestDist = Math.abs((best.rect.top + best.rect.height / 2) - clientY);
    // Prefer candidates near pointer with small threshold
    for (let i = 1; i < candidates.length; i++) {
      const c = candidates[i];
      const d = Math.abs((c.rect.top + c.rect.height / 2) - clientY);
      // also distance to pointer X for horizontal? ignore
      if (d < bestDist) { best = c; bestDist = d; }
    }
    // Only show if pointer is within ~36px of line center or inside candidate container threshold
    const THRESHOLD = 36;
    if (bestDist > THRESHOLD && best.rect.width > 0) {
      // for sibling boundaries, require proximity; for empty container, allow slightly larger
      if (this.insertionHint) { this.insertionHint = null; this.overlay.clearInsertion(); }
      return;
    }
    // No decorative illegal plus (§17)
    const parentNode = best.parentId == null ? null : findDocumentNode(best.parentId);
    if (!hasLegalInsertion(parentNode, best.index)) {
      if (this.insertionHint) { this.insertionHint = null; this.overlay.clearInsertion(); }
      return;
    }
    // Only one affordance at a time §18
    this.insertionHint = best;
    // Build insertion line rect: horizontal line spanning parent width (or page content width for root)
    // Keep selection and insertion visually distinct: if line coincides with selected outline edge, offset slightly outside
    let lineRect = this.buildInsertionLineRect(best);
    if (!lineRect) { this.overlay.clearInsertion(); return; }
    // Offset insertion boundary slightly outside selected block when they coincide (§5)
    try {
      if (this.selected && this.selected.instanceKey) {
        const selInst = this.index.get(this.selected.instanceKey);
        const selRect = selInst ? this.visualRect(selInst) : null;
        if (selRect) {
          const selTop = Math.round(selRect.top);
          const selBottom = Math.round(selRect.top + selRect.height);
          const lineTop = Math.round(lineRect.top);
          if (Math.abs(lineTop - selTop) <= 2) {
            const newTop = lineRect.top - 6;
            lineRect = { ...lineRect, top: Math.max(8, newTop) };
          } else if (Math.abs(lineTop - selBottom) <= 2) lineRect = { ...lineRect, top: lineRect.top + 6 };
        }
      }
    } catch (_) {}
    this.overlay.setInsertion(lineRect, (e, anchorRect) => {
      // clicking + opens Quick Inserter anchored to the interactive control itself (§7-9)
      this.openQuickInserter(best, anchorRect || lineRect);
    });
  }

  // Helpers for gap probing (§10) — bounded offsets, no global scan
  hitFromPoint(x, y) {
    if (!this.doc || typeof this.doc.elementsFromPoint !== "function") return null;
    try {
      const els = this.doc.elementsFromPoint(x, y);
      for (const el of els) {
        if (!el) continue;
        // Skip overlay host if ever hit (pointer-events:none generally excludes it)
        if (el.tagName && el.tagName.toLowerCase() === "stratum-editor-overlay-root") continue;
        const hit = this.hitForTarget(el);
        if (hit) return hit;
      }
    } catch (_) {}
    return null;
  }

  probeGapTarget(clientX, clientY) {
    if (!this.doc || clientX == null || clientY == null) return null;
    const offsets = [4, 8, 16, 24, 36];
    let above = null, below = null;
    let aboveInfo = null, belowInfo = null;
    for (const off of offsets) {
      if (!above) {
        const h = this.hitFromPoint(clientX, clientY - off);
        if (h && h.editable && h.nodeId && findDocumentNode(h.nodeId)) {
          const info = this.findSDTParent(h.nodeId);
          if (info && info.node) { above = h; aboveInfo = info; }
        }
      }
      if (!below) {
        const h = this.hitFromPoint(clientX, clientY + off);
        if (h && h.editable && h.nodeId && findDocumentNode(h.nodeId)) {
          const info = this.findSDTParent(h.nodeId);
          if (info && info.node) { below = h; belowInfo = info; }
        }
      }
      if (above && below) break;
    }
    if (above && below) {
      const parentIdAbove = aboveInfo.parent ? aboveInfo.parent.id : null;
      const parentIdBelow = belowInfo.parent ? belowInfo.parent.id : null;
      if (parentIdAbove === parentIdBelow && belowInfo.index === aboveInfo.index + 1) {
        const parentNode = belowInfo.parent || null;
        if (!hasLegalInsertion(parentNode, belowInfo.index)) return null;
        const rectAbove = this.visualRect(above);
        const rectBelow = this.visualRect(below);
        let rect = null;
        if (rectAbove && rectBelow) {
          const left = Math.min(rectAbove.left, rectBelow.left);
          const right = Math.max(rectAbove.left + rectAbove.width, rectBelow.left + rectBelow.width);
          rect = { left, top: rectBelow.top, width: right - left, height: 2 };
        } else if (rectBelow) rect = { left: rectBelow.left, top: rectBelow.top, width: rectBelow.width, height: 2 };
        else if (rectAbove) rect = { left: rectAbove.left, top: rectAbove.top + rectAbove.height, width: rectAbove.width, height: 2 };
        if (rect) return { parentId: parentIdBelow, index: belowInfo.index, rect, contextInstanceKey: below.instanceKey || above.instanceKey };
      }
    }
    return null;
  }

  deriveInsertionCandidates(hit, e) {
    if (!hit) {
      // Local gap probing first (§10) — whitespace between adjacent siblings
      if (e && typeof e.clientX === "number" && typeof e.clientY === "number") {
        const probed = this.probeGapTarget(e.clientX, e.clientY);
        if (probed) return [probed];
      }
      // Fallback: empty root / scope edges only
      const scopeRect = this.editableScopeRect();
      if (!scopeRect || !e) return null;
      const y = e.clientY;
      const outsideThreshold = 40;
      const nodes = state.document?.nodes || [];
      if (!nodes.length) return null;
      // near top of scope -> before first
      if (Math.abs(y - scopeRect.top) < outsideThreshold) {
        const firstNode = nodes[0];
        const firstKeys = this.nodeToKeys.get(firstNode.id) || [];
        if (firstKeys.length) {
          const inst = this.index.get(firstKeys[0]);
          if (inst && inst.editable) {
            const r = this.visualRect(inst);
            if (r) {
              return [{ parentId: null, index: 0, rect: { left: r.left, top: r.top, width: r.width, height: 2 }, contextInstanceKey: inst.instanceKey }];
            }
          }
        }
        return [{ parentId: null, index: 0, rect: { left: scopeRect.left, top: scopeRect.top, width: scopeRect.width, height: 2 } }];
      }
      if (Math.abs(y - (scopeRect.top + scopeRect.height)) < outsideThreshold) {
        const lastNode = nodes[nodes.length - 1];
        const lastKeys = this.nodeToKeys.get(lastNode.id) || [];
        let r = null;
        if (lastKeys.length) {
          const inst = this.index.get(lastKeys[0]);
          if (inst) r = this.visualRect(inst);
        }
        if (r) return [{ parentId: null, index: nodes.length, rect: { left: r.left, top: r.top + r.height, width: r.width, height: 2 } }];
        return [{ parentId: null, index: nodes.length, rect: { left: scopeRect.left, top: scopeRect.top + scopeRect.height, width: scopeRect.width, height: 2 } }];
      }
      return null;
    }
    // hit exists and editable
    const nodeId = hit.instanceKey ? hit.nodeId : null;
    if (!nodeId) return null;
    const sdtInfo = this.findSDTParent(nodeId);
    if (!sdtInfo || !sdtInfo.node) return null; // should not happen
    const sdtNode = sdtInfo.node;
    const parentIdForBoundaries = sdtInfo.parent ? sdtInfo.parent.id : null;
    const siblings = sdtInfo.siblings || state.document.nodes;
    const idx = sdtInfo.index;
    // Need actual SDT parent for canInsert check uses parentNode object (not hit's immediate visual parent)
    // For sibling boundaries: before current and after current in its SDT parent/root
    const rect = this.visualRect(hit);
    if (!rect) return null;
    const candidates = [];
    // before
    candidates.push({
      parentId: parentIdForBoundaries,
      index: idx,
      rect: { left: rect.left, top: rect.top, width: rect.width, height: 2 },
      contextInstanceKey: hit.instanceKey,
    });
    // after
    candidates.push({
      parentId: parentIdForBoundaries,
      index: idx + 1,
      rect: { left: rect.left, top: rect.top + rect.height, width: rect.width, height: 2 },
      contextInstanceKey: hit.instanceKey,
    });
    // If container and empty, also consider inside (already same as parent boundaries but handle empty special)
    // The overlay empty state handles empty container click, so no line needed inside empty; but we still allow parent boundary
    return candidates;
  }

  editableScopeRect() {
    // union of editable rects (same as updateScope but returns rect)
    const instances = [];
    for (const inst of this.index.values()) if (inst.editable) instances.push(inst);
    if (!instances.length) return null;
    const rects = [];
    for (const inst of instances) {
      const r = this.visualRect(inst);
      if (r && r.width > 0 && r.height > 0) rects.push(r);
    }
    if (!rects.length) return null;
    let union = rects[0];
    for (let i = 1; i < rects.length; i++) {
      const r = rects[i];
      const left = Math.min(union.left, r.left);
      const top = Math.min(union.top, r.top);
      const right = Math.max(union.left + union.width, r.left + r.width);
      const bottom = Math.max(union.top + union.height, r.top + r.height);
      union = { left, top, width: right - left, height: bottom - top, right, bottom };
    }
    return union;
  }

  buildInsertionLineRect(hint) {
    if (!hint || !hint.rect) return hint.rect;
    // For now, use hint rect as is (full width of sibling). For grid/horizontal layout, could narrow but keep simple.
    // Ensure rect stays inside viewport
    return hint.rect;
  }

  openQuickInserter(hint, anchorRect) {
    if (!hint || !this.quickInserter) return;
    // Must not activate page (§51) — stopPropagation already in plus click
    this.quickInserter.open(hint, anchorRect);
  }

  closeQuickInserter() {
    if (this.quickInserter) this.quickInserter.close();
  }

  onClick(e) {
    if (!this.doc || !this.overlay) return;
    if (this.isEditorUIEvent(e)) return;
    if (e.type === "submit") {
      try { e.preventDefault(); e.stopPropagation(); } catch (_) {}
      const formEl = e.target;
      const hit = formEl ? this.hitForTarget(formEl) : null;
      if (hit) this.selectInstance(hit);
      return;
    }

    const target = e.target;
    const hit = this.hitForTarget(target);
    // In edit mode every interactive click is inert and selects its block.
    // No distinction between same-page anchor, cross-page, _blank, button, form.
    try { e.preventDefault(); e.stopPropagation(); } catch (_) {}
    if (hit) {
      this.selectInstance(hit);
      // clear stale insertion hint after selection (hover will repopulate)
      if (this.insertionHint) { this.insertionHint = null; if (this.overlay) this.overlay.clearInsertion(); }
      return;
    }
    // No mapped block: if click was on anchor/button/form without mapping, keep blocking navigation but don't select.
    // If truly empty area, clear selection.
    try {
      const anchor = target.closest ? target.closest("a[href], button, input[type='submit'], input[type='button'], [role='link'], [role='button'], form") : null;
      if (anchor) return;
    } catch (_) {}
    this.clearSelection();
    if (this.insertionHint) { this.insertionHint = null; if (this.overlay) this.overlay.clearInsertion(); }
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
      block: instance.block || "",
      version: instance.version || 0,
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

  selectNode(node) {
    if (!node || !node.id) return;
    const keys = (this.nodeToKeys.get(node.id) || []).filter((key) => {
      const instance = this.index.get(key);
      return instance && instance.editable;
    });

    if (this.selected && this.selected.nodeId === node.id && this.index.has(this.selected.instanceKey)) {
      this.selectInstance(this.index.get(this.selected.instanceKey));
      return;
    }
    if (keys.length === 1) {
      this.selectInstance(this.index.get(keys[0]));
      return;
    }
    if (keys.length > 1) {
      const visible = keys.filter((key) => {
        const rect = this.visualRect(this.index.get(key));
        if (!rect || !this.doc?.documentElement) return false;
        const width = this.doc.documentElement.clientWidth || 0;
        const height = this.doc.documentElement.clientHeight || 0;
        return rect.bottom > 0 && rect.right > 0 && rect.top < height && rect.left < width;
      });
      if (visible.length === 1) {
        this.selectInstance(this.index.get(visible[0]));
        return;
      }
    }

    // A document node can render as several Collection occurrences. Keep one
    // shared logical selection without drawing a misleading occurrence outline.
    this.selected = null;
    state.selection = {
      nodeId: node.id,
      instanceKey: null,
      editable: true,
      ownerType: "entry",
      ownerId: state.entryId,
      ownerLabel: state.title,
      block: node.block || "",
      version: node.version || 0,
      logical: true,
    };
    if (this.overlay) this.overlay.clearSelection();
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
    // Close Quick Inserter on scroll (simplest correct rule, do not float stale)
    if (this.quickInserter && this.quickInserter.isOpen()) {
      this.quickInserter.close();
    }
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

  rectForTarget(target) {
    if (!target) return null;
    const parentId = target.parentId;
    const idx = target.index;
    if (parentId == null) {
      const nodes = state.document?.nodes || [];
      if (!nodes.length) {
        const scope = this.editableScopeRect();
        if (scope) return { left: scope.left, top: scope.top, width: scope.width, height: 2 };
        return null;
      }
      if (idx < nodes.length) {
        const n = nodes[idx];
        const keys = this.nodeToKeys.get(n.id) || [];
        for (const k of keys) {
          const inst = this.index.get(k);
          if (inst && inst.editable) {
            const r = this.visualRect(inst);
            if (r) return { left: r.left, top: r.top, width: r.width, height: 2 };
          }
        }
        // fallback to scope
        const scope = this.editableScopeRect();
        if (scope) return { left: scope.left, top: scope.top, width: scope.width, height: 2 };
        return null;
      } else {
        // after last
        const n = nodes[nodes.length - 1];
        const keys = this.nodeToKeys.get(n.id) || [];
        for (const k of keys) {
          const inst = this.index.get(k);
          if (inst && inst.editable) {
            const r = this.visualRect(inst);
            if (r) return { left: r.left, top: r.top + r.height, width: r.width, height: 2 };
          }
        }
        const scope = this.editableScopeRect();
        if (scope) return { left: scope.left, top: scope.top + scope.height, width: scope.width, height: 2 };
        return null;
      }
    } else {
      const parent = findDocumentNode(parentId);
      if (!parent) return null;
      const kids = parent.children || [];
      if (idx < kids.length) {
        const n = kids[idx];
        const keys = this.nodeToKeys.get(n.id) || [];
        for (const k of keys) {
          const inst = this.index.get(k);
          if (inst && inst.editable) {
            const r = this.visualRect(inst);
            if (r) return { left: r.left, top: r.top, width: r.width, height: 2 };
          }
        }
        // empty container case: if kids length 0, show inside container
        const parentKeys = this.nodeToKeys.get(parentId) || [];
        for (const k of parentKeys) {
          const inst = this.index.get(k);
          if (inst && inst.editable) {
            const r = this.visualRect(inst);
            if (r) {
              if (kids.length === 0) return { left: r.left + 8, top: r.top + r.height / 2, width: r.width - 16, height: 2 };
              return { left: r.left, top: r.top, width: r.width, height: 2 };
            }
          }
        }
        return null;
      } else {
        // append at end of container or after last child
        if (kids.length > 0) {
          const last = kids[kids.length - 1];
          const keys = this.nodeToKeys.get(last.id) || [];
          for (const k of keys) {
            const inst = this.index.get(k);
            if (inst && inst.editable) {
              const r = this.visualRect(inst);
              if (r) return { left: r.left, top: r.top + r.height, width: r.width, height: 2 };
            }
          }
        }
        const parentKeys = this.nodeToKeys.get(parentId) || [];
        for (const k of parentKeys) {
          const inst = this.index.get(k);
          if (inst && inst.editable) {
            const r = this.visualRect(inst);
            if (r) {
              if (kids.length === 0) return { left: r.left + 8, top: r.top + r.height / 2, width: r.width - 16, height: 2 };
              return { left: r.left, top: r.top + r.height, width: r.width, height: 2 };
            }
          }
        }
        return null;
      }
    }
  }

  updateBlocksTarget() {
    if (!this.overlay) return;
    const target = getInsertionTarget();
    if (!target) {
      this.overlay.clearBlocksTarget();
      return;
    }
    // Empty doc owns empty-state button, not persistent line (§24)
    if ((state.document?.nodes || []).length === 0) {
      this.overlay.clearBlocksTarget();
      return;
    }
    // Only show persistent indicator when Quick Inserter is not open (to avoid duplicate line+plus)
    if (this.quickInserter && this.quickInserter.isOpen()) {
      this.overlay.clearBlocksTarget();
      return;
    }
    const rect = this.rectForTarget(target);
    if (!rect) {
      this.overlay.clearBlocksTarget();
      return;
    }
    this.overlay.setBlocksTarget(rect);
  }

  syncGeometry() {
    if (!this.overlay || !this.doc) return;
    // Persistent Blocks target indicator (§22) — show before hover/selection so it stays visible while panel open
    this.updateBlocksTarget();
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
      // Decide if selected container can Add inside (§27) — small plus on handle
      let handleOpts = { external: isExternal };
      try {
        if (isExternal === false) {
          const node = findDocumentNode(this.selected.nodeId);
          if (node && isContainerNode(node) && (node.children || []).length >= 0) {
            // hasLegalInsertion for append inside
            if (hasLegalInsertion(node, (node.children||[]).length)) {
              handleOpts.showHandlePlus = true;
              handleOpts.onHandlePlusClick = (e, anchorRect) => {
                e.preventDefault(); e.stopPropagation();
                const target = { parentId: node.id, index: (node.children||[]).length };
                // anchor to the handle plus control itself, not an estimated line
                const anchor = anchorRect || (e && e.target ? e.target.getBoundingClientRect() : null) || rect;
                this.openQuickInserter(target, anchor);
              };
            }
          }
        }
      } catch (_) {}
      this.overlay.setSelected(rect, label, handleOpts);
    } else {
      this.overlay.clearSelection();
    }
    // Empty states (overlay/editor instrumentation, never SDT) (§24-25)
    this.updateEmptyStates();
    // Re-establish insertion line after geometry sync if we still have a hint (recompute rect)
    if (this.insertionHint && !(this.quickInserter && this.quickInserter.isOpen())) {
      const hint = this.insertionHint;
      const parentNode = hint.parentId == null ? null : findDocumentNode(hint.parentId);
      if (!hasLegalInsertion(parentNode, hint.index)) {
        this.insertionHint = null;
        this.overlay.clearInsertion();
      } else {
        // recompute line rect from current layout (hint.rect may be stale after scroll/resize)
        // try to find a representative instance for parent or sibling
        let lineRect = hint.rect;
        // If hint was sibling boundary, try to re-derive from live rect of sibling or parent
        try {
          if (hint.parentId == null) {
            const nodes = state.document.nodes || [];
            if (nodes.length) {
              if (hint.index < nodes.length) {
                const n = nodes[hint.index];
                const keys = this.nodeToKeys.get(n.id) || [];
                if (keys.length) {
                  const inst = this.index.get(keys[0]);
                  if (inst) {
                    const r = this.visualRect(inst);
                    if (r) lineRect = { left: r.left, top: r.top, width: r.width, height: 2 };
                  }
                }
              } else if (hint.index === nodes.length) {
                const n = nodes[nodes.length-1];
                const keys = this.nodeToKeys.get(n.id) || [];
                if (keys.length) {
                  const inst = this.index.get(keys[0]);
                  if (inst) {
                    const r = this.visualRect(inst);
                    if (r) lineRect = { left: r.left, top: r.top + r.height, width: r.width, height: 2 };
                  }
                }
              }
            } else {
              // empty root already handled by empty state, clear line
              this.overlay.clearInsertion();
              lineRect = null;
            }
          } else {
            const parent = findDocumentNode(hint.parentId);
            if (parent) {
              const kids = parent.children || [];
              if (hint.index < kids.length) {
                const n = kids[hint.index];
                const keys = this.nodeToKeys.get(n.id) || [];
                if (keys.length) {
                  const inst = this.index.get(keys[0]);
                  if (inst) {
                    const r = this.visualRect(inst);
                    if (r) lineRect = { left: r.left, top: r.top, width: r.width, height: 2 };
                  }
                }
              } else if (hint.index === kids.length && kids.length>0) {
                const n = kids[kids.length-1];
                const keys = this.nodeToKeys.get(n.id) || [];
                if (keys.length) {
                  const inst = this.index.get(keys[0]);
                  if (inst) {
                    const r = this.visualRect(inst);
                    if (r) lineRect = { left: r.left, top: r.top + r.height, width: r.width, height: 2 };
                  }
                }
              } else if (kids.length===0) {
                // empty container: line is not needed, empty state covers it
                this.overlay.clearInsertion();
                lineRect = null;
              }
            }
          }
        } catch (_) {}
        if (lineRect) {
          this.overlay.setInsertion(lineRect, (e, anchorRect) => this.openQuickInserter(hint, anchorRect || lineRect));
        }
      }
    } else if (!this.insertionHint) {
      // ensure no stale line when nothing hinted
      // don't clear if quick inserter open (it may have hidden line intentionally)
      if (!(this.quickInserter && this.quickInserter.isOpen())) this.overlay.clearInsertion();
    }
  }

  updateEmptyStates() {
    if (!this.overlay) return;
    this.overlay.clearEmptyStates();
    const doc = state.document;
    const nodes = doc?.nodes || [];
    // Root empty
    if (nodes.length === 0) {
      // Show editor-only empty state in Page content scope (§24)
      const scopeRect = this.editableScopeRect();
      // scopeRect may be null when no editable instances rendered (empty doc has no markers yet)
      // fallback to viewport centered rect
      let rect = scopeRect;
      if (!rect) {
        try {
          const vw = this.doc.documentElement.clientWidth || 800;
          const vh = Math.min(400, this.doc.documentElement.clientHeight || 400);
          rect = { left: 24, top: 24, width: vw - 48, height: Math.max(80, vh - 80) };
        } catch (_) { rect = { left: 24, top: 24, width: 600, height: 120 }; }
      }
      // hasLegalInsertion root always if catalog non-empty
      if (hasLegalInsertion(null, 0)) {
        // don't recreate if already exists with same rect
        if (!this.overlay.emptyRootEl) {
          this.overlay.setEmptyRoot(rect, "+ Add block", (e, anchorRect) => {
            const target = { parentId: null, index: 0 };
            this.openQuickInserter(target, anchorRect || rect);
          });
        }
      } else {
        this.overlay.clearEmptyStates();
      }
      // No container empties when root empty
      return;
    } else {
      // clear root empty if exists when doc not empty
      if (this.overlay.emptyRootEl) {
        try { this.overlay.emptyRootEl.remove(); } catch (_) {}
        this.overlay.emptyRootEl = null;
      }
    }
    // Empty containers: iterate editable nodes that are containers with legal children and zero children
    this.overlay.clearEmptyStates();
    // Re-establish root empty cleared above; now per-container
    const walk = (list) => {
      for (const node of list || []) {
        try {
          if (isContainerNode(node) && (node.children || []).length === 0) {
            const parentCheck = node; // hasLegalInsertion checks max/allowed
            if (hasLegalInsertion(parentCheck, 0)) {
              const keys = this.nodeToKeys.get(node.id) || [];
              let rect = null;
              for (const k of keys) {
                const inst = this.index.get(k);
                if (!inst || !inst.editable) continue;
                const r = this.visualRect(inst);
                if (r && r.width > 10 && r.height > 10) { rect = r; break; }
              }
              // If no rendered rect but SDT says empty container, still show subtle placeholder at scope position?
              // For containers with no visual (maybe not yet rendered due to empty), use fallback near scope
              if (!rect) {
                const scope = this.editableScopeRect();
                if (scope) rect = { left: scope.left + 12, top: scope.top + 12, width: Math.min(400, scope.width - 24), height: 72 };
              }
              if (rect) {
                let finalLabel = "+ Add block";
                try {
                  const defForNode = definitionForBlock(node.block, node.version) || (() => {
                    for (const c of bootstrap.catalog || []) if (c.block === node.block) return c;
                    return null;
                  })();
                  if (defForNode && defForNode.schema && defForNode.schema.children && defForNode.schema.children.mode === "allowed" && Array.isArray(defForNode.schema.children.blocks) && defForNode.schema.children.blocks.length === 1) {
                    const single = defForNode.schema.children.blocks[0];
                    finalLabel = `+ Add ${displayNameForBlock(single)}`;
                  }
                } catch (_) {}
                this.overlay.setEmptyContainer(node.id, rect, finalLabel, (e, anchorRect) => {
                  const target = { parentId: node.id, index: 0 };
                  this.openQuickInserter(target, anchorRect || rect);
                });
              }
            }
          }
        } catch (_) {}
        if (node.children && node.children.length) walk(node.children);
      }
    };
    walk(nodes);
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
      // priority: quick inserter open → close it (§49)
      if (this.quickInserter && this.quickInserter.isOpen()) {
        e.preventDefault(); e.stopPropagation();
        this.closeQuickInserter();
        return;
      }
      if (typeof this.onEscape === "function" && this.onEscape(e)) return;
      // If selection exists, clear it. Do not prevent overflow menu's own Escape if no selection
      if (state.selection) {
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
