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
  height: 24px;
  line-height: 24px;
  padding: 0 8px;
  font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.02em;
  background: #2563eb;
  color: #fff;
  border-radius: 4px 4px 0 0;
  white-space: nowrap;
  max-width: 200px;
  overflow: hidden;
  text-overflow: ellipsis;
  box-sizing: border-box;
}
.overlay-handle--external {
  background: #d97706;
}
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

    // Handle — attached to top-left of outline, slightly above
    const handle = this.handleEl;
    let text = label || "Block";
    if (isExternal && externalLabel) {
      text = externalLabel;
    }
    if (isExternal) {
      // Append Read-only indicator for external selection
      if (!text.toLowerCase().includes("read-only") && !text.toLowerCase().includes("read–only")) {
        text = text + " · Read-only";
      }
      handle.classList.add("overlay-handle--external");
    } else {
      handle.classList.remove("overlay-handle--external");
    }
    handle.textContent = text;
    handle.style.display = "block";

    // Measure handle size after text set
    // Need to compute position: top-left of rect, but handle sits above or inside
    // Spec: small tab attached to top-left outline
    // We position handle so its bottom edge touches top edge of rect
    // If rect near top of viewport (< 28px), show handle inside/top
    let handleLeft = Math.round(rect.left);
    let handleTop = Math.round(rect.top - 24);
    // Clamp to viewport visible
    try {
      const vw = this.doc.documentElement.clientWidth || (this.doc.defaultView && this.doc.defaultView.innerWidth) || 1024;
      // Estimate handle width: we can measure via getBoundingClientRect after display, but before positioning we need width
      // For now, approximate: text length * 7 + 16
      // Instead, set left then measure and adjust if overflow right
      handle.style.left = handleLeft + "px";
      handle.style.top = handleTop + "px";
      // Now measure and clamp
      const hr = handle.getBoundingClientRect();
      if (hr.width) {
        if (handleLeft + hr.width > vw - 4) {
          handleLeft = Math.max(4, vw - hr.width - 4);
          handle.style.left = handleLeft + "px";
        }
      }
      if (handleTop < 0) {
        // Not enough space above, place handle inside top edge of rect
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
