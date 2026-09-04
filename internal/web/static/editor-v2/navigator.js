import { displayNameForBlock, nodeSummaryFor, state, subscribeDocument, subscribeSelection, findDocumentParent, findDocumentNode, isContainerNode } from "./state.js";
import { canMove } from "./insertion.js";
import { moveNode } from "./commands.js";
import { startSession, clearSession, getSession } from "./drag-session.js";
import { deleteBlock, duplicateBlock } from "./actions.js";

function createElement(tag, className, text) {
  const element = document.createElement(tag);
  if (className) element.className = className;
  if (text !== undefined) element.textContent = text;
  return element;
}

export class NavigatorView {
  constructor() {
    this.root = null;
    this.canvas = null;
    this.expanded = new Set();
    this.initialized = false;
    this.unsubscribe = subscribeSelection(() => this.selectionChanged());
    this.unsubscribeDoc = subscribeDocument(() => {
      // Immediate update even before preview (§47)
      this.render();
    });
    this._dragNodeId = null;
    this._dropTarget = null;
    this._autoScrollRAF = 0;
    this._autoScrollDir = 0;
    this.actionsMenu = null;
    this.actionsTrigger = null;
    this.actionsBound = false;
    this._onOutsideActions = (event) => {
      if (!this.actionsMenu) return;
      if (this.actionsMenu.contains(event.target) || this.actionsTrigger?.contains(event.target)) return;
      this.closeActionsMenu();
    };
  }

  mount(root, canvas) {
    this.root = root;
    this.canvas = canvas;
    if (!this.initialized) {
      this.expandShallow(state.document?.nodes || [], 0);
      this.initialized = true;
    }
    if (!this.actionsBound) {
      document.addEventListener("pointerdown", this._onOutsideActions, true);
      this.actionsBound = true;
    }
    this.render();
  }

  expandShallow(nodes, depth) {
    for (const node of nodes || []) {
      if ((node.children || []).length && depth < 2) this.expanded.add(node.id);
      if (depth < 2) this.expandShallow(node.children || [], depth + 1);
    }
  }

  render() {
    if (!this.root) return;
    this.closeActionsMenu();
    this.root.replaceChildren();

    const scope = createElement("div", "editor-v2-navigator__scope", "Page content");
    this.root.append(scope);

    const tree = createElement("div", "editor-v2-navigator__tree");
    tree.setAttribute("aria-label", "Page content blocks");
    const nodes = state.document?.nodes || [];
    if (nodes.length === 0) {
      tree.append(createElement("p", "editor-v2-panel__empty", "No blocks in this page."));
    } else {
      for (const node of nodes) tree.append(this.renderNode(node, 0));
    }
    this.root.append(tree);

    const note = createElement("p", "editor-v2-navigator__note", "Template content is read-only in this editor.");
    this.root.append(note);
    this.syncSelection(false);
  }

  renderNode(node, depth) {
    const item = createElement("div", "editor-v2-navigator__item");
    item.dataset.nodeId = node.id;
    const children = node.children || [];
    const isContainer = children.length > 0;
    if (isContainer) item.classList.add("is-container");
    if (depth === 0) item.classList.add("is-root");

    const expanded = isContainer && this.expanded.has(node.id);

    const row = createElement("div", "editor-v2-navigator__row");
    row.style.setProperty("--navigator-depth", String(depth));

    if (isContainer) {
      const toggle = createElement("button", "editor-v2-navigator__toggle");
      toggle.type = "button";
      toggle.setAttribute("aria-label", `${expanded ? "Collapse" : "Expand"} ${displayNameForBlock(node.block)}`);
      toggle.setAttribute("aria-expanded", String(expanded));
      toggle.innerHTML = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="m9 18 6-6-6-6"/></svg>';
      toggle.addEventListener("click", () => {
        if (expanded) this.expanded.delete(node.id);
        else this.expanded.add(node.id);
        this.render();
      });
      row.append(toggle);
    } else {
      row.append(createElement("span", "editor-v2-navigator__toggle-spacer"));
    }

    // Drag grip for movable editable row (§46) — small, not whole row
    const grip = createElement("button", "editor-v2-navigator__drag-handle");
    grip.type = "button";
    grip.setAttribute("draggable", "true");
    const disp = displayNameForBlock(node.block);
    grip.setAttribute("aria-label", `Move ${disp}`);
    grip.setAttribute("title", "Move block");
    grip.textContent = "⠿";
    grip.addEventListener("dragstart", (e) => this.onDragStart(e, node));
    grip.addEventListener("dragend", (e) => this.onDragEnd(e));
    grip.addEventListener("click", (e) => { e.stopPropagation(); });
    row.append(grip);

    const select = createElement("button", "editor-v2-navigator__select");
    select.type = "button";
    select.dataset.nodeId = node.id;
    const name = createElement("span", "editor-v2-navigator__name", displayNameForBlock(node.block));
    select.append(name);
    const rawSummary = nodeSummaryFor(node);
    if (rawSummary) {
      const truncated = rawSummary.length > 36 ? rawSummary.slice(0, 33) + "\u2026" : rawSummary;
      const summaryEl = createElement("span", "editor-v2-navigator__summary", truncated);
      summaryEl.title = rawSummary;
      select.append(summaryEl);
    }
    select.addEventListener("click", () => {
      if (this.canvas && typeof this.canvas.selectNode === "function") this.canvas.selectNode(node, { reveal: true });
    });
    row.append(select);
    const actions = createElement("button", "editor-v2-navigator__row-actions", "•••");
    actions.type = "button";
    actions.setAttribute("data-stratum-editor-ui", "true");
    actions.setAttribute("aria-label", `Actions for ${displayNameForBlock(node.block)}`);
    actions.setAttribute("aria-haspopup", "menu");
    actions.setAttribute("aria-expanded", "false");
    actions.setAttribute("title", "Block actions");
    actions.addEventListener("pointerdown", (event) => event.stopPropagation());
    actions.addEventListener("click", (event) => {
      event.preventDefault();
      event.stopPropagation();
      if (this.actionsTrigger === actions) this.closeActionsMenu(true);
      else this.openActionsMenu(node, row, actions);
    });
    row.append(actions);
    // Row drop handling (§47)
    item.addEventListener("dragover", (e) => this.onDragOver(e, node, item, row));
    item.addEventListener("dragleave", (e) => this.onDragLeave(e, item));
    item.addEventListener("drop", (e) => this.onDrop(e, node, item));
    item.append(row);

    if (isContainer && expanded) {
      const group = createElement("div", "editor-v2-navigator__group");
      for (const child of children) group.append(this.renderNode(child, depth + 1));
      item.append(group);
    }
    return item;
  }

  openActionsMenu(node, row, trigger) {
    this.closeActionsMenu();
    if (!node?.id || !findDocumentNode(node.id)) return false;
    const menu = createElement("div", "editor-v2-navigator__actions-menu");
    menu.setAttribute("data-stratum-editor-ui", "true");
    menu.setAttribute("role", "menu");
    for (const action of [
      { name: "Duplicate", destructive: false, run: () => duplicateBlock(node.id) },
      { name: "Delete", destructive: true, run: () => deleteBlock(node.id) },
    ]) {
      const button = createElement("button", "editor-v2-navigator__actions-item" + (action.destructive ? " is-destructive" : ""), action.name);
      button.type = "button";
      button.setAttribute("data-stratum-editor-ui", "true");
      button.setAttribute("role", "menuitem");
      button.addEventListener("click", (event) => {
        event.preventDefault();
        event.stopPropagation();
        this.closeActionsMenu();
        action.run();
      });
      menu.append(button);
    }
    row.append(menu);
    this.actionsMenu = menu;
    this.actionsTrigger = trigger;
    trigger.setAttribute("aria-expanded", "true");
    try { menu.querySelector("button")?.focus({ preventScroll: true }); } catch (_) {}
    return true;
  }

  closeActionsMenu(focusTrigger = false) {
    if (!this.actionsMenu) return false;
    const trigger = this.actionsTrigger;
    try { this.actionsMenu.remove(); } catch (_) {}
    this.actionsMenu = null;
    this.actionsTrigger = null;
    if (trigger) trigger.setAttribute("aria-expanded", "false");
    if (focusTrigger && trigger?.isConnected) {
      try { trigger.focus({ preventScroll: true }); } catch (_) {}
    }
    return true;
  }

  onDragStart(e, node) {
    if (!node || !node.id) { e.preventDefault(); return; }
    const info = findDocumentParent(node.id);
    if (!info) { e.preventDefault(); return; }
    try { e.dataTransfer.effectAllowed = "move"; e.dataTransfer.setData("text/plain", node.id); } catch (_) {}
    this._dragNodeId = node.id;
    startSession({ kind: "node", nodeId: node.id, instanceKey: null, source: { parentId: info.parent ? info.parent.id : null, index: info.index } });
    try { e.stopPropagation(); } catch (_) {}
    try {
      if (this.canvas && state.selection?.nodeId !== node.id) {
        this.canvas.selectNode(node);
      }
    } catch (_) {}
    try { if (e.target) e.target.style.opacity = "0.6"; } catch (_) {}
  }

  onDragEnd(e) {
    try { if (e.target) e.target.style.opacity = ""; } catch (_) {}
    this.clearNavigatorDrag();
    try { clearSession(); } catch (_) {}
    this._dragNodeId = null;
    this.stopNavigatorAutoScroll();
  }

  clearNavigatorDrag() {
    if (!this.root) return;
    for (const el of this.root.querySelectorAll(".is-drop-before, .is-drop-after, .is-drop-inside")) {
      el.classList.remove("is-drop-before", "is-drop-after", "is-drop-inside");
    }
    const indicator = this.root.querySelector(".editor-v2-navigator__drop-line");
    if (indicator) try { indicator.remove(); } catch (_) {}
    this._dropTarget = null;
  }

  onDragOver(e, node, itemEl, rowEl) {
    const sess = getSession();
    const dragId = sess?.nodeId;
    if (!dragId) return;
    if (dragId === node.id) { try{ e.dataTransfer.dropEffect = "none"; }catch(_){} return; }
    e.preventDefault();
    e.stopPropagation();
    try { e.dataTransfer.dropEffect = "move"; } catch (_) {}
    const rect = rowEl.getBoundingClientRect();
    const y = e.clientY - rect.top;
    const h = rect.height || 30;
    let zone = "before";
    if (h > 0) {
      if (y < h * 0.3) zone = "before";
      else if (y > h * 0.7) zone = "after";
      else zone = "inside";
    }
    const info = findDocumentParent(node.id);
    if (!info) return;
    let target = null;
    if (zone === "before") {
      const parentId = info.parent ? info.parent.id : null;
      target = { parentId, index: info.index, zone: "before", itemEl };
    } else if (zone === "after") {
      const parentId = info.parent ? info.parent.id : null;
      target = { parentId, index: info.index + 1, zone: "after", itemEl };
    } else {
      // center → inside if legal container
      const isContainer = isContainerNode(node);
      if (!isContainer) {
        // fallback to before/after based on y vs mid
        if (y < h / 2) {
          const parentId = info.parent ? info.parent.id : null;
          target = { parentId, index: info.index, zone: "before", itemEl };
        } else {
          const parentId = info.parent ? info.parent.id : null;
          target = { parentId, index: info.index + 1, zone: "after", itemEl };
        }
      } else {
        const insideIdx = (node.children || []).length;
        target = { parentId: node.id, index: insideIdx, zone: "inside", itemEl };
      }
    }
    // Validate legality (§31, §49-50)
    const legal = canMove(dragId, target.parentId, target.index);
    if (!legal.ok) {
      // For before/after inside center fallback already handled, but if inside illegal, try before/after of that container's parent?
      // For now, suppress center and treat as before/after of original row if inside illegal and zone was inside
      if (target.zone === "inside") {
        // Try before as fallback
        const parentId = info.parent ? info.parent.id : null;
        const beforeTarget = { parentId, index: info.index, zone: "before", itemEl };
        const afterTarget = { parentId, index: info.index + 1, zone: "after", itemEl };
        const beforeLegal = canMove(dragId, beforeTarget.parentId, beforeTarget.index);
        const afterLegal = canMove(dragId, afterTarget.parentId, afterTarget.index);
        if (y < h/2 && beforeLegal.ok) target = beforeTarget;
        else if (afterLegal.ok) target = afterTarget;
        else { try{ e.dataTransfer.dropEffect="none"; }catch(_){} this.clearNavigatorDrag(); return; }
        if (!canMove(dragId, target.parentId, target.index).ok) { try{ e.dataTransfer.dropEffect="none"; }catch(_){} this.clearNavigatorDrag(); return; }
      } else {
        try{ e.dataTransfer.dropEffect="none"; }catch(_){} this.clearNavigatorDrag(); return;
      }
    }
    this._dropTarget = target;
    // Visual feedback (§51)
    for (const el of this.root.querySelectorAll(".is-drop-before, .is-drop-after, .is-drop-inside")) el.classList.remove("is-drop-before","is-drop-after","is-drop-inside");
    // Remove previous line
    const prevLine = this.root.querySelector(".editor-v2-navigator__drop-line");
    if (prevLine) try{ prevLine.remove(); }catch(_){}
    if (target.zone === "inside") {
      itemEl.classList.add("is-drop-inside");
    } else {
      itemEl.classList.add(target.zone === "before" ? "is-drop-before" : "is-drop-after");
      // Add thin blue line
      const line = document.createElement("div");
      line.className = "editor-v2-navigator__drop-line";
      line.style.height = "3px";
      line.style.background = "#2563eb";
      line.style.borderRadius = "2px";
      line.style.margin = target.zone === "before" ? "0 0 -3px 0" : "-3px 0 0 0";
      if (target.zone === "before") itemEl.prepend(line);
      else itemEl.append(line);
    }
    // Auto-scroll for layers panel (§52) simple
    try {
      const panel = this.root.closest ? this.root.closest(".editor-v2-panel__body") || this.root.parentElement : this.root.parentElement;
      if (panel) {
        const pr = panel.getBoundingClientRect();
        if (e.clientY < pr.top + 48) this.startNavigatorAutoScroll(-1, panel);
        else if (e.clientY > pr.bottom - 48) this.startNavigatorAutoScroll(1, panel);
        else this.stopNavigatorAutoScroll();
      }
    } catch (_) {}
  }

  onDragLeave(e, itemEl) {
    // Only clear if leaving item entirely
    try {
      const related = e.relatedTarget;
      if (related && itemEl.contains(related)) return;
    } catch (_) {}
    // Timeout to allow dragover to set new target
    setTimeout(() => {
      if (this._dropTarget && this._dropTarget.itemEl === itemEl) {
        // keep if still dragging over same item via child? dragover will re-add
      }
    }, 20);
  }

  onDrop(e, node, itemEl) {
    const sess = getSession();
    const dragId = sess?.nodeId;
    if (!dragId) return;
    e.preventDefault();
    e.stopPropagation();
    this.stopNavigatorAutoScroll();
    const target = this._dropTarget;
    this.clearNavigatorDrag();
    try { clearSession(); } catch (_) {}
    this._dragNodeId = null;
    if (!target) return;
    const legal = canMove(dragId, target.parentId, target.index);
    if (!legal.ok) return;
    const res = moveNode({ nodeId: dragId, parentId: target.parentId, index: target.index });
    if (!res || !res.ok) {
      try { if (window.stratumToast) window.stratumToast("error", res?.reason || "Could not move block."); } catch (_) {}
    }
  }

  startNavigatorAutoScroll(dir, panel) {
    if (this._autoScrollRAF) return;
    this._autoScrollDir = dir;
    const el = panel || this.root;
    const step = () => {
      if (!this._autoScrollDir || !el) { this._autoScrollRAF = 0; return; }
      try { el.scrollTop += this._autoScrollDir * 8; } catch (_) {}
      this._autoScrollRAF = requestAnimationFrame(step);
    };
    this._autoScrollRAF = requestAnimationFrame(step);
  }

  stopNavigatorAutoScroll() {
    this._autoScrollDir = 0;
    if (this._autoScrollRAF) { try{ cancelAnimationFrame(this._autoScrollRAF); }catch(_){} this._autoScrollRAF = 0; }
  }

  syncSelection(scroll) {
    if (!this.root) return;
    const selectedId = state.selection?.nodeId || "";
    let active = null;
    for (const item of this.root.querySelectorAll(".editor-v2-navigator__item")) {
      const selected = !!selectedId && item.dataset.nodeId === selectedId;
      item.classList.toggle("is-active", selected);
      const button = item.querySelector(":scope > .editor-v2-navigator__row .editor-v2-navigator__select");
      if (button) button.setAttribute("aria-current", selected ? "true" : "false");
      if (selected) active = item;
    }
    if (scroll && active && typeof active.scrollIntoView === "function") {
      active.scrollIntoView({ block: "nearest" });
    }
  }

  selectionChanged() {
    this.closeActionsMenu();
    const nodeId = state.selection?.nodeId;
    if (!nodeId) {
      this.syncSelection(false);
      return;
    }
    const path = [];
    const findPath = (nodes) => {
      for (const node of nodes || []) {
        path.push(node);
        if (node.id === nodeId) return true;
        if (findPath(node.children || [])) return true;
        path.pop();
      }
      return false;
    };
    if (!findPath(state.document?.nodes || [])) {
      this.syncSelection(false);
      return;
    }
    let changed = false;
    for (const ancestor of path.slice(0, -1)) {
      if ((ancestor.children || []).length && !this.expanded.has(ancestor.id)) {
        this.expanded.add(ancestor.id);
        changed = true;
      }
    }
    if (changed) this.render();
    this.syncSelection(true);
  }
}
