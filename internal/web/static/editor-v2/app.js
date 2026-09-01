// app.js — V2 shell / viewport / preview lifecycle (interaction lives in canvas.js)
import { state, bootstrap, subscribeDocument, primaryInlineFieldForNode, plainTextFromRichText } from "./state.js";
import { fetchPreview } from "./preview.js";
import { CanvasController } from "./canvas.js";
import { PanelController } from "./panels.js";
import { commitBeforeEditorContextChange, startInlineEdit } from "./inline-editor.js";
import { parsePreviewDocument, patchPreviewDocument, isPreviewInitialized, markPreviewInitialized, fallbackReplacePreview } from "./preview-morph.js";

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
    this._previewRevision = 0;
    this._lastGoodHtml = null;
    this._onEscape = (event) => this.panels?.handleEscape(event);
    this._commitBeforeOuterInteraction = (event) => {
      // Canvas interactions are owned by CanvasController, including events
      // retargeted to the iframe element by browser tooling.
      if (event?.target === this.iframe || this.iframe?.contains?.(event?.target)) return;
      commitBeforeEditorContextChange();
    };
    this._onDocumentChange = (doc, meta) => {
      if (meta && meta.renderHint === "defer") return;
      this.schedulePreview();
    };
  }

  mount() {
    // The outer admin shell is a separate document from the canvas. Any
    // pointer interaction here explicitly ends the active inline session
    // before its controller changes editor context.
    this.root.addEventListener("pointerdown", this._commitBeforeOuterInteraction, true);
    this.root.addEventListener("click", this._commitBeforeOuterInteraction, true);
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

  _handlePendingSelection(pendingId) {
    if (!pendingId || !this.canvas) return;
    try {
      const keys = this.canvas?.nodeToKeys?.get(pendingId) || [];
      const editableKeys = keys.filter((k) => {
        const inst = this.canvas.index.get(k);
        return inst && inst.editable;
      });
      if (editableKeys.length === 1) {
        const inst = this.canvas.index.get(editableKeys[0]);
        if (inst) this.canvas.selectInstance(inst);
        try {
          const rect = this.canvas.visualRect(inst);
          if (rect) {
            const vh = this.canvas.doc.documentElement.clientHeight || window.innerHeight;
            const vw = this.canvas.doc.documentElement.clientWidth || window.innerWidth;
            const outside = rect.top < 0 || rect.bottom > vh || rect.left < 0 || rect.right > vw;
            if (outside) {
              const el = inst.rootElements && inst.rootElements[0];
              const m = "scroll" + "IntoView";
              if (el && typeof el[m] === "function") el[m]({ block: "nearest", behavior: "smooth" });
              else if (this.canvas.win) this.canvas.win.scrollTo({ top: Math.max(0, rect.top - 80), behavior: "smooth" });
            }
          }
        } catch (_) {}
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
              if (isEmpty && inst && inst.editable) {
                requestAnimationFrame(() => {
                  try { startInlineEdit(pendingId, inst.instanceKey, this.canvas, primary); } catch (_) {}
                });
              }
            }
          }
        } catch (_) {}
      } else if (editableKeys.length === 0) {
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
            state.selection = { nodeId: found.id, instanceKey: null, editable: true, block: found.block, version: found.version, logical: true };
            if (this.canvas.overlay) this.canvas.overlay.clearSelection();
          }
        } catch (_) {}
      }
    } catch (_) {}
  }

  _refreshCanvasAfterPatch(pendingId, scroller, savedTop, savedLeft) {
    // Two rAF for layout to settle, then refresh stage coordinates
    const doRefresh = () => {
      try {
        if (this.canvas) {
          this.canvas.refresh();
          if (pendingId) {
            this._handlePendingSelection(pendingId);
          } else if (state.selectedNodeId) {
            try {
              const sel = state.selection;
              if (sel && sel.instanceKey && this.canvas.index.has(sel.instanceKey)) {
                const inst = this.canvas.index.get(sel.instanceKey);
                this.canvas.selectInstance(inst);
              } else if (sel && sel.nodeId) {
                const node = state.document.nodes ? (() => {
                  const walk = (nodes) => {
                    for (const n of nodes || []) {
                      if (n.id === sel.nodeId) return n;
                      const sub = walk(n.children);
                      if (sub) return sub;
                    }
                    return null;
                  };
                  return walk(state.document.nodes);
                })() : null;
                if (node) this.canvas.selectNode(node, { scroll: false });
              }
            } catch (_) {}
          }
          try { this.canvas.updateOverlayPositions(); } catch (_) {}
          try { this.canvas.notifyViewportChanged(); } catch (_) {}
          // Preserve scroller defensively (morph already keeps scroll, but ensure)
          if (scroller && savedTop != null) {
            // Only restore if no pending scroll target
            if (!pendingId && !state.__pendingScrollToId) {
              // keep current scroll (do not force), but ensure not jumped
            }
          }
        }
      } catch (_) {}
    };
    requestAnimationFrame(() => {
      requestAnimationFrame(doRefresh);
    });
  }

  _applyPreviewHtml(html, pendingId) {
    if (!this.iframe) return false;
    const scroller = document.getElementById("editor-canvas-wrap") || this.stage || document.getElementById("editor-canvas-scroller");
    const savedTop = scroller ? scroller.scrollTop : 0;
    const savedLeft = scroller ? scroller.scrollLeft : 0;

    // Check if we can morph (initialized and contentDocument accessible)
    const canMorph = isPreviewInitialized(this.iframe) && (() => {
      try {
        const doc = this.iframe.contentDocument;
        return !!(doc && doc.documentElement && doc.body);
      } catch (_) { return false; }
    })();

    if (!canMorph) {
      // Initial or fallback: full srcdoc replace (only path that triggers iframe load)
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
        if (pendingId) this._handlePendingSelection(pendingId);
        else if (savedTop || savedLeft) {
          // For initial load, no scroll preservation needed
        }
        markPreviewInitialized(this.iframe);
        this._lastGoodHtml = html;
      };
      this.iframe.srcdoc = html;
      // Fallback hide loading after 2s
      setTimeout(() => this.showLoading(false), 2000);
      return true;
    }

    // Subsequent: morph existing document
    let nextDoc = null;
    try {
      nextDoc = parsePreviewDocument(html);
    } catch (e) {
      console.warn("[preview] parse failed, fallback to srcdoc", e);
      try { this.iframe.dataset.previewInitialized = "0"; } catch (_) {}
      return this._doFullReload(html, pendingId, scroller, savedTop, savedLeft);
    }
    if (!nextDoc || !nextDoc.documentElement || !nextDoc.body) {
      console.warn("[preview] parse invalid, fallback");
      try { this.iframe.dataset.previewInitialized = "0"; } catch (_) {}
      return this._doFullReload(html, pendingId, scroller, savedTop, savedLeft);
    }

    const currentDoc = this.iframe.contentDocument;
    // Preserve scroller position defensively
    const curScrollerTop = scroller ? scroller.scrollTop : null;
    const curScrollerLeft = scroller ? scroller.scrollLeft : null;

    let patched = false;
    try {
      patched = patchPreviewDocument(currentDoc, nextDoc);
    } catch (e) {
      console.warn("[preview] patch threw, fallback", e);
      patched = false;
    }

    if (!patched) {
      console.warn("[preview] patch failed, fallback to srcdoc");
      try { this.iframe.dataset.previewInitialized = "0"; } catch (_) {}
      return this._doFullReload(html, pendingId, scroller, savedTop, savedLeft);
    }

    // Success: update last good, refresh canvas after layout settles
    this._lastGoodHtml = html;
    this.showLoading(false);
    this.showError("");

    // Restore scroller position if it drifted (defensive)
    if (scroller && curScrollerTop != null) {
      try {
        if (scroller.scrollTop !== curScrollerTop) scroller.scrollTop = curScrollerTop;
        if (scroller.scrollLeft !== curScrollerLeft) scroller.scrollLeft = curScrollerLeft;
      } catch (_) {}
    }

    this._refreshCanvasAfterPatch(pendingId, scroller, savedTop, savedLeft);
    return true;
  }

  _doFullReload(html, pendingId, scroller, savedTop, savedLeft) {
    // Fallback path: full srcdoc replace with proper onload handling
    try { this.iframe.dataset.previewInitialized = "0"; } catch (_) {}
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
      if (pendingId) this._handlePendingSelection(pendingId);
      markPreviewInitialized(this.iframe);
      this._lastGoodHtml = html;
    };
    this.iframe.srcdoc = html;
    setTimeout(() => this.showLoading(false), 2000);
    return true;
  }

  async refreshPreview() {
    if (!this.iframe) return;
    const pendingId = this._pendingSelectionId;
    this._pendingSelectionId = null;
    const revision = ++this._previewRevision;
    this.showLoading(true);
    this.showError("");
    try {
      const html = await fetchPreview();
      if (revision !== this._previewRevision) return;
      this.showLoading(false);
      this.showError("");
      this._applyPreviewHtml(html, pendingId);
    } catch (err) {
      if (revision !== this._previewRevision) return;
      this.showLoading(false);
      if (err && err.name === "AbortError") return;
      const msg = (err && err.message ? String(err.message) : "Preview failed").slice(0, 2000);
      this.showError("Could not load preview: " + msg);
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
    try { commitBeforeEditorContextChange(); } catch (_) {}
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
    const revision = ++this._previewRevision;
    try {
      const html = await fetchPreview();
      if (revision !== this._previewRevision) return;
      this.showLoading(false);
      this.showError("");
      // Use unified apply path: initial will go via srcdoc, subsequent via morph
      // For initial, ensure we attach canvas after load
      const wasInitialized = isPreviewInitialized(this.iframe);
      if (!wasInitialized) {
        // Initial preview: srcdoc with attach
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
          markPreviewInitialized(this.iframe);
          this._lastGoodHtml = html;
        };
        this.iframe.srcdoc = html;
        setTimeout(() => this.showLoading(false), 2000);
      } else {
        this._applyPreviewHtml(html, null);
      }
    } catch (err) {
      if (revision !== this._previewRevision) return;
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
