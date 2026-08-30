// navigator.js — tree view in left panel + breadcrumbs
import { state, definitions, definitionFor } from "./state.js";
import { findNode, isContainer, subtreeHasChildren } from "./tree.js";
import { canInsert, canRemove, canDuplicate, canMove } from "./mutations.js";

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
    wrapper.classList.remove("navigator-drop-target--before","navigator-drop-target--after","navigator-drop-target--inside","navigator-drop-target");
  });
  // drop handling with before/inside/after intent
  let currentPosition = "after";
  wrapper.addEventListener("dragover", (e) => {
    if (!window.__stratum_currentDrag) return;
    e.preventDefault();
    const rect = wrapper.getBoundingClientRect();
    const edge = Math.min(24, rect.height * 0.25);
    const y = e.clientY - rect.top;
    if (container && y > edge && y < rect.height - edge) currentPosition = "inside";
    else if (y <= rect.height/2) currentPosition = "before";
    else currentPosition = "after";
    wrapper.classList.remove("navigator-drop-target","navigator-drop-target--before","navigator-drop-target--after","navigator-drop-target--inside");
    wrapper.classList.add(`navigator-drop-target--${currentPosition}`);
    wrapper.classList.add("navigator-drop-target");
    const drag = window.__stratum_currentDrag;
    const block = drag.type==="library" ? drag.definition.block : (findNode(drag.nodeId)?.node.block || "");
    // Validate and show invalid style
    let valid = false;
    try {
      if (currentPosition === "inside") valid = canInsert(node, block, node.children.length, drag).ok;
      else {
        const info = findNode(node.id);
        if (info) {
          const parent = info.parent;
          const idx = currentPosition==="before"? info.index : info.index+1;
          valid = canInsert(parent, block, idx, drag).ok;
          // also need source min for moves
          if (drag.type==="node") valid = canMove(drag.nodeId, currentPosition==="inside"?node:parent, currentPosition==="inside"?node.children.length:idx).ok;
        }
      }
    } catch (_) {}
    wrapper.classList.toggle("is-invalid", !valid);
    e.dataTransfer.dropEffect = valid ? (drag.type==="library"?"copy":"move") : "none";
  });
  wrapper.addEventListener("dragleave", (e) => {
    if (e.relatedTarget && wrapper.contains(e.relatedTarget)) return;
    wrapper.classList.remove("navigator-drop-target","navigator-drop-target--before","navigator-drop-target--after","navigator-drop-target--inside","is-invalid");
  });
  wrapper.addEventListener("drop", (e) => {
    e.preventDefault();
    wrapper.classList.remove("navigator-drop-target","navigator-drop-target--before","navigator-drop-target--after","navigator-drop-target--inside","is-invalid");
    const drag = window.__stratum_currentDrag;
    if (!drag) return;
    if (drag.type === "node" && drag.nodeId === node.id) return;
    if (window.__stratum_performNavigatorDrop) window.__stratum_performNavigatorDrop(drag, node, currentPosition);
    window.__stratum_currentDrag = null;
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
  // Hover actions: + (add inside/after), duplicate, delete
  const actions = element("span", "navigator-node__actions");
  actions.style.cssText = "display:flex;gap:2px;opacity:0;transition:opacity .12s";
  wrapper.addEventListener("mouseenter", ()=> actions.style.opacity="1");
  wrapper.addEventListener("mouseleave", ()=> actions.style.opacity="0");
  wrapper.addEventListener("focusin", ()=> actions.style.opacity="1");
  // Add inside (for containers) or Add after
  const addInside = element("button", "navigator-action", "+");
  addInside.type = "button";
  addInside.title = container ? "Add inside" : "Add after";
  addInside.style.cssText = "width:20px;height:20px;border:1px solid #cbd5e1;background:white;border-radius:4px;font-size:11px;cursor:pointer";
  const addCheck = container ? canInsert(node, "core/text", node.children.length) : canInsert(findNode(node.id)?.parent || null, "core/text", (findNode(node.id)?.index||0)+1);
  // Generic check: if container allows any, we assume add is possible; precise check will happen on click via canInsert with real block (library will filter)
  addInside.addEventListener("click", (e)=>{
    e.stopPropagation();
    if (container) {
      if (window.__stratum_setInsertionTarget) window.__stratum_setInsertionTarget(node.id, node.children.length);
    } else {
      const info = findNode(node.id);
      if (info) {
        const parentId = info.parent ? info.parent.id : null;
        if (window.__stratum_setInsertionTarget) window.__stratum_setInsertionTarget(parentId, info.index+1);
      }
    }
    document.querySelectorAll(".library-tab").forEach(t=>{ if(t.dataset.tab==="blocks") t.click(); });
    if (window.__stratum_renderCatalog) window.__stratum_renderCatalog();
  });
  actions.append(addInside);
  const moreBtn = element("button", "navigator-action", "⋮");
  moreBtn.type = "button";
  moreBtn.title = "More actions";
  moreBtn.style.cssText = "width:20px;height:20px;border:1px solid #cbd5e1;background:white;border-radius:4px;font-size:11px;cursor:pointer";
  moreBtn.addEventListener("click", (e)=>{
    e.stopPropagation();
    // Simple inline menu: duplicate/delete
    const dup = canDuplicate(node.id);
    const rem = canRemove(node.id);
    const choice = prompt(`Actions for ${def?def.displayName:node.block}:\n1=Duplicate ${dup.ok?"":"(blocked: "+dup.reason+")"}\n2=Delete ${rem.ok?"":"(blocked: "+rem.reason+")"}\n3=Add after\nEnter 1/2/3`);
    if (choice==="1") {
      if (!dup.ok) alert(dup.reason); else if (window.__stratum_duplicateNode) window.__stratum_duplicateNode(node.id);
    } else if (choice==="2") {
      if (!rem.ok) alert(rem.reason); else if (window.__stratum_removeNode) window.__stratum_removeNode(node.id);
    } else if (choice==="3") {
      const info = findNode(node.id);
      if (info) {
        const parentId = info.parent ? info.parent.id : null;
        if (window.__stratum_setInsertionTarget) window.__stratum_setInsertionTarget(parentId, info.index+1);
        document.querySelectorAll(".library-tab").forEach(t=>{ if(t.dataset.tab==="blocks") t.click(); });
      }
    }
  });
  actions.append(moreBtn);
  // Direct duplicate/delete icons (visible on hover)
  const dupCheck = canDuplicate(node.id);
  const dupBtn = element("button", "navigator-action", "⧉");
  dupBtn.type = "button"; dupBtn.title = dupCheck.ok ? "Duplicate" : dupCheck.reason;
  dupBtn.disabled = !dupCheck.ok; dupBtn.style.opacity = dupCheck.ok?"1":"0.4";
  dupBtn.style.cssText += ";width:20px;height:20px;border:1px solid #cbd5e1;background:white;border-radius:4px;font-size:10px;cursor:"+(dupCheck.ok?"pointer":"not-allowed");
  dupBtn.addEventListener("click", (e)=>{ e.stopPropagation(); if (!dupCheck.ok) { alert(dupCheck.reason); return; } if (window.__stratum_duplicateNode) window.__stratum_duplicateNode(node.id); });
  actions.append(dupBtn);
  const delCheck = canRemove(node.id);
  const delBtn = element("button", "navigator-action--danger", "✕");
  delBtn.type = "button"; delBtn.title = delCheck.ok ? "Delete" : delCheck.reason;
  delBtn.disabled = !delCheck.ok; delBtn.style.opacity = delCheck.ok?"1":"0.4";
  delBtn.style.cssText += ";width:20px;height:20px;border:1px solid #fecaca;background:white;border-radius:4px;font-size:10px;cursor:"+(delCheck.ok?"pointer":"not-allowed")+";color:#b42318";
  delBtn.addEventListener("click", (e)=>{ e.stopPropagation(); if (!delCheck.ok) { alert(delCheck.reason); return; } if (window.__stratum_removeNode) window.__stratum_removeNode(node.id); });
  actions.append(delBtn);
  header.append(summary);
  header.append(actions);
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
