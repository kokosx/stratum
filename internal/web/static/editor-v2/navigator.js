import { displayNameForBlock, state, subscribeSelection } from "./state.js";

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
  }

  mount(root, canvas) {
    this.root = root;
    this.canvas = canvas;
    if (!this.initialized) {
      this.expandShallow(state.document?.nodes || [], 0);
      this.initialized = true;
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
    const expanded = children.length > 0 && this.expanded.has(node.id);

    const row = createElement("div", "editor-v2-navigator__row");
    row.style.setProperty("--navigator-depth", String(depth));

    if (children.length > 0) {
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

    const select = createElement("button", "editor-v2-navigator__select", displayNameForBlock(node.block));
    select.type = "button";
    select.dataset.nodeId = node.id;
    select.addEventListener("click", () => {
      if (this.canvas && typeof this.canvas.selectNode === "function") this.canvas.selectNode(node);
    });
    row.append(select);
    item.append(row);

    if (children.length > 0 && expanded) {
      const group = createElement("div", "editor-v2-navigator__group");
      for (const child of children) group.append(this.renderNode(child, depth + 1));
      item.append(group);
    }
    return item;
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
