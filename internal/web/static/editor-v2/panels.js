import {
  activatePanel,
  blockCatalog,
  displayNameForBlock,
  findDocumentNode,
  mostRecentPanelSlot,
  panelState,
  setPanel,
  state,
  subscribeDocument,
  subscribePanels,
  subscribeSelection,
  togglePanel,
} from "./state.js";
import { clearInsertionTarget, describeBlocksTarget, getInsertionSource, getInsertionTarget, legalBlocksFor, resolveGlobalInsertion, setInsertionTarget, subscribeInsertionTarget, targetForBlocksFromSelection } from "./insertion.js";
import { insertBlock } from "./commands.js";
import { NavigatorView } from "./navigator.js";
import { inspectorTitle, renderDocumentBody, renderInspectorBody } from "./inspector.js";
import { commitBeforeEditorContextChange } from "./inline-editor.js";

function createElement(tag, className, text) {
  const element = document.createElement(tag);
  if (className) element.className = className;
  if (text !== undefined) element.textContent = text;
  return element;
}

function panelTitle(panel) {
  if (panel === "blocks") return "Blocks";
  if (panel === "navigator") return "Layers";
  if (panel === "document") return "Document";
  if (panel === "inspector") return inspectorTitle(state.selection);
  return "Panel";
}

function categoryLabel(category) {
  const labels = {
    layout: "Layout",
    content: "Content",
    media: "Media",
    design: "Design",
    dynamic: "Dynamic",
    data: "Data",
  };
  return labels[category] || (category ? category.charAt(0).toUpperCase() + category.slice(1) : "Other");
}

function normalizedCategory(category) {
  if (category === "text") return "content";
  if (category === "query") return "data";
  return category || "other";
}

const CATEGORY_ORDER = ["layout", "content", "media", "design", "dynamic", "data", "branding", "reusable", "widgets", "other"];

function blockIcon(iconName) {
  const wrapper = createElement("span", "editor-v2-block-item__icon");
  wrapper.dataset.icon = iconName || "block";
  wrapper.innerHTML = '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" aria-hidden="true"><rect x="4" y="4" width="6" height="6" rx="1"/><rect x="14" y="4" width="6" height="6" rx="1"/><rect x="4" y="14" width="6" height="6" rx="1"/><rect x="14" y="14" width="6" height="6" rx="1"/></svg>';
  return wrapper;
}

export class PanelController {
  constructor({ root, workspace, canvas, closeMenu }) {
    this.root = root;
    this.workspace = workspace;
    this.canvas = canvas;
    this.closeMenu = closeMenu;
    this.leftSlot = root.querySelector("#editor-v2-panel-left");
    this.rightSlot = root.querySelector("#editor-v2-panel-right");
    this.buttons = {
      blocks: root.querySelector("#editor-v2-blocks-btn"),
      navigator: root.querySelector("#editor-v2-layers-btn"),
      document: root.querySelector("#editor-v2-document-btn"),
    };
    this.navigator = new NavigatorView();
    this.blocksQuery = "";
    this.selectedCatalogItem = "";
    this.suppressedInspectorKey = "";
    this.inspectorReturnFocus = null;
    this.lastFocusedButton = { left: null, right: null };
    this.focusNext = { left: false, right: false };
    this.unsubscribePanels = null;
    this.unsubscribeSelection = null;
    this.unsubscribeDocument = null;
    this.unsubscribeInsertion = null;
    this.onResize = () => this.updateStacking();
  }

  mount() {
    this.bindButton("blocks", "left", "blocks");
    this.bindButton("navigator", "left", "navigator");
    this.bindButton("document", "right", "document");
    this.unsubscribePanels = subscribePanels(() => this.render());
    this.unsubscribeSelection = subscribeSelection((next, previous) => {
      this.selectionChanged(next, previous);
      // If Blocks is open with selection-derived target, recompute immediately (§18)
      if (panelState.left === "blocks" && getInsertionSource() === "selection") {
        const derived = targetForBlocksFromSelection();
        const cur = getInsertionTarget();
        if (!derived) { if (cur) clearInsertionTarget(); }
        else if (!cur || cur.parentId !== derived.parentId || cur.index !== derived.index) {
          setInsertionTarget(derived, { source: "selection" });
        }
        // render will be triggered via insertion subscription
      }
    });
    this.unsubscribeDocument = subscribeDocument(() => {
      // Blocks may need re-render if filtering depends on doc max children
      if (panelState.left === "blocks") this.renderSlot("left", true);
    });
    this.unsubscribeInsertion = subscribeInsertionTarget(() => {
      if (panelState.left === "blocks") this.renderSlot("left", true);
    });
    window.addEventListener("resize", this.onResize);
    // open Blocks from Quick Inserter's Browse all blocks (keeps exact contextual target §19)
    window.addEventListener("stratum:open-blocks", (e) => {
      // panel will be target-aware via getInsertionTarget
      if (e && e.detail && typeof e.detail === "object") {
        try { setInsertionTarget(e.detail, { source: "contextual" }); } catch (_) {}
      }
      if (panelState.left !== "blocks") {
        this.focusNext.left = true;
        activatePanel("left", "blocks");
      } else {
        this.renderSlot("left", true);
      }
    });
    this.render();
  }

  bindButton(name, slot, panel) {
    const button = this.buttons[name];
    if (!button) return;
    button.addEventListener("click", () => {
      // Commit active inline edit before switching panels (§51/52)
      try { commitBeforeEditorContextChange(); } catch (_) {}
      this.lastFocusedButton[slot] = button;
      this.focusNext[slot] = panelState[slot] !== panel;
      // Switching Blocks → Layers clears insertionTarget (§37)
      const prevLeft = panelState.left;
      const covered = window.matchMedia("(max-width: 760px)").matches
        && panelState[slot] === panel
        && Boolean(panelState.left && panelState.right)
        && mostRecentPanelSlot() !== slot;
      if (prevLeft === "blocks" && panel === "navigator" && slot === "left") {
        clearInsertionTarget();
      }
      const wasBlocksOpen = prevLeft === "blocks" && slot === "left" && panel === "blocks";
      const openingBlocks = slot === "left" && panel === "blocks" && panelState.left !== "blocks";
      if (openingBlocks) {
        // Topbar Blocks must derive fresh selection-derived target, never reuse stale contextual (§21)
        clearInsertionTarget();
        const derived = targetForBlocksFromSelection();
        if (derived) setInsertionTarget(derived, { source: "selection" });
      }
      if (covered) {
        this.focusNext[slot] = true;
        activatePanel(slot, panel);
        if (wasBlocksOpen) clearInsertionTarget();
        return;
      }
      togglePanel(slot, panel);
      if (wasBlocksOpen) clearInsertionTarget();
    });
  }

  selectionKey(selection) {
    return selection ? selection.instanceKey || `node:${selection.nodeId || ""}` : "";
  }

  selectionChanged(next, previous) {
    if (!next) {
      this.suppressedInspectorKey = "";
      if (panelState.right === "inspector") {
        const shouldRestore = this.rightSlot?.contains(document.activeElement);
        setPanel("right", null);
        if (shouldRestore) this.restoreInspectorFocus();
      }
      if (panelState.left === "blocks" && getInsertionSource() === "selection") {
        clearInsertionTarget();
      }
      return;
    }
    const key = this.selectionKey(next);
    const previousKey = this.selectionKey(previous);
    const isDifferent = key !== previousKey;
    if (isDifferent) this.suppressedInspectorKey = "";
    if (panelState.right !== "inspector") {
      const active = document.activeElement;
      this.inspectorReturnFocus = active && !this.rightSlot?.contains(active) ? active : null;
      this.focusNext.right = window.matchMedia("(max-width: 760px)").matches
        && Boolean(active && this.leftSlot?.contains(active));
    }

    if (panelState.right === "document") {
      activatePanel("right", "inspector");
      return;
    }
    if (panelState.right === "inspector") {
      activatePanel("right", "inspector");
      this.renderSlot("right", true);
      this.updateStacking();
      return;
    }
    if (this.suppressedInspectorKey === key) return;
    activatePanel("right", "inspector");
  }

  render() {
    this.updateButtons();
    this.renderSlot("left", false);
    this.renderSlot("right", false);
    this.updateStacking();
  }

  updateButtons() {
    const active = {
      blocks: panelState.left === "blocks",
      navigator: panelState.left === "navigator",
      document: panelState.right === "document",
    };
    for (const [name, button] of Object.entries(this.buttons)) {
      if (!button) continue;
      button.classList.toggle("is-active", active[name]);
      button.setAttribute("aria-expanded", String(active[name]));
    }
  }

  renderSlot(slot, force) {
    const element = slot === "left" ? this.leftSlot : this.rightSlot;
    if (!element) return;
    const panel = panelState[slot];
    if (!panel) {
      element.replaceChildren();
      element.hidden = true;
      delete element.dataset.panel;
      return;
    }
    if (!force && element.dataset.panel === panel && element.childElementCount > 0) return;

    element.hidden = false;
    element.dataset.panel = panel;
    const surface = createElement("section", "editor-v2-panel");
    surface.setAttribute("role", "dialog");
    surface.setAttribute("aria-modal", "false");
    surface.setAttribute("aria-label", panelTitle(panel));

    const header = createElement("header", "editor-v2-panel__header");
    const heading = createElement("h2", "editor-v2-panel__title", panelTitle(panel));
    heading.tabIndex = -1;
    header.append(heading);
    const close = createElement("button", "editor-v2-panel__close");
    close.type = "button";
    close.setAttribute("aria-label", `Close ${panelTitle(panel)} panel`);
    close.innerHTML = '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M18 6 6 18M6 6l12 12"/></svg>';
    close.addEventListener("click", () => this.close(slot, true));
    header.append(close);
    surface.append(header);

    const body = createElement("div", "editor-v2-panel__body");
    if (panel === "blocks") this.renderBlocks(body);
    else if (panel === "navigator") this.navigator.mount(body, this.canvas);
    else if (panel === "inspector") renderInspectorBody(body, state.selection);
    else if (panel === "document") renderDocumentBody(body);
    surface.append(body);
    element.replaceChildren(surface);
    requestAnimationFrame(() => surface.classList.add("is-open"));

    if (this.focusNext[slot]) {
      this.focusNext[slot] = false;
      requestAnimationFrame(() => {
        const focusTarget = panel === "blocks" ? body.querySelector('input[type="search"]') : heading;
        if (focusTarget && element.dataset.panel === panel) focusTarget.focus({ preventScroll: true });
      });
    }
  }

  close(slot, restoreFocus) {
    const panel = panelState[slot];
    if (panel === "inspector") this.suppressedInspectorKey = this.selectionKey(state.selection);
    const button = panel === "blocks" ? this.buttons.blocks
      : panel === "navigator" ? this.buttons.navigator
        : panel === "document" ? this.buttons.document
          : null;
    // clearing insertionTarget when Blocks closes after explicit contextual insertion (§37)
    // Keep target while Blocks is open; clear on close or when switching left slot
    const wasBlocks = panel === "blocks" && slot === "left";
    setPanel(slot, null);
    if (wasBlocks) clearInsertionTarget();
    // Switching Blocks → Layers clears insertionTarget
    if (slot === "left" && panel === "navigator") {
      // already handled via setPanel observer, but ensure
    }
    if (restoreFocus && panel === "inspector") this.restoreInspectorFocus();
    else if (restoreFocus && button && typeof button.focus === "function") button.focus();
  }

  restoreInspectorFocus() {
    const target = this.inspectorReturnFocus?.isConnected
      ? this.inspectorReturnFocus
      : this.root.querySelector("#editor-v2-canvas");
    this.inspectorReturnFocus = null;
    requestAnimationFrame(() => target?.focus?.({ preventScroll: true }));
  }

  updateStacking() {
    const narrow = window.matchMedia("(max-width: 760px)").matches;
    const bothOpen = Boolean(panelState.left && panelState.right);
    const front = mostRecentPanelSlot();
    for (const [slot, element] of [["left", this.leftSlot], ["right", this.rightSlot]]) {
      if (!element) continue;
      const covered = narrow && bothOpen && front !== slot;
      element.classList.toggle("is-front", front === slot);
      element.classList.toggle("is-covered", covered);
      element.inert = covered;
    }
  }

  handleEscape(event) {
    if (!event || event.key !== "Escape") return false;
    // quick inserter has priority (§49) — canvas handles it via its own onKey, but also close if panels focused
    try {
      if (this.canvas && this.canvas.quickInserter && this.canvas.quickInserter.isOpen()) {
        event.preventDefault();
        this.canvas.closeQuickInserter();
        return true;
      }
    } catch (_) {}
    if (typeof this.closeMenu === "function" && this.closeMenu()) {
      event.preventDefault();
      return true;
    }
    if (state.selection) {
      event.preventDefault();
      this.canvas?.clearSelection();
      return true;
    }
    const slot = mostRecentPanelSlot();
    if (slot) {
      event.preventDefault();
      this.close(slot, true);
      return true;
    }
    return false;
  }

  renderBlocks(body) {
    // Ensure explicit target when Blocks opened with selection (§17,19)
    let target = getInsertionTarget();
    if (!target && panelState.left === "blocks") {
      const derived = targetForBlocksFromSelection();
      if (derived) {
        setInsertionTarget(derived, { source: "selection" });
        target = getInsertionTarget();
      }
    }
    // Target-aware header (§23)
    if (target) {
      const header = createElement("div", "editor-v2-insertion-context");
      header.style.cssText = "background:#eff6ff;border:1px solid #bfdbfe;border-radius:7px;padding:8px 10px;margin-bottom:10px;display:flex;align-items:center;justify-content:space-between;gap:8px";
      const left = createElement("div");
      const desc = describeBlocksTarget(target, state.selection);
      const label = desc.main;
      const subLabel = desc.sub;
      const title = createElement("strong", "", label);
      title.style.fontSize = "12px";
      title.style.color = "#1e40af";
      left.append(title);
      const sub = createElement("small", "", subLabel);
      sub.style.display = "block";
      sub.style.color = "#64748b";
      sub.style.fontSize = "11px";
      left.append(sub);
      const cancel = createElement("button", "editor-v2-panel__close");
      cancel.type = "button";
      cancel.textContent = "Cancel";
      cancel.style.cssText = "padding:4px 8px;font-size:11px;border:1px solid #bfdbfe;background:white;border-radius:6px;cursor:pointer;color:#1e40af;";
      cancel.addEventListener("click", () => { clearInsertionTarget(); });
      header.append(left, cancel);
      body.append(header);
    } else {
      // No explicit target: non-empty page with no selection (§24)
      const nodes = state.document?.nodes || [];
      if (nodes.length > 0 && !state.selection) {
        const header = createElement("div", "editor-v2-insertion-context");
        header.style.cssText = "background:#fffbeb;border:1px solid #fde68a;border-radius:7px;padding:8px 10px;margin-bottom:10px;display:flex;align-items:center;gap:8px";
        const left = createElement("div");
        const title = createElement("strong", "", "Add block");
        title.style.fontSize = "12px";
        title.style.color = "#92400e";
        left.append(title);
        const sub = createElement("small", "", "Choose a position on the canvas");
        sub.style.display = "block";
        sub.style.color = "#b45309";
        sub.style.fontSize = "11px";
        left.append(sub);
        header.append(left);
        body.append(header);
      }
    }
    const search = createElement("input", "editor-v2-panel__search");
    search.type = "search";
    search.placeholder = "Search blocks…";
    search.setAttribute("aria-label", "Search blocks");
    search.value = this.blocksQuery;
    const results = createElement("div", "editor-v2-block-catalog");
    const update = () => {
      this.blocksQuery = search.value;
      this.renderCatalog(results);
    };
    search.addEventListener("input", update);
    body.append(search, results);
    // global fallback hint element for conservative insertion (§38)
    const hintEl = createElement("div", "editor-v2-panel__hint");
    hintEl.style.cssText = "display:none;margin-top:8px;padding:8px 10px;background:#fffbeb;border:1px solid #fde68a;color:#92400e;border-radius:6px;font-size:12px;";
    hintEl.textContent = "Choose where to add this block.";
    body.append(results, hintEl);
    this.renderCatalog(results);
    // expose hint for renderCatalog to toggle
    this._blocksHintEl = hintEl;
  }

  renderCatalog(root) {
    root.replaceChildren();
    const query = this.blocksQuery.trim().toLocaleLowerCase();
    const target = getInsertionTarget();
    let targetParentNode = null;
    let targetIndex = 0;
    if (target) {
      targetParentNode = target.parentId == null ? null : findDocumentNode(target.parentId);
      targetIndex = target.index;
    }
    // When targeted, only show legal blocks (§36)
    let candidateItems = blockCatalog();
    if (target) {
      candidateItems = legalBlocksFor(targetParentNode, targetIndex);
      if (!candidateItems.length) {
        root.append(createElement("p", "editor-v2-panel__empty", targetParentNode ? `No blocks allowed in ${displayNameForBlock(targetParentNode.block)}.` : "No blocks available."));
        if (this._blocksHintEl) this._blocksHintEl.style.display = "none";
        return;
      }
    }
    const groups = new Map();
    for (const item of candidateItems) {
      if (!item || !item.block) continue;
      const displayName = item.displayName || displayNameForBlock(item.block);
      const haystack = `${displayName} ${item.block} ${item.description || ""}`.toLocaleLowerCase();
      if (query && !haystack.includes(query)) continue;
      const category = normalizedCategory(item.schema?.editor?.category);
      if (!groups.has(category)) groups.set(category, []);
      groups.get(category).push({ ...item, displayName });
    }
    if (groups.size === 0) {
      root.append(createElement("p", "editor-v2-panel__empty", "No blocks found"));
      if (this._blocksHintEl) this._blocksHintEl.style.display = "none";
      return;
    }
    if (this._blocksHintEl) this._blocksHintEl.style.display = "none";

    const categories = [...groups.keys()].sort((a, b) => {
      const ai = CATEGORY_ORDER.indexOf(a);
      const bi = CATEGORY_ORDER.indexOf(b);
      return (ai === -1 ? 99 : ai) - (bi === -1 ? 99 : bi) || a.localeCompare(b);
    });
    for (const category of categories) {
      const section = createElement("section", "editor-v2-block-category");
      section.append(createElement("h3", "editor-v2-block-category__title", categoryLabel(category)));
      const list = createElement("div", "editor-v2-block-category__list");
      for (const item of groups.get(category)) {
        const key = `${item.block}@${item.version}`;
        const row = createElement("button", "editor-v2-block-item");
        row.type = "button";
        row.setAttribute("aria-pressed", String(this.selectedCatalogItem === key));
        row.classList.toggle("is-selected", this.selectedCatalogItem === key);
        row.append(blockIcon(item.schema?.editor?.icon));
        const copy = createElement("span", "editor-v2-block-item__copy");
        copy.append(createElement("span", "editor-v2-block-item__name", item.displayName));
        if (item.description) copy.append(createElement("span", "editor-v2-block-item__description", item.description));
        row.append(copy);
        // insertion on click (§39, no draggable yet §40)
        row.addEventListener("click", (e) => {
          e.preventDefault(); e.stopPropagation();
          let res;
          if (target) {
            res = insertBlock({ definition: item, parentId: target.parentId, index: target.index });
          } else {
            const fallback = resolveGlobalInsertion(item);
            if (fallback) {
              res = insertBlock({ definition: item, parentId: fallback.parentId, index: fallback.index });
            } else {
              // conservative: no surprising teleport (§38,70)
              if (this._blocksHintEl) this._blocksHintEl.style.display = "block";
              return;
            }
          }
          if (!res || !res.ok) {
            const reason = res?.reason || "Could not insert block.";
            try {
              if (window.stratumToast) window.stratumToast("error", reason);
              else if (this._blocksHintEl) { this._blocksHintEl.textContent = reason; this._blocksHintEl.style.display = "block"; setTimeout(() => { const el=this._blocksHintEl; if(el) el.style.display = "none"; }, 3000); }
            } catch (_) {}
            return;
          }
          if (this._blocksHintEl) this._blocksHintEl.style.display = "none";
          this.selectedCatalogItem = key;
          try {
            state.__pendingSelectionIds ||= []; state.__pendingSelectionIds.push(res.node.id); state.__pendingSelectionBlock = res.node.block;
            // mirror quick-inserter: set logical selection immediately so navigator updates before preview (§44)
            state.selection = { nodeId: res.node.id, instanceKey: null, editable: true, block: res.node.block, version: res.node.version, logical: true };
          } catch (_) {}
          // After successful insert, clear insertion target per §27 (predictability > rapid multi-insert)
          // Blocks panel returns to non-targeted state; next insert will re-derive from new selection if needed
          if (target) {
            try { clearInsertionTarget(); } catch (_) {}
          }
          // Re-render catalog to reflect new max/legal
          this.renderCatalog(root);
        });
        list.append(row);
      }
      section.append(list);
      root.append(section);
    }
  }
}
