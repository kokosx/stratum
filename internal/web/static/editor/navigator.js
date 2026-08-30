// navigator.js — tree view in left panel + breadcrumbs
import { state, definitions, definitionFor } from "./state.js";
import { findNode, isContainer, subtreeHasChildren } from "./tree.js";

function element(tag, className, text) {
  const n = document.createElement(tag);
  if (className) n.className = className;
  if (text !== undefined) n.textContent = text;
  return n;
}

function nodeSummary(node) {
  const def = definitionFor(node);
  const fields = def?.schema?.editor?.summaryFields || def?.summaryFields;
  if (fields && fields.length) {
    for (const path of fields) {
      const parts = path.split(".");
      const scope = parts[0] === "props" ? node.props : parts[0] === "settings" ? node.settings : null;
      if (!scope) continue;
      const key = parts.slice(1).join(".");
      const val = scope[key];
      if (typeof val === "string" && val.trim()) return val.slice(0, 70);
      if (val?.version === 1 && Array.isArray(val.content)) {
        return val.content.map((run) => run.text || "").join("").slice(0, 70);
      }
      if (val) return String(val).slice(0, 70);
    }
  }
  const block = def?.block || node.block;
  const p = node.props || {};
  const s = node.settings || {};
  if (block === "core/heading" || block === "core/entry-title") return p.text || "";
  if (block === "core/text") return (p.text || "").slice(0, 70);
  if (block === "core/button") return p.label || "";
  if (block === "core/image") return p.alt || p.caption || (p.mediaId ? "" : "(no image)");
  if (block === "core/section") return s.width || "content";
  if (block === "core/stack") return `${s.direction || "vertical"} · ${s.gap || "md"}`;
  if (block === "core/grid") return `${s.columns || 2} cols`;
  if (block === "core/card") return s.variant || "default";
  const value = Object.values(p).find((v) => typeof v === "string" && v);
  return value ? value.slice(0, 70) : "";
}

export function renderNavigator() {
  const container = document.getElementById("navigator-content");
  if (!container) return;
  container.replaceChildren();
  if (!state.document.nodes.length) {
    container.append(element("p", "editor-empty", "No blocks yet."));
    return;
  }
  const root = document.createElement("div");
  root.className = "navigator-tree";
  state.document.nodes.forEach((node) => root.append(renderNavigatorNode(node, 0)));
  container.append(root);
  updateBreadcrumbs();
}

function renderNavigatorNode(node, depth) {
  const def = definitionFor(node);
  const container = isContainer(node);
  const isSelected = node.id === state.selectedNodeId;
  const collapsed = state.collapsed.has(node.id);
  const wrapper = element("div", `navigator-node ${isSelected ? "is-selected" : ""} ${collapsed ? "is-collapsed" : ""}`);
  wrapper.dataset.nodeId = node.id;
  wrapper.style.setProperty("--depth", depth);
  wrapper.draggable = true;
  wrapper.tabIndex = 0;
  wrapper.setAttribute("role", "treeitem");
  wrapper.setAttribute("aria-selected", isSelected ? "true" : "false");
  wrapper.addEventListener("click", (e) => {
    e.stopPropagation();
    state.selectedNodeId = node.id;
    state.selectedInstanceKey = null;
    if (window.__stratum_onSelectionChange) window.__stratum_onSelectionChange(node.id, null);
    renderNavigator();
    if (window.__stratum_renderInspector) window.__stratum_renderInspector();
    if (window.__stratum_canvasController) window.__stratum_canvasController.selectNode(node.id, null);
  });
  wrapper.addEventListener("keydown", (e) => {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      wrapper.click();
    }
    if (e.key === "Delete" || e.key === "Backspace") {
      if (window.__stratum_removeNode) window.__stratum_removeNode(node.id);
    }
  });
  wrapper.addEventListener("dragstart", (e) => {
    window.__stratum_currentDrag = { type: "node", nodeId: node.id };
    e.dataTransfer.effectAllowed = "move";
    e.dataTransfer.setData("text/plain", node.id);
  });
  wrapper.addEventListener("dragend", () => {
    window.__stratum_currentDrag = null;
  });
  // drop handling for navigator reorder
  wrapper.addEventListener("dragover", (e) => {
    if (!window.__stratum_currentDrag) return;
    e.preventDefault();
    wrapper.classList.add("navigator-drop-target");
  });
  wrapper.addEventListener("dragleave", () => wrapper.classList.remove("navigator-drop-target"));
  wrapper.addEventListener("drop", (e) => {
    e.preventDefault();
    wrapper.classList.remove("navigator-drop-target");
    const drag = window.__stratum_currentDrag;
    if (!drag) return;
    if (drag.type === "node" && drag.nodeId === node.id) return;
    if (window.__stratum_performNavigatorDrop) window.__stratum_performNavigatorDrop(drag, node);
  });

  const header = element("div", "navigator-node__header");
  const indent = element("span", "navigator-node__indent");
  indent.style.marginLeft = (depth * 12) + "px";
  header.append(indent);
  if (container && node.children.length > 0) {
    const btn = element("button", "navigator-node__collapse", collapsed ? "▸" : "▾");
    btn.type = "button";
    btn.addEventListener("click", (e) => {
      e.stopPropagation();
      if (state.collapsed.has(node.id)) state.collapsed.delete(node.id);
      else state.collapsed.add(node.id);
      renderNavigator();
    });
    header.append(btn);
  } else {
    header.append(element("span", "navigator-node__spacer", ""));
  }
  const typeBadge = element("span", "navigator-node__type", def ? def.displayName : node.block);
  header.append(typeBadge);
  const summary = element("span", "navigator-node__summary", nodeSummary(node));
  header.append(summary);
  wrapper.append(header);
  if (container && !collapsed && node.children.length) {
    const childrenWrap = element("div", "navigator-node__children");
    node.children.forEach(child => childrenWrap.append(renderNavigatorNode(child, depth + 1)));
    wrapper.append(childrenWrap);
  }
  return wrapper;
}

export function updateBreadcrumbs() {
  const el = document.getElementById("editor-canvas-breadcrumbs");
  if (!el) return;
  if (!state.selectedNodeId) {
    el.textContent = "";
    el.hidden = true;
    return;
  }
  const path = buildPath(state.selectedNodeId);
  if (!path.length) {
    el.textContent = "";
    el.hidden = true;
    return;
  }
  el.replaceChildren();
  el.hidden = false;
  path.forEach((node, idx) => {
    const def = definitions.get(`${node.block}@${node.version}`);
    const label = def ? def.displayName : node.block;
    const span = element("span", "breadcrumb-item", label);
    span.title = node.id;
    span.addEventListener("click", () => {
      state.selectedNodeId = node.id;
      state.selectedInstanceKey = null;
      renderNavigator();
      if (window.__stratum_renderInspector) window.__stratum_renderInspector();
      if (window.__stratum_canvasController) window.__stratum_canvasController.selectNode(node.id, null);
    });
    el.append(span);
    if (idx < path.length - 1) el.append(element("span", "breadcrumb-sep", " › "));
  });
}

function buildPath(targetId) {
  const path = [];
  function walk(nodes, ancestors) {
    for (const n of nodes) {
      const curAnc = [...ancestors, n];
      if (n.id === targetId) {
        path.push(...curAnc);
        return true;
      }
      if (n.children && walk(n.children, curAnc)) return true;
    }
    return false;
  }
  walk(state.document.nodes, []);
  return path;
}
