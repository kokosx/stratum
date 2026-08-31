// canvas.js — CanvasController for visual editor (marker parsing, Range, geometry, selection)
import { state, bootstrap } from "./state.js";
import { isInlineEditable, startInlineEdit } from "./inline-edit.js";
import { openContextMenu } from "./context-menu.js";
import { hasLegalInsertion } from "./mutations.js";

export class CanvasController {
  constructor(iframe, overlay) {
    this.iframe = iframe;
    this.overlay = overlay || document.getElementById("editor-canvas-overlay");
    this.breadcrumbs = document.getElementById("editor-canvas-breadcrumbs");
    this.wrap = document.getElementById("editor-canvas-wrap");
    // scroller is wrap (now .editor-canvas-scroller) ; stage is inner
    this.scroller = this.wrap;
    this.stage = document.getElementById("editor-canvas-stage") || this.wrap;
    this.hoverKey = null;
    this.selectedKey = null;
    this.index = new Map(); // instanceKey -> {nodeId, editable, range, rects, ownerType, ownerId}
    this.nodeToKeys = new Map(); // nodeId -> [instanceKey]
    this.pendingRefresh = null;
    this.lastClickKey = null;
    this.lastClickAt = 0;
    this._onResize = this.refresh.bind(this);
    this._rafPending = false;
    this._onScroll = () => {
      if (this._rafPending) return;
      this._rafPending = true;
      requestAnimationFrame(() => {
        this._rafPending = false;
        this.updateOverlayPositions();
      });
    };
    this._autoScrollRAF = null;
    this._autoScrollDir = 0;
    this.init();
  }

  init() {
    if (!this.iframe) return;
    this.iframe.addEventListener("load", () => {
      setTimeout(() => this.refresh(), 30);
    });
    window.addEventListener("resize", this._onResize);
    if (this.scroller) this.scroller.addEventListener("scroll", this._onScroll, { passive: true });
    // delegate hover/selection via overlay
    if (this.overlay) {
      this.overlay.addEventListener("mousemove", (e) => this.onOverlayMouseMove(e));
      this.overlay.addEventListener("click", (e) => this.onOverlayClick(e));
      this.overlay.addEventListener("mouseleave", () => this.clearHover());
    }
    // expose globally
    window.__stratum_canvasController = this;
  }

  refresh() {
    if (!this.iframe || !this.iframe.contentDocument) return;
    const doc = this.iframe.contentDocument;
    if (!doc.body) return;
    try {
      // prevent double scroll: iframe content should not scroll internally, outer scroller does
      try {
        if (doc.documentElement) doc.documentElement.style.overflow = "hidden";
        if (doc.body) doc.body.style.overflow = "hidden";
      } catch (_) {}
      this.buildIndex(doc);
      this.renderOverlays();
      this.updateOverlayPositions();
      // attach capture-phase blockers inside iframe
      this.attachIframeBlockers(doc);
      // viewport height autosize
      try {
        const body = doc.body;
        const html = doc.documentElement;
        const h = Math.max(body.scrollHeight, html ? html.scrollHeight : 0, body.offsetHeight, 600);
        this.iframe.style.height = Math.max(400, h + 32) + "px";
        if (this.stage && this.stage !== this.scroller) {
          this.stage.style.minHeight = this.iframe.style.height;
        }
      } catch (_) {}
    } catch (e) {
      console.error("canvas refresh failed", e);
    }
  }

  buildIndex(doc) {
    this.index.clear();
    this.nodeToKeys.clear();
    const walker = doc.createTreeWalker(doc.body, NodeFilter.SHOW_COMMENT, null);
    const stack = [];
    let node;
    while ((node = walker.nextNode())) {
      const data = node.data.trim();
      if (data.startsWith("stratum-node-start:")) {
        const payload = data.slice("stratum-node-start:".length);
        const parts = payload.split(":");
        if (parts.length < 3) continue;
        let nodeId, instanceKey, editable = false, ownerType = "", ownerId = "", ownerLabel = "";
        let isNew = (parts.length === 3 || parts.length === 5 || parts.length === 6);
        if (isNew) {
          try { nodeId = decodeURIComponent(parts[0]); } catch { nodeId = parts[0]; }
          try { instanceKey = decodeURIComponent(parts[1]); } catch { instanceKey = parts[1]; }
          editable = parts[2] === "true";
          if (parts.length >= 5) {
            try { ownerType = decodeURIComponent(parts[3]); } catch { ownerType = parts[3]; }
            try { ownerId = decodeURIComponent(parts[4]); } catch { ownerId = parts[4]; }
          }
          if (parts.length === 6) {
            try { ownerLabel = decodeURIComponent(parts[5]); } catch { ownerLabel = parts[5]; }
          }
        } else {
          nodeId = parts[0];
          try { nodeId = decodeURIComponent(nodeId); } catch {}
          const editableStr = parts[parts.length - 1];
          editable = editableStr === "true";
          instanceKey = parts.slice(1, parts.length - 1).join(":");
          try { instanceKey = decodeURIComponent(instanceKey); } catch {}
          if (parts.length >= 5) {
            const maybeEditable = parts[parts.length - 3];
            if (maybeEditable === "true" || maybeEditable === "false") {
              editable = maybeEditable === "true";
              ownerType = parts[parts.length - 2];
              ownerId = parts[parts.length - 1];
              try { ownerType = decodeURIComponent(ownerType); } catch {}
              try { ownerId = decodeURIComponent(ownerId); } catch {}
              instanceKey = parts.slice(1, parts.length - 3).join(":");
              try { instanceKey = decodeURIComponent(instanceKey); } catch {}
            }
          }
        }
        stack.push({ nodeId, instanceKey, editable, ownerType, ownerId, ownerLabel, startComment: node, range: null });
      } else if (data.startsWith("stratum-node-end:")) {
        const payload = data.slice("stratum-node-end:".length);
        const parts = payload.split(":");
        if (parts.length < 2) continue;
        let nodeId, instanceKey;
        if (parts.length === 2) {
          try { nodeId = decodeURIComponent(parts[0]); } catch { nodeId = parts[0]; }
          try { instanceKey = decodeURIComponent(parts[1]); } catch { instanceKey = parts[1]; }
        } else {
          nodeId = parts[0];
          try { nodeId = decodeURIComponent(nodeId); } catch {}
          instanceKey = parts.slice(1).join(":");
          try { instanceKey = decodeURIComponent(instanceKey); } catch {}
        }
        let idx = stack.length - 1;
        while (idx >= 0) {
          if (stack[idx].nodeId === nodeId && stack[idx].instanceKey === instanceKey) break;
          idx--;
        }
        if (idx < 0) continue;
        const startInfo = stack[idx];
        try {
          const range = doc.createRange();
          range.setStartAfter(startInfo.startComment);
          range.setEndBefore(node);
          startInfo.range = range;
          let rects = [];
          // Prefer single top-level element rect for containers with padding/background
          let singleEl = null;
          try {
            let probe = startInfo.startComment.nextSibling;
            while (probe && probe.nodeType === Node.COMMENT_NODE) probe = probe.nextSibling;
            let count = 0;
            let lastEl = null;
            let cur = startInfo.startComment.nextSibling;
            while (cur && cur !== node) {
              if (cur.nodeType === 1) { count++; lastEl = cur; if (count>1) break; }
              cur = cur.nextSibling;
            }
            if (count===1 && lastEl && lastEl.getBoundingClientRect) singleEl = lastEl;
          } catch (_) {}
          if (singleEl) {
            try {
              const er = singleEl.getBoundingClientRect();
              if (er && (er.width>0 || er.height>0)) rects = [er];
            } catch (_) {}
          }
          if (!rects.length) {
            try {
              rects = Array.from(range.getClientRects());
            } catch (_) { rects = []; }
          }
          if (!rects.length) {
            try {
              const rect = range.getBoundingClientRect();
              if (rect && (rect.width > 0 || rect.height > 0)) rects = [rect];
              else {
                let probe = startInfo.startComment.nextSibling;
                while (probe && probe.nodeType === Node.COMMENT_NODE) probe = probe.nextSibling;
                if (probe && probe.getBoundingClientRect) {
                  const pr = probe.getBoundingClientRect();
                  if (pr.width > 0 || pr.height > 0) rects = [pr];
                  else rects = [{ top: 0, left: 0, width: 120, height: 64, _empty: true, _interactive: true }];
                } else {
                  rects = [{ top: 0, left: 0, width: 120, height: 64, _empty: true, _interactive: true }];
                }
              }
            } catch (_) {}
          }
          // Build interactiveRects: ensure minimum 44px height, empty 64-80px
          const interactiveRects = rects.map(r => {
            const isEmpty = !!r._empty;
            const w = r.width || 120;
            const h = r.height || (isEmpty ? 64 : 44);
            const minH = isEmpty ? 64 : 44;
            const minW = isEmpty ? 120 : 44;
            const expW = Math.max(w, minW);
            const expH = Math.max(h, minH);
            // For empty, ensure at least 64-80px height if container empty
            let finalH = expH;
            if(isEmpty){
              const found = window.__stratum_findNode ? window.__stratum_findNode(startInfo.nodeId)?.node : null;
              if(found && window.__stratum_isContainer && window.__stratum_isContainer(found)){
                finalH = Math.max(expH, 72);
                if(found.block==="core/section" || found.block==="core/stack" || found.block==="core/grid") finalH = Math.max(finalH, 80);
              }
            }
            // Center expanded rect on original if original small
            const dx = (expW - w)/2;
            const dy = (finalH - h)/2;
            return {
              left: (r.left||0) - (isNaN(dx)?0:dx),
              top: (r.top||0) - (isNaN(dy)?0:dy),
              width: expW,
              height: finalH,
              right: (r.left||0) - (isNaN(dx)?0:dx) + expW,
              bottom: (r.top||0) - (isNaN(dy)?0:dy) + finalH,
              _empty: !!r._empty,
              _interactive: true,
              _visual: r
            };
          });
          const key = instanceKey;
          this.index.set(key, {
            nodeId: startInfo.nodeId,
            instanceKey: key,
            editable: startInfo.editable,
            ownerType: startInfo.ownerType,
            ownerId: startInfo.ownerId,
            ownerLabel: startInfo.ownerLabel || "",
            range,
            rects,
            interactiveRects,
            startComment: startInfo.startComment,
            endComment: node,
          });
          if (!this.nodeToKeys.has(startInfo.nodeId)) this.nodeToKeys.set(startInfo.nodeId, []);
          this.nodeToKeys.get(startInfo.nodeId).push(key);
        } catch (_) {}
        stack.splice(idx, 1);
      }
    }
  }

  attachIframeBlockers(doc) {
    if (doc.__stratumBlockersAttached) return;
    doc.__stratumBlockersAttached = true;
    const block = (e) => {
      const target = e.target;
      // allow selection but block navigation
      if (target.closest && target.closest("a, button[type='submit'], form")) {
        e.preventDefault();
        e.stopPropagation();
        // Find nearest stratum node for click selection
        // Walk up to find comment before?
        // Let overlay handle selection; just prevent nav
      }
    };
    doc.addEventListener("click", (e) => {
      e.preventDefault();
      e.stopPropagation();
      // Find instanceKey under click point
      const x = e.clientX, y = e.clientY;
      const hit = this.hitTest(x, y);
      if (hit) {
        this.selectNode(hit.nodeId, hit.instanceKey);
        if (hit.editable) {
          state.selectedNodeId = hit.nodeId;
          state.selectedInstanceKey = hit.instanceKey;
          if (window.__stratum_onSelectionChange) window.__stratum_onSelectionChange(hit.nodeId, hit.instanceKey);
          if (window.__stratum_renderInspector) window.__stratum_renderInspector();
          const nav = document.getElementById("navigator-tree");
          if (nav && window.__stratum_renderNavigator) window.__stratum_renderNavigator();
          if (window.__stratum_updateBreadcrumbs) window.__stratum_updateBreadcrumbs();
        } else {
          // external — show toast/selection but inspector shows external message
          state.selectedNodeId = hit.nodeId; // still set for highlight but inspector will show external
          state.selectedInstanceKey = hit.instanceKey;
          if (window.__stratum_onSelectionChange) window.__stratum_onSelectionChange(hit.nodeId, hit.instanceKey, hit);
          if (window.__stratum_renderInspector) window.__stratum_renderInspector(hit);
        }
        this.renderOverlays();
      }
    }, true);
    doc.addEventListener("submit", (e) => {
      e.preventDefault();
      e.stopPropagation();
    }, true);
    doc.addEventListener("mousemove", (e) => {
      const hit = this.hitTest(e.clientX, e.clientY);
      if (hit) this.setHover(hit.instanceKey);
      else this.clearHover();
    });
    doc.addEventListener("dblclick", (e) => {
      e.preventDefault();
      e.stopPropagation();
      const hit = this.hitTest(e.clientX, e.clientY);
      if (hit && hit.editable) {
        try {
          const node = window.__stratum_findNode ? window.__stratum_findNode(hit.nodeId)?.node : null;
          if (node && isInlineEditable(node)) {
            // Select first
            state.selectedNodeId = hit.nodeId;
            state.selectedInstanceKey = hit.instanceKey;
            if (window.__stratum_onSelectionChange) window.__stratum_onSelectionChange(hit.nodeId, hit.instanceKey);
            if (window.__stratum_renderInspector) window.__stratum_renderInspector();
            startInlineEdit(hit.nodeId, hit.instanceKey, this);
          }
        } catch (_) {}
      }
    }, true);
    // also listen for scroll inside iframe
    doc.addEventListener("scroll", this._onScroll, true);
    const win = doc.defaultView;
    if (win) win.addEventListener("resize", this._onScroll);
  }

  hitTest(clientX, clientY) {
    // Find deepest (smallest visual area) using interactiveRects — empty nodes participate via expanded hit target
    let best = null;
    let bestArea = Infinity;
    for (const [key, info] of this.index.entries()) {
      const testRects = info.interactiveRects || info.rects;
      const visualRects = info.rects;
      for (let idx=0; idx<testRects.length; idx++){
        const r = testRects[idx];
        const v = visualRects[idx] || r;
        const left = r.left, top = r.top, right = r.right ?? (r.left + r.width), bottom = r.bottom ?? (r.top + r.height);
        if (clientX >= left && clientX <= right && clientY >= top && clientY <= bottom) {
          const area = (v.width || 0) * (v.height || 0);
          // Prefer smaller visual area; for empty, use interactive size but still prefer smallest
          const score = area > 0 ? area : (r.width * r.height);
          if (score < bestArea) {
            bestArea = score;
            best = info;
          }
        }
      }
    }
    return best;
  }

  dropIntent(clientX, clientY) {
    const iframeRect = this.iframe.getBoundingClientRect();
    const x = clientX - iframeRect.left;
    const y = clientY - iframeRect.top;
    const hit = this.hitTest(x, y);
    if (!hit) return { hit: null, position: "root", x, y, rect: null };
    const rect = this.boundsForInfo(hit);
    if (!rect) return { hit, position: "inside", x, y, rect: null };
    const edge = Math.min(24, Math.max(8, rect.height * 0.25));
    let position = "inside";
    if (y <= rect.top + edge) position = "before";
    else if (y >= rect.bottom - edge) position = "after";
    return { hit, position, x, y, rect };
  }

  boundsForInfo(info) {
    if (!info || !info.rects || !info.rects.length) return null;
    const rects = info.rects.filter((rect) => !rect._empty && (rect.width > 0 || rect.height > 0));
    if (!rects.length) return null;
    const left = Math.min(...rects.map((rect) => rect.left));
    const top = Math.min(...rects.map((rect) => rect.top));
    const right = Math.max(...rects.map((rect) => rect.right ?? rect.left + rect.width));
    const bottom = Math.max(...rects.map((rect) => rect.bottom ?? rect.top + rect.height));
    return { left, top, right, bottom, width: right - left, height: bottom - top };
  }

  boundsForNode(nodeId) {
    const keys = this.nodeToKeys.get(nodeId) || [];
    const bounds = keys
      .map((key) => this.index.get(key))
      .filter((info) => info && info.editable)
      .map((info) => this.boundsForInfo(info))
      .filter(Boolean);
    if (!bounds.length) return null;
    const left = Math.min(...bounds.map((rect) => rect.left));
    const top = Math.min(...bounds.map((rect) => rect.top));
    const right = Math.max(...bounds.map((rect) => rect.right));
    const bottom = Math.max(...bounds.map((rect) => rect.bottom));
    return { left, top, right, bottom, width: right - left, height: bottom - top };
  }

  showDropIndicator(rect, position, valid) {
    if (!this.overlay) return;
    let indicator = this.overlay.querySelector(".canvas-drop-indicator");
    if (!indicator) {
      indicator = document.createElement("div");
      indicator.className = "canvas-drop-indicator";
      this.overlay.append(indicator);
    }
    const fallbackWidth = Math.max(80, this.iframe.clientWidth - 32);
    const target = rect || { left: 16, top: 24, width: fallbackWidth, height: 72, bottom: 96 };
    indicator.className = `canvas-drop-indicator canvas-drop-indicator--${position} ${valid ? "is-valid" : "is-invalid"}`;
    indicator.style.left = `${target.left}px`;
    indicator.style.width = `${Math.max(24, target.width)}px`;
    if (position === "inside" || position === "root-empty") {
      indicator.style.top = `${target.top}px`;
      indicator.style.height = `${Math.max(24, target.height)}px`;
    } else {
      const top = position === "before" ? target.top : target.bottom;
      indicator.style.top = `${top - 2}px`;
      indicator.style.height = "4px";
    }
  }

  clearDropIndicator() {
    if (!this.overlay) return;
    const indicator = this.overlay.querySelector(".canvas-drop-indicator");
    if (indicator) indicator.remove();
  }

  onOverlayMouseMove(e) {
    const rect = this.overlay.getBoundingClientRect();
    const x = e.clientX - rect.left;
    const y = e.clientY - rect.top;
    // Convert overlay coords to iframe viewport coords
    const iframeRect = this.iframe.getBoundingClientRect();
    // overlay is positioned over iframe wrap, same size
    // Need to map to iframe content coords (clientX/Y already iframe viewport relative plus iframe offset)
    // Simpler: use iframe hitTest with same clientX/Y as overlay event is relative to viewport, but iframe contentWindow coordinates equal viewport coords offset by iframe rect
    // For now use hitTest with e.clientX - iframeRect.left etc?
    // Actually hitTest expects iframe content viewport client coords (same as iframe's contentWindow)
    // e.clientX is page viewport, iframe content at iframeRect.left/top
    const ix = e.clientX - iframeRect.left;
    const iy = e.clientY - iframeRect.top;
    const hit = this.hitTest(ix, iy);
    if (hit) this.setHover(hit.instanceKey);
    else this.clearHover();
  }

  onOverlayClick(e) {
    const iframeRect = this.iframe.getBoundingClientRect();
    const ix = e.clientX - iframeRect.left;
    const iy = e.clientY - iframeRect.top;
    const hit = this.hitTest(ix, iy);
    if (hit) {
      const now = Date.now();
      const isDoubleClick = this.lastClickKey === hit.instanceKey && now - this.lastClickAt < 500;
      this.lastClickKey = isDoubleClick ? null : hit.instanceKey;
      this.lastClickAt = isDoubleClick ? 0 : now;
      this.selectNode(hit.nodeId, hit.instanceKey);
      if (hit.editable) {
        state.selectedNodeId = hit.nodeId;
        state.selectedInstanceKey = hit.instanceKey;
        if (window.__stratum_onSelectionChange) window.__stratum_onSelectionChange(hit.nodeId, hit.instanceKey);
        if (window.__stratum_renderInspector) window.__stratum_renderInspector();
      } else {
        state.selectedNodeId = hit.nodeId;
        state.selectedInstanceKey = hit.instanceKey;
        if (window.__stratum_onSelectionChange) window.__stratum_onSelectionChange(hit.nodeId, hit.instanceKey, hit);
        if (window.__stratum_renderInspector) window.__stratum_renderInspector(hit);
      }
      this.renderOverlays();
      if (isDoubleClick && hit.editable) {
        try {
          const node = window.__stratum_findNode ? window.__stratum_findNode(hit.nodeId)?.node : null;
          if (node && isInlineEditable(node)) {
            startInlineEdit(hit.nodeId, hit.instanceKey, this);
          }
        } catch (_) {}
      }
      e.preventDefault();
      e.stopPropagation();
    }
  }

  setHover(instanceKey) {
    if (this.hoverKey === instanceKey) return;
    this.hoverKey = instanceKey;
    this.renderOverlays();
  }
  clearHover() {
    if (this.hoverKey === null) return;
    this.hoverKey = null;
    this.renderOverlays();
  }

  selectNode(nodeId, instanceKey, opts = {}) {
    this.selectedKey = instanceKey || (this.nodeToKeys.get(nodeId) ? this.nodeToKeys.get(nodeId)[0] : null);
    if (opts.scroll !== false && this.selectedKey && this.index.has(this.selectedKey)) {
      try {
        const info = this.index.get(this.selectedKey);
        if (info && info.rects && info.rects.length) {
          // scroll outer scroller to show selection (nearest)
          const scroller = this.scroller;
          if (scroller) {
            const first = info.rects[0];
            const scRect = scroller.getBoundingClientRect();
            const stageRect = this.stage ? this.stage.getBoundingClientRect() : scRect;
            // first.top is iframe viewport coordinate (0 = top of visible iframe content)
            // Convert to scroller scroll offset: scroller.scrollTop + first.top - scRect.top + offset
            // Simpler: compute overlay position: info rect is already relative to overlay (which is at 0,0 of stage)
            // We can scroll scroller so that rect is visible with block:nearest
            const overlayTop = first.top; // overlay is at stage 0,0
            const visibleTop = scroller.scrollTop;
            const visibleBottom = visibleTop + scroller.clientHeight;
            const targetTop = overlayTop;
            const targetBottom = overlayTop + (first.height || 44);
            if (targetTop < visibleTop + 16) {
              scroller.scrollTo({ top: Math.max(0, targetTop - 24), behavior: opts.behavior || "smooth" });
            } else if (targetBottom > visibleBottom - 16) {
              scroller.scrollTo({ top: targetBottom - scroller.clientHeight + 24, behavior: opts.behavior || "smooth" });
            }
          }
        } else if (info && info.range) {
          let target = info.startComment.nextSibling;
          while (target && target.nodeType !== 1) target = target.nextSibling;
          if (target && target.scrollIntoView) target.scrollIntoView({ block: "nearest", behavior: "smooth" });
        }
      } catch (_) {}
    }
    this.renderOverlays();
    // notify breadcrumbs
    if (window.__stratum_updateBreadcrumbs) window.__stratum_updateBreadcrumbs();
  }

  // Scroll scroller to show a specific node (used after insertion)
  scrollToNode(nodeId, behavior = "smooth") {
    const keys = this.nodeToKeys.get(nodeId) || [];
    if (!keys.length) return;
    const info = this.index.get(keys[0]);
    if (!info || !info.rects || !info.rects.length) return;
    const scroller = this.scroller;
    if (!scroller) return;
    const first = info.rects[0];
    const targetTop = first.top;
    const targetBottom = targetTop + (first.height || 64);
    const visibleTop = scroller.scrollTop;
    const visibleBottom = visibleTop + scroller.clientHeight;
    if (targetTop < visibleTop || targetBottom > visibleBottom) {
      scroller.scrollTo({ top: Math.max(0, targetTop - 80), behavior });
    }
  }

  // Return representative instanceKey for a nodeId (dedup Collection repeats)
  representativeKeyFor(nodeId) {
    const keys = this.nodeToKeys.get(nodeId) || [];
    if (!keys.length) return null;
    // Prefer selectedInstanceKey if it belongs to this node
    if (state.selectedNodeId === nodeId && state.selectedInstanceKey && keys.includes(state.selectedInstanceKey)) {
      return state.selectedInstanceKey;
    }
    // Otherwise first editable, else first
    for (const k of keys) {
      const inf = this.index.get(k);
      if (inf && inf.editable) return k;
    }
    return keys[0];
  }

  // Set of nodeIds that are repeated via Collection (more than one instanceKey)
  isRepeatedNode(nodeId) {
    const keys = this.nodeToKeys.get(nodeId) || [];
    return keys.length > 1;
  }

  renderOverlays() {
    if (!this.overlay || !this.iframe.contentDocument) return;
    const doc = this.iframe.contentDocument;
    const iframeRect = this.iframe.getBoundingClientRect();
    // Clear
    this.overlay.replaceChildren();
    // overlay itself is pointer-events:none via CSS; children manage themselves
    // Hover outline (thin) – only for representative
    if (this.hoverKey && this.index.has(this.hoverKey) && this.hoverKey !== this.selectedKey) {
      const info = this.index.get(this.hoverKey);
      // For repeated nodes, only show hover for representative
      const rep = this.representativeKeyFor(info.nodeId);
      if (rep === this.hoverKey || !this.isRepeatedNode(info.nodeId)) {
        this.renderRects(info, "hover", doc, iframeRect);
        const label = this.labelFor(info);
        this.renderLabel(info, label, "hover", doc, iframeRect);
      } else {
        // Hovered non-representative occurrence: show very subtle related highlight
        this.renderRects(info, "related", doc, iframeRect);
      }
    }
    // Selection outline (strong) + toolbar
    if (this.selectedKey && this.index.has(this.selectedKey)) {
      const info = this.index.get(this.selectedKey);
      this.renderRects(info, "selected", doc, iframeRect);
      const label = this.labelFor(info);
      this.renderLabel(info, label, "selected", doc, iframeRect);
      this.renderToolbar(info, doc, iframeRect);
      // Also subtle highlight other occurrences of same nodeId (related)
      const keys = this.nodeToKeys.get(info.nodeId) || [];
      for (const k of keys) {
        if (k === this.selectedKey) continue;
        const other = this.index.get(k);
        if (!other) continue;
        this.renderRects(other, "related", doc, iframeRect);
      }
    } else if (state.selectedNodeId && !this.selectedKey) {
      // No instance yet (maybe empty block) — try representative
      const rep = this.representativeKeyFor(state.selectedNodeId);
      if (rep && this.index.has(rep)) {
        this.selectedKey = rep;
        const info = this.index.get(rep);
        if (info) {
          this.renderRects(info, "selected", doc, iframeRect);
          this.renderLabel(info, this.labelFor(info), "selected", doc, iframeRect);
          this.renderToolbar(info, doc, iframeRect);
        }
      }
    }
    // External boundary indicator
    if (this.selectedKey && this.index.has(this.selectedKey)) {
      const info = this.index.get(this.selectedKey);
      if (!info.editable) {
        this.renderExternalNotice(info, doc, iframeRect);
      }
    }
    // Insertion affordances & empty containers – contextual, legal only
    this.renderInsertionAffordances(doc, iframeRect);
    this.renderEmptyPlaceholders(doc, iframeRect);
  }

  // Resolve representative rect for a nodeId (for insertion lane positioning)
  representativeRect(nodeId) {
    const rep = this.representativeKeyFor(nodeId);
    if (!rep) return null;
    return this.boundsForInfo(this.index.get(rep));
  }

  createLane({ parentId, index, top, left, width, legal, reason }) {
    const lane = document.createElement("div");
    lane.className = "canvas-insertion-lane" + (legal ? "" : " canvas-insertion-lane--disabled");
    lane.tabIndex = legal ? 0 : -1;
    lane.setAttribute("role", "button");
    lane.dataset.parentId = parentId || "root";
    lane.dataset.index = String(index);
    const label = legal ? `Add block here` : (reason || "Cannot add here");
    lane.setAttribute("aria-label", label);
    lane.title = label;
    lane.style.position = "absolute";
    lane.style.left = (left || 8) + "px";
    lane.style.top = (top - 16) + "px";
    lane.style.width = Math.max(80, width || 260) + "px";
    lane.style.height = "32px";
    const plus = document.createElement("button");
    plus.type = "button";
    plus.className = "canvas-insertion-plus";
    plus.textContent = "+";
    plus.tabIndex = -1;
    plus.style.pointerEvents = "none";
    lane.append(plus);
    if (legal) {
      lane.addEventListener("click", (e) => {
        e.preventDefault(); e.stopPropagation();
        if (window.__stratum_setInsertionTarget) window.__stratum_setInsertionTarget(parentId, index);
        document.querySelectorAll("[data-library-tab]").forEach(t => { if ((t.getAttribute('data-library-tab')||t.dataset.tab)==="blocks") t.click(); });
        if (window.__stratum_renderCatalog) window.__stratum_renderCatalog();
      });
      lane.addEventListener("keydown", (e) => {
        if (e.key === "Enter" || e.key === " ") { e.preventDefault(); lane.click(); }
      });
    } else {
      lane.addEventListener("click", (e) => {
        e.preventDefault(); e.stopPropagation();
        if (reason && window.stratumToast) window.stratumToast("error", reason);
      });
    }
    return lane;
  }

  renderInsertionAffordances(doc, iframeRect) {
    const rootNodes = state.document.nodes || [];
    // Empty document: centered CTA
    if (!rootNodes.length) {
      const scW = this.scroller ? this.scroller.clientWidth : 600;
      const box = document.createElement("div");
      box.className = "canvas-empty-document";
      box.style.position = "absolute";
      box.style.left = Math.max(16, (scW - 320)/2) + "px";
      box.style.top = "40px";
      box.style.width = "320px";
      box.style.minHeight = "120px";
      box.style.border = "1px dashed #cbd5e1";
      box.style.borderRadius = "8px";
      box.style.background = "#f8fafc";
      box.style.display = "flex";
      box.style.flexDirection = "column";
      box.style.alignItems = "center";
      box.style.justifyContent = "center";
      box.style.gap = "8px";
      box.style.padding = "16px";
      box.style.pointerEvents = "auto";
      const title = document.createElement("div");
      title.textContent = "Start building this page";
      title.style.fontWeight = "600";
      title.style.fontSize = "14px";
      const sub = document.createElement("div");
      sub.textContent = "Add your first block to get started.";
      sub.style.fontSize = "12px";
      sub.style.color = "#64748b";
      const actions = document.createElement("div");
      actions.style.display = "flex";
      actions.style.gap = "8px";
      const addBtn = document.createElement("button");
      addBtn.type = "button";
      addBtn.className = "button button-primary";
      addBtn.textContent = "+ Add block";
      addBtn.style.padding = "6px 12px";
      addBtn.addEventListener("click", (e)=>{
        e.preventDefault();
        if (window.__stratum_setInsertionTarget) window.__stratum_setInsertionTarget(null, 0);
        document.querySelectorAll("[data-library-tab]").forEach(t=>{ if((t.getAttribute('data-library-tab')||t.dataset.tab)==="blocks") t.click(); });
        if (window.__stratum_renderCatalog) window.__stratum_renderCatalog();
      });
      const patBtn = document.createElement("button");
      patBtn.type = "button";
      patBtn.className = "button";
      patBtn.textContent = "Browse patterns";
      patBtn.addEventListener("click", ()=>{
        document.querySelectorAll("[data-library-tab]").forEach(t=>{ if((t.getAttribute('data-library-tab')||t.dataset.tab)==="patterns") t.click(); });
      });
      actions.append(addBtn, patBtn);
      box.append(title, sub, actions);
      this.overlay.append(box);
      return;
    }

    // Determine contextual target: which container's boundaries to show?
    // Default: 0 lanes. Show only for relevant context.
    let contextParentId = null;
    let contextIndex = null;
    let showMode = "none"; // none | hover | selected

    // If insertionTarget is set, highlight its lane
    if (state.insertionTarget) {
      contextParentId = state.insertionTarget.parentId;
      contextIndex = state.insertionTarget.index;
      showMode = "selected";
    } else if (this.hoverKey && this.index.has(this.hoverKey)) {
      const hoverInfo = this.index.get(this.hoverKey);
      const found = window.__stratum_findNode ? window.__stratum_findNode(hoverInfo.nodeId) : null;
      if (found) {
        // For leaf: lane belongs to its parent, near leaf boundary
        const isCont = window.__stratum_isContainer ? window.__stratum_isContainer(found.node) : false;
        if (isCont) {
          contextParentId = found.node.id;
          showMode = "hover";
        } else {
          // Hovering leaf – show lane before/after leaf within parent
          contextParentId = found.parent ? found.parent.id : null;
          showMode = "hover";
        }
      }
    } else if (this.selectedKey && this.index.has(this.selectedKey)) {
      const selInfo = this.index.get(this.selectedKey);
      const found = window.__stratum_findNode ? window.__stratum_findNode(selInfo.nodeId) : null;
      if (found) {
        const isCont = window.__stratum_isContainer ? window.__stratum_isContainer(found.node) : false;
        if (isCont && hasLegalInsertion(found.node, found.node.children.length)) {
          contextParentId = found.node.id;
          showMode = "selected";
        } else if (found.parent) {
          contextParentId = found.parent.id;
          showMode = "selected";
        } else {
          contextParentId = null;
          showMode = "selected";
        }
      }
    } else if (state.selectedNodeId) {
      const found = window.__stratum_findNode ? window.__stratum_findNode(state.selectedNodeId) : null;
      if (found) {
        const isCont = window.__stratum_isContainer ? window.__stratum_isContainer(found.node) : false;
        if (isCont && hasLegalInsertion(found.node, found.node.children.length)) {
          contextParentId = found.node.id;
          showMode = "selected";
        } else if (found.parent) {
          contextParentId = found.parent.id;
          showMode = "selected";
        } else {
          contextParentId = null;
          showMode = "selected";
        }
      }
    }

    if (showMode === "none" || contextParentId === undefined) return;

    // Resolve parent node object
    let parentNode = null;
    if (contextParentId !== null) {
      const pf = window.__stratum_findNode ? window.__stratum_findNode(contextParentId) : null;
      parentNode = pf ? pf.node : null;
      if (!parentNode) return;
    }

    // Only render lanes for this single parent (contextual)
    const siblings = parentNode ? parentNode.children : rootNodes;
    const legal = hasLegalInsertion(parentNode, 0); // quick guard – if root/container illegal, still show disabled lane with reason?
    // Collect boundaries for this parent only
    const boundaries = [];
    for (let i = 0; i <= siblings.length; i++) {
      const ok = hasLegalInsertion(parentNode, i);
      let reason = "";
      if (!ok) {
        if (parentNode) {
          const def = window.__stratum_definitionFor ? window.__stratum_definitionFor(parentNode) : null;
          if (def) {
            const rule = def.schema.children;
            if (rule.mode === "none") reason = `${def.displayName} does not allow child blocks.`;
            else if (rule.max != null && siblings.length >= rule.max) reason = `${def.displayName} allows at most ${rule.max} child blocks.`;
            else if (rule.mode === "allowed" && rule.blocks && rule.blocks.length) reason = `${def.displayName} allows only ${rule.blocks.length} block type(s).`;
            else reason = "Cannot add here.";
          }
        } else {
          reason = "Cannot add here.";
        }
      }
      boundaries.push({ parentId: contextParentId, index: i, legal: ok, reason });
    }

    // For selected/hover context, show before-hover and after-hover logic for leaves vs general
    // For simplicity in this pass: show lane(s) that are hover-adjacent or all for selected container.
    // Decide which indices to actually render:
    let indicesToShow = [];
    if (showMode === "hover" && this.hoverKey) {
      const hoverInfo = this.index.get(this.hoverKey);
      const hoverFound = window.__stratum_findNode ? window.__stratum_findNode(hoverInfo.nodeId) : null;
      if (hoverFound) {
        const isCont = window.__stratum_isContainer ? window.__stratum_isContainer(hoverFound.node) : false;
        if (isCont && hoverFound.node.id === contextParentId) {
          // Hovering container itself – show inside? For non-empty container show lane at bottom (index = len)
          // and also maybe between? Keep single "inside at end" for clarity
          indicesToShow = [siblings.length];
        } else {
          // Hovering a child/leaf – show before and after that child within its parent
          // But contextParentId is its parent, so find child's index
          const idx = hoverFound.index;
          if (typeof idx === "number") {
            // Determine if near top or bottom edge – use vertical split 40%
            const hoverRect = this.boundsForInfo(hoverInfo);
            let y = null;
            // Approximate cursor is centered on rect; always show both before/after as two lanes
            // Instead show single lane closest to cursor: we use hoverRect top/bottom heuristic
            // For now show both lanes for that sibling gap
            indicesToShow = [idx, idx+1];
          } else {
            indicesToShow = boundaries.map(b=>b.index);
          }
        }
      }
    } else if (showMode === "selected") {
      // For selected container: show lane at end (inside) + maybe before/after if contextual
      if (parentNode && window.__stratum_isContainer && window.__stratum_isContainer(parentNode)) {
        // If selected node itself is this container, show only inside-end lane
        if (state.selectedNodeId === contextParentId) {
          indicesToShow = [siblings.length];
        } else {
          // Selected is child – show before/after selected child
          const selFound = window.__stratum_findNode ? window.__stratum_findNode(state.selectedNodeId) : null;
          if (selFound && selFound.parent && selFound.parent.id === contextParentId) {
            indicesToShow = [selFound.index, selFound.index+1];
          } else {
            indicesToShow = [siblings.length];
          }
        }
      } else {
        // Root selected
        const selFound = state.selectedNodeId ? (window.__stratum_findNode ? window.__stratum_findNode(state.selectedNodeId) : null) : null;
        if (selFound && !selFound.parent) {
          indicesToShow = [selFound.index, selFound.index+1];
        } else {
          indicesToShow = boundaries.map(b=>b.index);
        }
      }
      // If insertionTarget set, highlight only that index plus ensure it's visible
      if (state.insertionTarget && state.insertionTarget.parentId === contextParentId) {
        if (!indicesToShow.includes(state.insertionTarget.index)) indicesToShow.push(state.insertionTarget.index);
      }
    }

    // De-duplicate and clamp
    indicesToShow = [...new Set(indicesToShow)].filter(i => i>=0 && i<=siblings.length).sort((a,b)=>a-b);
    // Limit to max 2 lanes for hover, 3 for selected to avoid christmas tree
    if (showMode === "hover") indicesToShow = indicesToShow.slice(0,2);
    if (showMode === "selected") indicesToShow = indicesToShow.slice(0,3);

    for (const idx of indicesToShow) {
      const b = boundaries.find(x=>x.index===idx);
      if (!b) continue;
      // Compute lane geometry from neighboring rects (use representative)
      const beforeRect = idx>0 ? this.representativeRect(siblings[idx-1]?.id) : null;
      const afterRect = idx < siblings.length ? this.representativeRect(siblings[idx]?.id) : null;
      let top = 8;
      let left = 16;
      let width = this.iframe ? this.iframe.clientWidth - 32 : 300;
      if (beforeRect && afterRect) {
        top = (beforeRect.bottom + afterRect.top)/2;
        left = Math.min(beforeRect.left, afterRect.left);
        width = Math.max(beforeRect.width, afterRect.width);
      } else if (beforeRect) {
        top = beforeRect.bottom + 6;
        left = beforeRect.left;
        width = beforeRect.width;
      } else if (afterRect) {
        top = afterRect.top - 16;
        left = afterRect.left;
        width = afterRect.width;
      } else {
        // Parent bounds fallback (e.g. empty container but handled elsewhere)
        const parentRep = parentNode ? this.representativeRect(parentNode.id) : null;
        if (parentRep) {
          top = parentRep.top + parentRep.height/2;
          left = parentRep.left;
          width = parentRep.width;
        } else {
          top = 40 + idx*32;
        }
      }
      // Clamp to scroller viewport so lane stays visible
      const scrollerRect = this.scroller ? this.scroller.getBoundingClientRect() : null;
      // Lane is positioned inside overlay which is inside stage – already matches scroller coords
      const lane = this.createLane({ parentId: contextParentId, index: idx, top, left, width, legal: b.legal, reason: b.reason });
      // Highlight if this is current insertionTarget
      if (state.insertionTarget && state.insertionTarget.parentId === contextParentId && state.insertionTarget.index === idx) {
        lane.style.outline = "2px solid #2563eb";
        lane.style.background = "rgba(37,99,235,0.08)";
        lane.style.borderRadius = "6px";
      }
      this.overlay.append(lane);
    }

    // For empty parent, placeholder handles Add – no lane needed (avoid duplicate)
    // If parent empty and legal, we already have placeholder; skip lane if both null rects
  }

  renderEmptyPlaceholders(doc, iframeRect) {
    // Empty containers (Section, Stack, Grid, Card, Accordion Item, etc.)
    const seenEmpty = new Set();
    for (const info of this.index.values()) {
      if (!info.editable) continue;
      if (seenEmpty.has(info.nodeId)) continue;
      // Dedup Collection repeats – only representative
      if (this.isRepeatedNode(info.nodeId) && info.instanceKey !== this.representativeKeyFor(info.nodeId)) continue;
      if (!info.rects.some(r=>r._empty)) continue;
      let r = info.rects.find(x=>x._empty) || info.rects[0];
      if (!r) continue;
      const found = window.__stratum_findNode ? window.__stratum_findNode(info.nodeId) : null;
      if (!found) continue;
      const isContainer = window.__stratum_isContainer ? window.__stratum_isContainer(found.node) : false;
      if (!isContainer) continue;
      // Skip if node actually has children but rendered empty due to collection zero? handled below
      // Improve placement for collapsed empty rect at 0,0: try parent bounds, previous sibling, or flow position
      let placeLeft = r.left;
      let placeTop = r.top;
      let placeWidth = Math.max(160, r.width);
      let placeHeight = 72;
      const isFallback = r._empty && r.left===0 && r.top===0;
      if(isFallback){
        let parentRect=null;
        try{
          if(found.parent && this.boundsForNode(found.parent.id)) parentRect=this.boundsForNode(found.parent.id);
        }catch(e){}
        // Also try previous sibling bottom
        let siblingRect = null;
        try {
          if (found.parent) {
            for (let i = found.index - 1; i >= 0; i--) {
              const sib = found.siblings[i];
              const br = this.boundsForNode(sib.id);
              if (br) { siblingRect = br; break; }
            }
          } else {
            // root: try previous root node
            const idx = (found.index !== undefined ? found.index : -1);
            if (idx > 0) {
              for (let i = idx - 1; i >= 0; i--) {
                const n = window.__stratum_findNode ? null : null;
              }
            }
          }
        } catch(_) {}
        if(parentRect){
          placeLeft = parentRect.left + 16;
          // Prefer below sibling if exists
          if (siblingRect) {
            placeTop = siblingRect.bottom + 8;
            // clamp inside parent
            if (placeTop + placeHeight > parentRect.bottom - 8) placeTop = parentRect.top + 16;
          } else {
            placeTop = parentRect.top + 16;
            // if parent tall, center
            if (parentRect.height > 120) placeTop = parentRect.top + parentRect.height/2 - 36;
          }
          placeWidth = Math.max(160, Math.min(parentRect.width - 32, 360));
        } else if (siblingRect) {
          placeLeft = siblingRect.left;
          placeTop = siblingRect.bottom + 8;
          placeWidth = Math.max(160, siblingRect.width);
        } else {
          const scW = this.scroller ? this.scroller.clientWidth : (this.iframe ? this.iframe.clientWidth : 600);
          placeLeft = Math.max(16, (scW - 260)/2);
          placeTop = 40;
          placeWidth = 260;
        }
        // also update interactive rect for hit test consistency (centered)
        if(info.interactiveRects && info.interactiveRects[0]){
          info.interactiveRects[0].left = placeLeft;
          info.interactiveRects[0].top = placeTop;
          info.interactiveRects[0].width = placeWidth;
          info.interactiveRects[0].height = placeHeight;
          info.interactiveRects[0].right = placeLeft + placeWidth;
          info.interactiveRects[0].bottom = placeTop + placeHeight;
          info.interactiveRects[0]._empty = true;
        }
        r = {left: placeLeft, top: placeTop, width: placeWidth, height: placeHeight, _empty:true};
      }
      const def = window.__stratum_definitionFor ? window.__stratum_definitionFor(found.node) : null;
      const label = def ? `Empty ${def.displayName}` : `Empty ${info.nodeId.slice(0,8)}`;
      const box = document.createElement("div");
      box.className = "canvas-empty-placeholder";
      box.dataset.nodeId = info.nodeId;
      box.style.position = "absolute";
      box.style.left = placeLeft + "px";
      box.style.top = placeTop + "px";
      box.style.width = placeWidth + "px";
      box.style.minHeight = placeHeight + "px";
      box.style.border = "1px dashed #cbd5e1";
      box.style.borderRadius = "6px";
      box.style.background = "#f8fafc";
      box.style.display = "flex";
      box.style.flexDirection = "column";
      box.style.alignItems = "center";
      box.style.justifyContent = "center";
      box.style.gap = "4px";
      box.style.padding = "8px";
      box.style.cursor = "pointer";
      const span = document.createElement("span");
      span.style.fontSize = "11px";
      span.style.color = "#64748b";
      span.textContent = label;
      const hint = document.createElement("span");
      hint.style.fontSize = "10px";
      hint.style.color = "#94a3b8";
      hint.textContent = "Empty container";
      const btn = document.createElement("button");
      btn.type = "button";
      btn.textContent = "+ Add block";
      btn.style.padding = "4px 10px";
      btn.style.border = "1px solid #cbd5e1";
      btn.style.background = "white";
      btn.style.borderRadius = "14px";
      btn.style.fontSize = "11px";
      btn.style.cursor = "pointer";
      box.append(span, hint, btn);
      box.addEventListener("click", (e)=>{
        e.preventDefault(); e.stopPropagation();
        state.selectedNodeId = info.nodeId;
        state.selectedInstanceKey = info.instanceKey;
        if (window.__stratum_onSelectionChange) window.__stratum_onSelectionChange(info.nodeId, info.instanceKey);
        if (window.__stratum_renderInspector) window.__stratum_renderInspector();
        this.selectNode(info.nodeId, info.instanceKey, { scroll: false });
        this.renderOverlays();
        if (window.__stratum_setInsertionTarget) window.__stratum_setInsertionTarget(info.nodeId, 0);
        document.querySelectorAll("[data-library-tab]").forEach(t=>{ if((t.getAttribute('data-library-tab')||t.dataset.tab)==="blocks") t.click(); });
      });
      btn.addEventListener("click", (e)=>{
        e.stopPropagation();
        if (window.__stratum_setInsertionTarget) window.__stratum_setInsertionTarget(info.nodeId, 0);
        document.querySelectorAll("[data-library-tab]").forEach(t=>{ if((t.getAttribute('data-library-tab')||t.dataset.tab)==="blocks") t.click(); });
      });
      seenEmpty.add(info.nodeId);
      this.overlay.append(box);
    }
    // Empty leaf placeholders (Heading without text, Text without text, Button without label, Image without media)
    const seenLeaf = new Set();
    for (const info of this.index.values()) {
      if (!info.editable) continue;
      if (seenLeaf.has(info.nodeId)) continue;
      if (this.isRepeatedNode(info.nodeId) && info.instanceKey !== this.representativeKeyFor(info.nodeId)) continue;
      const found = window.__stratum_findNode ? window.__stratum_findNode(info.nodeId) : null;
      if (!found) continue;
      const isContainer = window.__stratum_isContainer ? window.__stratum_isContainer(found.node) : false;
      if (isContainer) continue;
      // Detect empty leaf: check props emptiness
      const node = found.node;
      let isEmptyLeaf = false;
      let label = "";
      // Use _empty flag or explicit prop check
      const hasEmptyRect = info.rects.some(r=>r._empty) || info.rects.length===0 || info.rects.every(r=> (r.height||0) < 4);
      // Also check props for these block types
      if (node.block === "core/heading") {
        const txt = node.props.text;
        const empty = !txt || (typeof txt === "string" && !txt.trim()) || (txt && txt.version===1 && Array.isArray(txt.content) && txt.content.map(c=>c.text||"").join("").trim()==="");
        if (empty) { isEmptyLeaf = true; label = "Heading — Add text"; }
      } else if (node.block === "core/text") {
        const txt = node.props.text;
        const str = typeof txt === "string" ? txt : (txt && txt.version===1 && Array.isArray(txt.content) ? txt.content.map(c=>c.text||"").join("") : "");
        if (!str || !str.trim()) { isEmptyLeaf = true; label = "Text — Start typing"; }
      } else if (node.block === "core/button") {
        if (!node.props.label || !String(node.props.label).trim()) { isEmptyLeaf = true; label = "Button — Add label"; }
      } else if (node.block === "core/image") {
        if (!node.props.mediaId) { isEmptyLeaf = true; label = "Image — Choose image"; }
      } else if (node.block === "core/accordion-item") {
        // Item title may be default "Item" -> not empty, but body empty container already handled via container placeholder?
        // Leaf placeholder for item with no children already covered; skip generic
        continue;
      } else {
        // Generic leaf empty if height tiny
        if (hasEmptyRect) { isEmptyLeaf = true; const def = window.__stratum_definitionFor ? window.__stratum_definitionFor(node) : null; label = def ? `${def.displayName} — Empty` : `${node.block} — Empty`; }
      }
      if (!isEmptyLeaf) continue;
      // Determine placement: use existing rect if not 0,0 else fallback near parent
      let r = info.rects.find(x=>!x._empty) || info.rects.find(x=>x._empty) || info.rects[0];
      let placeLeft = r ? r.left : 0;
      let placeTop = r ? r.top : 0;
      let placeWidth = r ? Math.max(120, r.width) : 200;
      let placeHeight = 44;
      const isFallback = !r || (r._empty && r.left===0 && r.top===0) || (r && r.height < 4);
      if (isFallback) {
        let parentRect=null;
        try{ if(found.parent && this.boundsForNode(found.parent.id)) parentRect=this.boundsForNode(found.parent.id); }catch(_){}
        if(parentRect){
          placeLeft = parentRect.left + 12;
          placeTop = parentRect.top + 12;
          // try to offset by index to avoid stacking
          if (found.index !== undefined) placeTop += found.index * 48;
          placeWidth = Math.max(180, Math.min(parentRect.width - 24, 320));
        } else {
          const scW = this.scroller ? this.scroller.clientWidth : 600;
          placeLeft = Math.max(12, (scW - 220)/2);
          placeTop = 40 + (found.index||0)*48;
          placeWidth = 220;
        }
        if (info.interactiveRects && info.interactiveRects[0]) {
          info.interactiveRects[0].left = placeLeft;
          info.interactiveRects[0].top = placeTop;
          info.interactiveRects[0].width = placeWidth;
          info.interactiveRects[0].height = placeHeight;
          info.interactiveRects[0].right = placeLeft + placeWidth;
          info.interactiveRects[0].bottom = placeTop + placeHeight;
        }
      }
      const box = document.createElement("div");
      box.className = "canvas-leaf-placeholder";
      box.dataset.nodeId = info.nodeId;
      box.style.position = "absolute";
      box.style.left = placeLeft + "px";
      box.style.top = placeTop + "px";
      box.style.width = placeWidth + "px";
      box.style.minHeight = placeHeight + "px";
      box.style.border = "1px dashed #cbd5e1";
      box.style.borderRadius = "6px";
      box.style.background = "#f8fafc";
      box.style.display = "flex";
      box.style.alignItems = "center";
      box.style.justifyContent = "center";
      box.style.padding = "6px 10px";
      box.style.cursor = "pointer";
      box.style.fontSize = "11px";
      box.style.color = "#94a3b8";
      box.textContent = label;
      box.title = "Click to edit";
      box.addEventListener("click", (e)=>{
        e.preventDefault(); e.stopPropagation();
        state.selectedNodeId = info.nodeId;
        state.selectedInstanceKey = info.instanceKey;
        if (window.__stratum_onSelectionChange) window.__stratum_onSelectionChange(info.nodeId, info.instanceKey);
        if (window.__stratum_renderInspector) window.__stratum_renderInspector();
        this.selectNode(info.nodeId, info.instanceKey, { scroll: false });
        this.renderOverlays();
      });
      seenLeaf.add(info.nodeId);
      this.overlay.append(box);
    }
    // Collection empty state (has SDT children but no rendered child markers)
    const seenColl = new Set();
    for (const info of this.index.values()) {
      if (!info.editable) continue;
      if (seenColl.has(info.nodeId)) continue;
      if (this.isRepeatedNode(info.nodeId) && info.instanceKey !== this.representativeKeyFor(info.nodeId)) continue;
      const found = window.__stratum_findNode ? window.__stratum_findNode(info.nodeId) : null;
      if (!found) continue;
      if (found.node.block !== "core/collection") continue;
      if (!found.node.children || !found.node.children.length) continue;
      // Check if any child has a marker
      let hasRenderedChildren = false;
      for (const child of found.node.children) {
        if (this.nodeToKeys.get(child.id) && this.nodeToKeys.get(child.id).length) { hasRenderedChildren = true; break; }
        // also check nested?
        const checkNested = (nodes) => {
          for (const n of nodes) {
            if (this.nodeToKeys.get(n.id)?.length) return true;
            if (n.children && checkNested(n.children)) return true;
          }
          return false;
        };
        if (checkNested(found.node.children)) { hasRenderedChildren=true; break; }
      }
      if (hasRenderedChildren) continue;
      const rect = this.boundsForInfo(info);
      if (!rect) continue;
      const banner = document.createElement("div");
      banner.className = "canvas-collection-empty";
      banner.style.position = "absolute";
      banner.style.left = (rect.left) + "px";
      banner.style.top = (rect.top + rect.height + 4) + "px";
      banner.style.background = "#fffbeb";
      banner.style.border = "1px solid #fde68a";
      banner.style.borderRadius = "6px";
      banner.style.padding = "10px 12px";
      banner.style.fontSize = "12px";
      banner.style.color = "#92400e";
      banner.style.maxWidth = Math.max(220, rect.width) + "px";
      banner.style.boxShadow = "0 1px 4px rgba(0,0,0,0.08)";
      banner.style.display = "grid";
      banner.style.gap = "4px";
      const title = document.createElement("div");
      title.style.fontWeight = "600";
      title.textContent = "Collection";
      const b1 = document.createElement("div");
      b1.textContent = "No matching entries";
      const b2 = document.createElement("div");
      b2.style.fontSize = "11px";
      b2.style.color = "#a16207";
      b2.textContent = "The item layout can still be edited.";
      const btn = document.createElement("button");
      btn.type = "button";
      btn.textContent = "Edit item layout";
      btn.style.padding = "6px 10px";
      btn.style.fontSize = "11px";
      btn.style.border = "1px solid #fde68a";
      btn.style.background = "white";
      btn.style.borderRadius = "14px";
      btn.style.cursor = "pointer";
      btn.style.justifySelf = "start";
      btn.addEventListener("click", (e)=>{
        e.preventDefault(); e.stopPropagation();
        // Switch left panel to Navigator, expand Collection, select first child
        document.querySelectorAll("[data-library-tab]").forEach(t=>{ if((t.getAttribute('data-library-tab')||t.dataset.tab)==="navigator") t.click(); });
        // Expand ancestors
        try {
          if (state.collapsed.has(info.nodeId)) state.collapsed.delete(info.nodeId);
          if (window.__stratum_renderNavigator) window.__stratum_renderNavigator();
          // Select collection or first child
          const firstChild = found.node.children[0];
          const targetId = firstChild ? firstChild.id : info.nodeId;
          state.selectedNodeId = targetId;
          if (window.__stratum_onSelectionChange) window.__stratum_onSelectionChange(targetId, null);
          // Scroll navigator to target
          setTimeout(()=>{
            const el = document.querySelector(`[data-node-id="${targetId}"]`);
            if (el) el.scrollIntoView({ block:"nearest", inline:"nearest" });
          }, 50);
          if (window.__stratum_canvasController) window.__stratum_canvasController.selectNode(targetId, null, { scroll: false });
          if (window.__stratum_renderInspector) window.__stratum_renderInspector();
        } catch(_) {}
      });
      banner.append(title, b1, b2, btn);
      seenColl.add(info.nodeId);
      this.overlay.append(banner);
    }
  }

  renderRects(info, kind, doc, iframeRect) {
    for (const r of info.rects) {
      if (r._empty && kind !== "selected") continue;
      const div = document.createElement("div");
      div.className = `canvas-outline canvas-outline--${kind} ${info.editable ? "" : "is-external"}`;
      // r is relative to iframe viewport, need to convert to overlay coords (which is same as iframeRect)
      // r.left/top are viewport coords inside iframe (relative to iframe content viewport origin)
      // overlay covers iframe, so position = r.left/top
      // But need to account for iframe scroll? getClientRects already viewport-relative.
      div.style.position = "absolute";
      div.style.left = (r.left) + "px";
      div.style.top = (r.top) + "px";
      div.style.width = (r.width || 100) + "px";
      div.style.height = (r.height || 24) + "px";
      div.style.pointerEvents = "none";
      div.style.boxSizing = "border-box";
      if (kind === "hover") {
        div.style.border = "1px dashed #3b82f6";
        div.style.background = "rgba(59,130,246,0.06)";
      } else if (kind === "selected") {
        div.style.border = info.editable ? "2px solid #2563eb" : "2px dashed #f59e0b";
        div.style.background = info.editable ? "rgba(37,99,235,0.06)" : "rgba(245,158,11,0.08)";
      } else if (kind === "related") {
        div.style.border = "1px dotted #93c5fd";
        div.style.background = "rgba(147,197,253,0.08)";
      }
      this.overlay.append(div);
    }
  }

  renderLabel(info, label, kind, doc, iframeRect) {
    if (!info.rects.length) return;
    const first = info.rects[0];
    const div = document.createElement("div");
    div.className = `canvas-label canvas-label--${kind}`;
    div.textContent = label;
    div.style.position = "absolute";
    div.style.left = (first.left) + "px";
    div.style.top = Math.max(0, first.top - 18) + "px";
    div.style.fontSize = "10px";
    div.style.padding = "1px 4px";
    div.style.borderRadius = "3px";
    div.style.pointerEvents = "none";
    div.style.fontFamily = "monospace";
    if (kind === "selected") {
      div.style.background = info.editable ? "#2563eb" : "#f59e0b";
      div.style.color = "white";
    } else {
      div.style.background = "#e0f2fe";
      div.style.color = "#1e40af";
    }
    this.overlay.append(div);
  }

  renderToolbar(info, doc, iframeRect) {
    if (!info.editable) return;
    const first = info.rects[0];
    if (!first) return;
    const bar = document.createElement("div");
    bar.className = "canvas-toolbar";
    // Clamp to scroller viewport
    const scrollerRect = this.scroller ? this.scroller.getBoundingClientRect() : { left:0, right:window.innerWidth, top:0, bottom:window.innerHeight };
    const barW = 220; // approx
    const barH = 34;
    let left = first.left;
    let top = first.top - barH - 6;
    if (left + barW > scrollerRect.right - 8) left = scrollerRect.right - barW - 8;
    if (left < scrollerRect.left + 8) left = scrollerRect.left + 8;
    if (top < scrollerRect.top + 8) top = first.bottom + 6;
    if (top + barH > scrollerRect.bottom - 8) top = scrollerRect.bottom - barH - 8;
    bar.style.position = "absolute";
    bar.style.left = (left) + "px";
    bar.style.top = Math.max(0, top) + "px";
    bar.style.display = "flex";
    bar.style.gap = "4px";
    bar.style.background = "white";
    bar.style.border = "1px solid #e5e7eb";
    bar.style.borderRadius = "8px";
    bar.style.padding = "2px 4px";
    bar.style.boxShadow = "0 4px 12px rgba(0,0,0,0.12)";
    bar.style.fontSize = "11px";
    let inlineNode = null;
    try {
      inlineNode = window.__stratum_findNode ? window.__stratum_findNode(info.nodeId)?.node : null;
    } catch (_) {}
    // Check constraints for toolbar
    let canDup = {ok:true, reason:""}, canDel = {ok:true, reason:""};
    try {
      if (window.__stratum_canDuplicate) canDup = window.__stratum_canDuplicate(info.nodeId);
      if (window.__stratum_canRemove) canDel = window.__stratum_canRemove(info.nodeId);
    } catch (_) {}
    // Preferred toolbar: [Drag] [↑] [↓] [⋯] [+] — Duplicate/Delete inside ⋯
    const actions = [
      ...(inlineNode && isInlineEditable(inlineNode) ? [{ label: "Edit", title: "Edit text", action: "edit" }] : []),
      { label: "Drag", title: "Drag", action: "drag" },
      { label: "↑", title: "Move up", action: "up" },
      { label: "↓", title: "Move down", action: "down" },
      { label: "⋯", title: "More actions", action: "more" },
    ];
    actions.forEach(a => {
      const btn = document.createElement("button");
      btn.type = "button";
      btn.textContent = a.label;
      btn.title = a.title;
      btn.style.padding = "2px 6px";
      btn.style.border = "1px solid #e5e7eb";
      btn.style.borderRadius = "4px";
      btn.style.background = "#f8fafc";
      btn.style.cursor = a.disabled ? "not-allowed" : "pointer";
      if (a.disabled) { btn.disabled = true; btn.style.opacity = "0.45"; btn.style.background = "#f1f5f9"; }
      if (a.action === "drag") {
        btn.draggable = true;
        btn.setAttribute("aria-label", "Drag block");
        btn.addEventListener("dragstart", (e) => {
          window.__stratum_currentDrag = { type: "node", nodeId: info.nodeId };
          e.dataTransfer.effectAllowed = "move";
          e.dataTransfer.setData("text/plain", info.nodeId);
        });
        btn.addEventListener("dragend", () => {
          window.__stratum_currentDrag = null;
          this.clearDropIndicator();
          if (this.wrap) this.wrap.classList.remove("canvas--dragover");
        });
      }
      btn.addEventListener("click", (e) => {
        e.preventDefault();
        e.stopPropagation();
        if (a.disabled) {
          if (window.stratumToast) window.stratumToast("error", a.reason || "Action not allowed");
          return;
        }
        if (a.action === "edit") startInlineEdit(info.nodeId, info.instanceKey, this);
        if (a.action === "up" && window.__stratum_moveNode) window.__stratum_moveNode(info.nodeId, -1);
        if (a.action === "down" && window.__stratum_moveNode) window.__stratum_moveNode(info.nodeId, 1);
        if (a.action === "duplicate" && window.__stratum_duplicateNode) window.__stratum_duplicateNode(info.nodeId);
        if (a.action === "delete" && window.__stratum_removeNode) window.__stratum_removeNode(info.nodeId);
        if (a.action === "more") {
          try { openContextMenu({ anchor: btn, nodeId: info.nodeId }); } catch(_) { if (window.__stratum_openContextMenu) window.__stratum_openContextMenu({ anchor: btn, nodeId: info.nodeId }); }
        }
      });
      bar.append(btn);
    });
    // Add inside for containers
    try {
      const def = window.__stratum_definitionFor ? window.__stratum_definitionFor({ block: "", version: 0 }) : null;
    } catch (_) {}
    // Check if container -> Add inside with validation (use hasLegalInsertion)
    if (info.nodeId && window.__stratum_isContainer) {
      try {
        const found = window.__stratum_findNode ? window.__stratum_findNode(info.nodeId) : null;
        if (found && window.__stratum_isContainer(found.node)) {
          let canAdd = hasLegalInsertion(found.node, found.node.children.length);
          let reason = "";
          if (!canAdd) {
            const def = window.__stratum_definitionFor ? window.__stratum_definitionFor(found.node) : null;
            if (def) {
              const rule = def.schema.children;
              if (rule.mode==="none") reason="Cannot contain blocks";
              else if (rule.max!=null && found.node.children.length >= rule.max) reason=`Maximum ${rule.max} reached`;
              else if (rule.mode==="allowed" && rule.blocks) reason=`${def.displayName} allows only ${rule.blocks.length} type(s).`;
              else reason="Cannot add inside";
            } else reason="Cannot add inside";
          }
          const addBtn = document.createElement("button");
          addBtn.type = "button";
          addBtn.textContent = "+";
          addBtn.title = canAdd ? "Add inside" : reason;
          addBtn.disabled = !canAdd;
          addBtn.style.padding = "2px 6px";
          addBtn.style.border = "1px solid #e5e7eb";
          addBtn.style.borderRadius = "4px";
          addBtn.style.background = canAdd ? "#f8fafc" : "#f1f5f9";
          addBtn.style.opacity = canAdd ? "1" : "0.45";
          addBtn.style.cursor = canAdd ? "pointer" : "not-allowed";
          addBtn.addEventListener("click", (e) => {
            e.preventDefault(); e.stopPropagation();
            if (!canAdd) {
              if (window.stratumToast) window.stratumToast("error", reason);
              return;
            }
            if (window.__stratum_setInsertionTarget) window.__stratum_setInsertionTarget(found.node.id, found.node.children.length);
            document.querySelectorAll("[data-library-tab]").forEach(t=>{ if((t.getAttribute('data-library-tab')||t.dataset.tab)==="blocks") t.click(); });
            const lib = document.querySelector(".block-library") || document.getElementById("block-catalog");
            if (lib) lib.scrollIntoView({ behavior: "smooth" });
          });
          bar.append(addBtn);
        }
      } catch (_) {}
    }
    this.overlay.append(bar);
  }

  renderExternalNotice(info, doc, iframeRect) {
    const first = info.rects[0];
    if (!first) return;
    const notice = document.createElement("div");
    notice.className = "canvas-external-notice";
    notice.style.position = "absolute";
    notice.style.left = (first.left) + "px";
    notice.style.top = (first.top + (first.height || 24) + 4) + "px";
    notice.style.background = "#fffbeb";
    notice.style.border = "1px solid #f59e0b";
    notice.style.borderRadius = "6px";
    notice.style.padding = "6px 8px";
    notice.style.fontSize = "12px";
    notice.style.maxWidth = "320px";
    notice.style.boxShadow = "0 2px 8px rgba(0,0,0,0.12)";
    let label = info.ownerLabel || "";
    // Fallback lookup in bootstrap siteParts
    if (!label && info.ownerId) {
      try {
        const bs = document.getElementById("editor-bootstrap") ? JSON.parse(document.getElementById("editor-bootstrap").textContent) : bootstrap;
        const parts = bs.siteParts || bs.SiteParts || [];
        if (Array.isArray(parts)) {
          const found = parts.find(p=>p.id===info.ownerId || p.ID===info.ownerId);
          if (found) label = found.name || found.Name || "";
        } else if (parts && typeof parts === "object") {
          for (const k in parts) if (k===info.ownerId) label = parts[k];
        }
        // layout template name fallback
        if (!label && info.ownerType==="layout-template" && bs.resource && bs.resource.id===info.ownerId) label = bs.resource.label || "";
      } catch (_) {}
    }
    if (!label) label = info.ownerId ? info.ownerId.slice(0,8) : "";
    const typeLabel = info.ownerType === "site-part" ? "Site Part" : info.ownerType === "layout-template" ? "Template" : info.ownerType || "External";
    const title = info.ownerLabel ? `${info.ownerLabel}` : (label ? `${label}` : (info.ownerType ? `${typeLabel} ${info.ownerId.slice(0,8)}` : "External content"));
    const effectiveLabel = info.ownerLabel || label;
    const sub = info.ownerType ? `Global content — changes affect the whole website.` : `This content is read-only here.`;
    const strong = document.createElement("strong");
    strong.style.color = "#b45309";
    strong.textContent = title;
    const spanSub = document.createElement("span");
    spanSub.style.color = "#92400e";
    spanSub.textContent = sub;
    const meta = document.createElement("span");
    meta.style.color = "#92400e";
    meta.style.fontSize = "11px";
    meta.textContent = info.ownerType? typeLabel+" • "+(info.ownerId?.slice(0,8) || ""):"";
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "button button-small";
    btn.style.marginTop = "6px";
    btn.style.display = "block";
    btn.textContent = `Edit ${effectiveLabel || typeLabel}`;
    notice.append(strong, document.createElement("br"), spanSub, document.createElement("br"), meta, document.createElement("br"), btn);
    if (btn) {
      btn.addEventListener("click", () => {
        // Navigate to owner editor
        let url = "";
        if (info.ownerType === "site-part" || info.ownerType === "sitepart") {
          url = `/admin/appearance/site-parts/${info.ownerId}/edit`;
        } else if (info.ownerType === "layout-template") {
          url = `/admin/appearance/templates/${info.ownerId}/edit`;
        } else if (info.ownerType === "entry") {
          // try to infer content type from bootstrap? fallback to page
          url = `/admin/pages/${info.ownerId}/edit`;
        }
        if (url) window.location.href = url;
        else {
          const forms = bootstrap.forms || [];
          // fallback: show toast
          if (window.stratumToast) window.stratumToast("info", "External content — edit its source.");
        }
      });
    }
    this.overlay.append(notice);
  }

  labelFor(info) {
    try {
      const node = window.__stratum_findNode ? window.__stratum_findNode(info.nodeId)?.node : null;
      if (node && window.__stratum_definitionFor) {
        const def = window.__stratum_definitionFor(node);
        if (def) return def.displayName;
      }
    } catch (_) {}
    return info.nodeId.slice(0, 8);
  }

  updateOverlayPositions() {
    // Recompute rects on scroll/resize by re-reading range rects (they are viewport-relative, need update)
    if (!this.iframe || !this.iframe.contentDocument) return;
    // Update rects for each info
    for (const info of this.index.values()) {
      try {
        const rects = Array.from(info.range.getClientRects());
        if (rects.length) info.rects = rects;
        else {
          const r = info.range.getBoundingClientRect();
          if (r.width || r.height) info.rects = [r];
        }
      } catch (_) {}
    }
    this.renderOverlays();
  }

  onLibraryDragStart(definition) {
    if (this.wrap) this.wrap.classList.add("canvas--drag-ready");
  }
  onLibraryDragEnd() {
    if (this.wrap) this.wrap.classList.remove("canvas--drag-ready", "canvas--dragover");
    this.clearDropIndicator();
  }

  destroy() {
    window.removeEventListener("resize", this._onResize);
  }
}
