// quick-inserter.js — contextual inserter UI (Shadow DOM, legal-only, search + recent + Browse)
import { displayNameForBlock } from "./state.js";
import { getInsertionTarget, legalBlocksFor, clearInsertionTarget, setInsertionTarget } from "./insertion.js";
import { insertBlock } from "./commands.js";
import { findDocumentNode } from "./state.js";

/** in-memory recent list, no persistence (§32) */
const recent = []; // array of block names string
const MAX_RECENT = 5;

function pushRecent(blockName) {
  if (!blockName) return;
  const idx = recent.indexOf(blockName);
  if (idx !== -1) recent.splice(idx, 1);
  recent.unshift(blockName);
  while (recent.length > MAX_RECENT) recent.pop();
}

export class QuickInserter {
  constructor(canvas) {
    this.canvas = canvas;
    this.target = null; // {parentId,index,contextInstanceKey?}
    this.anchorRect = null; // viewport rect for positioning
    this.host = null; // shadow overlay reference via canvas.overlay.shadow
    this.popover = null;
    this.searchInput = null;
    this.listEl = null;
    this._onDocClick = this.onDocumentClick.bind(this);
    this._onKey = this.onKey.bind(this);
  }

  isOpen() { return !!this.popover && !!this.popover.isConnected; }

  open(target, anchorRect) {
    this.close();
    if (!target || !this.canvas || !this.canvas.overlay || !this.canvas.overlay.shadow) return;
    this.target = { parentId: target.parentId ?? null, index: Number(target.index) || 0 };
    if (target.contextInstanceKey) this.target.contextInstanceKey = target.contextInstanceKey;
    this.anchorRect = anchorRect ? { ...anchorRect } : null;

    const parentNode = this.target.parentId == null ? null : findDocumentNode(this.target.parentId);
    const legal = legalBlocksFor(parentNode, this.target.index);
    if (!legal.length) return; // no legal → no UI (§17 already prevents showing plus, but guard)

    const shadow = this.canvas.overlay.shadow;
    const doc = this.canvas.doc;
    if (!doc) return;

    const wrap = doc.createElement("div");
    wrap.className = "quick-inserter";
    wrap.setAttribute("data-role", "quick-inserter");
    wrap.style.cssText = [
      "position:fixed",
      "pointer-events:auto",
      "z-index:2147483647",
      "width:min(300px, calc(100vw - 24px))",
      "max-height:min(360px, calc(100vh - 32px))",
      "display:flex",
      "flex-direction:column",
      "overflow:hidden",
      "background:#fff",
      "border:1px solid #e2e8f0",
      "border-radius:10px",
      "box-shadow:0 8px 32px rgba(15,23,42,.12), 0 2px 8px rgba(15,23,42,.08)",
      "font-family:system-ui,-apple-system,BlinkMacSystemFont,sans-serif",
    ].join(";");

    // position near anchor
    this.positionPopover(wrap, anchorRect, shadow);

    // Search
    const searchWrap = doc.createElement("div");
    searchWrap.style.cssText = "padding:8px 8px 0 8px;flex:0 0 auto;";
    const input = doc.createElement("input");
    input.type = "search";
    input.placeholder = "Search blocks…";
    input.setAttribute("aria-label", "Search blocks");
    input.style.cssText = [
      "box-sizing:border-box",
      "width:100%",
      "height:34px",
      "padding:0 10px",
      "border:1px solid #cbd5e1",
      "border-radius:7px",
      "font:400 13px/1 sans-serif",
      "outline:none",
    ].join(";");
    input.addEventListener("input", () => this.renderList(legal, input.value, list, footer));
    // prevent click propagation to canvas selection
    input.addEventListener("click", (e) => { e.stopPropagation(); });
    input.addEventListener("pointerdown", (e) => e.stopPropagation());
    input.addEventListener("keydown", (e) => {
      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        // simple focus move
        e.preventDefault();
        const items = Array.from(list.querySelectorAll("button[data-block]"));
        if (!items.length) return;
        const active = doc.activeElement;
        const idx = items.indexOf(active);
        let next = 0;
        if (e.key === "ArrowDown") next = idx < items.length - 1 ? idx + 1 : 0;
        else next = idx > 0 ? idx - 1 : items.length - 1;
        items[next]?.focus();
      }
      if (e.key === "Enter") {
        const first = list.querySelector("button[data-block]");
        if (first) { e.preventDefault(); first.click(); }
      }
      if (e.key === "Escape") {
        e.preventDefault(); e.stopPropagation();
        this.close();
      }
    });
    searchWrap.appendChild(input);
    wrap.appendChild(searchWrap);

    // List
    const list = doc.createElement("div");
    list.style.cssText = "flex:1 1 auto;overflow:auto;padding:8px;display:grid;gap:6px;align-content:start;";
    wrap.appendChild(list);

    // Footer Browse all
    const footer = doc.createElement("div");
    footer.style.cssText = "flex:0 0 auto;border-top:1px solid #eef2f7;padding:6px 8px;display:flex;justify-content:stretch;";
    const browseBtn = doc.createElement("button");
    browseBtn.type = "button";
    browseBtn.textContent = "Browse all blocks";
    browseBtn.style.cssText = [
      "width:100%",
      "height:32px",
      "border:1px solid #e2e8f0",
      "border-radius:7px",
      "background:#f8fafc",
      "color:#334155",
      "font:600 12px/1 sans-serif",
      "cursor:pointer",
    ].join(";");
    browseBtn.addEventListener("click", (e) => {
      e.preventDefault(); e.stopPropagation();
      const t = { ...this.target };
      this.close();
      // keep exact target and open left Blocks panel target-aware (§34)
      setInsertionTarget(t);
      // trigger panels open - single mechanism, panels subscribes to this event
      try { window.dispatchEvent(new CustomEvent("stratum:open-blocks", { detail: t })); } catch (_) {}
      // ensure insertion target retained after panel open
      setTimeout(() => setInsertionTarget(t), 0);
    });
    browseBtn.addEventListener("click", (e) => e.stopPropagation());
    footer.appendChild(browseBtn);

    wrap.addEventListener("click", (e) => e.stopPropagation());
    wrap.addEventListener("pointerdown", (e) => e.stopPropagation());
    wrap.addEventListener("mousedown", (e) => e.stopPropagation());

    shadow.appendChild(wrap);
    this.popover = wrap;
    this.searchInput = input;
    this.listEl = list;

    this.renderList(legal, "", list, footer);
    // attach document listeners for outside close/Escape priority
    try {
      doc.addEventListener("pointerdown", this._onDocClick, true);
      doc.addEventListener("keydown", this._onKey, true);
      document.addEventListener("pointerdown", this._onDocClick, true);
      document.addEventListener("keydown", this._onKey, true);
      if (this.canvas.win) {
        // close on scroll — do not float at stale coordinates (§10)
        this._onWinScroll = () => this.close();
        this.canvas.win.addEventListener("scroll", this._onWinScroll, { passive: true });
        // also listen on doc scroll
        try { this.canvas.doc.addEventListener("scroll", this._onWinScroll, true); } catch (_) {}
      }
    } catch (_) {}
    // autofocus
    requestAnimationFrame(() => {
      try { input.focus({ preventScroll: true }); } catch (_) { try { input.focus(); } catch (_) {} }
    });
    // after render, adjust position if overflow
    this.clampToViewport();
  }

  positionPopover(wrap, anchorRect, shadow) {
    // initial: centered below anchor line, fallback to below insertion plus
    if (!anchorRect) {
      wrap.style.left = "50%";
      wrap.style.top = "80px";
      wrap.style.transform = "translateX(-50%)";
      return;
    }
    // anchorRect is viewport rect (inside iframe)
    const vw = (this.canvas.win && this.canvas.win.innerWidth) || this.canvas.doc.documentElement.clientWidth || 1024;
    const vh = (this.canvas.win && this.canvas.win.innerHeight) || this.canvas.doc.documentElement.clientHeight || 700;
    const popW = 300;
    const popH = 320;
    let left = Math.round(anchorRect.left + (anchorRect.width - popW) / 2);
    let top = Math.round(anchorRect.top + anchorRect.height + 10);
    // if near bottom, show above
    if (top + popH > vh - 8) {
      top = Math.round(anchorRect.top - popH - 10);
      if (top < 8) top = 8;
    }
    if (left < 8) left = 8;
    if (left + popW > vw - 8) left = vw - popW - 8;
    if (top < 8) top = 8;
    wrap.style.left = left + "px";
    wrap.style.top = top + "px";
  }

  clampToViewport() {
    if (!this.popover || !this.canvas || !this.canvas.win) return;
    try {
      const r = this.popover.getBoundingClientRect();
      const vw = this.canvas.win.innerWidth;
      const vh = this.canvas.win.innerHeight;
      if (r.right > vw - 8) {
        this.popover.style.left = Math.max(8, vw - r.width - 8) + "px";
      }
      if (r.bottom > vh - 8) {
        // if still overflow, enable scroll inside popover (already scroll)
        // if anchor far offscreen, close rather than float wrong (§48)
        const anchor = this.anchorRect;
        if (anchor && (anchor.top < -100 || anchor.top > vh + 100 || anchor.left < -100 || anchor.left > vw + 100)) {
          this.close();
        }
      }
    } catch (_) {}
  }

  maybeCloseOnScroll() {
    if (!this.isOpen()) return;
    // if anchor moved far offscreen close rather than float wrong
    this.clampToViewport();
  }

  renderList(legal, query, listEl, footerEl) {
    if (!listEl) return;
    const doc = this.canvas.doc;
    listEl.replaceChildren();
    const q = (query || "").trim().toLocaleLowerCase();

    let filtered = legal;
    if (q) {
      filtered = legal.filter((item) => {
        const display = item.displayName || displayNameForBlock(item.block);
        const hay = `${display} ${item.block} ${item.description || ""}`.toLocaleLowerCase();
        return hay.includes(q);
      });
    }
    // No fuzzy library, simple substring (§31)

    // Build sections: Recent / Suggestions if no query and legal filtered
    const sections = [];

    if (!q) {
      const legalSet = new Set(legal.map((i) => i.block));
      const recentLegal = recent.map((blk) => legal.find((i) => i.block === blk)).filter(Boolean);
      if (recentLegal.length) {
        sections.push({ title: "Recent", items: recentLegal.slice(0, 5) });
        // remaining suggestions excluding recent
        const remaining = filtered.filter((i) => !recentLegal.some((r) => r.block === i.block));
        if (remaining.length) sections.push({ title: "Suggested", items: remaining.slice(0, 6) });
      } else {
        // show first 6-8 legal as initial suggestions, catalog order already given (§33)
        sections.push({ title: null, items: filtered.slice(0, 7) });
      }
    } else {
      if (filtered.length === 0) {
        const empty = doc.createElement("div");
        empty.textContent = "No blocks found";
        empty.style.cssText = "padding:12px;text-align:center;color:#64748b;font:400 13px/1.4 sans-serif;";
        listEl.appendChild(empty);
        return;
      }
      sections.push({ title: null, items: filtered });
    }

    for (const sec of sections) {
      if (sec.title) {
        const h = doc.createElement("div");
        h.textContent = sec.title;
        h.style.cssText = "margin:4px 0 2px 2px;color:#64748b;font:700 11px/1 sans-serif;letter-spacing:.06em;text-transform:uppercase;";
        listEl.appendChild(h);
      }
      for (const item of sec.items) {
        const btn = doc.createElement("button");
        btn.type = "button";
        btn.dataset.block = item.block;
        btn.dataset.version = String(item.version);
        const display = item.displayName || displayNameForBlock(item.block);
        btn.style.cssText = [
          "display:flex",
          "align-items:center",
          "gap:8px",
          "width:100%",
          "min-height:36px",
          "padding:6px 8px",
          "border:1px solid transparent",
          "border-radius:7px",
          "background:#fff",
          "text-align:left",
          "cursor:pointer",
          "font:500 13px/1.2 sans-serif",
          "color:#1e293b",
        ].join(";");
        btn.addEventListener("mouseenter", () => { btn.style.background = "#f8fafc"; btn.style.borderColor = "#e2e8f0"; });
        btn.addEventListener("mouseleave", () => { btn.style.background = "#fff"; btn.style.borderColor = "transparent"; });
        // icon placeholder
        const icon = doc.createElement("span");
        icon.textContent = "◫";
        icon.style.cssText = "flex:0 0 24px;display:inline-flex;align-items:center;justify-content:center;width:24px;height:24px;border:1px solid #e2e8f0;border-radius:5px;background:#f8fafc;color:#475569;font-size:11px;";
        const label = doc.createElement("span");
        label.textContent = display;
        label.style.cssText = "flex:1 1 auto;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;";
        btn.append(icon, label);
        if (item.description) btn.title = item.description;

        btn.addEventListener("click", (e) => {
          e.preventDefault(); e.stopPropagation();
          const def = item;
          const target = { ...this.target };
          const res = insertBlock({ definition: def, parentId: target.parentId, index: target.index });
          if (!res.ok) {
            try { window.stratumToast ? window.stratumToast("error", res.reason) : alert(res.reason); } catch (_) { alert(res.reason); }
            return;
          }
          pushRecent(def.block);
          this.close();
          // after successful insertion, app's subscribeDocument will trigger preview + selection
          // request selection after preview reload; if ambiguous, navigator will still highlight
          try {
            // optimistic select logical node immediately so navigator updates before preview
            const selParent = this.canvas;
            if (selParent && typeof selParent.selectNode === "function") {
              // delay until preview markers rebuilt; app will select after preview
              // store pending selection id queue for app to pick up (prevents overwrite in 80ms debounce)
              if (!res.node) return;
              state.__pendingSelectionIds ||= [];
              state.__pendingSelectionIds.push(res.node.id);
              // also set state.selection logically now for navigator
              state.selection = { nodeId: res.node.id, instanceKey: null, editable: true, block: res.node.block, version: res.node.version, logical: true };
            }
          } catch (_) {}
        });
        btn.addEventListener("pointerdown", (e) => e.stopPropagation());
        listEl.appendChild(btn);
      }
    }
    // responsive width already via min(300px, calc(100vw -24))
  }

  onDocumentClick(e) {
    if (!this.isOpen()) return;
    // if click inside popover, ignore
    try {
      const path = e.composedPath ? e.composedPath() : [];
      if (path.includes(this.popover) || (this.popover && this.popover.contains(e.target))) return;
      // also ignore clicks on insertion plus (they reopen) - but those stopPropagation already
    } catch (_) {}
    // clicking plus/overlay insertion controls already stopPropagation so they won't reach here
    // otherwise close
    // Do not close merely because iframe scrolls (§48 already handled)
    if (e.target && this.popover && this.popover.contains(e.target)) return;
    // check if click is on quick-inserter host shadow
    this.close();
  }

  onKey(e) {
    if (!this.isOpen()) return;
    if (e.key === "Escape") {
      e.preventDefault(); e.stopPropagation();
      this.close();
    }
  }

  close() {
    if (this.popover) {
      try { this.popover.remove(); } catch (_) {}
      this.popover = null;
    }
    this.target = null;
    this.anchorRect = null;
    this.searchInput = null;
    this.listEl = null;
    try {
      const doc = this.canvas?.doc;
      if (doc) {
        doc.removeEventListener("pointerdown", this._onDocClick, true);
        doc.removeEventListener("keydown", this._onKey, true);
        if (this._onWinScroll) doc.removeEventListener("scroll", this._onWinScroll, true);
      }
      document.removeEventListener("pointerdown", this._onDocClick, true);
      document.removeEventListener("keydown", this._onKey, true);
      if (this.canvas?.win && this._onWinScroll) {
        try { this.canvas.win.removeEventListener("scroll", this._onWinScroll); } catch (_) {}
      }
      this._onWinScroll = null;
    } catch (_) {}
  }

  // For panels Browse reusing
  static getRecent() { return [...recent]; }
}
