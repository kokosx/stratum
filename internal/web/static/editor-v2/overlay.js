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
  border: 1px dashed #cbd5e1;
  border-radius: 6px;
  background: transparent;
}
.overlay-scope-label {
  position: fixed;
  pointer-events: none;
  height: 20px;
  line-height: 20px;
  padding: 0 6px;
  font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  background: #f8fafc;
  color: #64748b;
  border: 1px solid #cbd5e1;
  border-radius: 4px;
  white-space: nowrap;
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
    this.hoverEl = null;
    this.selectedEl = null;
    this.handleEl = null;
    this.scopeEl = null;
    this.scopeLabelEl = null;
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

    // Create outlines
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

    this.scopeEl = this.doc.createElement("div");
    this.scopeEl.className = "overlay-scope";
    this.scopeEl.setAttribute("data-role", "scope");
    this.scopeEl.style.display = "none";
    shadow.appendChild(this.scopeEl);

    this.scopeLabelEl = this.doc.createElement("div");
    this.scopeLabelEl.className = "overlay-scope-label";
    this.scopeLabelEl.setAttribute("data-role", "scope-label");
    this.scopeLabelEl.textContent = "Page content";
    this.scopeLabelEl.style.display = "none";
    shadow.appendChild(this.scopeLabelEl);

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
    this.scopeEl = null;
    this.scopeLabelEl = null;
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
    if (this.scopeEl) this.scopeEl.style.display = "none";
    if (this.scopeLabelEl) this.scopeLabelEl.style.display = "none";
  }

  setScope(rect) {
    if (!this.scopeEl || !this.scopeLabelEl || !rect) {
      this.clearScope();
      return;
    }
    // Show subtle scope boundary. If rect is very large (covers most of page), still show but low contrast.
    this.scopeEl.style.display = "block";
    this.scopeEl.style.left = Math.round(rect.left) + "px";
    this.scopeEl.style.top = Math.round(rect.top) + "px";
    this.scopeEl.style.width = Math.round(rect.width) + "px";
    this.scopeEl.style.height = Math.round(rect.height) + "px";
    this.scopeLabelEl.style.display = "block";
    // Label at top-left of scope, slightly above
    let labelLeft = Math.round(rect.left);
    let labelTop = Math.round(rect.top - 20);
    if (labelTop < 2) labelTop = Math.round(rect.top + 2);
    this.scopeLabelEl.style.left = labelLeft + "px";
    this.scopeLabelEl.style.top = labelTop + "px";
    // Clamp horizontal
    try {
      const vw = this.doc.documentElement.clientWidth || (this.doc.defaultView && this.doc.defaultView.innerWidth) || 1024;
      const lr = this.scopeLabelEl.getBoundingClientRect();
      if (lr.width && labelLeft + lr.width > vw - 4) {
        labelLeft = Math.max(4, vw - lr.width - 4);
        this.scopeLabelEl.style.left = labelLeft + "px";
      }
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
