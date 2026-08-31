// canvas.js — iframe interaction lifecycle for V2
import { buildMarkerIndex, visualRectForInstance, resolveVisualElements } from "./markers.js";
import { Overlay } from "./overlay.js";
import { state, displayNameForBlock, getVisualRootForBlock } from "./state.js";

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
    try {
      resolveVisualElements(this.index, this.elementToNode, getVisualRootForBlock);
    } catch (_) {}

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
    try {
      resolveVisualElements(this.index, this.elementToNode, getVisualRootForBlock);
    } catch (_) {}
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
      return;
    }
    // No mapped block: if click was on anchor/button/form without mapping, keep blocking navigation but don't select.
    // If truly empty area, clear selection.
    try {
      const anchor = target.closest ? target.closest("a[href], button, input[type='submit'], input[type='button'], [role='link'], [role='button'], form") : null;
      if (anchor) return;
    } catch (_) {}
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
