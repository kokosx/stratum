// overlay.js — Shadow DOM editor UI inside iframe
// Parent V2 JS controls everything, but overlay lives inside iframe document.

const HANDLE_HEIGHT = 28;
const COMPACT_HANDLE_HEIGHT = 22;
const HEADER_MAX = 40;
const HEADER_RATIO = 0.45;
const COMPACT_THRESHOLD_W = 240;
const COMPACT_THRESHOLD_H = 90;

function rectsOverlap(a, b) {
  if (!a || !b) return false;
  return !(a.left + a.width <= b.left || b.left + b.width <= a.left || a.top + a.height <= b.top || b.top + b.height <= a.top);
}

function pointInRect(pt, rect) {
  return pt.x >= rect.left && pt.x <= rect.left + rect.width && pt.y >= rect.top && pt.y <= rect.top + rect.height;
}

// Generic handle placement helper — NO block-type checks.
// Default ABOVE, but falls back to BELOW or compact inside if ABOVE would obscure ancestor chrome.
// Bounded: checks only parentRects chain (≤4) with 3 sample points per candidate.
export function resolveHandlePlacement(opts) {
  if (!opts || !opts.selectedRect) return null;
  const sel = opts.selectedRect;
  // Spec allows singular parentRect; we normalize to array (supports up to 4 ancestors)
  const parentRects = opts.parentRects || (opts.parentRect ? [opts.parentRect] : []) || [];
  const handleWidth = (opts.handleSize && opts.handleSize.width) || opts.handleWidth || COMPACT_THRESHOLD_W;
  const handleHeight = (opts.handleSize && opts.handleSize.height) || opts.handleHeight || HANDLE_HEIGHT;
  const viewportWidth = opts.viewportWidth || (opts.document && opts.document.documentElement ? opts.document.documentElement.clientWidth : 0) || (typeof window !== "undefined" && window.innerWidth) || 1024;
  const viewportHeight = opts.viewportHeight || (opts.document && opts.document.documentElement ? opts.document.documentElement.clientHeight : 0) || 1024;

  const candidates = [
    { placement: "above", left: Math.round(sel.left), top: Math.round(sel.top - handleHeight), width: handleWidth, height: handleHeight, compact: false },
    { placement: "below", left: Math.round(sel.left), top: Math.round(sel.top + sel.height + 4), width: handleWidth, height: handleHeight, compact: false },
    { placement: "inside-top", left: Math.round(sel.left + 4), top: Math.round(sel.top + 4), width: Math.min(handleWidth, Math.max(80, Math.round(sel.width - 8))), height: Math.min(handleHeight, COMPACT_HANDLE_HEIGHT), compact: true },
    { placement: "inside-bottom", left: Math.round(sel.left + 4), top: Math.round(sel.top + sel.height - COMPACT_HANDLE_HEIGHT - 4), width: Math.min(handleWidth, Math.max(80, Math.round(sel.width - 8))), height: COMPACT_HANDLE_HEIGHT, compact: true },
  ];

  const clampLeft = (c) => {
    let l = c.left;
    if (l + c.width > viewportWidth - 4) l = Math.max(4, viewportWidth - c.width - 4);
    if (l < 4) l = 4;
    return { ...c, left: l };
  };

  const obscuresParent = (candidate) => {
    if (!parentRects || !parentRects.length) return false;
    // Header zone heuristic §4: top HEADER_MAX px or HEADER_RATIO of parent height — dimension-based, not block-type.
    // Bounded hit test: 3 sample points (center + edges) per candidate against each parent header.
    for (const pr of parentRects) {
      if (!pr) continue;
      const headerH = Math.min(HEADER_MAX, pr.height * HEADER_RATIO);
      if (headerH <= 0) continue;
      const headerRect = { left: pr.left, top: pr.top, width: pr.width, height: headerH };
      if (rectsOverlap(candidate, headerRect)) return true;
      const samples = [
        { x: candidate.left + candidate.width / 2, y: candidate.top + candidate.height / 2 },
        { x: candidate.left + 8, y: candidate.top + candidate.height / 2 },
        { x: candidate.left + candidate.width - 8, y: candidate.top + candidate.height / 2 },
      ];
      for (const s of samples) if (pointInRect(s, headerRect)) return true;
      // Optional DOM hit test if document provided — bounded (≤4 parents ×3 points) and no global scan.
      // We keep geometric fallback as source of truth; DOM path only confirms obscuring when ancestor element is hit.
      if (opts.document && typeof opts.document.elementFromPoint === "function") {
        try {
          for (const s of samples) {
            const el = opts.document.elementFromPoint(s.x, s.y);
            if (el && typeof el.closest === "function" && el.closest("details, [data-stratum-block]")) {
              // If hit element is inside headerRect, treat as obscuring — still bounded and generic.
              if (pointInRect(s, headerRect)) return true;
            }
          }
        } catch (_) {}
      }
    }
    return false;
  };

  for (const cand of candidates) {
    const clamped = clampLeft(cand);
    if (cand.placement === "above" && clamped.top < 0) continue;
    // below far outside viewport → skip to inside
    if (cand.placement === "below" && clamped.top + clamped.height > viewportHeight + 20) continue;
    if (obscuresParent(clamped)) continue;
    return clamped;
  }
  // Final fallback — inside-top clamped (never obscures header by definition of header check, but we force)
  return clampLeft(candidates[2]);
}

const OVERLAY_CSS = `
:host {
  position: fixed;
  inset: 0;
  pointer-events: none;
  z-index: 2147483646;
  contain: layout;
  display: block;
}
.overlay-outline {
  position: fixed;
  pointer-events: none;
  box-sizing: border-box;
  border-style: solid;
  border-radius: 2px;
}
.overlay-outline--hover {
  border-width: 1px;
  border-color: #60a5fa;
  /* subtle, no fill */
}
.overlay-outline--hover.overlay-outline--external {
  border-color: #94a3b8;
  border-style: dashed;
}
.overlay-outline--selected {
  border-width: 2px;
  border-color: #2563eb;
}
.overlay-outline--selected.overlay-outline--external {
  border-style: dashed;
  border-color: #64748b;
  border-width: 2px;
}
.overlay-scope {
  position: fixed;
  pointer-events: none;
  box-sizing: border-box;
  height: 0;
  border: 0;
  border-top: 1px dashed #cbd5e1;
  background: transparent;
}
.overlay-scope--bottom {
  border-top: 1px dashed #cbd5e1;
}
.overlay-scope-label {
  position: fixed;
  pointer-events: none;
  height: 18px;
  line-height: 18px;
  padding: 0 6px;
  font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0;
  text-transform: none;
  background: #f8fafc;
  color: #64748b;
  border: 1px solid #cbd5e1;
  border-radius: 4px;
  white-space: nowrap;
}
.overlay-scope-label--end {
  background: #f1f5f9;
  color: #475569;
  font-size: 11px;
  border-color: #cbd5e1;
}
.overlay-handle {
  position: fixed;
  pointer-events: none;
  display: inline-flex;
  align-items: center;
  height: ${HANDLE_HEIGHT}px;
  padding: 0 0 0 8px;
  font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.02em;
  background: #2563eb;
  color: #fff;
  border-radius: 4px 4px 0 0;
  white-space: nowrap;
  max-width: 280px;
  overflow: hidden;
  box-sizing: border-box;
  gap: 0;
}
.overlay-handle__label {
  flex: 1 1 auto;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  padding-right: 8px;
}
.overlay-handle__parent {
  flex: 0 0 auto;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 3px 6px;
  margin-right: 4px;
  height: 18px;
  border: 0;
  border-radius: 3px;
  background: rgba(255,255,255,0.18);
  color: #fff;
  font: 600 11px/1 system-ui, sans-serif;
  cursor: pointer;
  pointer-events: auto;
  white-space: nowrap;
}
.overlay-handle__parent:hover { background: rgba(255,255,255,0.28); }
.overlay-handle__sep {
  flex: 0 0 auto;
  margin-right: 6px;
  opacity: 0.75;
  color: rgba(255,255,255,0.9);
  font-weight: 400;
}
.overlay-handle__plus {
  flex: 0 0 auto;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: 0;
  border-left: 1px solid rgba(255,255,255,.3);
  background: rgba(255,255,255,.15);
  color: #fff;
  font: 700 13px/1 system-ui, sans-serif;
  cursor: pointer;
  pointer-events: auto;
  border-radius: 0 4px 0 0;
}
.overlay-handle__plus:hover { background: rgba(255,255,255,.25); }
.overlay-handle__actions {
  flex: 0 0 auto;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 28px;
  padding: 0 0 4px;
  border: 0;
  border-left: 1px solid rgba(255,255,255,.3);
  border-radius: 0 4px 0 0;
  background: rgba(255,255,255,.15);
  color: #fff;
  font: 700 13px/1 system-ui, sans-serif;
  letter-spacing: 1px;
  cursor: pointer;
  pointer-events: auto;
}
.overlay-handle__actions:hover,
.overlay-handle__actions[aria-expanded="true"] { background: rgba(255,255,255,.28); }
.overlay-actions-menu {
  position: fixed;
  display: grid;
  min-width: 148px;
  padding: 4px;
  border: 1px solid #dbe3ec;
  border-radius: 7px;
  background: #fff;
  box-shadow: 0 8px 24px rgba(15,23,42,.18);
  pointer-events: auto;
  z-index: 2147483647;
}
.overlay-actions-menu[hidden] { display: none; }
.overlay-actions-menu__item {
  min-height: 32px;
  padding: 0 10px;
  border: 0;
  border-radius: 5px;
  background: transparent;
  color: #1e293b;
  font: 500 12px/1 system-ui, sans-serif;
  text-align: left;
  cursor: pointer;
}
.overlay-actions-menu__item:hover,
.overlay-actions-menu__item:focus-visible { background: #f1f5f9; outline: none; }
.overlay-actions-menu__item--destructive { color: #b91c1c; }
.overlay-actions-menu__item--destructive:hover,
.overlay-actions-menu__item--destructive:focus-visible { background: #fef2f2; }
.overlay-handle__drag {
  flex: 0 0 auto;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 28px;
  border: 0;
  border-right: 1px solid rgba(255,255,255,.3);
  background: transparent;
  color: #fff;
  font: 700 15px/1 system-ui, sans-serif;
  cursor: grab;
  pointer-events: auto;
  user-select: none;
  padding: 0;
  margin-right: 6px;
  letter-spacing: 0;
}
.overlay-handle__drag:active { cursor: grabbing; }
.overlay-handle__drag:hover { background: rgba(255,255,255,.15); }
.overlay-handle--dragging .overlay-handle__drag { cursor: grabbing; }
.overlay-outline--dragging { opacity: 0.55; }
.overlay-drag-line {
  position: fixed;
  pointer-events: none;
  height: 3px;
  background: #2563eb;
  border-radius: 2px;
  z-index: 2147483645;
  box-shadow: 0 0 0 2px rgba(37,99,235,.18);
}
.overlay-drag-chip {
  position: fixed;
  pointer-events: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 18px;
  padding: 0 8px;
  font: 600 11px/1 system-ui, sans-serif;
  background: #2563eb;
  color: #fff;
  border-radius: 999px;
  border: 1px solid #fff;
  box-shadow: 0 1px 6px rgba(37,99,235,.3);
  z-index: 2147483645;
  white-space: nowrap;
}
.overlay-handle--external {
  background: #d97706;
}
.overlay-handle--external .overlay-handle__plus { border-left-color: rgba(255,255,255,.3); }
.overlay-outline--editing {
  border-color: #1d4ed8 !important;
  border-style: solid !important;
}
.overlay-handle--editing {
  background: #1e3a8a;
}
.overlay-insertion-line {
  position: fixed;
  pointer-events: none;
  height: 2px;
  background: #2563eb;
  border-radius: 1px;
  z-index: 2147483645;
}
.overlay-insertion-plus {
  position: fixed;
  pointer-events: auto;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border-radius: 999px;
  background: #2563eb;
  color: #fff;
  font: 700 14px/1 system-ui, sans-serif;
  box-shadow: 0 1px 6px rgba(37,99,235,.35);
  cursor: pointer;
  user-select: none;
  z-index: 2147483646;
  border: 2px solid #fff;
}
.overlay-insertion-plus:hover { background: #1d4ed8; }
.overlay-blocks-target-line {
  position: fixed;
  pointer-events: none;
  height: 2px;
  background: #2563eb;
  border-radius: 1px;
  z-index: 2147483645;
  opacity: .95;
}
.overlay-blocks-target-plus {
  position: fixed;
  pointer-events: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border-radius: 999px;
  background: #2563eb;
  color: #fff;
  font: 700 12px/1 system-ui, sans-serif;
  border: 2px solid #fff;
  box-shadow: 0 1px 4px rgba(37,99,235,.3);
  z-index: 2147483645;
}
.overlay-blocks-target-chip {
  position: fixed;
  pointer-events: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 18px;
  padding: 0 8px;
  font: 600 11px/1 system-ui, sans-serif;
  background: #2563eb;
  color: #fff;
  border-radius: 999px;
  border: 1px solid #fff;
  box-shadow: 0 1px 6px rgba(37,99,235,.3);
  z-index: 2147483645;
  white-space: nowrap;
}
.overlay-empty {
  position: fixed;
  pointer-events: none;
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 48px;
  border: 1px dashed #cbd5e1;
  border-radius: 8px;
  background: transparent;
  z-index: 2147483644;
}
.overlay-empty--subtle {
  background: rgba(255,255,255,.35);
  border-color: #cbd5e1;
}
.overlay-empty__button {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
  background: rgba(255,255,255,.95);
  color: #334155;
  font: 600 12px/1 system-ui, sans-serif;
  cursor: pointer;
  pointer-events: auto;
  box-shadow: 0 1px 3px rgba(0,0,0,.06);
}
.overlay-empty__button:hover { border-color:#94a3b8; background:#fff; }
.overlay-handle--compact {
  height: 22px;
  font-size: 10px;
  border-radius: 4px;
  max-width: 200px;
}
.overlay-handle--compact .overlay-handle__parent {
  padding: 2px 5px;
  height: 16px;
  font-size: 10px;
  margin-right: 3px;
}
.overlay-handle--compact .overlay-handle__sep {
  margin-right: 4px;
}
.overlay-handle--compact .overlay-handle__drag {
  width: 24px;
  height: 22px;
  margin-right: 4px;
  font-size: 13px;
}
.overlay-handle--compact .overlay-handle__plus {
  width: 22px;
  height: 22px;
  font-size: 12px;
}
.overlay-handle--compact .overlay-handle__actions {
  width: 24px;
  height: 22px;
  padding-bottom: 3px;
  font-size: 11px;
}
.overlay-handle--compact .overlay-handle__label {
  padding-right: 6px;
  font-size: 10px;
}
.overlay-empty--compact {
  min-height: 0;
  border: none;
  background: transparent;
  justify-content: center;
  pointer-events: none;
  border-radius: 999px;
}
.overlay-empty--compact .overlay-empty__button {
  padding: 4px 10px;
  font-size: 11px;
  border-radius: 999px;
  box-shadow: 0 1px 4px rgba(0,0,0,.08);
  background: #fff;
}
.quick-inserter { pointer-events:auto; }
`;

export class Overlay {
  constructor(doc) {
    this.doc = doc;
    this.host = null;
    this.shadow = null;
    this.scopeTopEl = null;
    this.scopeTopLabelEl = null;
    this.scopeBottomEl = null;
    this.scopeBottomLabelEl = null;
    this.hoverEl = null;
    this.selectedEl = null;
    this.handleEl = null;
    this.actionsMenuEl = null;
    this.actionsTriggerEl = null;
    this.insertionLineEl = null;
    this.insertionPlusEl = null;
    this.blocksTargetLineEl = null;
    this.blocksTargetPlusEl = null;
    this.blocksTargetChipEl = null;
    this.dragLineEl = null;
    this.dragChipEl = null;
    this.emptyRootEl = null;
    this.emptyContainerEls = new Map(); // nodeId -> element
    this._beforeHeight = 0;
    this._actionNodeId = null;
    this._actionCallbacks = null;
    this._onOutsidePointerDown = (event) => {
      if (!this.isActionsMenuOpen()) return;
      const path = event?.composedPath ? event.composedPath() : [];
      if (path.includes(this.actionsMenuEl) || path.includes(this.actionsTriggerEl)) return;
      this.closeActionsMenu();
    };
  }

  attach() {
    if (!this.doc) return;
    // Already attached?
    const existing = this.doc.querySelector("stratum-editor-overlay-root");
    if (existing) {
      try { existing.remove(); } catch (_) {}
    }
    // Measure BEFORE
    try {
      const de = this.doc.documentElement;
      this._beforeHeight = de ? de.scrollHeight : 0;
    } catch (_) { this._beforeHeight = 0; }

    this.host = this.doc.createElement("stratum-editor-overlay-root");
    // Ensure host does not affect layout
    this.host.setAttribute("data-stratum-overlay", "true");
    // host style before shadow: fixed overlay, not in flow
    this.host.style.cssText = "position:fixed;inset:0;pointer-events:none;display:block;contain:layout;z-index:2147483646;";

    // Append to body — fixed overlay must not affect documentElement layout; body is valid container
    try {
      if (this.doc.body) this.doc.body.appendChild(this.host);
      else this.doc.documentElement.appendChild(this.host);
    } catch (_) {
      try { this.doc.documentElement.appendChild(this.host); } catch (_) {}
    }

    let shadow;
    try {
      shadow = this.host.attachShadow({ mode: "open" });
    } catch (_) {
      // Shadow DOM not supported fallback: use host as container
      shadow = this.host;
    }
    this.shadow = shadow;

    // Inject style
    const style = this.doc.createElement("style");
    style.textContent = OVERLAY_CSS;
    shadow.appendChild(style);

    // Create scope boundaries first (behind selection/hover)
    this.scopeTopEl = this.doc.createElement("div");
    this.scopeTopEl.className = "overlay-scope";
    this.scopeTopEl.setAttribute("data-role", "scope-top");
    this.scopeTopEl.style.display = "none";
    shadow.appendChild(this.scopeTopEl);

    this.scopeTopLabelEl = this.doc.createElement("div");
    this.scopeTopLabelEl.className = "overlay-scope-label";
    this.scopeTopLabelEl.setAttribute("data-role", "scope-top-label");
    this.scopeTopLabelEl.textContent = "Page content";
    this.scopeTopLabelEl.style.display = "none";
    shadow.appendChild(this.scopeTopLabelEl);

    this.scopeBottomEl = this.doc.createElement("div");
    this.scopeBottomEl.className = "overlay-scope overlay-scope--bottom";
    this.scopeBottomEl.setAttribute("data-role", "scope-bottom");
    this.scopeBottomEl.style.display = "none";
    shadow.appendChild(this.scopeBottomEl);

    this.scopeBottomLabelEl = this.doc.createElement("div");
    this.scopeBottomLabelEl.className = "overlay-scope-label overlay-scope-label--end";
    this.scopeBottomLabelEl.setAttribute("data-role", "scope-bottom-label");
    this.scopeBottomLabelEl.textContent = "End of Page content";
    this.scopeBottomLabelEl.style.display = "none";
    shadow.appendChild(this.scopeBottomLabelEl);

    // Create outlines (on top of scope)
    this.hoverEl = this.doc.createElement("div");
    this.hoverEl.className = "overlay-outline overlay-outline--hover";
    this.hoverEl.setAttribute("data-role", "hover");
    this.hoverEl.style.display = "none";
    shadow.appendChild(this.hoverEl);

    this.selectedEl = this.doc.createElement("div");
    this.selectedEl.className = "overlay-outline overlay-outline--selected";
    this.selectedEl.setAttribute("data-role", "selected");
    this.selectedEl.style.display = "none";
    shadow.appendChild(this.selectedEl);

    this.handleEl = this.doc.createElement("div");
    this.handleEl.className = "overlay-handle";
    this.handleEl.setAttribute("data-role", "handle");
    this.handleEl.style.display = "none";
    shadow.appendChild(this.handleEl);

    this.actionsMenuEl = this.doc.createElement("div");
    this.actionsMenuEl.className = "overlay-actions-menu";
    this.actionsMenuEl.setAttribute("data-role", "block-actions-menu");
    this.actionsMenuEl.setAttribute("data-stratum-editor-ui", "true");
    this.actionsMenuEl.setAttribute("role", "menu");
    this.actionsMenuEl.hidden = true;
    for (const action of [
      { name: "duplicate", label: "Duplicate", destructive: false },
      { name: "delete", label: "Delete", destructive: true },
    ]) {
      const button = this.doc.createElement("button");
      button.type = "button";
      button.className = "overlay-actions-menu__item" + (action.destructive ? " overlay-actions-menu__item--destructive" : "");
      button.setAttribute("data-stratum-editor-ui", "true");
      button.setAttribute("data-action", action.name);
      button.setAttribute("role", "menuitem");
      button.textContent = action.label;
      button.addEventListener("pointerdown", (event) => event.stopPropagation());
      button.addEventListener("click", (event) => {
        event.preventDefault();
        event.stopPropagation();
        const callback = action.name === "duplicate" ? this._actionCallbacks?.duplicate : this._actionCallbacks?.delete;
        this.closeActionsMenu();
        if (typeof callback === "function") callback();
      });
      this.actionsMenuEl.appendChild(button);
    }
    shadow.appendChild(this.actionsMenuEl);
    try { this.doc.addEventListener("pointerdown", this._onOutsidePointerDown, true); } catch (_) {}

    // Insertion line + plus (single affordance)
    this.insertionLineEl = this.doc.createElement("div");
    this.insertionLineEl.className = "overlay-insertion-line";
    this.insertionLineEl.setAttribute("data-role", "insertion-line");
    this.insertionLineEl.style.display = "none";
    shadow.appendChild(this.insertionLineEl);

    this.insertionPlusEl = this.doc.createElement("button");
    this.insertionPlusEl.className = "overlay-insertion-plus";
    this.insertionPlusEl.setAttribute("data-role", "insertion-plus");
    this.insertionPlusEl.setAttribute("data-stratum-editor-ui", "true");
    this.insertionPlusEl.setAttribute("type", "button");
    this.insertionPlusEl.setAttribute("aria-label", "Add block");
    this.insertionPlusEl.textContent = "+";
    this.insertionPlusEl.style.display = "none";
    shadow.appendChild(this.insertionPlusEl);

    // Drag drop indicator (transient, separate from insertion)
    this.dragLineEl = this.doc.createElement("div");
    this.dragLineEl.className = "overlay-drag-line";
    this.dragLineEl.setAttribute("data-role", "drag-line");
    this.dragLineEl.style.display = "none";
    shadow.appendChild(this.dragLineEl);

    this.dragChipEl = this.doc.createElement("div");
    this.dragChipEl.className = "overlay-drag-chip";
    this.dragChipEl.setAttribute("data-role", "drag-chip");
    this.dragChipEl.textContent = "Move here";
    this.dragChipEl.style.display = "none";
    shadow.appendChild(this.dragChipEl);

    // Persistent Blocks target indicator (§22) — line + Add here chip
    this.blocksTargetLineEl = this.doc.createElement("div");
    this.blocksTargetLineEl.className = "overlay-blocks-target-line";
    this.blocksTargetLineEl.setAttribute("data-role", "blocks-target-line");
    this.blocksTargetLineEl.style.display = "none";
    shadow.appendChild(this.blocksTargetLineEl);
    this.blocksTargetPlusEl = this.doc.createElement("div");
    this.blocksTargetPlusEl.className = "overlay-blocks-target-plus";
    this.blocksTargetPlusEl.setAttribute("data-role", "blocks-target-plus");
    this.blocksTargetPlusEl.textContent = "+";
    this.blocksTargetPlusEl.style.display = "none";
    shadow.appendChild(this.blocksTargetPlusEl);
    this.blocksTargetChipEl = this.doc.createElement("div");
    this.blocksTargetChipEl.className = "overlay-blocks-target-chip";
    this.blocksTargetChipEl.setAttribute("data-role", "blocks-target-chip");
    this.blocksTargetChipEl.textContent = "Add here";
    this.blocksTargetChipEl.style.display = "none";
    shadow.appendChild(this.blocksTargetChipEl);

    // Verify host didn't change scrollHeight
    try {
      const de = this.doc.documentElement;
      const after = de ? de.scrollHeight : 0;
      // If host added height, we still keep fixed so it shouldn't, but log if it did
      if (this._beforeHeight && after && Math.abs(after - this._beforeHeight) > 2) {
        // In debug, warn
        if (typeof console !== "undefined" && console.warn) {
          console.warn("[stratum-overlay] scrollHeight changed before:", this._beforeHeight, "after:", after);
        }
      }
    } catch (_) {}
  }

  destroy() {
    try { this.doc?.removeEventListener("pointerdown", this._onOutsidePointerDown, true); } catch (_) {}
    try {
      if (this.host && this.host.parentNode) this.host.parentNode.removeChild(this.host);
    } catch (_) {}
    this.host = null;
    this.shadow = null;
    this.hoverEl = null;
    this.selectedEl = null;
    this.handleEl = null;
    this.actionsMenuEl = null;
    this.actionsTriggerEl = null;
    this.insertionLineEl = null;
    this.insertionPlusEl = null;
    this.blocksTargetLineEl = null;
    this.blocksTargetPlusEl = null;
    this.blocksTargetChipEl = null;
    this.dragLineEl = null;
    this.dragChipEl = null;
    this.emptyRootEl = null;
    this.emptyContainerEls = new Map();
    this.scopeTopEl = null;
    this.scopeTopLabelEl = null;
    this.scopeBottomEl = null;
    this.scopeBottomLabelEl = null;
    this._actionNodeId = null;
    this._actionCallbacks = null;
    this.doc = null;
  }

  clearHover() {
    if (this.hoverEl) this.hoverEl.style.display = "none";
  }

  clearSelection() {
    this.closeActionsMenu();
    this._actionNodeId = null;
    this._actionCallbacks = null;
    if (this.selectedEl) this.selectedEl.style.display = "none";
    if (this.handleEl) this.handleEl.style.display = "none";
    // Also clear external style
    if (this.selectedEl) {
      this.selectedEl.classList.remove("overlay-outline--external");
      this.selectedEl.classList.remove("overlay-outline--editing");
    }
    if (this.handleEl) {
      this.handleEl.classList.remove("overlay-handle--external");
      this.handleEl.classList.remove("overlay-handle--editing");
      this.handleEl.classList.remove("overlay-handle--compact");
    }
  }

  isActionsMenuOpen() {
    return !!(this.actionsMenuEl && !this.actionsMenuEl.hidden);
  }

  closeActionsMenu(focusTrigger = false) {
    if (!this.actionsMenuEl) return false;
    const wasOpen = !this.actionsMenuEl.hidden;
    this.actionsMenuEl.hidden = true;
    if (this.actionsTriggerEl) this.actionsTriggerEl.setAttribute("aria-expanded", "false");
    if (focusTrigger && this.actionsTriggerEl?.isConnected) {
      try { this.actionsTriggerEl.focus({ preventScroll: true }); } catch (_) {}
    }
    return wasOpen;
  }

  positionActionsMenu(trigger) {
    if (!this.actionsMenuEl || !trigger || this.actionsMenuEl.hidden) return;
    try {
      const triggerRect = trigger.getBoundingClientRect();
      const menuRect = this.actionsMenuEl.getBoundingClientRect();
      const width = menuRect.width || 148;
      const height = menuRect.height || 72;
      const viewportWidth = this.doc.documentElement.clientWidth || this.doc.defaultView?.innerWidth || 1024;
      const viewportHeight = this.doc.documentElement.clientHeight || this.doc.defaultView?.innerHeight || 700;
      let left = triggerRect.right - width;
      let top = triggerRect.bottom + 4;
      const selectedRect = this._selectedRect;
      const below = { left, top, width, height };
      if (selectedRect && rectsOverlap(below, selectedRect) && triggerRect.top - height - 4 >= 4) {
        top = triggerRect.top - height - 4;
      }
      left = Math.max(4, Math.min(left, viewportWidth - width - 4));
      top = Math.max(4, Math.min(top, viewportHeight - height - 4));
      this.actionsMenuEl.style.left = Math.round(left) + "px";
      this.actionsMenuEl.style.top = Math.round(top) + "px";
    } catch (_) {}
  }

  toggleActionsMenu(trigger) {
    if (!this.actionsMenuEl || !trigger) return false;
    if (this.isActionsMenuOpen()) {
      this.closeActionsMenu(true);
      return false;
    }
    this.actionsTriggerEl = trigger;
    this.actionsMenuEl.hidden = false;
    trigger.setAttribute("aria-expanded", "true");
    this.positionActionsMenu(trigger);
    const first = this.actionsMenuEl.querySelector?.("button");
    try { first?.focus({ preventScroll: true }); } catch (_) {}
    return true;
  }

  clearInsertion() {
    if (this.insertionLineEl) this.insertionLineEl.style.display = "none";
    if (this.insertionPlusEl) this.insertionPlusEl.style.display = "none";
  }
  clearDragTarget() {
    if (this.dragLineEl) this.dragLineEl.style.display = "none";
    if (this.dragChipEl) this.dragChipEl.style.display = "none";
  }
  setDragTarget(rect, label) {
    if (!this.dragLineEl || !rect) {
      this.clearDragTarget();
      return;
    }
    const line = this.dragLineEl;
    line.style.display = "block";
    line.style.left = Math.round(rect.left) + "px";
    line.style.top = Math.round(rect.top) + "px";
    line.style.width = Math.round(rect.width) + "px";
    if (this.dragChipEl) {
      const chip = this.dragChipEl;
      chip.textContent = label || "Move here";
      chip.style.display = "inline-flex";
      chip.style.left = Math.round(rect.left + rect.width / 2) + "px";
      chip.style.top = Math.round(rect.top - 14) + "px";
      chip.style.transform = "translateX(-50%)";
    }
  }
  setDragging(isDragging) {
    if (this.selectedEl) this.selectedEl.classList.toggle("overlay-outline--dragging", !!isDragging);
    if (this.handleEl) this.handleEl.classList.toggle("overlay-handle--dragging", !!isDragging);
  }
  clearBlocksTarget() {
    if (this.blocksTargetLineEl) this.blocksTargetLineEl.style.display = "none";
    if (this.blocksTargetPlusEl) this.blocksTargetPlusEl.style.display = "none";
    if (this.blocksTargetChipEl) this.blocksTargetChipEl.style.display = "none";
  }
  setBlocksTarget(rect) {
    if (!this.blocksTargetLineEl || !this.blocksTargetPlusEl || !rect) {
      this.clearBlocksTarget();
      return;
    }
    const line = this.blocksTargetLineEl;
    const plus = this.blocksTargetPlusEl;
    const chip = this.blocksTargetChipEl;
    line.style.display = "block";
    line.style.left = Math.round(rect.left) + "px";
    line.style.top = Math.round(rect.top) + "px";
    line.style.width = Math.round(rect.width) + "px";
    // Hide legacy plus when chip is present — Blocks target is chip-only per §23
    if (plus) plus.style.display = "none";
    if (chip) {
      chip.style.display = "inline-flex";
      chip.textContent = "Add here";
      // Center via transform to avoid forced layout measurement on every scroll
      chip.style.left = Math.round(rect.left + rect.width / 2) + "px";
      chip.style.top = Math.round(rect.top - 13) + "px";
      chip.style.transform = "translateX(-50%)";
    }
  }

  clearEmptyStates() {
    if (this.emptyRootEl) {
      try { this.emptyRootEl.remove(); } catch (_) {}
      this.emptyRootEl = null;
    }
    for (const el of this.emptyContainerEls.values()) {
      try { el.remove(); } catch (_) {}
    }
    this.emptyContainerEls.clear();
  }

  clearScope() {
    if (this.scopeTopEl) this.scopeTopEl.style.display = "none";
    if (this.scopeTopLabelEl) this.scopeTopLabelEl.style.display = "none";
    if (this.scopeBottomEl) this.scopeBottomEl.style.display = "none";
    if (this.scopeBottomLabelEl) this.scopeBottomLabelEl.style.display = "none";
  }

  setScope(rect) {
    if (!this.scopeTopEl || !this.scopeTopLabelEl || !this.scopeBottomEl || !this.scopeBottomLabelEl || !rect) {
      this.clearScope();
      return;
    }
    const left = Math.round(rect.left);
    const width = Math.round(rect.width);
    const top = Math.round(rect.top);
    const bottom = Math.round(rect.top + rect.height);

    // Top boundary
    this.scopeTopEl.style.display = "block";
    this.scopeTopEl.style.left = left + "px";
    this.scopeTopEl.style.top = top + "px";
    this.scopeTopEl.style.width = width + "px";

    this.scopeTopLabelEl.style.display = "block";
    this.scopeTopLabelEl.textContent = "Page content";
    let labelLeft = left + 6;
    let labelTop = top - 18;
    if (labelTop < 4) labelTop = top + 4;
    this.scopeTopLabelEl.style.left = labelLeft + "px";
    this.scopeTopLabelEl.style.top = labelTop + "px";

    // Bottom boundary — explicit end
    this.scopeBottomEl.style.display = "block";
    this.scopeBottomEl.style.left = left + "px";
    this.scopeBottomEl.style.top = bottom + "px";
    this.scopeBottomEl.style.width = width + "px";

    this.scopeBottomLabelEl.style.display = "block";
    // Try to center label on bottom line, but keep within viewport
    this.scopeBottomLabelEl.textContent = "End of Page content";
    // Temporarily place to measure
    this.scopeBottomLabelEl.style.left = left + "px";
    this.scopeBottomLabelEl.style.top = (bottom + 6) + "px";
    try {
      const vw = this.doc.documentElement.clientWidth || (this.doc.defaultView && this.doc.defaultView.innerWidth) || 1024;
      const inset = 6;
      // Clamp top label
      const tlr = this.scopeTopLabelEl.getBoundingClientRect();
      if (tlr.width && labelLeft + tlr.width > vw - inset) {
        labelLeft = Math.max(inset, vw - tlr.width - inset);
        this.scopeTopLabelEl.style.left = labelLeft + "px";
      }
      // Bottom label: try centered, else left
      const blr = this.scopeBottomLabelEl.getBoundingClientRect();
      if (blr.width) {
        let bLeft = Math.round(left + (width - blr.width) / 2);
        // If centered would be clipped or too narrow, use left
        if (bLeft < inset) bLeft = left + 6;
        if (bLeft + blr.width > vw - inset) bLeft = Math.max(inset, vw - blr.width - inset);
        // If scope is very wide, centered is fine; if narrow, left is fine
        // Prefer centered when width > 200
        if (width > 240) {
          this.scopeBottomLabelEl.style.left = bLeft + "px";
        } else {
          // narrow scope, keep label near left
          let bLeft2 = left + 6;
          if (bLeft2 + blr.width > vw - inset) bLeft2 = Math.max(inset, vw - blr.width - inset);
          this.scopeBottomLabelEl.style.left = bLeft2 + "px";
        }
      }
      // If bottom label would overlap selected handle (when last section selected), hide scope bottom behind selection visually
      // Since scope is behind selection in DOM order, selection will paint on top — no extra logic needed
    } catch (_) {}
  }

  setHover(rect, isExternal) {
    if (!this.hoverEl || !rect) {
      this.clearHover();
      return;
    }
    const el = this.hoverEl;
    el.style.display = "block";
    el.style.left = Math.round(rect.left) + "px";
    el.style.top = Math.round(rect.top) + "px";
    el.style.width = Math.round(rect.width) + "px";
    el.style.height = Math.round(rect.height) + "px";
    if (isExternal) {
      el.style.borderColor = "#94a3b8";
      el.style.borderStyle = "dashed";
      el.classList.add("overlay-outline--external");
    } else {
      el.style.borderColor = "#60a5fa";
      el.style.borderStyle = "solid";
      el.classList.remove("overlay-outline--external");
    }
  }

  setInsertion(rect, onPlusClick) {
    if (!this.insertionLineEl || !this.insertionPlusEl || !rect) {
      this.clearInsertion();
      return;
    }
    const line = this.insertionLineEl;
    const plus = this.insertionPlusEl;
    line.style.display = "block";
    line.style.left = Math.round(rect.left) + "px";
    line.style.top = Math.round(rect.top) + "px";
    line.style.width = Math.round(rect.width) + "px";
    // plus centered on line — viewport-fixed coordinates
    plus.style.display = "inline-flex";
    const left = Math.round(rect.left + rect.width / 2 - 10);
    const top = Math.round(rect.top - 10);
    plus.style.left = left + "px";
    plus.style.top = top + "px";
    // bind click — capture anchor rect of the interactive control itself
    plus.onclick = (e) => {
      e.preventDefault(); e.stopPropagation();
      let anchor = null;
      try { anchor = plus.getBoundingClientRect(); } catch (_) { anchor = rect; }
      if (typeof onPlusClick === "function") onPlusClick(e, anchor);
    };
    plus.onpointerdown = (e) => { e.stopPropagation(); };
    plus.onmousedown = (e) => { e.stopPropagation(); };
  }

  setEmptyRoot(rect, label, onClick) {
    this.clearEmptyStates(); // root exclusive? keep but clear first
    if (!rect || !this.shadow) return;
    const wrap = this.doc.createElement("div");
    wrap.className = "overlay-empty overlay-empty--subtle";
    wrap.setAttribute("data-role", "empty-root");
    wrap.style.left = Math.round(rect.left) + "px";
    wrap.style.top = Math.round(rect.top) + "px";
    wrap.style.width = Math.round(rect.width) + "px";
    wrap.style.height = Math.max(72, Math.round(rect.height)) + "px";
    wrap.style.pointerEvents = "none";
    const btn = this.doc.createElement("button");
    btn.type = "button";
    btn.className = "overlay-empty__button";
    btn.setAttribute("data-stratum-editor-ui", "true");
    btn.textContent = label || "+ Add block";
    btn.addEventListener("click", (e) => { e.preventDefault(); e.stopPropagation(); if (typeof onClick === "function") onClick(e, btn.getBoundingClientRect()); });
    btn.addEventListener("pointerdown", (e) => e.stopPropagation());
    btn.addEventListener("mousedown", (e) => e.stopPropagation());
    wrap.appendChild(btn);
    this.shadow.appendChild(wrap);
    this.emptyRootEl = wrap;
  }

  setEmptyContainer(nodeId, rect, label, onClick) {
    if (!rect || !nodeId || !this.shadow) return;
    // remove existing for same node
    const existing = this.emptyContainerEls.get(nodeId);
    if (existing) try { existing.remove(); } catch (_) {}
    // Generic compact empty-state — dimension based (§7, COMPACT_THRESHOLD_W/H min tappable + padding), not block-type specific
    const isCompact = rect.width < COMPACT_THRESHOLD_W || rect.height < COMPACT_THRESHOLD_H;
    const wrap = this.doc.createElement("div");
    if (isCompact) {
      wrap.className = "overlay-empty overlay-empty--compact";
      wrap.setAttribute("data-role", "empty-container");
      wrap.dataset.nodeId = nodeId;
      wrap.dataset.compact = "true";
      // Small pill centered inside container, does not cover component
      wrap.style.left = Math.round(rect.left + rect.width / 2) + "px";
      wrap.style.top = Math.round(rect.top + rect.height / 2) + "px";
      wrap.style.width = "auto";
      wrap.style.height = "auto";
      wrap.style.transform = "translate(-50%, -50%)";
      wrap.style.minHeight = "0";
      wrap.style.pointerEvents = "none";
      wrap.style.border = "none";
      wrap.style.background = "transparent";
    } else {
      wrap.className = "overlay-empty overlay-empty--subtle";
      wrap.setAttribute("data-role", "empty-container");
      wrap.dataset.nodeId = nodeId;
      // overlay existing rendered bounds without altering layout: use transparent dashed with centered button
      // Do not cover whole theme with opaque white — subtle centered control only
      const inset = 6;
      // Keep wrap pointer-events none so underlying theme visible, button is interactive
      wrap.style.left = Math.round(rect.left + inset) + "px";
      wrap.style.top = Math.round(rect.top + inset) + "px";
      wrap.style.width = Math.round(Math.max(100, rect.width - inset * 2)) + "px";
      // Limit height to avoid huge white boxes: center vertically if container very tall, otherwise fit
      const h = Math.max(48, Math.min(80, Math.round(rect.height - inset * 2)));
      wrap.style.height = h + "px";
      // center vertically within original rect if container taller
      if (rect.height > 120) {
        const extra = (rect.height - h) / 2;
        wrap.style.top = Math.round(rect.top + extra) + "px";
      }
      wrap.style.minHeight = "48px";
      wrap.style.pointerEvents = "none";
    }
    const btn = this.doc.createElement("button");
    btn.type = "button";
    btn.className = "overlay-empty__button";
    btn.setAttribute("data-stratum-editor-ui", "true");
    btn.textContent = label || "+ Add block";
    btn.addEventListener("click", (e) => { e.preventDefault(); e.stopPropagation(); if (typeof onClick === "function") onClick(e, btn.getBoundingClientRect()); });
    btn.addEventListener("pointerdown", (e) => e.stopPropagation());
    btn.addEventListener("mousedown", (e) => e.stopPropagation());
    wrap.appendChild(btn);
    this.shadow.appendChild(wrap);
    this.emptyContainerEls.set(nodeId, wrap);
  }

  // Geometry-only update during active native drag: keep grip DOM identity stable.
  // Avoid forced layout (getBoundingClientRect) on hot dragover path — use known max-width.
  setSelectedGeometry(rect) {
    if (!this.selectedEl || !this.handleEl || !rect) return;
    const sel = this.selectedEl;
    sel.style.display = "block";
    sel.style.left = Math.round(rect.left) + "px";
    sel.style.top = Math.round(rect.top) + "px";
    sel.style.width = Math.round(rect.width) + "px";
    sel.style.height = Math.round(rect.height) + "px";
    const handle = this.handleEl;
    handle.style.display = "inline-flex";
    let handleLeft = Math.round(rect.left);
    let handleTop = Math.round(rect.top - HANDLE_HEIGHT);
    try {
      const vw = this.doc.documentElement.clientWidth || (this.doc.defaultView && this.doc.defaultView.innerWidth) || 1024;
      // Clamp without measuring: max-width 240px per CSS
      const maxW = 240;
      if (handleLeft + maxW > vw - 4) handleLeft = Math.max(4, vw - maxW - 4);
      handle.style.left = handleLeft + "px";
      handle.style.top = handleTop + "px";
      if (handleTop < 0) {
        handleTop = Math.round(rect.top);
        handle.style.top = handleTop + "px";
        handle.style.borderRadius = "0 0 4px 4px";
      } else {
        handle.style.borderRadius = "4px 4px 0 0";
      }
    } catch (_) {
      handle.style.left = handleLeft + "px";
      handle.style.top = handleTop + "px";
    }
  }

  setSelected(rect, label, opts) {
    if (!this.selectedEl || !this.handleEl || !rect) {
      this.clearSelection();
      return;
    }
    const isExternal = !!(opts && opts.external);
    const isEditing = !!(opts && opts.editing);
    const externalLabel = opts && opts.externalLabel;
    const actionNodeId = !isExternal && !isEditing && opts?.actionNodeId ? String(opts.actionNodeId) : null;
    const keepActionsOpen = this.isActionsMenuOpen() && this._actionNodeId === actionNodeId;
    if (this._actionNodeId !== actionNodeId) this.closeActionsMenu();
    this._actionNodeId = actionNodeId;
    this._actionCallbacks = actionNodeId ? {
      duplicate: opts?.onDuplicate,
      delete: opts?.onDelete,
    } : null;
    this._selectedRect = { left: rect.left, top: rect.top, width: rect.width, height: rect.height };

    // Selected outline
    const sel = this.selectedEl;
    sel.style.display = "block";
    sel.style.left = Math.round(rect.left) + "px";
    sel.style.top = Math.round(rect.top) + "px";
    sel.style.width = Math.round(rect.width) + "px";
    sel.style.height = Math.round(rect.height) + "px";
    if (isExternal) {
      sel.classList.add("overlay-outline--external");
    } else {
      sel.classList.remove("overlay-outline--external");
    }
    if (isEditing) sel.classList.add("overlay-outline--editing");
    else sel.classList.remove("overlay-outline--editing");

    // Handle — coherent [ Section | + ] (plus is visually part of handle)
    const handle = this.handleEl;
    let text = label || "Block";
    if (isExternal && externalLabel) {
      text = externalLabel;
    }
    if (isExternal) {
      if (!text.toLowerCase().includes("read-only") && !text.toLowerCase().includes("read–only")) {
        text = text + " · Read-only";
      }
      handle.classList.add("overlay-handle--external");
    } else {
      handle.classList.remove("overlay-handle--external");
    }
    if (isEditing) handle.classList.add("overlay-handle--editing");
    else handle.classList.remove("overlay-handle--editing");
    // Build handle content: drag grip + [parent › current] + optional plus inside handle (Add inside)
    // During active native drag this full rebuild must be avoided (handled via setSelectedGeometry).
    handle.replaceChildren();
    // Drag grip for movable editable block (§21-22) — document owns drag lifecycle, grip only exposes draggable attribute.
    const showGrip = !isExternal && !isEditing && opts && opts.showDragGrip;
    if (showGrip) {
      const grip = this.doc.createElement("button");
      grip.type = "button";
      grip.className = "overlay-handle__drag";
      grip.setAttribute("data-stratum-editor-ui", "true");
      grip.setAttribute("draggable", "true");
      // Drag grip carries explicit identity (M6): must not rely solely on this.selected
      const gripNodeId = (opts && opts.gripNodeId) ? String(opts.gripNodeId) : (this._lastGripNodeId || "");
      const gripInstanceKey = (opts && opts.gripInstanceKey) ? String(opts.gripInstanceKey) : (this._lastGripInstanceKey || "");
      if (gripNodeId) grip.setAttribute("data-node-id", gripNodeId);
      if (gripInstanceKey) grip.setAttribute("data-instance-key", gripInstanceKey);
      if (opts && opts.gripBlock) grip.setAttribute("data-block", String(opts.gripBlock));
      const gripLabel = `Move ${text}`;
      grip.setAttribute("aria-label", gripLabel);
      grip.setAttribute("title", "Move block");
      grip.textContent = "⠿";
      // Prevent handle click from selecting; grip handles drag via document-level composedPath
      grip.addEventListener("pointerdown", (e) => e.stopPropagation());
      grip.addEventListener("mousedown", (e) => e.stopPropagation());
      grip.addEventListener("click", (e) => { e.preventDefault(); e.stopPropagation(); });
      handle.appendChild(grip);
    }
    // Parent breadcrumb (nearest editable SDT parent) — generic, no block hardcode
    const parentLabel = opts && opts.parentLabel;
    const parentNodeId = opts && opts.parentNodeId;
    const onParentClick = opts && opts.onParentClick;
    if (!isExternal && !isEditing && parentLabel && parentNodeId && typeof onParentClick === "function") {
      const parentBtn = this.doc.createElement("button");
      parentBtn.type = "button";
      parentBtn.className = "overlay-handle__parent";
      parentBtn.setAttribute("data-stratum-editor-ui", "true");
      parentBtn.setAttribute("aria-label", `Go to parent ${parentLabel}`);
      parentBtn.setAttribute("title", `Select parent ${parentLabel}`);
      parentBtn.textContent = parentLabel;
      parentBtn.addEventListener("pointerdown", (e) => { e.stopPropagation(); });
      parentBtn.addEventListener("mousedown", (e) => { e.stopPropagation(); });
      parentBtn.addEventListener("click", (e) => { e.preventDefault(); e.stopPropagation(); try { onParentClick(); } catch (_) {} });
      handle.appendChild(parentBtn);
      const sep = this.doc.createElement("span");
      sep.className = "overlay-handle__sep";
      sep.textContent = "›";
      sep.setAttribute("aria-hidden", "true");
      handle.appendChild(sep);
    }
    const labelSpan = this.doc.createElement("span");
    labelSpan.className = "overlay-handle__label";
    labelSpan.textContent = text;
    handle.appendChild(labelSpan);
    let plusInside = null;
    const showPlus = !isExternal && opts && opts.showHandlePlus;
    if (showPlus) {
      plusInside = this.doc.createElement("button");
      plusInside.type = "button";
      plusInside.className = "overlay-handle__plus";
      plusInside.setAttribute("data-stratum-editor-ui", "true");
      const plusLabel = `Add block at end of ${text}`;
      plusInside.setAttribute("aria-label", plusLabel);
      plusInside.setAttribute("title", plusLabel);
      plusInside.textContent = "+";
      plusInside.addEventListener("click", (e) => { e.preventDefault(); e.stopPropagation(); if (typeof opts.onHandlePlusClick === "function") opts.onHandlePlusClick(e, plusInside.getBoundingClientRect()); });
      plusInside.addEventListener("pointerdown", (e) => e.stopPropagation());
      plusInside.addEventListener("mousedown", (e) => e.stopPropagation());
      handle.appendChild(plusInside);
    }
    if (actionNodeId) {
      if (plusInside) plusInside.style.borderRadius = "0";
      const actions = this.doc.createElement("button");
      actions.type = "button";
      actions.className = "overlay-handle__actions";
      actions.setAttribute("data-stratum-editor-ui", "true");
      actions.setAttribute("aria-label", `Actions for ${text}`);
      actions.setAttribute("aria-haspopup", "menu");
      actions.setAttribute("aria-expanded", keepActionsOpen ? "true" : "false");
      actions.setAttribute("title", "Block actions");
      actions.textContent = "•••";
      actions.addEventListener("pointerdown", (event) => event.stopPropagation());
      actions.addEventListener("mousedown", (event) => event.stopPropagation());
      actions.addEventListener("click", (event) => {
        event.preventDefault();
        event.stopPropagation();
        this.toggleActionsMenu(actions);
      });
      handle.appendChild(actions);
      this.actionsTriggerEl = actions;
    } else {
      this.actionsTriggerEl = null;
      this.closeActionsMenu();
    }
    // Mark handle as editor UI container for composedPath checks
    try { handle.setAttribute("data-stratum-editor-ui", "true"); } catch (_) {}
    handle.style.display = "inline-flex";

    // Position handle with generic placement helper (M6 corrective §4) — no block-type checks
    let isCompactPlacement = false;
    try {
      const parentRects = (opts && opts.parentRects) || [];
      const vw = this.doc.documentElement.clientWidth || (this.doc.defaultView && this.doc.defaultView.innerWidth) || 1024;
      const vh = this.doc.documentElement.clientHeight || (this.doc.defaultView && this.doc.defaultView.innerHeight) || 700;
      const estimatedW = 240;
      const placement = resolveHandlePlacement({
        selectedRect: rect,
        parentRects,
        handleSize: { width: estimatedW, height: HANDLE_HEIGHT },
        document: this.doc,
        viewportWidth: vw,
        viewportHeight: vh,
      });
      let handleLeft = placement ? placement.left : Math.round(rect.left);
      let handleTop = placement ? placement.top : Math.round(rect.top - HANDLE_HEIGHT);
      isCompactPlacement = !!(placement && placement.compact);
      if (isCompactPlacement) handle.classList.add("overlay-handle--compact");
      else handle.classList.remove("overlay-handle--compact");
      handle.style.left = handleLeft + "px";
      handle.style.top = handleTop + "px";
      // Measure actual width after content and clamp
      try {
        const hr = handle.getBoundingClientRect();
        if (hr && hr.width) {
          if (handleLeft + hr.width > vw - 4) {
            handleLeft = Math.max(4, vw - hr.width - 4);
            handle.style.left = handleLeft + "px";
          }
        }
      } catch (_) {}
      if (isCompactPlacement) handle.style.borderRadius = "4px";
      else if (placement && placement.placement === "below") handle.style.borderRadius = "0 0 4px 4px";
      else if (handleTop < 0) handle.style.borderRadius = "0 0 4px 4px";
      else handle.style.borderRadius = "4px 4px 0 0";
      if (keepActionsOpen && this.actionsTriggerEl) this.positionActionsMenu(this.actionsTriggerEl);
    } catch (_) {
      try {
        handle.style.left = Math.round(rect.left) + "px";
        handle.style.top = Math.round(rect.top - HANDLE_HEIGHT) + "px";
        handle.classList.remove("overlay-handle--compact");
        handle.style.borderRadius = "4px 4px 0 0";
      } catch (_) {}
    }
  }

  syncRects(hoverRect, selectedRect, hoverLabel, selectedLabel, selectedExternal) {
    // Utility if needed
    if (hoverRect) this.setHover(hoverRect, false);
    else this.clearHover();
    if (selectedRect) this.setSelected(selectedRect, selectedLabel, selectedExternal);
    else this.clearSelection();
  }
}
