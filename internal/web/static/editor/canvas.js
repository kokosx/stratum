// canvas.js — CanvasController for visual editor (marker parsing, Range, geometry, selection)
import { state, bootstrap } from "./state.js";
import { isInlineEditable, startInlineEdit } from "./inline-edit.js";

export class CanvasController {
  constructor(iframe, overlay) {
    this.iframe = iframe;
    this.overlay = overlay || document.getElementById("editor-canvas-overlay");
    this.breadcrumbs = document.getElementById("editor-canvas-breadcrumbs");
    this.wrap = document.getElementById("editor-canvas-wrap");
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
    this.init();
  }

  init() {
    if (!this.iframe) return;
    this.iframe.addEventListener("load", () => {
      setTimeout(() => this.refresh(), 30);
    });
    window.addEventListener("resize", this._onResize);
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
                  else rects = [{ top: 0, left: 0, width: 100, height: 60, _empty: true }];
                } else {
                  rects = [{ top: 0, left: 0, width: 100, height: 60, _empty: true }];
                }
              }
            } catch (_) {}
          }
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
    // Find deepest (smallest area) rect containing point
    let best = null;
    let bestArea = Infinity;
    for (const [key, info] of this.index.entries()) {
      for (const r of info.rects) {
        if (r._empty) continue;
        const left = r.left, top = r.top, right = r.right ?? (r.left + r.width), bottom = r.bottom ?? (r.top + r.height);
        if (clientX >= left && clientX <= right && clientY >= top && clientY <= bottom) {
          const area = (r.width || 0) * (r.height || 0);
          if (area < bestArea) {
            bestArea = area;
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
    if (opts.scroll !== false && instanceKey && this.index.has(instanceKey)) {
      try {
        const info = this.index.get(instanceKey);
        if (info.range) {
          const el = info.range.startContainer;
          // Try to scroll into view inside iframe
          const doc = this.iframe.contentDocument;
          let target = info.startComment.nextSibling;
          while (target && target.nodeType !== 1) target = target.nextSibling;
          if (target && target.scrollIntoView) target.scrollIntoView({ block: "center", behavior: "smooth" });
          else if (info.range.startContainer && info.range.startContainer.scrollIntoView) info.range.startContainer.scrollIntoView({block:"center"});
        }
      } catch (_) {}
    }
    this.renderOverlays();
    // notify breadcrumbs
    if (window.__stratum_updateBreadcrumbs) window.__stratum_updateBreadcrumbs();
  }

  renderOverlays() {
    if (!this.overlay || !this.iframe.contentDocument) return;
    const doc = this.iframe.contentDocument;
    const iframeRect = this.iframe.getBoundingClientRect();
    // Clear
    this.overlay.replaceChildren();
    this.overlay.style.pointerEvents = "auto";
    // Hover outline (thin)
    if (this.hoverKey && this.index.has(this.hoverKey) && this.hoverKey !== this.selectedKey) {
      const info = this.index.get(this.hoverKey);
      this.renderRects(info, "hover", doc, iframeRect);
      // label
      const label = this.labelFor(info);
      this.renderLabel(info, label, "hover", doc, iframeRect);
    }
    // Selection outline (strong) + toolbar
    if (this.selectedKey && this.index.has(this.selectedKey)) {
      const info = this.index.get(this.selectedKey);
      this.renderRects(info, "selected", doc, iframeRect);
      const label = this.labelFor(info);
      this.renderLabel(info, label, "selected", doc, iframeRect);
      this.renderToolbar(info, doc, iframeRect);
      // Also subtle highlight other occurrences of same nodeId
      const keys = this.nodeToKeys.get(info.nodeId) || [];
      for (const k of keys) {
        if (k === this.selectedKey) continue;
        const other = this.index.get(k);
        if (!other) continue;
        this.renderRects(other, "related", doc, iframeRect);
      }
    } else if (state.selectedNodeId && !this.selectedKey) {
      // No instance yet (maybe empty block) — try first occurrence
      const keys = this.nodeToKeys.get(state.selectedNodeId) || [];
      if (keys.length) {
        this.selectedKey = keys[0];
        const info = this.index.get(keys[0]);
        if (info) {
          this.renderRects(info, "selected", doc, iframeRect);
          this.renderLabel(info, this.labelFor(info), "selected", doc, iframeRect);
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
    // Insertion affordances & empty containers
    this.renderInsertionAffordances(doc, iframeRect);
    this.renderEmptyPlaceholders(doc, iframeRect);
  }

  renderInsertionAffordances(doc, iframeRect) {
    // Root affordances: between blocks and before/after
    try {
      const rootNodes = (window.__stratum_findNode ? null : null);
    } catch (_) {}
    // Show subtle + buttons for selected container and for empty containers
    // Iterate over all editable nodes that are containers
    for (const info of this.index.values()) {
      if (!info.editable) continue;
      const nodeId = info.nodeId;
      let found = null;
      try { found = window.__stratum_findNode ? window.__stratum_findNode(nodeId) : null; } catch (_) {}
      if (!found) continue;
      if (!window.__stratum_isContainer || !window.__stratum_isContainer(found.node)) continue;
      // Empty container placeholder is handled separately, but also show + button centered
      const isEmpty = !found.node.children || found.node.children.length===0;
      if (isEmpty) continue; // handled in renderEmptyPlaceholders
      // Show Add button at bottom of container
      const rect = this.boundsForInfo(info);
      if (!rect) continue;
      const def = window.__stratum_definitionFor ? window.__stratum_definitionFor(found.node) : null;
      const canAdd = (() => {
        if (!def) return false;
        const rule = def.schema.children;
        if (rule.mode==="none") return false;
        if (rule.max!=null && found.node.children.length >= rule.max) return false;
        return true;
      })();
      if (!canAdd) continue;
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "canvas-insertion-btn";
      // Label: specific when single allowed type
      let label = "+ Add block";
      if (def && def.schema.children.mode==="allowed" && def.schema.children.blocks.length===1) {
        const childBlock = def.schema.children.blocks[0];
        let childName = childBlock;
        try {
          for (const d of (window.__stratum_catalog||[])) if (d.block===childBlock) { childName = d.displayName; break; }
          if (childName===childBlock && def) {
            // try bootstrap catalog
            const cat = document.getElementById("editor-bootstrap") ? JSON.parse(document.getElementById("editor-bootstrap").textContent).catalog||[] : [];
            for (const d of cat) if (d.block===childBlock) childName = d.displayName;
          }
        } catch (_) {}
        label = `+ Add ${childName}`;
      }
      btn.textContent = label;
      btn.title = label;
      btn.style.position = "absolute";
      btn.style.left = (rect.left + rect.width/2 - 60) + "px";
      btn.style.top = (rect.bottom - 12) + "px";
      btn.style.transform = "translateY(-50%)";
      btn.style.padding = "4px 10px";
      btn.style.fontSize = "11px";
      btn.style.border = "1px solid #cbd5e1";
      btn.style.borderRadius = "14px";
      btn.style.background = "white";
      btn.style.boxShadow = "0 1px 4px rgba(0,0,0,0.12)";
      btn.style.cursor = "pointer";
      btn.style.opacity = "0.88";
      btn.style.zIndex = "5";
      btn.addEventListener("click", (e)=>{
        e.preventDefault(); e.stopPropagation();
        if (window.__stratum_setInsertionTarget) window.__stratum_setInsertionTarget(found.node.id, found.node.children.length);
        // Switch to library blocks tab
        document.querySelectorAll(".library-tab").forEach(t=>{ if(t.dataset.tab==="blocks") t.click(); });
        const lib = document.querySelector(".block-library") || document.getElementById("block-catalog");
        if (lib) lib.scrollIntoView({behavior:"smooth"});
      });
      // Hide when not hovered over container (show on hover)
      btn.style.opacity = "0";
      // Use parent hover via overlay mousemove already handles hoverKey; show only when hovered or selected
      const isHovered = this.hoverKey && this.index.get(this.hoverKey)?.nodeId === nodeId;
      const isSelected = this.selectedKey && this.index.get(this.selectedKey)?.nodeId === nodeId;
      if (isHovered || isSelected) btn.style.opacity = "0.95";
      this.overlay.append(btn);
    }
    // Root insertion: show before first, between, after last when hovering canvas
    // Only show if there is at least one node
    const hasNodes = this.nodeToKeys.size > 0;
    if (hasNodes) {
      // Between logic handled via drop indicator; insertion affordances for root are subtle dots visible on overlay hover
    } else {
      // Empty document handled in editor.js tree, but also show centered CTA in canvas when no nodes
      const canvasRect = this.iframe.getBoundingClientRect();
      // Don't duplicate if tree already shows empty; just ensure canvas shows placeholder
      const isEmptyDoc = (()=>{ try{ const docState = window.__stratum_findNode? null : null; return document.querySelectorAll(".empty-document-state").length===0; } catch(_){return true;} })();
      // We will not render duplicate; tree already handles empty. Canvas placeholder only if needed.
    }
  }

  renderEmptyPlaceholders(doc, iframeRect) {
    for (const info of this.index.values()) {
      if (!info.editable) continue;
      if (!info.rects.some(r=>r._empty)) continue;
      const r = info.rects.find(x=>x._empty) || info.rects[0];
      if (!r) continue;
      const found = window.__stratum_findNode ? window.__stratum_findNode(info.nodeId) : null;
      if (!found || !window.__stratum_isContainer || !window.__stratum_isContainer(found.node)) continue;
      const def = window.__stratum_definitionFor ? window.__stratum_definitionFor(found.node) : null;
      const label = def ? def.displayName : info.nodeId;
      const box = document.createElement("div");
      box.className = "canvas-empty-placeholder";
      box.style.position = "absolute";
      box.style.left = (r.left) + "px";
      box.style.top = (r.top) + "px";
      box.style.width = Math.max(120, r.width) + "px";
      box.style.minHeight = "60px";
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
      const btn = document.createElement("button");
      btn.type = "button";
      btn.textContent = "+ Add block";
      btn.style.padding = "4px 10px";
      btn.style.border = "1px solid #cbd5e1";
      btn.style.background = "white";
      btn.style.borderRadius = "14px";
      btn.style.fontSize = "11px";
      btn.style.cursor = "pointer";
      box.append(span, btn);
      box.addEventListener("click", (e)=>{
        e.preventDefault(); e.stopPropagation();
        if (window.__stratum_setInsertionTarget) window.__stratum_setInsertionTarget(info.nodeId, 0);
        document.querySelectorAll(".library-tab").forEach(t=>{ if(t.dataset.tab==="blocks") t.click(); });
      });
      btn.addEventListener("click", (e)=>{
        e.stopPropagation();
        if (window.__stratum_setInsertionTarget) window.__stratum_setInsertionTarget(info.nodeId, 0);
        document.querySelectorAll(".library-tab").forEach(t=>{ if(t.dataset.tab==="blocks") t.click(); });
      });
      this.overlay.append(box);
    }
    // Collection empty state (has SDT children but no rendered child markers)
    for (const info of this.index.values()) {
      if (!info.editable) continue;
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
      banner.style.padding = "8px 10px";
      banner.style.fontSize = "11px";
      banner.style.color = "#92400e";
      banner.style.maxWidth = Math.max(200, rect.width) + "px";
      banner.style.boxShadow = "0 1px 4px rgba(0,0,0,0.08)";
      const b1 = document.createElement("div");
      b1.textContent = "No entries match this collection.";
      const b2 = document.createElement("span");
      b2.style.fontSize = "11px";
      b2.style.color = "#a16207";
      b2.textContent = "Edit item layout via Navigator";
      banner.append(b1, document.createElement("br"), b2);
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
    bar.style.position = "absolute";
    bar.style.left = (first.left) + "px";
    bar.style.top = Math.max(0, first.top - 28) + "px";
    bar.style.display = "flex";
    bar.style.gap = "4px";
    bar.style.background = "white";
    bar.style.border = "1px solid #e5e7eb";
    bar.style.borderRadius = "6px";
    bar.style.padding = "2px 4px";
    bar.style.boxShadow = "0 2px 8px rgba(0,0,0,0.12)";
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
    const actions = [
      ...(inlineNode && isInlineEditable(inlineNode) ? [{ label: "Edit", title: "Edit text", action: "edit" }] : []),
      { label: "Drag", title: "Drag", action: "drag" },
      { label: "↑", title: "Move up", action: "up" },
      { label: "↓", title: "Move down", action: "down" },
      { label: "⧉", title: canDup.ok ? "Duplicate" : canDup.reason, action: "duplicate", disabled: !canDup.ok, reason: canDup.reason },
      { label: "✕", title: canDel.ok ? "Delete" : canDel.reason, action: "delete", disabled: !canDel.ok, reason: canDel.reason },
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
      });
      bar.append(btn);
    });
    // Add inside for containers
    try {
      const def = window.__stratum_definitionFor ? window.__stratum_definitionFor({ block: "", version: 0 }) : null;
    } catch (_) {}
    // Check if container -> Add inside with validation
    if (info.nodeId && window.__stratum_isContainer) {
      try {
        const found = window.__stratum_findNode ? window.__stratum_findNode(info.nodeId) : null;
        if (found && window.__stratum_isContainer(found.node)) {
          const def = window.__stratum_definitionFor ? window.__stratum_definitionFor(found.node) : null;
          let canAdd = true; let reason = "";
          if (def) {
            const rule = def.schema.children;
            if (rule.mode==="none") { canAdd=false; reason="Cannot contain blocks"; }
            else if (rule.max!=null && found.node.children.length >= rule.max) { canAdd=false; reason=`Maximum ${rule.max} reached`; }
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
            document.querySelectorAll(".library-tab").forEach(t=>{ if(t.dataset.tab==="blocks") t.click(); });
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
