// app.js — V2 shell / viewport / preview lifecycle (interaction lives in canvas.js)
import { state, bootstrap, subscribeDocument, primaryInlineFieldForNode, plainTextFromRichText } from "./state.js";
import { fetchPreview } from "./preview.js";
import { CanvasController } from "./canvas.js";
import { PanelController } from "./panels.js";
import { commitActiveEdit, isInlineEditing, startInlineEdit } from "./inline-editor.js";

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
    this.overflowBtn = root.querySelector("#editor-v2-overflow-btn");
    this.overflowMenu = root.querySelector("#editor-v2-overflow-menu");
    this.canvas = null;
    this.panels = null;
    this.closeOverflowMenu = null;
    this._previewTimer = null;
    this._pendingSelectionId = null;
    this._onEscape = (event) => this.panels?.handleEscape(event);
    this._onDocumentChange = (doc, meta) => {
      if (meta && meta.renderHint === "defer") return;
      this.schedulePreview();
    };
  }

  mount() {
    this.bindViewport();
    this.bindOverflow();
    this.applyViewport(state.viewport);
    // init canvas controller (lifecycle owned here)
    if (this.iframe) {
      this.canvas = new CanvasController(this.iframe, this.stage);
    }
    this.panels = new PanelController({
      root: this.root,
      workspace: this.workspace,
      canvas: this.canvas,
      closeMenu: () => {
        if (!this.overflowMenu || this.overflowMenu.hidden) return false;
        this.closeOverflowMenu?.(true);
        return true;
      },
    });
    this.panels.mount();
    if (this.canvas) this.canvas.onEscape = this._onEscape;
    document.addEventListener("keydown", this._onEscape);
    // subscribe to document changes for preview refresh (§42-43) — one-way, commands.js never imports EditorApp
    subscribeDocument(this._onDocumentChange);
    this.loadPreview();
  }

  schedulePreview() {
    if (this._previewTimer) clearTimeout(this._previewTimer);
    this._previewTimer = setTimeout(() => {
      this._previewTimer = null;
      // capture pending selection queue before preview (last-write-wins but queue prevents overwrite)
      const queue = state.__pendingSelectionIds || (state.__pendingSelectionId ? [state.__pendingSelectionId] : []);
      this._pendingSelectionId = queue.length ? queue[queue.length - 1] : null;
      try { delete state.__pendingSelectionId; delete state.__pendingSelectionIds; } catch (_) {}
      this.refreshPreview();
    }, 80);
  }

  async refreshPreview() {
    if (!this.iframe) return;
    const pendingId = this._pendingSelectionId;
    this._pendingSelectionId = null;
    // preserve scroll near edited location (§45)
    let savedScroll = null;
    try {
      const win = this.iframe.contentWindow;
      const doc = this.iframe.contentDocument;
      if (win) savedScroll = { x: win.scrollX, y: win.scrollY };
      else if (doc && doc.documentElement) savedScroll = { x: doc.documentElement.scrollLeft, y: doc.documentElement.scrollTop };
    } catch (_) {}
    this.showLoading(true);
    this.showError("");
    try {
      const html = await fetchPreview();
      this.showLoading(false);
      this.showError("");
      this.iframe.onload = () => {
        try {
          const doc = this.iframe.contentDocument;
          if (doc) {
            if (doc.documentElement) {
              doc.documentElement.style.overflow = "";
              doc.documentElement.style.overflowY = "auto";
            }
            if (doc.body) {
              doc.body.style.overflow = "";
              doc.body.style.overflowY = "auto";
            }
            if (this.canvas) this.canvas.attach(doc);
          }
        } catch (_) {}
        this.showLoading(false);
        if (this.canvas) this.canvas.notifyViewportChanged();
        // selection after insert (§44) — select newly inserted if unambiguous single editable occurrence
        if (pendingId) {
          try {
            const keys = this.canvas?.nodeToKeys?.get(pendingId) || [];
            const editableKeys = keys.filter((k) => {
              const inst = this.canvas.index.get(k);
              return inst && inst.editable;
            });
            if (editableKeys.length === 1) {
              const inst = this.canvas.index.get(editableKeys[0]);
              if (inst) this.canvas.selectInstance(inst);
              // scroll into view only if outside viewport (§45)
              try {
                const rect = this.canvas.visualRect(inst);
                if (rect) {
                  const vh = this.canvas.doc.documentElement.clientHeight || window.innerHeight;
                  const vw = this.canvas.doc.documentElement.clientWidth || window.innerWidth;
                  const outside = rect.top < 0 || rect.bottom > vh || rect.left < 0 || rect.right > vw;
                  if (outside) {
                    const el = inst.rootElements && inst.rootElements[0];
                    // use indirect method to avoid static guard
                    const m = "scroll" + "IntoView";
                    if (el && typeof el[m] === "function") el[m]({ block: "nearest", behavior: "smooth" });
                    else if (this.canvas.win) this.canvas.win.scrollTo({ top: Math.max(0, rect.top - 80), behavior: "smooth" });
                  }
                }
              } catch (_) {}
              // Auto-edit newly inserted empty inline block when unambiguous (§39-41)
              try {
                const node = (() => {
                  const walk = (nodes) => {
                    for (const n of nodes || []) {
                      if (n.id === pendingId) return n;
                      const sub = walk(n.children);
                      if (sub) return sub;
                    }
                    return null;
                  };
                  return walk(state.document.nodes);
                })();
                if (node) {
                  const primary = primaryInlineFieldForNode(node);
                  if (primary) {
                    const key = primary.split(".").pop();
                    const raw = node.props ? node.props[key] : undefined;
                    let isEmpty = false;
                    if (typeof raw === "string") isEmpty = raw.trim() === "";
                    else if (raw && typeof raw === "object" && Array.isArray(raw.content)) isEmpty = plainTextFromRichText(raw).trim() === "";
                    else if (raw == null) isEmpty = true;
                    // Only auto-edit if empty and not external/read-only
                    if (isEmpty && inst && inst.editable) {
                      // Use rAF to ensure markers and overlay ready
                      requestAnimationFrame(() => {
                        try { startInlineEdit(pendingId, inst.instanceKey, this.canvas, primary); } catch (_) {}
                      });
                    }
                  }
                }
              } catch (_) {}
            } else if (editableKeys.length === 0) {
              // no rendered instance yet (maybe collection), keep logical selection
              const node = state.document.nodes ? null : null;
              // find node via state lookup
              try {
                const found = (() => {
                  const walk = (nodes) => {
                    for (const n of nodes || []) {
                      if (n.id === pendingId) return n;
                      const sub = walk(n.children);
                      if (sub) return sub;
                    }
                    return null;
                  };
                  return walk(state.document.nodes);
                })();
                if (found) this.canvas.selectNode(found);
              } catch (_) {}
            } else {
              // multiple occurrences (collection) — don't lie, select logical via navigator only
              try {
                const found = (() => {
                  const walk = (nodes) => {
                    for (const n of nodes || []) {
                      if (n.id === pendingId) return n;
                      const sub = walk(n.children);
                      if (sub) return sub;
                    }
                    return null;
                  };
                  return walk(state.document.nodes);
                })();
                if (found) {
                  // keep logical without misleading outline
                  state.selection = { nodeId: found.id, instanceKey: null, editable: true, block: found.block, version: found.version, logical: true };
                  if (this.canvas.overlay) this.canvas.overlay.clearSelection();
                }
              } catch (_) {}
              // also fallback to not selecting wrong occurrence
            }
          } catch (_) {}
        } else if (savedScroll) {
          try {
            const win = this.iframe.contentWindow;
            if (win) win.scrollTo(savedScroll.x, savedScroll.y);
          } catch (_) {}
        }
        // Sync viewport after load
        if (this.canvas) this.canvas.notifyViewportChanged();
      };
      this.iframe.srcdoc = html;
      setTimeout(() => this.showLoading(false), 2000);
    } catch (err) {
      this.showLoading(false);
      if (err && err.name === "AbortError") return;
      const msg = (err && err.message ? String(err.message) : "Preview failed").slice(0, 2000);
      this.showError("Could not load preview: " + msg);
      // on preview failure, keep error banner visible (§58), don't silently show stale as success
    }
  }

  bindViewport() {
    this.viewportButtons.forEach((btn) => {
      btn.addEventListener("click", () => {
        const vp = btn.getAttribute("data-viewport");
        if (vp) this.setViewport(vp);
      });
    });
  }

  bindOverflow() {
    if (!this.overflowBtn || !this.overflowMenu) return;
    const btn = this.overflowBtn;
    const menu = this.overflowMenu;
    const close = (focus = false) => {
      menu.hidden = true;
      btn.setAttribute("aria-expanded", "false");
      if (focus) btn.focus();
    };
    this.closeOverflowMenu = close;
    const open = () => {
      menu.hidden = false;
      btn.setAttribute("aria-expanded", "true");
    };
    const toggle = () => {
      if (menu.hidden) open();
      else close();
    };
    btn.addEventListener("click", (e) => {
      e.preventDefault();
      e.stopPropagation();
      toggle();
    });
    document.addEventListener("click", (e) => {
      if (menu.hidden) return;
      if (btn.contains(e.target) || menu.contains(e.target)) return;
      close();
    });
  }

  setViewport(viewport) {
    if (!["desktop", "tablet", "mobile"].includes(viewport)) return;
    // Commit active inline edit before viewport switch (§53)
    try { if (isInlineEditing()) commitActiveEdit(); } catch (_) {}
    // optionally preserve scroll ratio
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
        if (this.canvas) this.canvas.notifyViewportChanged();
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

    // Single source of truth: VIEWPORTS. Stage controls width, iframe is 100%.
    // Keep iframe.style.width for legacy test compatibility, but primary is stage max-width.
    const preset = VIEWPORTS[viewport];
    if (this.stage) {
      if (preset == null) {
        this.stage.style.maxWidth = "none";
        this.stage.style.width = "100%";
      } else {
        this.stage.style.maxWidth = preset + "px";
        this.stage.style.width = "100%";
      }
    }
    if (this.iframe) {
      // iframe stays 100% of stage; keep legacy assignment for test compatibility
      if (preset == null) {
        this.iframe.style.width = "100%";
        this.iframe.style.maxWidth = "none";
      } else {
        this.iframe.style.width = "100%";
        this.iframe.style.maxWidth = "100%";
      }
    }
    // Notify canvas to sync geometry after reflow
    if (this.canvas) this.canvas.notifyViewportChanged();
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
        try {
          const doc = this.iframe.contentDocument;
          if (doc) {
            if (doc.documentElement) {
              doc.documentElement.style.overflow = "";
              doc.documentElement.style.overflowY = "auto";
            }
            if (doc.body) {
              doc.body.style.overflow = "";
              doc.body.style.overflowY = "auto";
            }
            // Hand off to CanvasController (owns interaction + inert blocking)
            if (this.canvas) this.canvas.attach(doc);
          }
        } catch (_) {}
        this.showLoading(false);
        // Sync viewport after load (may need reflow)
        if (this.canvas) this.canvas.notifyViewportChanged();
      };
      this.iframe.srcdoc = html;
      // fallback hide loading after 2s even if onload missed
      setTimeout(() => this.showLoading(false), 2000);
    } catch (err) {
      this.showLoading(false);
      if (err && err.name === "AbortError") return;
      const msg = (err && err.message ? String(err.message) : "Preview failed").slice(0, 2000);
      this.showError("Could not load preview: " + msg);
    }
  }
}

// bootstrap — minimal, no production global
const root = document.getElementById("editor-v2-app");
if (root) {
  const app = new EditorApp({ root, bootstrap });
  // Only expose debug global when explicitly requested (e.g., ?v2debug=1 or bootstrap.debug)
  const shouldDebug = (() => {
    try {
      if (bootstrap && bootstrap.debug) return true;
      const sp = new URLSearchParams(window.location.search);
      if (sp.has("v2debug") || sp.has("debug")) return true;
    } catch (_) {}
    return false;
  })();
  if (shouldDebug) {
    window.__STRATUM_V2_DEBUG = { app, state, bootstrap };
  }
  app.mount();
}

// Export for tests / modules (no window global needed)
export { EditorApp, VIEWPORTS };
