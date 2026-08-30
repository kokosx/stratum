// library.js — block library + patterns + search + tabs
import { state, bootstrap } from "./state.js";

function element(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

export function renderCatalog(filter = "") {
  const catalogElement = document.getElementById("block-catalog");
  if (!catalogElement) return;
  catalogElement.replaceChildren();
  const query = filter.trim().toLowerCase();
  const matches = state.catalog.filter((d) =>
    `${d.displayName} ${d.description || ""} ${d.block}`.toLowerCase().includes(query)
  );
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
      button.addEventListener("click", () => {
        if (window.__stratum_addBlock) window.__stratum_addBlock(d);
      });
      catalogElement.append(button);
    });
  if (!matches.length) catalogElement.append(element("p", "editor-empty", "No matching blocks."));
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
