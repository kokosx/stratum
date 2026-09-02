// canvas.js — iframe interaction lifecycle for V2
import { buildMarkerIndex, visualRectForInstance } from "./markers.js";
import { Overlay } from "./overlay.js";
import { QuickInserter } from "./quick-inserter.js";
import { state, bootstrap, displayNameForBlock, findDocumentNode, findDocumentParent, isContainerNode, definitionForBlock, subscribeDocument, primaryInlineFieldForNode, inlineFieldsForNode } from "./state.js";
import { hasLegalInsertion, getInsertionTarget, setInsertionTarget, subscribeInsertionTarget, canMove, canInsert } from "./insertion.js";
import { moveNode, insertBlock } from "./commands.js";
import { startSession, clearSession, getSession } from "./drag-session.js";
import { startInlineEdit, isInlineEditing, commitActiveEdit, cancelActiveEdit, isActiveEditorSessionEvent, isActiveFieldElement, commitBeforeEditorContextChange, findFieldElement } from "./inline-editor.js";

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

export function allowsPreviewDefault(target) {
  if (!target) return false;
  let el = target;
  // text node → parent element
  if (el.nodeType === 3) {
    el = el.parentElement;
    if (!el) return false;
  }
  // non-element without closest
  if (!el || typeof el.closest !== "function") return false;
  try {
    const summary = el.closest("summary");
    if (!summary) return false;
    const details = summary.parentElement;
    if (!details) return false;
    if (typeof details.matches === "function") {
      try {
        if (details.matches("details")) return true;
      } catch (_) {}
    }
    return !!(details.tagName && details.tagName.toLowerCase() === "details");
  } catch (_) {
    return false;
  }
}

export function findCanvasDragGrip(event) {
  if (!event) return null;
  try {
    const path = event.composedPath ? event.composedPath() : [];
    for (const n of path) {
      if (!n) continue;
      if (n.getAttribute && typeof n.getAttribute === "function" && n.getAttribute("draggable") === "true" && n.classList && n.classList.contains("overlay-handle__drag")) return n;
      if (n.matches && typeof n.matches === "function") {
        try { if (n.matches('button.overlay-handle__drag[draggable="true"]')) return n; } catch (_) {}
      }
    }
    const t = event.target;
    if (t) {
      if (t.matches && typeof t.matches === "function") {
        try { if (t.matches('button.overlay-handle__drag[draggable="true"]')) return t; } catch (_) {}
      }
      if (t.getAttribute && typeof t.getAttribute === "function" && t.getAttribute("draggable") === "true" && t.classList && t.classList.contains("overlay-handle__drag")) return t;
    }
  } catch (_) {}
  return null;
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
    this._onPointerDown = this.onPointerDown.bind(this);
    this._onClick = this.onClick.bind(this);
    this._onScroll = this.onScroll.bind(this);
    this._onResize = this.onResize.bind(this);
    this._onKey = this.onKey.bind(this);
    this._onAux = this.onAuxClick.bind(this);
    this._onDblClick = this.onDblClick.bind(this);
    this._onSubmitBlock = this.onSubmitBlock.bind(this);
    this._onDragStart = this.onDragStart.bind(this);
    this._onDragOver = this.onDragOver.bind(this);
    this._onDragLeave = this.onDragLeave.bind(this);
    this._onDrop = this.onDrop.bind(this);
    this._onDragEnd = this.onDragEnd.bind(this);
    this._boundWinEvents = false;
    this.onEscape = null;
    // insertion affordance (runtime only, never SDT)
    this.insertionHint = null; // {parentId,index,rect,contextInstanceKey}
    this._insertionRaf = 0;
    this._lastMoveEvent = null;
    this.quickInserter = null;
    // drag session — transient runtime only
    this.dragTarget = null;
    this._autoScrollRAF = 0;
    this._autoScrollDir = 0;
    this._dragOverY = 0;
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
        if (this.dragTarget) {
          const sess = getSession();
          if (sess && sess.kind === "node") {
            const legal = canMove(sess.nodeId, this.dragTarget.parentId, this.dragTarget.index);
            if (!legal.ok) this.clearDragState();
            else this.overlay && this.overlay.setDragTarget(this.dragTarget.rect, "Move here");
          } else if (sess && sess.kind === "block" && sess.definition) {
            const parentNode = this.dragTarget.parentId == null ? null : findDocumentNode(this.dragTarget.parentId);
            const legal = canInsert(parentNode, sess.definition, this.dragTarget.index);
            if (!legal.ok) this.clearDragState();
            else this.overlay && this.overlay.setDragTarget(this.dragTarget.rect, "Add here");
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
      doc.addEventListener("pointerdown", this._onPointerDown, true);
      doc.addEventListener("mousedown", this._onPointerDown, true);
      doc.addEventListener("click", this._onClick, true);
      doc.addEventListener("dblclick", this._onDblClick, true);
      doc.addEventListener("auxclick", this._onAux, true);
      doc.addEventListener("submit", this._onSubmitBlock, true);
      doc.addEventListener("keydown", this._onKey, true);
      doc.addEventListener("dragstart", this._onDragStart, true);
      doc.addEventListener("dragover", this._onDragOver, true);
      doc.addEventListener("dragleave", this._onDragLeave, true);
      doc.addEventListener("drop", this._onDrop, true);
      doc.addEventListener("dragend", this._onDragEnd, true);
      // Inject editor placeholder CSS for inline fields
      try { this.injectInlineCSS(doc); } catch (_) {}
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
    // Commit active edit before tearing down (avoid stale contenteditable)
    try { if (isInlineEditing()) commitActiveEdit(); } catch (_) {}
    try { this.clearDragState(); } catch (_) {}
    try {
      if (this.doc) {
        this.doc.removeEventListener("pointermove", this._onMove, true);
        this.doc.removeEventListener("mousemove", this._onMove, true);
        this.doc.removeEventListener("pointerdown", this._onPointerDown, true);
        this.doc.removeEventListener("mousedown", this._onPointerDown, true);
        this.doc.removeEventListener("click", this._onClick, true);
        this.doc.removeEventListener("dblclick", this._onDblClick, true);
        this.doc.removeEventListener("auxclick", this._onAux, true);
        this.doc.removeEventListener("submit", this._onSubmitBlock, true);
        this.doc.removeEventListener("keydown", this._onKey, true);
        this.doc.removeEventListener("dragstart", this._onDragStart, true);
        this.doc.removeEventListener("dragover", this._onDragOver, true);
        this.doc.removeEventListener("dragleave", this._onDragLeave, true);
        this.doc.removeEventListener("drop", this._onDrop, true);
        this.doc.removeEventListener("dragend", this._onDragEnd, true);
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
    this.dragTarget = null;
    this._autoScrollRAF = 0;
    this._autoScrollDir = 0;
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

  getContainerLayout(parentNode) {
    if (!parentNode) return "vertical";
    // Prefer rendered truth (theme-controlled presentation) over block-type hardcodes.
    try {
      const keys = this.nodeToKeys.get(parentNode.id) || [];
      for (const k of keys) {
        const inst = this.index.get(k);
        if (!inst || !inst.editable) continue;
        const el = inst.rootElements && inst.rootElements[0];
        if (!el) continue;
        try {
          const cs = this.doc.defaultView ? this.doc.defaultView.getComputedStyle(el) : null;
          if (!cs) continue;
          if (cs.display === "grid" || cs.display === "inline-grid") return "grid";
          if ((cs.display === "flex" || cs.display === "inline-flex") && (cs.flexDirection === "row" || cs.flexDirection === "row-reverse")) return "horizontal";
        } catch (_) {}
      }
    } catch (_) {}
    // Fallback to vertical; block-type is not required to determine drag orientation.
    return "vertical";
  }

  // Operation-neutral geometry: returns rects without insertion/move legality filtering.
  // Callers filter via hasLegalInsertion (insert) or canMove (move).
  probeGapTargetGeometry(clientX, clientY) {
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
        const rectAbove = this.visualRect(above);
        const rectBelow = this.visualRect(below);
        let rect = null;
        if (rectAbove && rectBelow) {
          const pNode = belowInfo.parent || null;
          const layout = this.getContainerLayout(pNode);
          const isHoriz = layout === "horizontal" || layout === "grid";
          if (isHoriz) {
            // For horizontal row, gap is vertical line between siblings at boundary
            const gapX = rectBelow.left;
            const top = Math.min(rectAbove.top, rectBelow.top);
            const bottom = Math.max(rectAbove.top + rectAbove.height, rectBelow.top + rectBelow.height);
            rect = { left: gapX, top, width: 2, height: bottom - top };
          } else {
            const left = Math.min(rectAbove.left, rectBelow.left);
            const right = Math.max(rectAbove.left + rectAbove.width, rectBelow.left + rectBelow.width);
            rect = { left, top: rectBelow.top, width: right - left, height: 2 };
          }
        } else if (rectBelow) {
          const pNode = belowInfo.parent || null;
          const layout = this.getContainerLayout(pNode);
          const isHoriz = layout === "horizontal" || layout === "grid";
          if (isHoriz) rect = { left: rectBelow.left, top: rectBelow.top, width: 2, height: rectBelow.height };
          else rect = { left: rectBelow.left, top: rectBelow.top, width: rectBelow.width, height: 2 };
        } else if (rectAbove) {
          const pNode = aboveInfo.parent || null;
          const layout = this.getContainerLayout(pNode);
          const isHoriz = layout === "horizontal" || layout === "grid";
          if (isHoriz) rect = { left: rectAbove.left + rectAbove.width, top: rectAbove.top, width: 2, height: rectAbove.height };
          else rect = { left: rectAbove.left, top: rectAbove.top + rectAbove.height, width: rectAbove.width, height: 2 };
        }
        if (rect) return { parentId: parentIdBelow, index: belowInfo.index, rect, contextInstanceKey: below.instanceKey || above.instanceKey };
      }
    }
    return null;
  }

  rawBoundaries(hit, e) {
    if (!hit) {
      if (e && typeof e.clientX === "number" && typeof e.clientY === "number") {
        const probed = this.probeGapTargetGeometry(e.clientX, e.clientY);
        if (probed) return [probed];
      }
      const scopeRect = this.editableScopeRect();
      if (!scopeRect || !e) return null;
      const y = e.clientY;
      const outsideThreshold = 40;
      const nodes = state.document?.nodes || [];
      if (!nodes.length) return null;
      if (Math.abs(y - scopeRect.top) < outsideThreshold) {
        const firstNode = nodes[0];
        const firstKeys = this.nodeToKeys.get(firstNode.id) || [];
        if (firstKeys.length) {
          const inst = this.index.get(firstKeys[0]);
          if (inst && inst.editable) {
            const r = this.visualRect(inst);
            if (r) {
              const pNode = null;
              const layout = this.getContainerLayout(pNode);
              const isHoriz = layout === "horizontal" || layout === "grid";
              if (isHoriz) return [{ parentId: null, index: 0, rect: { left: r.left, top: r.top, width: 2, height: r.height }, contextInstanceKey: inst.instanceKey }];
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
        if (r) {
          const pNode = null;
          const layout = this.getContainerLayout(pNode);
          const isHoriz = layout === "horizontal" || layout === "grid";
          if (isHoriz) return [{ parentId: null, index: nodes.length, rect: { left: r.left + r.width, top: r.top, width: 2, height: r.height } }];
          return [{ parentId: null, index: nodes.length, rect: { left: r.left, top: r.top + r.height, width: r.width, height: 2 } }];
        }
        return [{ parentId: null, index: nodes.length, rect: { left: scopeRect.left, top: scopeRect.top + scopeRect.height, width: scopeRect.width, height: 2 } }];
      }
      return null;
    }
    const nodeId = hit.instanceKey ? hit.nodeId : null;
    if (!nodeId) return null;
    const sdtInfo = this.findSDTParent(nodeId);
    if (!sdtInfo || !sdtInfo.node) return null;
    const parentIdForBoundaries = sdtInfo.parent ? sdtInfo.parent.id : null;
    const idx = sdtInfo.index;
    const rect = this.visualRect(hit);
    if (!rect) return null;
    const parentNode = sdtInfo.parent || null;
    const layout = this.getContainerLayout(parentNode);
    const isHoriz = layout === "horizontal" || layout === "grid";
    const candidates = [];
    if (isHoriz) {
      candidates.push({ parentId: parentIdForBoundaries, index: idx, rect: { left: rect.left, top: rect.top, width: 2, height: rect.height }, contextInstanceKey: hit.instanceKey });
      candidates.push({ parentId: parentIdForBoundaries, index: idx + 1, rect: { left: rect.left + rect.width, top: rect.top, width: 2, height: rect.height }, contextInstanceKey: hit.instanceKey });
    } else {
      candidates.push({ parentId: parentIdForBoundaries, index: idx, rect: { left: rect.left, top: rect.top, width: rect.width, height: 2 }, contextInstanceKey: hit.instanceKey });
      candidates.push({ parentId: parentIdForBoundaries, index: idx + 1, rect: { left: rect.left, top: rect.top + rect.height, width: rect.width, height: 2 }, contextInstanceKey: hit.instanceKey });
    }
    return candidates;
  }

  deriveDragBoundaries(hit, e, session) {
    if (!session || (session.kind !== "node" && session.kind !== "block")) return null;
    let base = this.rawBoundaries(hit, e);
    if (hit && hit.editable) {
      const node = findDocumentNode(hit.nodeId);
      if (node && isContainerNode(node) && (node.children || []).length === 0) {
        const r = this.visualRect(hit);
        if (r) {
          const inside = { parentId: node.id, index: 0, rect: { left: r.left + 8, top: r.top + r.height / 2, width: r.width - 16, height: 2 }, contextInstanceKey: hit.instanceKey };
          base = base ? [...base, inside] : [inside];
        }
      } else if (node && isContainerNode(node) && (node.children || []).length > 0) {
        const r = this.visualRect(hit);
        if (r && e && typeof e.clientY === "number") {
          const insideEnd = { parentId: node.id, index: (node.children || []).length, rect: { left: r.left + 8, top: r.top + r.height - 4, width: r.width - 16, height: 2 }, contextInstanceKey: hit.instanceKey };
          if (!base || !base.some(c => c.parentId === insideEnd.parentId && c.index === insideEnd.index)) {
            base = base ? [...base, insideEnd] : [insideEnd];
          }
        }
      }
    }
    if (!hit && e && typeof e.clientX === "number") {
      const under = this.hitFromPoint(e.clientX, e.clientY);
      if (under && under.editable) {
        const node = findDocumentNode(under.nodeId);
        if (node && isContainerNode(node) && (node.children || []).length === 0) {
          const r = this.visualRect(under);
          if (r) return [{ parentId: node.id, index: 0, rect: { left: r.left + 8, top: r.top + r.height / 2, width: r.width - 16, height: 2 }, contextInstanceKey: under.instanceKey }];
        }
        if (node && isContainerNode(node) && (node.children || []).length > 0) {
          const r = this.visualRect(under);
          if (r && e.clientY > r.top + r.height - 36) {
            return [{ parentId: node.id, index: (node.children || []).length, rect: { left: r.left + 8, top: r.top + r.height - 2, width: r.width - 16, height: 2 }, contextInstanceKey: under.instanceKey }];
          }
        }
      }
    }
    return base;
  }

  clearDragState() {
    if (this.dragTarget) {
      this.dragTarget = null;
      try { this.overlay && this.overlay.clearDragTarget(); } catch (_) {}
    }
    try { this.overlay && this.overlay.setDragging(false); } catch (_) {}
    try { clearSession(); } catch (_) {}
    this.stopAutoScroll();
    this._dragOverY = 0;
  }

  startAutoScroll(dir) {
    if (this._autoScrollRAF) return;
    this._autoScrollDir = dir;
    const step = () => {
      if (!this._autoScrollDir || !this.doc || !this.win) { this._autoScrollRAF = 0; return; }
      try {
        const scrollEl = this.doc.documentElement || this.doc.body;
        const curTop = scrollEl.scrollTop || this.doc.body.scrollTop || 0;
        const nextTop = curTop + this._autoScrollDir * 12;
        this.win.scrollTo({ top: Math.max(0, nextTop), behavior: "auto" });
        try { this.requestSync(); } catch (_) {}
      } catch (_) {}
      this._autoScrollRAF = (this.win.requestAnimationFrame || requestAnimationFrame).call(this.win || window, step);
    };
    this._autoScrollRAF = (this.win.requestAnimationFrame || requestAnimationFrame).call(this.win || window, step);
  }

  stopAutoScroll() {
    this._autoScrollDir = 0;
    if (this._autoScrollRAF) {
      try {
        const caf = this.win && this.win.cancelAnimationFrame ? this.win.cancelAnimationFrame.bind(this.win) : cancelAnimationFrame;
        caf(this._autoScrollRAF);
      } catch (_) {}
      this._autoScrollRAF = 0;
    }
  }

  onMove(e) {
    if (!this.doc || !this.overlay) return;
    if (getSession()) return;
    if (isInlineEditing()) {
      if (this.hoverInst) { this.hoverInst = null; state.hoveredKey = null; try { this.overlay.clearHover(); } catch (_) {} }
      if (this.insertionHint) { this.insertionHint = null; try { this.overlay.clearInsertion(); } catch (_) {} }
      return;
    }
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
    if (getSession()) return;
    if (isInlineEditing()) {
      if (this.insertionHint) { this.insertionHint = null; this.overlay.clearInsertion(); }
      return;
    }
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
  // === Drag handlers (native DnD) — document owns lifecycle, grip is only source via composedPath ===
  onDragStart(e) {
    if (!this.doc || !this.overlay) return;
    // Prevent double startSession when both doc and grip listeners would fire (only one owner).
    // Also blocks block-drag while node drag is active and vice versa.
    if (getSession()) return;
    const grip = findCanvasDragGrip(e);
    if (!grip) return;
    // Prevent text selection drag inside contenteditable
    try { if (isActiveFieldElement(e.target) || isActiveFieldElement(document.activeElement)) { } } catch (_) {}
    // Commit inline edit first (§20)
    try { if (isInlineEditing()) commitBeforeEditorContextChange(); } catch (_) {}
    if (isInlineEditing()) { e.preventDefault(); return; }
    const selected = this.selected;
    if (!selected || !selected.nodeId) { e.preventDefault(); return; }
    // Physical grip identity must be unambiguous (M6): read from DOM, verify existence/editable.
    let gripNodeId = null;
    let gripInstanceKey = null;
    try {
      gripNodeId = grip.getAttribute("data-node-id") || (grip.dataset && grip.dataset.nodeId) || null;
      gripInstanceKey = grip.getAttribute("data-instance-key") || (grip.dataset && grip.dataset.instanceKey) || null;
    } catch (_) {}
    // Grip should carry identity; if present, it must match selected (or be valid). If mismatch, trust grip if it resolves.
    let effectiveNodeId = selected.nodeId;
    let effectiveInstanceKey = selected.instanceKey;
    if (gripNodeId) {
      const gripNode = findDocumentNode(gripNodeId);
      if (!gripNode) { e.preventDefault(); return; }
      // Verify grip node is editable (non-editable never has grip, but guard)
      const gripInstKey = gripInstanceKey;
      let isEditable = true;
      if (gripInstKey && this.index && this.index.has(gripInstKey)) {
        const inst = this.index.get(gripInstKey);
        if (inst && inst.editable === false) isEditable = false;
      } else if (selected.editable === false) {
        isEditable = false;
      }
      if (!isEditable) { e.preventDefault(); return; }
      effectiveNodeId = gripNodeId;
      if (gripInstanceKey) effectiveInstanceKey = gripInstanceKey;
      // If grip vs selected mismatch, selection should have been that node — still allow but ensure SDT parent exists.
      if (gripNodeId !== selected.nodeId) {
        // Sync both canvas-local and domain selection to grip's node for consistency (generic, no hardcode)
        try {
          const inst = gripInstanceKey && this.index.get(gripInstanceKey);
          if (inst) {
            const sel = { nodeId: inst.nodeId, instanceKey: inst.instanceKey, editable: inst.editable, block: inst.block, version: inst.version, ownerType: inst.ownerType, ownerId: inst.ownerId, ownerLabel: inst.ownerLabel };
            this.selected = sel;
            try { state.selection = sel; } catch (_) {}
          } else {
            const sel = { nodeId: gripNode.id, instanceKey: gripInstanceKey || null, editable: true, block: gripNode.block, version: gripNode.version, logical: true };
            this.selected = sel;
            try { state.selection = sel; } catch (_) {}
          }
        } catch (_) {}
      }
    }
    const node = findDocumentNode(effectiveNodeId);
    if (!node) { e.preventDefault(); return; }
    // External/template-owned non-editable nodes have no grip, but guard
    if (selected.editable === false && !gripNodeId) { e.preventDefault(); return; }
    const srcInfo = this.findSDTParent(effectiveNodeId);
    if (!srcInfo || !srcInfo.node) { e.preventDefault(); return; }
    // Clear hover/insertion that would conflict
    try { this.hoverInst = null; state.hoveredKey = null; this.overlay.clearHover(); } catch (_) {}
    try { if (this.insertionHint) { this.insertionHint = null; this.overlay.clearInsertion(); } } catch (_) {}
    startSession({ kind: "node", nodeId: effectiveNodeId, instanceKey: effectiveInstanceKey, source: { parentId: srcInfo.parentId, index: srcInfo.index } });
    try { this.overlay.setDragging(true); } catch (_) {}
    try {
      e.dataTransfer.effectAllowed = "move";
      e.dataTransfer.setData("text/plain", effectiveNodeId);
      // Firefox requires data
      if (e.dataTransfer.setDragImage && grip) {
        try { e.dataTransfer.setDragImage(grip, 8, 12); } catch (_) {}
      }
    } catch (_) {}
    try { e.stopPropagation(); } catch (_) {}
  }

  onDragOver(e) {
    if (!this.doc || !this.overlay) return;
    const sess = getSession();
    if (!sess || (sess.kind !== "node" && sess.kind !== "block")) return;
    const isBlock = sess.kind === "block";
    e.preventDefault();
    e.stopPropagation();
    try { e.dataTransfer.dropEffect = isBlock ? "copy" : "move"; } catch (_) {}
    this._dragOverY = e.clientY || 0;
    // Auto-scroll near viewport edges (§40)
    try {
      const vh = this.doc.documentElement?.clientHeight || this.win?.innerHeight || 700;
      if (e.clientY < 48) this.startAutoScroll(-1);
      else if (e.clientY > vh - 48) this.startAutoScroll(1);
      else this.stopAutoScroll();
    } catch (_) {}
    const hit = this.hitForTarget(e.target) || this.hitFromPoint(e.clientX, e.clientY);
    // Derive candidates shared geometry — same neutral geometry for both moves and inserts
    let candidates = this.deriveDragBoundaries(hit, e, sess);
    if (!candidates || !candidates.length) {
      candidates = this.rawBoundaries(hit, e);
    }
    if (!candidates || !candidates.length) {
      if (this.dragTarget) { this.dragTarget = null; this.overlay.clearDragTarget(); try{ e.dataTransfer.dropEffect = "none"; } catch(_){} }
      return;
    }
    // Filter illegal via canMove (nodes) or canInsert (blocks) and hide no-op
    const legal = [];
    for (const c of candidates) {
      let res;
      if (isBlock) {
        if (!sess.definition) continue;
        const parentNode = c.parentId == null ? null : findDocumentNode(c.parentId);
        res = canInsert(parentNode, sess.definition, c.index);
      } else {
        res = canMove(sess.nodeId, c.parentId, c.index);
      }
      if (!res.ok) continue;
      legal.push(c);
    }
    if (!legal.length) {
      if (this.dragTarget) { this.dragTarget = null; this.overlay.clearDragTarget(); }
      try { e.dataTransfer.dropEffect = "none"; } catch (_) {}
      return;
    }
    // Pick closest to pointer — per-candidate layout awareness
    let best = legal[0];
    let bestDist = Infinity;
    for (const c of legal) {
      const pNode = c.parentId == null ? null : findDocumentNode(c.parentId);
      const layout = this.getContainerLayout(pNode);
      const isHoriz = layout === "horizontal" || layout === "grid";
      let d;
      if (isHoriz && e.clientX != null) {
        d = Math.abs((c.rect.left + c.rect.width / 2) - e.clientX);
      } else {
        d = Math.abs((c.rect.top + c.rect.height / 2) - (e.clientY || 0));
        if (e.clientX != null && c.rect.left != null) {
          const xDist = Math.max(0, c.rect.left - e.clientX, e.clientX - (c.rect.left + c.rect.width));
          d += xDist * 0.3;
        }
      }
      if (d < bestDist) { best = c; bestDist = d; }
    }
    // Show strong blue line — same geometry, different semantics
    this.dragTarget = { parentId: best.parentId, index: best.index, rect: best.rect };
    this.overlay.setDragTarget(best.rect, isBlock ? "Add here" : "Move here");
    try { e.dataTransfer.dropEffect = isBlock ? "copy" : "move"; } catch (_) {}
  }

  onDragLeave(e) {
    if (!this.doc || !this.overlay) return;
    // Only clear if leaving iframe viewport entirely
    try {
      const related = e.relatedTarget;
      if (related && this.doc.contains(related)) return;
      // Check if still inside doc
      const x = e.clientX, y = e.clientY;
      if (x != null && y != null && x >= 0 && y >= 0) {
        const el = this.doc.elementFromPoint ? this.doc.elementFromPoint(x, y) : null;
        if (el && this.doc.contains(el)) return;
      }
    } catch (_) {}
    // If we still have a session, keep dragTarget until dragover updates or dragend
  }

  onDrop(e) {
    if (!this.doc || !this.overlay) return;
    const sess = getSession();
    if (!sess || (sess.kind !== "node" && sess.kind !== "block")) return;
    e.preventDefault();
    e.stopPropagation();
    this.stopAutoScroll();
    const target = this.dragTarget;
    if (!target) { this.clearDragState(); return; }
    if (sess.kind === "block") {
      if (!sess.definition) { this.clearDragState(); return; }
      const parentNode = target.parentId == null ? null : findDocumentNode(target.parentId);
      const legal = canInsert(parentNode, sess.definition, target.index);
      if (!legal.ok) { this.clearDragState(); return; }
      const result = insertBlock({ definition: sess.definition, parentId: target.parentId, index: target.index });
      // Queue pending selection for preview morph handling (mirrors moveNode and panels click)
      try {
        if (result && result.ok && result.node) {
          state.__pendingSelectionIds ||= [];
          state.__pendingSelectionIds.push(result.node.id);
          state.__pendingSelectionBlock = result.node.block;
          state.selection = { nodeId: result.node.id, instanceKey: null, editable: true, block: result.node.block, version: result.node.version, logical: true };
        }
      } catch (_) {}
      this.clearDragState();
      if (!result || !result.ok) {
        try {
          const msg = result?.reason || "Could not add block.";
          if (typeof window !== "undefined" && window.stratumToast) window.stratumToast("error", msg);
        } catch (_) {}
        return;
      }
      try { this.requestSync(); } catch (_) {}
      return;
    }
    // Validate again before mutation (node move)
    const legal = canMove(sess.nodeId, target.parentId, target.index);
    if (!legal.ok) { this.clearDragState(); return; }
    const result = moveNode({ nodeId: sess.nodeId, parentId: target.parentId, index: target.index });
    this.clearDragState();
    if (!result || !result.ok) {
      try {
        const msg = result?.reason || "Could not move block.";
        if (typeof window !== "undefined" && window.stratumToast) window.stratumToast("error", msg);
      } catch (_) {}
      return;
    }
    try { this.requestSync(); } catch (_) {}
  }

  onDragEnd(e) {
    const sess = getSession();
    if (!sess) return;
    try { e.preventDefault(); } catch (_) {}
    this.clearDragState();
    try { this.requestSync(); } catch (_) {}
  }

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

  // Insertion-filtered wrapper: geometry is neutral, legality filtered here for insertion.
  probeGapTarget(clientX, clientY) {
    const geom = this.probeGapTargetGeometry(clientX, clientY);
    if (!geom) return null;
    const parentNode = geom.parentId == null ? null : findDocumentNode(geom.parentId);
    if (!hasLegalInsertion(parentNode, geom.index)) return null;
    return geom;
  }

  deriveInsertionCandidates(hit, e) {
    // Reuse shared raw geometry for consistency with drag (§30)
    return this.rawBoundaries(hit, e);
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

  onPointerDown(event) {
    if (event.type === "mousedown") {
      try {
        const view = event.view || this.win;
        if (view && typeof view.PointerEvent !== "undefined" && "onpointerdown" in (view || {})) return;
      } catch (_) {}
    }
    if (this.isEditorUIEvent(event)) return;
    if (isInlineEditing()) {
      if (isActiveEditorSessionEvent(event) || isActiveFieldElement(event.target)) return;
      // Inside active field browser owns caret/word selection
      try {
        const ae = event.target;
        if (ae && ae.nodeType === 3) {
          // text node, check parent
          if (isActiveFieldElement(ae.parentElement)) return;
        }
      } catch (_) {}
      commitBeforeEditorContextChange();
      return;
    }
    // Not editing: if pointerdown inside already-selected editable field, start native pointer editing
    const hit = this.hitForTarget(event.target);
    if (!hit || !hit.editable) return;
    if (!this.selected || this.selected.instanceKey !== hit.instanceKey) return;
    const node = findDocumentNode(hit.nodeId);
    if (!node) return;
    const fields = inlineFieldsForNode(node);
    if (!fields || fields.length === 0) return;
    let insideField = false;
    for (const p of fields) {
      const fieldEl = findFieldElement(this, hit, p);
      if (fieldEl && (event.target === fieldEl || (fieldEl.contains && fieldEl.contains(event.target)))) {
        insideField = true;
        break;
      }
      try {
        const closest = event.target.closest ? event.target.closest('[data-stratum-editor-field]') : null;
        if (closest && fieldEl && (closest === fieldEl || fieldEl.contains(closest))) {
          insideField = true;
          break;
        }
      } catch (_) {}
    }
    if (!insideField) {
      try {
        const closestField = event.target.closest ? event.target.closest('[data-stratum-editor-field]') : null;
        if (closestField) {
          const fieldHit = this.hitForTarget(closestField);
          if (fieldHit && fieldHit.instanceKey === hit.instanceKey) insideField = true;
        }
      } catch (_) {}
    }
    if (!insideField) return;
    const started = startInlineEdit(hit.nodeId, hit.instanceKey, this, undefined, { nativePointer: true });
    if (started) {
      try { this.requestSync(); } catch (_) {}
    }
  }

  onClick(e) {
    if (!this.doc || !this.overlay) return;
    if (this.isEditorUIEvent(e)) return;
    // Pointer intent was already decided on pointerdown. An owned control may
    // hide itself before the resulting click (Link swaps toolbar for popover),
    // so never reinterpret that click as an outside action. Keyboard-generated
    // clicks have detail=0 and explicitly commit here.
    if (isInlineEditing()) {
      if (isActiveEditorSessionEvent(e) || isActiveFieldElement(e.target)) return;
      try {
        const ae = e.target;
        if (ae && ae.nodeType === 3 && ae.parentElement && isActiveFieldElement(ae.parentElement)) return;
      } catch (_) {}
      if (e.detail === 0) commitBeforeEditorContextChange();
      else return;
    }
    if (e.type === "submit") {
      try { e.preventDefault(); e.stopPropagation(); } catch (_) {}
      const formEl = e.target;
      const hit = formEl ? this.hitForTarget(formEl) : null;
      if (hit) this.selectInstance(hit);
      return;
    }

    const target = e.target;
    const allowNativeDefault = allowsPreviewDefault(target);
    if (!allowNativeDefault) {
      try { e.preventDefault(); } catch (_) {}
    }
    try { e.stopPropagation(); } catch (_) {}
    const hit = this.hitForTarget(target);
    // Second click activation removed — native pointerdown handles editing entry
    if (hit) {
      this.selectInstance(hit);
      // clear stale insertion hint after selection (hover will repopulate)
      if (this.insertionHint) { this.insertionHint = null; if (this.overlay) this.overlay.clearInsertion(); }
      if (allowNativeDefault) {
        const raf = (this.win && this.win.requestAnimationFrame) ? this.win.requestAnimationFrame.bind(this.win) : (typeof requestAnimationFrame !== "undefined" ? requestAnimationFrame : (cb) => setTimeout(cb, 16));
        try { raf(() => this.requestSync()); } catch (_) {}
      }
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
    if (allowNativeDefault) {
      const raf = (this.win && this.win.requestAnimationFrame) ? this.win.requestAnimationFrame.bind(this.win) : (typeof requestAnimationFrame !== "undefined" ? requestAnimationFrame : (cb) => setTimeout(cb, 16));
      try { raf(() => this.requestSync()); } catch (_) {}
    }
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

  isRectComfortablyVisible(rect) {
    if (!rect || !this.doc) return false;
    try {
      const vw = this.doc.documentElement.clientWidth || (this.win && this.win.innerWidth) || 1024;
      const vh = this.doc.documentElement.clientHeight || (this.win && this.win.innerHeight) || 700;
      const inset = 64;
      return rect.top >= inset && rect.left >= inset && rect.bottom <= vh - inset && rect.right <= vw - inset;
    } catch (_) { return false; }
  }

  revealInstance(instance) {
    if (!instance || !this.doc) return;
    const rect = this.visualRect(instance);
    if (!rect) return;
    if (this.isRectComfortablyVisible(rect)) return;
    let behavior = "smooth";
    try {
      const queries = [];
      if (typeof window !== "undefined" && window.matchMedia) queries.push(window.matchMedia("(prefers-reduced-motion: reduce)"));
      if (this.win && this.win.matchMedia) queries.push(this.win.matchMedia("(prefers-reduced-motion: reduce)"));
      if (this.doc.defaultView && this.doc.defaultView.matchMedia) queries.push(this.doc.defaultView.matchMedia("(prefers-reduced-motion: reduce)"));
      for (const q of queries) if (q && q.matches) { behavior = "auto"; break; }
    } catch (_) {}
    const el = instance.rootElements && instance.rootElements[0];
    if (!el) return;
    try {
      const method = "scroll" + "IntoView";
      if (el && typeof el[method] === "function") {
        el[method]({ block: "nearest", inline: "nearest", behavior });
      }
    } catch (_) {
      try {
        const method2 = "scroll" + "IntoView";
        el[method2]({ block: "nearest" });
      } catch (_) {}
    }
    const raf = this.win && this.win.requestAnimationFrame ? this.win.requestAnimationFrame.bind(this.win) : (typeof requestAnimationFrame !== "undefined" ? requestAnimationFrame : (cb) => setTimeout(cb, 16));
    try {
      raf(() => {
        this.requestSync();
        raf(() => this.requestSync());
      });
    } catch (_) {}
  }

  selectNode(node, opts) {
    const reveal = !!(opts && opts.reveal);
    if (!node || !node.id) return;
    const keys = (this.nodeToKeys.get(node.id) || []).filter((key) => {
      const instance = this.index.get(key);
      return instance && instance.editable;
    });

    // A. preserve current selected occurrence if it belongs to this node
    if (this.selected && this.selected.nodeId === node.id && this.index.has(this.selected.instanceKey)) {
      const inst = this.index.get(this.selected.instanceKey);
      this.selectInstance(inst);
      if (reveal) this.revealInstance(inst);
      return;
    }
    if (keys.length === 1) {
      const inst = this.index.get(keys[0]);
      this.selectInstance(inst);
      if (reveal) this.revealInstance(inst);
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
        const inst = this.index.get(visible[0]);
        this.selectInstance(inst);
        if (reveal) this.revealInstance(inst);
        return;
      }
    }

    // C. otherwise keep logical selection
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
    // Optional: reveal nearest container context if trivially available — skipped for M4
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
    // Empty doc owns empty-state button, not persistent line (§24)
    if ((state.document?.nodes || []).length === 0) {
      this.overlay.clearBlocksTarget();
      return;
    }
    let target = getInsertionTarget();
    // Lightweight contextual line for active Quick Inserter (§13) — one target identity, same geometry
    if (!target && this.quickInserter && this.quickInserter.isOpen() && this.quickInserter.target) {
      target = this.quickInserter.target;
    }
    if (!target) {
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
    const isDragging = !!getSession();
    // During inline editing, suppress hover/insertion affordances (§29)
    if (isInlineEditing()) {
      try { this.overlay.clearHover(); } catch (_) {}
      try { this.overlay.clearInsertion(); } catch (_) {}
      try { this.overlay.clearBlocksTarget(); } catch (_) {}
    }
    // Persistent Blocks target indicator — hide during drag (transient drag target replaces it)
    if (!isInlineEditing() && !isDragging) this.updateBlocksTarget();
    else if (isDragging) try { this.overlay.clearBlocksTarget(); } catch (_) {}
    // Update scope boundary (primary editable region)
    this.updateScope();
    // Hover (suppressed while editing or dragging)
    if (isInlineEditing() || isDragging) {
      if (this.overlay) this.overlay.clearHover();
    } else if (this.hoverInst) {
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
      let label = labelForInstance(inst);
      const isExternal = !inst.editable;
      const editing = isInlineEditing() && state.editing && state.editing.nodeId === this.selected.nodeId;
      if (editing) {
        label = label + " · Editing";
      }
      // Preserve handle DOM during active drag so native drag source remains attached.
      if (isDragging) {
        try { this.overlay.setSelectedGeometry(rect); } catch (_) {}
      } else {
        // Decide if selected container can Add inside (§27) — small plus on handle
        let handleOpts = { external: isExternal, editing: !!editing };
        // Generic nearest-parent breadcrumb (M6) — when selected has editable SDT parent
        try {
          if (!editing && !isExternal && this.selected && this.selected.nodeId) {
            const parentInfo = findDocumentParent(this.selected.nodeId);
            if (parentInfo && parentInfo.parent) {
              const parentNode = parentInfo.parent;
              const parentDef = definitionForBlock(parentNode.block, parentNode.version);
              if (parentDef) {
                const renderedParentId = parentNode.id;
                const renderedNodeId = this.selected.nodeId;
                handleOpts.parentLabel = displayNameForBlock(parentNode.block);
                handleOpts.parentNodeId = renderedParentId;
                handleOpts.onParentClick = () => {
                  try {
                    const cur = findDocumentParent(renderedNodeId);
                    if (cur && cur.parent && cur.parent.id === renderedParentId) {
                      this.selectNode(cur.parent);
                      return;
                    }
                    const fallback = findDocumentParent(this.selected?.nodeId);
                    if (fallback && fallback.parent) this.selectNode(fallback.parent);
                  } catch (_) {}
                };
              }
            }
          }
        } catch (_) {}
        // Drag grip for movable editable block — only grip is draggable, not whole handle
        try {
          if (!editing && isExternal === false) {
            const node = findDocumentNode(this.selected.nodeId);
            if (node) {
              handleOpts.showDragGrip = true;
              // Physical drag source must be unambiguous (M6): grip carries explicit identity
              handleOpts.gripNodeId = this.selected.nodeId;
              handleOpts.gripInstanceKey = this.selected.instanceKey || "";
              handleOpts.gripBlock = this.selected.block || node.block || "";
            }
            if (node && isContainerNode(node) && (node.children || []).length >= 0) {
              if (hasLegalInsertion(node, (node.children||[]).length)) {
                handleOpts.showHandlePlus = true;
                handleOpts.onHandlePlusClick = (e, anchorRect) => {
                  e.preventDefault(); e.stopPropagation();
                  const target = { parentId: node.id, index: (node.children||[]).length };
                  try { setInsertionTarget(target, { source: "contextual" }); } catch (_) {}
                  const anchor = anchorRect || (e && e.target ? e.target.getBoundingClientRect() : null) || rect;
                  this.openQuickInserter(target, anchor);
                  try { this.requestSync(); } catch (_) {}
                };
              }
            }
          }
        } catch (_) {}
        this.overlay.setSelected(rect, label, handleOpts);
      }
    } else {
      this.overlay.clearSelection();
    }
    // Empty states (overlay/editor instrumentation, never SDT) — suppressed while editing or dragging (avoid hit-test interference)
    if (!isInlineEditing() && !isDragging) this.updateEmptyStates(); else this.overlay.clearEmptyStates();
    // Re-establish insertion line after geometry sync if we still have a hint (recompute rect) — suppressed while editing or dragging
    if (!isInlineEditing() && !isDragging && this.insertionHint && !(this.quickInserter && this.quickInserter.isOpen())) {
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
      if (!(this.quickInserter && this.quickInserter.isOpen()) && !isDragging) this.overlay.clearInsertion();
    }
    // Drag target line persistence (separate from insertion hint)
    if (isDragging && this.dragTarget && this.overlay) {
      const sess = getSession();
      const label = sess && sess.kind === "block" ? "Add here" : "Move here";
      this.overlay.setDragTarget(this.dragTarget.rect, label);
      // Only node moves show dragging outline; block insertions use copy semantics
      this.overlay.setDragging(!!(sess && sess.kind === "node"));
    } else if (!isDragging && this.overlay) {
      try { this.overlay.setDragging(false); } catch (_) {}
      // Clear stale drag target when no session (e.g., block drag canceled outside iframe)
      if (this.dragTarget) {
        this.dragTarget = null;
        try { this.overlay.clearDragTarget(); } catch (_) {}
      }
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

  injectInlineCSS(doc) {
    if (!doc || doc.__stratumInlineCSSInjected) return;
    try {
      const style = doc.createElement("style");
      style.setAttribute("data-stratum-inline", "true");
      style.textContent = `
[data-stratum-editor-field]{ position:relative; min-width: 1ch; min-height: 1em; outline: none; }
[data-stratum-editor-field][data-stratum-editing="true"]{ outline: none !important; box-shadow: none !important; caret-color: #2563eb; }
[data-stratum-editor-field]:empty::before{ content: attr(data-placeholder); color: #94a3b8; font-style: italic; pointer-events: none; opacity: 0.9; }
[data-stratum-editor-field][data-stratum-editing="true"]:empty::before{ opacity: 0.6; }
`;
      (doc.head || doc.documentElement).appendChild(style);
      doc.__stratumInlineCSSInjected = true;
    } catch (_) {}
  }

  onDblClick(e) {
    if (!this.doc || !this.overlay) return;
    if (this.isEditorUIEvent(e)) return;
    // Once contenteditable active, browser owns caret/word/paragraph/drag selection
    if (isInlineEditing() || isActiveFieldElement(e.target)) return;
    try {
      if (e.target && e.target.nodeType === 3 && e.target.parentElement && isActiveFieldElement(e.target.parentElement)) return;
    } catch (_) {}
    const hit = this.hitForTarget(e.target);
    if (!hit || !hit.editable) return;
    const node = findDocumentNode(hit.nodeId);
    if (!node) return;
    if (!hit.editable) return;
    // If already selected, native pointerdown already enabled editing — let browser do word selection
    if (this.selected && this.selected.instanceKey === hit.instanceKey) return;
    // First activation from unselected: select and enable editing without hijacking dblclick
    this.selectInstance(hit);
    const started = startInlineEdit(hit.nodeId, hit.instanceKey, this, undefined, { nativePointer: true });
    if (started) {
      try { this.requestSync(); } catch (_) {}
    }
  }

  onKey(e) {
    if (!e) return;
    if (this.isEditorUIEvent(e)) return;
    if (getSession() && e.key === "Escape") {
      e.preventDefault(); e.stopPropagation();
      this.clearDragState();
      return;
    }
    // Inline editing gets first Escape priority
    if (isInlineEditing()) {
      if (e.key === "Escape") {
        e.preventDefault(); e.stopPropagation();
        e.stopImmediatePropagation && e.stopImmediatePropagation();
        cancelActiveEdit();
        return;
      }
      // While editing, Enter is handled inside fieldEl's keydown (commit), not here
      // Tab also handled inside
      return;
    }
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
        this.clearSelection();
        return;
      }
    }
    if (e.key === "Enter" && !e.isComposing) {
      // Optional keyboard Enter to edit when selected block has exactly one plain inline field
      if (isInlineEditing()) return;
      if (this.quickInserter && this.quickInserter.isOpen()) return;
      const sel = state.selection;
      if (!sel || !sel.editable || sel.logical) return;
      if (this.selected && this.selected.instanceKey && this.index.has(this.selected.instanceKey)) {
        const node = findDocumentNode(sel.nodeId);
        if (!node) return;
        const primary = primaryInlineFieldForNode(node);
        if (!primary) return;
        // Must have single editable instance to avoid ambiguity
        const keys = (this.nodeToKeys.get(sel.nodeId) || []).filter(k => {
          const inst = this.index.get(k);
          return inst && inst.editable;
        });
        if (keys.length !== 1) return;
        // Respect composition
        if (e.isComposing) return;
        e.preventDefault(); e.stopPropagation();
        const instKey = keys[0];
        startInlineEdit(sel.nodeId, instKey, this, primary);
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
