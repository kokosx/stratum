// library.js — block library + patterns + search + tabs
import { state, bootstrap, definitionFor } from "./state.js";
import { findNode } from "./tree.js";
import { canInsert, canInsertRoots } from "./mutations.js";

function element(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

function getInsertionContext() {
  if (!state.insertionTarget) return null;
  const target = state.insertionTarget;
  if (target.parentId == null) return { parentNode: null, label: "Root", index: target.index };
  const found = findNode(target.parentId);
  if (!found) return null;
  const def = definitionFor(found.node);
  return { parentNode: found.node, label: def ? def.displayName : found.node.block, index: target.index };
}

function allowedBlocksForTarget() {
  const ctx = getInsertionContext();
  if (!ctx) return null; // no filter
  if (!ctx.parentNode) return null; // root accepts any (subject to context, but treat as any)
  const def = definitionFor(ctx.parentNode);
  if (!def) return [];
  const rule = def.schema.children;
  if (rule.mode === "any") return null;
  if (rule.mode === "allowed") return rule.blocks || [];
  return [];
}

export function renderCatalog(filter = "") {
  const catalogElement = document.getElementById("block-catalog");
  if (!catalogElement) return;
  catalogElement.replaceChildren();
  // Insertion context header
  const ctx = getInsertionContext();
  if (ctx) {
    const header = element("div", "insertion-context");
    header.style.cssText = "background:#eff6ff;border:1px solid #bfdbfe;border-radius:6px;padding:8px 10px;margin-bottom:10px;display:flex;align-items:center;justify-content:space-between;gap:8px";
    const left = element("div");
    const title = element("strong", "", `Adding ${ctx.parentNode ? "inside" : "at"}: ${ctx.label}`);
    title.style.fontSize = "12px";
    left.append(title);
    const sub = element("small", "muted", ctx.parentNode ? `Index ${ctx.index}` : `Position ${ctx.index}`);
    sub.style.display = "block";
    sub.style.color = "#64748b";
    left.append(sub);
    const cancel = element("button", "button button-small");
    cancel.type = "button";
    cancel.textContent = "Cancel";
    cancel.style.cssText = "padding:4px 8px;font-size:11px;border:1px solid #bfdbfe;background:white;border-radius:4px;cursor:pointer";
    cancel.addEventListener("click", () => {
      if (window.__stratum_clearInsertionTarget) window.__stratum_clearInsertionTarget();
      renderCatalog(filter);
    });
    header.append(left, cancel);
    catalogElement.append(header);
  }
  const query = filter.trim().toLowerCase();
  const allowed = allowedBlocksForTarget();
  const maxReached = (() => {
    if (!ctx || !ctx.parentNode) return false;
    const def = definitionFor(ctx.parentNode);
    if (!def || def.schema.children.max == null) return false;
    return ctx.parentNode.children.length >= def.schema.children.max;
  })();
  if (maxReached) {
    const warn = element("div", "insertion-warning");
    warn.style.cssText = "background:#fef2f2;border:1px solid #fecaca;color:#991b1b;padding:8px;border-radius:6px;font-size:12px;margin-bottom:10px";
    warn.textContent = `${ctx.label} has reached maximum children (${definitionFor(ctx.parentNode).schema.children.max}).`;
    catalogElement.append(warn);
  }
  let matches = state.catalog.filter((d) =>
    `${d.displayName} ${d.description || ""} ${d.block}`.toLowerCase().includes(query)
  );
  if (allowed !== null) {
    matches = matches.filter(d => allowed.includes(d.block));
  }
  if (allowed !== null && !matches.length && !maxReached) {
    catalogElement.append(element("p", "editor-empty", `No blocks allowed in ${ctx.label}.`));
    renderPatternCatalog(filter);
    return;
  }
  let category = null;
  matches
    .sort((a, b) =>
      (a.schema.editor.category || "other").localeCompare(b.schema.editor.category || "other") ||
      a.displayName.localeCompare(b.displayName)
    )
    .forEach((d) => {
      const nextCategory = d.schema.editor.category || "Other";
      if (nextCategory !== category) {
        category = nextCategory;
        catalogElement.append(element("h3", "catalog-category", category));
      }
      const button = element("button", "catalog-item");
      button.type = "button";
      button.draggable = true;
      button.dataset.block = d.block;
      button.dataset.version = String(d.version);
      // Disable if insertion target present and block not allowed or max reached
      let disabledReason = "";
      if (ctx && ctx.parentNode) {
        const c = canInsert(ctx.parentNode, d.block, ctx.index);
        if (!c.ok) {
          button.disabled = true;
          button.title = c.reason;
          button.style.opacity = "0.5";
          disabledReason = c.reason;
        }
      }
      if (maxReached) {
        button.disabled = true;
        button.title = `${ctx.label} max reached`;
      }
      button.addEventListener("dragstart", (event) => {
        window.__stratum_currentDrag = { type: "library", definition: d };
        const treeEl = document.getElementById("document-tree");
        const navEl = document.getElementById("navigator-content");
        [treeEl, navEl].forEach(el => el && el.classList.add("tree--dragging"));
        event.dataTransfer.effectAllowed = "copy";
        event.dataTransfer.setData("text/plain", `${d.block}@${d.version}`);
        const canvas = document.getElementById("editor-canvas");
        if (window.__stratum_canvasController) window.__stratum_canvasController.onLibraryDragStart(d);
      });
      button.addEventListener("dragend", () => {
        window.__stratum_currentDrag = null;
        document.querySelectorAll(".tree--dragging").forEach(el => el.classList.remove("tree--dragging"));
        if (window.__stratum_canvasController) window.__stratum_canvasController.onLibraryDragEnd();
      });
      const title = element("strong");
      title.append(document.createTextNode(d.displayName));
      button.append(title);
      if (d.description) button.append(element("small", "", d.description));
      if (disabledReason) button.append(element("small", "muted", disabledReason));
      if (button.disabled) {
        button.addEventListener("click", (e) => {
          e.preventDefault();
          if (window.stratumToast) window.stratumToast("error", disabledReason || "Not allowed here");
        });
      } else {
        button.addEventListener("click", () => {
          if (window.__stratum_addBlock) window.__stratum_addBlock(d);
        });
      }
      catalogElement.append(button);
    });
  if (!matches.length && allowed===null) catalogElement.append(element("p", "editor-empty", "No matching blocks."));
  renderPatternCatalog(filter);
}

export function renderPatternCatalog(filter = "") {
  const patternCatalog = document.getElementById("pattern-catalog");
  if (!patternCatalog) return;
  patternCatalog.replaceChildren();
  const query = (filter || "").trim().toLowerCase();
  let patterns = state.patterns;
  if (query) {
    patterns = patterns.filter((p) =>
      `${p.name} ${p.description || ""} ${p.category || ""}`.toLowerCase().includes(query)
    );
  }
  const byCategory = {};
  patterns.forEach((p) => {
    const cat = p.category || "Other";
    if (!byCategory[cat]) byCategory[cat] = [];
    byCategory[cat].push(p);
  });
  const categories = Object.keys(byCategory).sort();
  if (!patterns.length) {
    patternCatalog.append(element("p", "editor-empty", "No matching patterns."));
    return;
  }
  categories.forEach((cat) => {
    patternCatalog.append(element("h3", "catalog-category", cat));
    byCategory[cat].forEach((p) => {
      const card = element("div", "catalog-item pattern-card");
      const title = element("strong", "", p.name);
      card.append(title);
      if (p.description) card.append(element("small", "", p.description));
      const meta = element("small", "muted", p.category);
      card.append(meta);
      const btn = element("button", "button button-primary");
      btn.type = "button";
      btn.textContent = "Insert";
      btn.addEventListener("click", () => {
        if (window.__stratum_insertPattern) window.__stratum_insertPattern(p.id);
      });
      card.append(btn);
      patternCatalog.append(card);
    });
  });
}

export function initLibraryTabs() {
  const tabs = document.querySelectorAll(".library-tab");
  const blockCat = document.getElementById("block-catalog");
  const patternCat = document.getElementById("pattern-catalog");
  const navTree = document.getElementById("navigator-tree");
  const searchEl = document.getElementById("block-search");
  if (!tabs.length || !blockCat || !patternCat) return;
  tabs.forEach((tab) => {
    tab.addEventListener("click", () => {
      tabs.forEach((t) => {
        t.classList.remove("is-active");
        t.setAttribute("aria-selected", "false");
      });
      tab.classList.add("is-active");
      tab.setAttribute("aria-selected", "true");
      const wanted = tab.dataset.tab;
      state.libraryTab = wanted;
      if (wanted === "patterns") {
        blockCat.hidden = true;
        patternCat.hidden = false;
        if (navTree) navTree.hidden = true;
        if (searchEl) searchEl.placeholder = "Search patterns";
      } else if (wanted === "navigator") {
        blockCat.hidden = true;
        patternCat.hidden = true;
        if (navTree) navTree.hidden = false;
        if (searchEl) searchEl.placeholder = "Search navigator";
        if (window.__stratum_renderNavigator) window.__stratum_renderNavigator();
      } else {
        blockCat.hidden = false;
        patternCat.hidden = true;
        if (navTree) navTree.hidden = true;
        if (searchEl) searchEl.placeholder = "Search blocks";
      }
    });
  });
  if (state.libraryTab === "patterns") {
    blockCat.hidden = true;
    patternCat.hidden = false;
    if (navTree) navTree.hidden = true;
  } else if (state.libraryTab === "navigator") {
    blockCat.hidden = true;
    patternCat.hidden = true;
    if (navTree) navTree.hidden = false;
  } else {
    blockCat.hidden = false;
    patternCat.hidden = true;
    if (navTree) navTree.hidden = true;
  }
}
