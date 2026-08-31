// overlay.js — Shadow DOM editor UI inside iframe
// Parent V2 JS controls everything, but overlay lives inside iframe document.

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
  height: 24px;
  padding: 0 0 0 8px;
  font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.02em;
  background: #2563eb;
  color: #fff;
  border-radius: 4px 4px 0 0;
  white-space: nowrap;
  max-width: 240px;
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
.overlay-handle__plus {
  flex: 0 0 auto;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
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
.overlay-handle--external {
  background: #d97706;
}
.overlay-handle--external .overlay-handle__plus { border-left-color: rgba(255,255,255,.3); }
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
    this.insertionLineEl = null;
    this.insertionPlusEl = null;
    this.blocksTargetLineEl = null;
    this.blocksTargetPlusEl = null;
    this.blocksTargetChipEl = null;
    this.emptyRootEl = null;
    this.emptyContainerEls = new Map(); // nodeId -> element
    this._beforeHeight = 0;
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
    try {
      if (this.host && this.host.parentNode) this.host.parentNode.removeChild(this.host);
    } catch (_) {}
    this.host = null;
    this.shadow = null;
    this.hoverEl = null;
    this.selectedEl = null;
    this.handleEl = null;
    this.insertionLineEl = null;
    this.insertionPlusEl = null;
    this.blocksTargetLineEl = null;
    this.blocksTargetPlusEl = null;
    this.blocksTargetChipEl = null;
    this.emptyRootEl = null;
    this.emptyContainerEls = new Map();
    this.scopeTopEl = null;
    this.scopeTopLabelEl = null;
    this.scopeBottomEl = null;
    this.scopeBottomLabelEl = null;
    this.doc = null;
  }

  clearHover() {
    if (this.hoverEl) this.hoverEl.style.display = "none";
  }

  clearSelection() {
    if (this.selectedEl) this.selectedEl.style.display = "none";
    if (this.handleEl) this.handleEl.style.display = "none";
    // Also clear external style
    if (this.selectedEl) this.selectedEl.classList.remove("overlay-outline--external");
    if (this.handleEl) this.handleEl.classList.remove("overlay-handle--external");
  }

  clearInsertion() {
    if (this.insertionLineEl) this.insertionLineEl.style.display = "none";
    if (this.insertionPlusEl) this.insertionPlusEl.style.display = "none";
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
    const wrap = this.doc.createElement("div");
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

  setSelected(rect, label, opts) {
    if (!this.selectedEl || !this.handleEl || !rect) {
      this.clearSelection();
      return;
    }
    const isExternal = !!(opts && opts.external);
    const externalLabel = opts && opts.externalLabel;

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
    // Build handle content: label + optional plus inside handle (Add inside)
    handle.replaceChildren();
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
      const plusLabel = `Add inside ${text}`;
      plusInside.setAttribute("aria-label", plusLabel);
      plusInside.setAttribute("title", plusLabel);
      plusInside.textContent = "+";
      plusInside.addEventListener("click", (e) => { e.preventDefault(); e.stopPropagation(); if (typeof opts.onHandlePlusClick === "function") opts.onHandlePlusClick(e, plusInside.getBoundingClientRect()); });
      plusInside.addEventListener("pointerdown", (e) => e.stopPropagation());
      plusInside.addEventListener("mousedown", (e) => e.stopPropagation());
      handle.appendChild(plusInside);
    }
    // Mark handle as editor UI container for composedPath checks
    try { handle.setAttribute("data-stratum-editor-ui", "true"); } catch (_) {}
    handle.style.display = "inline-flex";

    // Position handle so its bottom edge touches top edge of rect
    let handleLeft = Math.round(rect.left);
    let handleTop = Math.round(rect.top - 24);
    try {
      const vw = this.doc.documentElement.clientWidth || (this.doc.defaultView && this.doc.defaultView.innerWidth) || 1024;
      handle.style.left = handleLeft + "px";
      handle.style.top = handleTop + "px";
      const hr = handle.getBoundingClientRect();
      if (hr.width) {
        if (handleLeft + hr.width > vw - 4) {
          handleLeft = Math.max(4, vw - hr.width - 4);
          handle.style.left = handleLeft + "px";
        }
      }
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

  syncRects(hoverRect, selectedRect, hoverLabel, selectedLabel, selectedExternal) {
    // Utility if needed
    if (hoverRect) this.setHover(hoverRect, false);
    else this.clearHover();
    if (selectedRect) this.setSelected(selectedRect, selectedLabel, selectedExternal);
    else this.clearSelection();
  }
}
