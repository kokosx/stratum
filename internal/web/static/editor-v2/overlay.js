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
  border-color: #2563eb;
  /* subtle, no fill */
}
.overlay-outline--selected {
  border-width: 2px;
  border-color: #2563eb;
}
.overlay-outline--external {
  border-style: dashed;
  border-color: #d97706;
  border-width: 2px;
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
      el.style.borderColor = "#d97706";
      el.style.borderStyle = "dashed";
    } else {
      el.style.borderColor = "#2563eb";
      el.style.borderStyle = "solid";
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
    handle.textContent = label || "Block";
    if (isExternal && externalLabel) {
      handle.textContent = externalLabel;
    }
    handle.style.display = "block";
    if (isExternal) handle.classList.add("overlay-handle--external");
    else handle.classList.remove("overlay-handle--external");

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
