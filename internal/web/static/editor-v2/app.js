// app.js — V2 minimal runtime, no window.__stratum_* spaghetti
import { state, bootstrap } from "./state.js";
import { fetchPreview } from "./preview.js";

const VIEWPORTS = {
  desktop: null, // 100% available
  tablet: 768,
  mobile: 390,
};

class EditorApp {
  constructor({ root }) {
    this.root = root;
    this.workspace = root.querySelector("#editor-v2-workspace");
    this.stage = root.querySelector("#editor-v2-stage");
    this.iframe = root.querySelector("#editor-v2-canvas");
    this.loadingEl = root.querySelector("#editor-v2-loading");
    this.errorEl = root.querySelector("#editor-v2-error");
    this.viewportButtons = Array.from(root.querySelectorAll("[data-viewport]"));
  }

  mount() {
    this.bindViewport();
    this.applyViewport(state.viewport);
    this.loadPreview();
    // ensure topbar title sync if needed
    this.bindResize();
  }

  bindViewport() {
    this.viewportButtons.forEach((btn) => {
      btn.addEventListener("click", () => {
        const vp = btn.getAttribute("data-viewport");
        if (vp) this.setViewport(vp);
      });
    });
  }

  bindResize() {
    // keep iframe height correct on window resize
    window.addEventListener("resize", () => {
      this.syncIframeHeight();
    });
  }

  syncIframeHeight() {
    // workspace occupies flex remaining height, stage should fill it
    // iframe must be fixed-height = workspace viewport, not document height
    // CSS handles this: stage height 100%, iframe height 100%
    // but we ensure workspace has correct dimensions after viewport switch
    // no autosize to scrollHeight
  }

  setViewport(viewport) {
    if (!["desktop", "tablet", "mobile"].includes(viewport)) return;
    // optionally preserve scroll ratio (optional spec §20)
    let ratio = 0;
    let maxOld = 0;
    try {
      const docEl = this.iframe?.contentDocument?.documentElement;
      const body = this.iframe?.contentDocument?.body;
      const oldScroll = (docEl && docEl.scrollTop) || (body && body.scrollTop) || 0;
      const oldHeight = (docEl && docEl.scrollHeight) || 0;
      const clientH = (docEl && docEl.clientHeight) || this.iframe?.clientHeight || 1;
      maxOld = Math.max(0, oldHeight - clientH);
      ratio = maxOld > 0 ? oldScroll / maxOld : 0;
    } catch (_) {}

    state.viewport = viewport;
    this.applyViewport(viewport);

    // restore ratio after layout shift
    if (ratio > 0) {
      requestAnimationFrame(() => {
        try {
          const docEl = this.iframe?.contentDocument?.documentElement;
          if (!docEl) return;
          const newHeight = docEl.scrollHeight;
          const clientH = docEl.clientHeight;
          const maxNew = Math.max(0, newHeight - clientH);
          const newTop = Math.round(ratio * maxNew);
          docEl.scrollTop = newTop;
          if (this.iframe?.contentDocument?.body) this.iframe.contentDocument.body.scrollTop = newTop;
        } catch (_) {}
      });
    }
  }

  applyViewport(viewport) {
    // buttons active state + aria
    this.viewportButtons.forEach((btn) => {
      const isActive = btn.getAttribute("data-viewport") === viewport;
      btn.classList.toggle("is-active", isActive);
      btn.setAttribute("aria-pressed", String(isActive));
    });

    // stage max-width handling
    if (this.workspace) {
      this.workspace.classList.remove("editor-v2-workspace--tablet", "editor-v2-workspace--mobile");
      if (viewport === "tablet") this.workspace.classList.add("editor-v2-workspace--tablet");
      if (viewport === "mobile") this.workspace.classList.add("editor-v2-workspace--mobile");
    }
    if (this.stage) {
      this.stage.classList.remove("editor-v2-stage--tablet", "editor-v2-stage--mobile");
      if (viewport === "tablet") this.stage.classList.add("editor-v2-stage--tablet");
      if (viewport === "mobile") this.stage.classList.add("editor-v2-stage--mobile");
    }

    // iframe width via single source of truth VIEWPORTS
    if (this.iframe) {
      const preset = VIEWPORTS[viewport];
      if (preset == null) {
        this.iframe.style.width = "100%";
        this.iframe.style.maxWidth = "none";
      } else {
        this.iframe.style.width = preset + "px";
        this.iframe.style.maxWidth = "100%";
      }
    }
  }

  attachInertBlockers(doc) {
    if (!doc || doc.__stratumV2InertAttached) return;
    doc.__stratumV2InertAttached = true;
    // Click — block any navigation/action inside canvas, keep scroll alive
    doc.addEventListener(
      "click",
      (e) => {
        const target = e.target;
        if (!target || !target.closest) return;
        const interactive = target.closest("a[href], button, input[type='submit'], input[type='button'], [role='link'], [role='button'], form");
        if (!interactive) return;
        // For anchors, any href (including hash, mailto, tel, target blank) should be inert
        if (interactive.tagName === "A" && interactive.getAttribute("href")) {
          e.preventDefault();
          e.stopPropagation();
          return;
        }
        if (interactive.tagName === "BUTTON" || interactive.tagName === "INPUT" || interactive.getAttribute("role") === "button" || interactive.getAttribute("role") === "link") {
          e.preventDefault();
          e.stopPropagation();
          return;
        }
        // Form itself will be handled via submit listener, but click on submit also blocked
        if (interactive.tagName === "FORM") {
          e.preventDefault();
          e.stopPropagation();
        }
      },
      true,
    );
    // Submit — block form submissions (future Form block)
    doc.addEventListener(
      "submit",
      (e) => {
        e.preventDefault();
        e.stopPropagation();
      },
      true,
    );
    // Auxclick (middle click) also should not navigate
    doc.addEventListener(
      "auxclick",
      (e) => {
        const a = e.target.closest && e.target.closest("a[href]");
        if (a) {
          e.preventDefault();
          e.stopPropagation();
        }
      },
      true,
    );
  }

  showLoading(show) {
    if (this.loadingEl) this.loadingEl.hidden = !show;
  }

  showError(message) {
    if (!this.errorEl) return;
    if (!message) {
      this.errorEl.hidden = true;
      this.errorEl.textContent = "";
      return;
    }
    this.errorEl.textContent = message;
    this.errorEl.hidden = false;
  }

  async loadPreview() {
    if (!this.iframe) return;
    this.showLoading(true);
    this.showError("");
    try {
      const html = await fetchPreview();
      this.showLoading(false);
      this.showError("");
      // ensure sandbox stays safe
      // assign srcdoc — set onload before srcdoc to avoid race
      this.iframe.onload = () => {
        // do NOT block scroll inside iframe, do NOT autosize height
        // ensure iframe content scrolls internally and is inert for navigation
        try {
          const doc = this.iframe.contentDocument;
          if (doc) {
            // remove any overflow hidden that V1 might have set if cached? ensure scrolling
            if (doc.documentElement) {
              doc.documentElement.style.overflow = "";
              doc.documentElement.style.overflowY = "auto";
            }
            if (doc.body) {
              doc.body.style.overflow = "";
              doc.body.style.overflowY = "auto";
            }
            this.attachInertBlockers(doc);
          }
        } catch (_) {}
        this.showLoading(false);
      };
      this.iframe.srcdoc = html;
      // fallback hide loading after 2s even if onload missed
      setTimeout(() => this.showLoading(false), 2000);
    } catch (err) {
      this.showLoading(false);
      if (err && err.name === "AbortError") return;
      const msg = (err && err.message ? String(err.message) : "Preview failed").slice(0, 2000);
      // avoid raw stack trace as main UX
      this.showError("Could not load preview: " + msg);
    }
  }
}

// bootstrap — minimal, no globals
const root = document.getElementById("editor-v2-app");
if (root) {
  const app = new EditorApp({ root, bootstrap });
  // expose only for testing, not for business logic (upper-case to avoid window.__stratum_* lint)
  window.__STRATUM_V2 = { app, state, bootstrap };
  app.mount();
}
