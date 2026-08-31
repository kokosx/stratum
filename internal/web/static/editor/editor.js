// Stratum Visual Editor — unified SDT editor (modular)
import { state, bootstrap, definitions, definitionFor, pushHistory, maybePushHistory, undo, redo, updateDirty, syncBaseline, initHistory, hydrateNode, isDirtyNow, commitMutation, setInsertionTarget, clearInsertionTarget } from "./state.js";
import { findNode, isContainer, createNode, duplicateSubtree, assignNewIDs, clonePatternNodes, insertionPoint, insertionPointForPattern, canInsertRoots, containerAccepts, isWithin, collectNodeIds } from "./tree.js";
import { createValidNode, canInsert, canInsertRoots as canInsertRootsM, canRemove, canMove, canDuplicate, canIndent as canIndentM, canOutdent as canOutdentM, insertionPointForBlock, resolveInsertionTarget, hasLegalInsertion } from "./mutations.js";
import { renderCatalog, renderPatternCatalog, initLibraryTabs } from "./library.js";
import { renderNavigator, updateBreadcrumbs } from "./navigator.js";
import { renderInspector } from "./inspector.js";
import { schedulePreview, updatePreview, initArchivePreviewContext } from "./preview.js";
import { CanvasController } from "./canvas.js";
import { validateDropTarget, clearAllDropUI } from "./dragdrop.js";

const form = document.getElementById("stratum-editor-form");
const documentInput = document.getElementById("document-json");
const treeElement = document.getElementById("document-tree");
const inspectorElement = document.getElementById("block-inspector");
const emptyElement = document.getElementById("empty-document");
const dirtyElement = document.getElementById("editor-dirty");
const previewElement = document.getElementById("editor-preview");
const errorElement = document.getElementById("editor-error");
const canvasEl = document.getElementById("editor-canvas");
const canvasOverlay = document.getElementById("editor-canvas-overlay");
const navigatorContent = document.getElementById("navigator-content");

let currentDrag = null;
window.__stratum_currentDrag = null;
let previewTimer = null;
let metadataTimer = null;

// Expose helpers for modules
window.__stratum_findNode = findNode;
window.__stratum_definitionFor = definitionFor;
window.__stratum_isContainer = isContainer;
window.__stratum_renderNavigator = renderNavigator;
window.__stratum_renderInspector = (ext) => renderInspector(ext);
window.__stratum_updateBreadcrumbs = updateBreadcrumbs;
window.__stratum_changed = changed;
window.__stratum_maybePushHistory = maybePushHistory;
window.__stratum_addBlock = addBlock;
window.__stratum_insertPattern = insertPattern;
window.__stratum_removeNode = removeNode;
window.__stratum_duplicateNode = duplicateNode;
window.__stratum_moveNode = moveNode;
window.__stratum_undo = () => { if(undo()){ renderTree(); renderInspector(); updateBreadcrumbs(); updateDirty(); schedulePreview(); if(canvasController) canvasController.refresh(); } };
window.__stratum_redo = () => { if(redo()){ renderTree(); renderInspector(); updateBreadcrumbs(); updateDirty(); schedulePreview(); if(canvasController) canvasController.refresh(); } };
window.__stratum_setInsertionTarget = (parentId, index) => { setInsertionTarget({parentId, index}); renderCatalog(document.getElementById("block-search")?.value||""); if (canvasController) canvasController.renderOverlays(); renderTree(); };
window.__stratum_clearInsertionTarget = () => { clearInsertionTarget(); renderCatalog(document.getElementById("block-search")?.value||""); if (canvasController) canvasController.renderOverlays(); renderTree(); };
window.__stratum_canRemove = canRemove;
window.__stratum_canDuplicate = canDuplicate;
window.__stratum_canInsert = canInsert;
window.__stratum_hasLegalInsertion = hasLegalInsertion;
window.__stratum_renderCatalog = (f) => renderCatalog(f || document.getElementById("block-search")?.value||"");
window.__stratum_onSelectionChange = (nodeId, instanceKey, externalInfo) => {
  state.selectedNodeId = nodeId;
  state.selectedInstanceKey = instanceKey || null;
  // expand ancestors so selected is visible
  if (nodeId) {
    try {
      const path = [];
      function walk(nodes, acc) {
        for (const n of nodes) {
          const cur = [...acc, n];
          if (n.id === nodeId) { path.push(...cur); return true; }
          if (n.children && walk(n.children, cur)) return true;
        }
        return false;
      }
      walk(state.document.nodes, []);
      path.forEach(p => { if (state.collapsed.has(p.id)) state.collapsed.delete(p.id); });
    } catch(_) {}
  }
  if (state._closeMenu) state._closeMenu();
  renderNavigator();
  renderInspector(externalInfo || null);
  updateBreadcrumbs();
  renderTree();
  // scroll navigator to selected
  if (nodeId) {
    setTimeout(() => {
      const el = document.querySelector(`[data-node-id="${CSS.escape(nodeId)}"]`) || document.querySelector(`[data-node-id="${nodeId}"]`);
      if (el) el.scrollIntoView({ block: "nearest", inline: "nearest" });
    }, 20);
  }
};
window.__stratum_renderTree = renderTree;

// Canvas controller
let canvasController = null;
if (canvasEl) {
  canvasController = new CanvasController(canvasEl, canvasOverlay);
  window.__stratum_canvasController = canvasController;
}
window.__stratum_catalog = state.catalog;
window.__stratum_bootstrap = bootstrap;

// Insertion affordance helpers
function showInsertionPrompt(msg) {
  if (errorElement) {
    errorElement.textContent = msg;
    errorElement.hidden = false;
    setTimeout(()=>{ if (errorElement.textContent===msg) errorElement.hidden=true; }, 3000);
  } else if (window.stratumToast) window.stratumToast("error", msg);
  else alert(msg);
}

// Legacy tree rendering (fallback) — kept for progressive enhancement and hidden legacy fallback
function renderTree() {
  if (!treeElement) return;
  treeElement.replaceChildren();
  // Empty document editor-only state
  if (state.document.nodes.length === 0) {
    const empty = document.createElement("div");
    empty.className = "empty-document-state";
    empty.style.cssText = "display:grid;gap:12px;place-items:center;padding:32px 16px;border:2px dashed #cbd5e1;border-radius:8px;background:#f8fafc;margin:8px";
    const title = element("h3", "", "Start building this page");
    title.style.cssText = "margin:0;font-size:16px;font-weight:600";
    const desc = element("p", "muted", "Add your first block to get started.");
    desc.style.cssText = "margin:0;color:#64748b;font-size:13px";
    const actions = element("div");
    actions.style.cssText = "display:flex;gap:8px";
    const addBtn = element("button", "button button-primary", "+ Add block");
    addBtn.type = "button";
    addBtn.addEventListener("click", () => {
      setInsertionTarget({parentId:null, index:0});
      renderCatalog("");
      const lib = document.querySelector(".block-library");
      if (lib) lib.scrollIntoView({behavior:"smooth"});
      const tabs = document.querySelectorAll("[data-library-tab]");
      tabs.forEach(t=>{ if((t.getAttribute('data-library-tab')||t.dataset.tab)==="blocks"){ t.click(); }});
    });
    const patBtn = element("button", "button", "Browse patterns");
    patBtn.type = "button";
    patBtn.addEventListener("click", () => {
      const tabs = document.querySelectorAll("[data-library-tab]");
      tabs.forEach(t=>{ if((t.getAttribute('data-library-tab')||t.dataset.tab)==="patterns"){ t.click(); }});
    });
    actions.append(addBtn, patBtn);
    empty.append(title, desc, actions);
    treeElement.append(empty);
    if (emptyElement) emptyElement.hidden = true;
    renderNavigator();
    updateBreadcrumbs();
    return;
  }
  treeElement.append(renderDropZone(state.document.nodes, null));
  if (emptyElement) emptyElement.hidden = state.document.nodes.length > 0;
  // also update navigator
  renderNavigator();
  updateBreadcrumbs();
}

function insertionLabelForContainer(containerNode) {
  if (!containerNode) return "Add block";
  const def = definitionFor(containerNode);
  if (!def) return "Add block";
  const rule = def.schema.children;
  if (rule.mode === "allowed" && rule.blocks.length===1) {
    const childBlock = rule.blocks[0];
    // find display name
    for (const d of state.catalog) if (d.block===childBlock) return `+ Add ${d.displayName}`;
  }
  return "+ Add block";
}

function renderDropZone(siblings, containerNode) {
  const zone = document.createElement("div");
  zone.className = containerNode ? "node__children" : "tree__root";
  const canAddInside = hasLegalInsertion(containerNode, 0);
  if (siblings.length === 0) {
    const slot = document.createElement("div");
    slot.className = "drop-slot drop-slot--empty";
    const inner = document.createElement("div");
    inner.className = "node__empty";
    if (canAddInside) {
      inner.style.cssText = "display:grid;gap:6px;place-items:center;padding:12px";
      const label = insertionLabelForContainer(containerNode);
      const btn = element("button", "button button-small", label);
      btn.type = "button";
      btn.addEventListener("click", (e)=>{
        e.stopPropagation();
        const parentId = containerNode ? containerNode.id : null;
        setInsertionTarget({parentId, index:0});
        renderCatalog("");
        const lib = document.querySelector(".block-library");
        if (lib) lib.scrollIntoView({behavior:"smooth"});
        // switch to blocks tab
        document.querySelectorAll("[data-library-tab]").forEach(t=>{ if((t.getAttribute('data-library-tab')||t.dataset.tab)==="blocks") t.click(); });
      });
      inner.replaceChildren();
      inner.append(element("span","muted", containerNode ? `${definitionFor(containerNode)?.displayName || "Container"} is empty` : "No blocks"), btn);
    } else {
      inner.textContent = containerNode ? "Drop blocks here" : "Drag blocks here to begin";
      if (!canAddInside) inner.textContent = "Maximum reached";
    }
    slot.append(inner);
    attachSlot(slot, containerNode, 0);
    // mark slot insertion target highlight
    if (state.insertionTarget && state.insertionTarget.parentId === (containerNode?containerNode.id:null) && state.insertionTarget.index===0) {
      slot.classList.add("drop-slot--active");
    }
    zone.append(slot);
    return zone;
  }
  siblings.forEach((child, index) => {
    // Only render insertion affordance if SDT boundary is legal
    if (hasLegalInsertion(containerNode, index)) {
      const slot = document.createElement("div");
      slot.className = "drop-slot drop-slot--insertion";
      const btn = element("button", "insertion-btn", "+");
      btn.type = "button";
      btn.title = "Add block here";
      btn.setAttribute("aria-label", "Add block here");
      btn.style.cssText = "width:100%;min-height:28px;height:28px;border:1px dashed transparent;background:transparent;color:#94a3b8;font-size:11px;border-radius:6px;cursor:pointer;opacity:0;transition:opacity .12s, border-color .12s; display:flex;align-items:center;justify-content:center;";
      btn.addEventListener("click", (e)=>{
        e.stopPropagation();
        const parentId = containerNode ? containerNode.id : null;
        setInsertionTarget({parentId, index});
        renderCatalog("");
        document.querySelectorAll("[data-library-tab]").forEach(t=>{ if((t.getAttribute('data-library-tab')||t.dataset.tab)==="blocks") t.click(); });
      });
      slot.addEventListener("mouseenter", ()=> btn.style.opacity="1");
      slot.addEventListener("mouseleave", ()=> { if (!slot.classList.contains("drop-slot--active")) btn.style.opacity="0"; });
      slot.addEventListener("focusin", ()=> btn.style.opacity="1");
      if (state.insertionTarget && state.insertionTarget.parentId === (containerNode?containerNode.id:null) && state.insertionTarget.index===index) {
        slot.classList.add("drop-slot--active");
        btn.style.opacity="1";
        btn.style.borderColor="#2563eb";
        btn.style.background="#eff6ff";
      }
      slot.append(btn);
      attachSlot(slot, containerNode, index);
      zone.append(slot);
    } else {
      // Still need a drop target for DnD (validation will reject) but no visual plus
      const slot = document.createElement("div");
      slot.className = "drop-slot drop-slot--insertion is-disabled";
      slot.style.minHeight = "4px";
      slot.style.height = "4px";
      // Attach for DnD feedback only (no button)
      attachSlot(slot, containerNode, index);
      // Hide when illegal to avoid christmas tree
      slot.style.display = "none";
      zone.append(slot);
    }
    zone.append(renderNode(child));
  });
  // Tail boundary
  if (hasLegalInsertion(containerNode, siblings.length)) {
    const tail = document.createElement("div");
    tail.className = "drop-slot drop-slot--insertion";
    const tailBtn = element("button", "insertion-btn", "+");
    tailBtn.type = "button";
    tailBtn.title = "Add block here";
    tailBtn.style.cssText = "width:100%;min-height:28px;height:28px;border:1px dashed transparent;background:transparent;color:#94a3b8;font-size:11px;border-radius:6px;cursor:pointer;opacity:0;transition:opacity .12s;display:flex;align-items:center;justify-content:center;";
    tailBtn.addEventListener("click", (e)=>{
      e.stopPropagation();
      const parentId = containerNode ? containerNode.id : null;
      setInsertionTarget({parentId, index:siblings.length});
      renderCatalog("");
      document.querySelectorAll("[data-library-tab]").forEach(t=>{ if((t.getAttribute('data-library-tab')||t.dataset.tab)==="blocks") t.click(); });
    });
    tail.addEventListener("mouseenter", ()=> tailBtn.style.opacity="1");
    tail.addEventListener("mouseleave", ()=> { if (!tail.classList.contains("drop-slot--active")) tailBtn.style.opacity="0"; });
    if (state.insertionTarget && state.insertionTarget.parentId === (containerNode?containerNode.id:null) && state.insertionTarget.index===siblings.length) {
      tail.classList.add("drop-slot--active");
      tailBtn.style.opacity="1";
      tailBtn.style.borderColor="#2563eb";
      tailBtn.style.background="#eff6ff";
    }
    tail.append(tailBtn);
    attachSlot(tail, containerNode, siblings.length);
    zone.append(tail);
  } else {
    const tail = document.createElement("div");
    tail.className = "drop-slot drop-slot--insertion is-disabled";
    tail.style.minHeight = "0";
    tail.style.display = "none";
    attachSlot(tail, containerNode, siblings.length);
    zone.append(tail);
  }
  return zone;
}

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
      if (val?.version === 1 && Array.isArray(val.content)) return val.content.map(r=>r.text||"").join("").slice(0,70);
      if (val) return String(val).slice(0,70);
    }
  }
  const block = def?.block || node.block;
  const p = node.props || {};
  const s = node.settings || {};
  if (block === "core/heading" || block === "core/entry-title") return p.text || "";
  if (block === "core/text") return (p.text||"").slice(0,70);
  if (block === "core/button") return p.label || "";
  if (block === "core/section") return s.width || "content";
  if (block === "core/stack") return `${s.direction||"vertical"} · ${s.gap||"md"}`;
  if (block === "core/grid") return `${s.columns||2} cols`;
  const v = Object.values(p).find(v=>typeof v==="string"&&v);
  return v? v.slice(0,70): "";
}
function renderNode(node) {
  const definition = definitionFor(node);
  const container = isContainer(node);
  const collapsed = state.collapsed.has(node.id);
  const wrapper = element("article", `node ${container?"node--container":"node--leaf"}${node.id===state.selectedNodeId?" is-selected":""}${collapsed?" is-collapsed":""}`);
  wrapper.dataset.nodeId = node.id;
  wrapper.draggable = true;
  wrapper.addEventListener("dragstart", (e)=>{
    currentDrag = { type:"node", nodeId: node.id };
    window.__stratum_currentDrag = currentDrag;
    clearDropUI();
    treeElement.classList.add("tree--dragging");
    e.dataTransfer.effectAllowed = "move";
    e.dataTransfer.setData("text/plain", node.id);
  });
  wrapper.addEventListener("dragend", ()=>{ currentDrag=null; window.__stratum_currentDrag=null; clearDropUI(); });
  const header = element("div","node__header");
  header.addEventListener("click", ()=>{
    state.selectedNodeId = node.id;
    state.selectedInstanceKey = null;
    if (canvasController) canvasController.selectNode(node.id, null);
    renderTree();
    renderInspector();
    updateBreadcrumbs();
  });
  header.append(element("span","node__drag","⋮⋮"));
  if (definition) {
    const badge = element("span","node__type", definition.displayName);
    badge.title = `${node.block}@${node.version}`;
    header.append(badge);
  }
  header.append(element("span","node__summary", nodeSummary(node)));
  const actions = element("span","node__actions");
  if (container && node.children.length>0){
    const btn = actionButton(collapsed?"▸":"▾", collapsed?"Expand":"Collapse", ()=>{
      if (state.collapsed.has(node.id)) state.collapsed.delete(node.id); else state.collapsed.add(node.id);
      renderTree();
    });
    btn.classList.add("node__collapse");
    actions.append(btn);
  }
  // Add inside button for containers — schema-legal only
  if (container) {
    const canInsertInside = hasLegalInsertion(node, node.children.length);
    let reason = "";
    if (!canInsertInside) {
      const def = definitionFor(node);
      if (def) {
        const rule = def.schema.children;
        if (rule.mode==="none") reason = `${def.displayName} does not allow child blocks.`;
        else if (rule.max!=null && node.children.length >= rule.max) reason = `${def.displayName} allows at most ${rule.max} child blocks.`;
        else if (rule.mode==="allowed" && rule.blocks && rule.blocks.length) reason = `${def.displayName} allows only ${rule.blocks.join(", ")}.`;
        else reason = "Cannot add here.";
      } else reason = "Cannot add here.";
    }
    const addInsideBtn = actionButton("+", canInsertInside? insertionLabelForContainer(node) : (reason || "Maximum reached"), ()=>{
      if (!canInsertInside) { showInsertionPrompt(reason || "Cannot add here."); return; }
      setInsertionTarget({parentId: node.id, index: node.children.length});
      renderCatalog("");
      document.querySelectorAll("[data-library-tab]").forEach(t=>{ if((t.getAttribute('data-library-tab')||t.dataset.tab)==="blocks") t.click(); });
    });
    if (!canInsertInside) { addInsideBtn.disabled=true; addInsideBtn.style.opacity="0.4"; addInsideBtn.title=reason; }
    actions.append(addInsideBtn);
  }
  // Duplicate with validation
  const dupCheck = canDuplicate(node.id);
  const dupBtn = actionButton("⧉","Duplicate", ()=>duplicateNode(node.id));
  if (!dupCheck.ok) { dupBtn.disabled=true; dupBtn.style.opacity="0.4"; dupBtn.title=dupCheck.reason; } else dupBtn.title="Duplicate";
  actions.append(dupBtn);
  const indentCheck = canIndentM(node.id);
  const indentBtn = actionButton("↳","Indent", ()=>indentNode(node.id));
  if (!indentCheck.ok) { indentBtn.disabled=true; indentBtn.style.opacity="0.4"; }
  indentBtn.title = indentCheck.ok ? "Indent" : indentCheck.reason;
  actions.append(indentBtn);
  const outdentCheck = canOutdentM(node.id);
  const outdentBtn = actionButton("↰","Outdent", ()=>outdentNode(node.id));
  if (!outdentCheck.ok) { outdentBtn.disabled=true; outdentBtn.style.opacity="0.4"; }
  outdentBtn.title = outdentCheck.ok ? "Outdent" : outdentCheck.reason;
  actions.append(outdentBtn);
  actions.append(actionButton("↑","Move up", ()=>moveNode(node.id,-1)));
  actions.append(actionButton("↓","Move down", ()=>moveNode(node.id,1)));
  const remCheck = canRemove(node.id);
  const remBtn = actionButton("✕","Remove", ()=>removeNode(node.id), true);
  if (!remCheck.ok) { remBtn.disabled=true; remBtn.style.opacity="0.4"; }
  remBtn.title = remCheck.ok ? "Remove" : remCheck.reason;
  actions.append(remBtn);
  actions.addEventListener("click", e=>e.stopPropagation());
  header.append(actions);
  wrapper.append(header);
  if (container && !collapsed) wrapper.append(renderDropZone(node.children, node));
  if (container && collapsed) wrapper.append(element("div","node__collapsed-hint", `${node.children.length} items`));
  return wrapper;
}
function actionButton(text,label,handler,danger){
  const b = element("button", danger?"node__action--danger":"", text);
  b.type="button"; b.title=label; b.setAttribute("aria-label",label);
  b.addEventListener("click", e=>{e.stopPropagation(); handler();});
  return b;
}
function clearDropUI(){
  if (!treeElement) return;
  treeElement.querySelectorAll(".drop-slot--active, .drop-slot--invalid").forEach(s=>s.classList.remove("drop-slot--active","drop-slot--invalid"));
  treeElement.querySelectorAll(".node--droptarget").forEach(n=>n.classList.remove("node--droptarget"));
  treeElement.classList.remove("tree--droptarget","tree--dragging");
}
function highlightContainer(containerNode){
  if (!containerNode){ treeElement.classList.add("tree--droptarget"); return; }
  const el = treeElement.querySelector(`.node[data-node-id="${containerNode.id}"]`);
  if (el) el.classList.add("node--droptarget");
}
function attachSlot(slot, containerNode, index){
  slot.addEventListener("dragover", (e)=>{
    if (!currentDrag && !window.__stratum_currentDrag) return;
    const drag = currentDrag || window.__stratum_currentDrag;
    e.preventDefault();
    const block = drag.type==="library"? drag.definition.block : definitionFor(findNode(drag.nodeId)?.node)?.block || findNode(drag.nodeId)?.node.block;
    const valid = containerAccepts(containerNode, block, drag) && !(drag.type==="node" && containerNode && isWithin(drag.nodeId, containerNode));
    slot.classList.remove("drop-slot--active","drop-slot--invalid");
    if (valid){ e.dataTransfer.dropEffect = drag.type==="library"?"copy":"move"; slot.classList.add("drop-slot--active"); highlightContainer(containerNode); }
    else { e.dataTransfer.dropEffect="none"; slot.classList.add("drop-slot--invalid"); }
  });
  slot.addEventListener("dragleave", ()=> slot.classList.remove("drop-slot--active","drop-slot--invalid"));
  slot.addEventListener("drop", (e)=>{
    e.preventDefault();
    slot.classList.remove("drop-slot--active","drop-slot--invalid");
    performDrop(currentDrag||window.__stratum_currentDrag, containerNode, index);
    clearDropUI();
  });
}
function blockForDrag(drag){
  if (!drag) return "";
  if (drag.type === "library") return drag.definition?.block || "";
  if (drag.type === "node") return findNode(drag.nodeId)?.node?.block || "";
  return "";
}
function canvasIndexAtY(nodes, y){
  for (let i = 0; i < nodes.length; i++) {
    const rect = canvasController?.boundsForNode(nodes[i].id);
    if (rect && y <= rect.top + rect.height / 2) return i;
  }
  return nodes.length;
}
function canvasRootIndicator(index){
  if (!state.document.nodes.length) return { rect: null, position: "root-empty" };
  if (index < state.document.nodes.length) {
    const rect = canvasController?.boundsForNode(state.document.nodes[index].id);
    if (rect) return { rect, position: "before" };
  }
  for (let i = state.document.nodes.length - 1; i >= 0; i--) {
    const rect = canvasController?.boundsForNode(state.document.nodes[i].id);
    if (rect) return { rect, position: "after" };
  }
  return { rect: null, position: "root-empty" };
}
function canvasExternalBoundaryTarget(y){
  if (!state.document.nodes.length) {
    return { containerNode: null, index: 0, ...canvasRootIndicator(0) };
  }
  const bounds = state.document.nodes.map((node) => canvasController?.boundsForNode(node.id)).filter(Boolean);
  if (!bounds.length) return null;
  const top = Math.min(...bounds.map((rect) => rect.top));
  const bottom = Math.max(...bounds.map((rect) => rect.bottom));
  if (y <= top) return { containerNode: null, index: 0, ...canvasRootIndicator(0) };
  if (y >= bottom) {
    const index = state.document.nodes.length;
    return { containerNode: null, index, ...canvasRootIndicator(index) };
  }
  return null;
}
function resolveCanvasDrop(e, drag){
  const block = blockForDrag(drag);
  if (!block || !canvasController) return { ok: false, reason: "missing" };
  const intent = canvasController.dropIntent(e.clientX, e.clientY);
  let target;
  if (!intent.hit) {
    const index = canvasIndexAtY(state.document.nodes, intent.y);
    target = { containerNode: null, index, ...canvasRootIndicator(index) };
  } else {
    const found = findNode(intent.hit.nodeId);
    if (!found || !intent.hit.editable) {
      target = canvasExternalBoundaryTarget(intent.y);
      if (!target) return { ok: false, reason: "external", rect: intent.rect, position: "inside" };
    } else {
      const canInsertInside = intent.position === "inside" && isContainer(found.node) && containerAccepts(found.node, block, drag);
      if (canInsertInside) {
        target = {
          containerNode: found.node,
          index: canvasIndexAtY(found.node.children || [], intent.y),
          rect: intent.rect,
          position: "inside",
        };
      } else {
        const before = intent.position === "before" || (intent.position === "inside" && intent.rect && intent.y < intent.rect.top + intent.rect.height / 2);
        target = {
          containerNode: found.parent,
          index: found.index + (before ? 0 : 1),
          rect: intent.rect,
          position: before ? "before" : "after",
        };
      }
    }
  }
  const validation = validateDropTarget(target.containerNode, block, drag, true);
  return { ...target, ok: validation.ok, reason: validation.reason };
}
function showCanvasDropError(reason){
  const message = reason === "external"
    ? "Cannot drop inside read-only header, footer, or template content. Use the blue line before or after editable content."
    : "This block is not allowed at that position.";
  if (errorElement) {
    errorElement.textContent = message;
    errorElement.hidden = false;
    setTimeout(() => { if (errorElement.textContent === message) errorElement.hidden = true; }, 3500);
  } else if (window.stratumToast) {
    window.stratumToast("error", message);
  }
}
function performDrop(drag, containerNode, index){
  if (!drag) return;
  const block = blockForDrag(drag);
  if (!block) return;
  const editable = true;
  const v = validateDropTarget(containerNode, block, drag, editable);
  if (!v.ok) { showInsertionPrompt(v.reason==="external" ? "Cannot drop inside read-only content." : "Not allowed at that position."); return; }
  if (!containerAccepts(containerNode, block, drag)) { showInsertionPrompt("Not allowed at that position."); return; }
  // Additional mutation validation for source min
  if (drag.type==="node") {
    const mv = canMove(drag.nodeId, containerNode, index);
    if (!mv.ok) { showInsertionPrompt(mv.reason); return; }
  } else {
    const ins = canInsert(containerNode, block, index);
    if (!ins.ok) { showInsertionPrompt(ins.reason); return; }
  }
  const target = containerNode? containerNode.children : state.document.nodes;
  let nodeId = null;
  const ok = commitMutation(()=>{
    if (drag.type==="library"){
      const node = createValidNode(drag.definition);
      target.splice(index,0,node);
      nodeId = node.id;
    } else {
      const found = findNode(drag.nodeId);
      if (!found) return false;
      if (containerNode && isWithin(drag.nodeId, containerNode)) return false;
      if (found.parent===containerNode && found.index===index) return false;
      const from = found.siblings;
      const [moved] = from.splice(found.index,1);
      let ti=index;
      if (found.parent===containerNode && found.index < index) ti-=1;
      target.splice(ti,0,moved);
      nodeId = moved.id;
    }
  });
  if (!ok) return;
  clearInsertionTarget();
  state.selectedNodeId=nodeId;
  state.selectedInstanceKey=null;
  state.__pendingScrollToId = nodeId;
  if (canvasController) canvasController.selectNode(nodeId, null, { scroll: false });
  changed();
  if (canvasController) setTimeout(()=>canvasController.scrollToNode(nodeId, "smooth"), 60);
}
window.__stratum_performNavigatorDrop = (drag, targetNode, position)=>{
  if (!drag) return;
  const block = drag.type==="library"? drag.definition.block : findNode(drag.nodeId)?.node.block;
  const targetInfo = findNode(targetNode.id);
  if (!targetInfo) return;
  // position can be "before", "inside", "after"
  if (position === "inside" && isContainer(targetNode) && canInsert(targetNode, block, targetNode.children.length, drag).ok) {
    performDrop(drag, targetNode, targetNode.children.length);
  } else if (position === "before") {
    performDrop(drag, targetInfo.parent, targetInfo.index);
  } else if (position === "after") {
    performDrop(drag, targetInfo.parent, targetInfo.index+1);
  } else {
    // legacy fallback
    const def = definitionFor(targetNode);
    if (def && isContainer(targetNode) && containerAccepts(targetNode, block, drag)) {
      performDrop(drag, targetNode, targetNode.children.length);
    } else {
      performDrop(drag, targetInfo.parent, targetInfo.index+1);
    }
  }
};

// Node operations
function addBlock(definition){
  const point = insertionPointForBlock(definition.block);
  if (!point) {
    showInsertionPrompt("Choose where to add this block.");
    // Only auto-target root when document is empty — otherwise require explicit placement
    if (state.document.nodes.length === 0) {
      const canRoot = canInsert(null, definition.block, 0);
      if (canRoot.ok) {
        setInsertionTarget({parentId:null, index: 0});
        renderCatalog("");
      }
    } else {
      // Highlight valid slots via insertion affordances; user must click a drop zone / Add button
      if (window.__stratum_renderCatalog) window.__stratum_renderCatalog("");
      if (canvasController) canvasController.renderOverlays();
    }
    return;
  }
  let newId = null;
  const ok = commitMutation(()=>{
    const node = createValidNode(definition);
    point.siblings.splice(point.index,0,node);
    newId = node.id;
  });
  if (!ok) { showInsertionPrompt("Could not add block."); return; }
  clearInsertionTarget();
  state.selectedNodeId=newId;
  state.__pendingScrollToId = newId;
  if (canvasController) canvasController.selectNode(newId, null, { scroll: false });
  changed();
  // ensure canvas scrolls to new node after render (before preview)
  if (canvasController) setTimeout(()=>canvasController.scrollToNode(newId, "smooth"), 60);
}
function moveNode(id, offset){
  const found=findNode(id); if(!found) return;
  const next=found.index+offset; if(next<0||next>=found.siblings.length) return;
  const ok = commitMutation(()=>{
    [found.siblings[found.index], found.siblings[next]]=[found.siblings[next], found.siblings[found.index]];
  });
  if (!ok) return;
  state.selectedNodeId=id;
  state.selectedInstanceKey=null;
  changed();
}
function canIndent(id){
  const r = canIndentM(id);
  return r.ok;
}
function indentNode(id){
  const check = canIndentM(id);
  if (!check.ok) { showInsertionPrompt(check.reason); return; }
  const found=findNode(id); if(!found) return;
  const prev=found.siblings[found.index-1];
  const ok = commitMutation(()=>{
    const [moved]=found.siblings.splice(found.index,1);
    prev.children.push(moved);
  });
  if (!ok) return;
  changed();
}
function canOutdent(id){
  const r = canOutdentM(id);
  return r.ok;
}
function outdentNode(id){
  const check = canOutdentM(id);
  if (!check.ok) { showInsertionPrompt(check.reason); return; }
  const found=findNode(id); if(!found||!found.parent) return;
  const parentFound=findNode(found.parent.id); if(!parentFound) return;
  const newContainer=parentFound.parent;
  const ok = commitMutation(()=>{
    const [moved]=found.siblings.splice(found.index,1);
    const target=newContainer? newContainer.children : state.document.nodes;
    target.splice(parentFound.index+1,0,moved);
  });
  if (!ok) return;
  changed();
}
function removeNode(id){
  const check = canRemove(id);
  if (!check.ok) { showInsertionPrompt(check.reason); return; }
  const found=findNode(id); if(!found) return;
  const hasChildren = (found.node.children||[]).length>0 || (found.node.children||[]).some(n=> (n.children||[]).length>0);
  if (hasChildren){
    const def=definitionFor(found.node);
    const label=def?.displayName||found.node.block;
    if(!confirm(`Remove "${label}" and all its children?`)) return;
  }
  const ok = commitMutation(()=>{
    found.siblings.splice(found.index,1);
  });
  if (!ok) return;
  state.collapsed.delete(id);
  if(state.selectedNodeId===id) state.selectedNodeId=found.parent?.id||null;
  changed();
}
function duplicateNode(id){
  const check = canDuplicate(id);
  if (!check.ok) { showInsertionPrompt(check.reason); return; }
  const found=findNode(id); if(!found) return;
  let dupeId = null;
  const ok = commitMutation(()=>{
    const dupe=duplicateSubtree(found.node);
    found.siblings.splice(found.index+1,0,dupe);
    dupeId = dupe.id;
  });
  if (!ok) return;
  state.selectedNodeId=dupeId;
  state.__pendingScrollToId = dupeId;
  changed();
  if (canvasController) setTimeout(()=>canvasController.scrollToNode(dupeId, "smooth"), 60);
}
function insertPattern(patternId){
  const pattern=state.patterns.find(p=>p.id===patternId);
  if(!pattern) return;
  const cloned=clonePatternNodes(pattern);
  if(!cloned.length) return;
  // Use insertionTarget if set
  let point = null;
  if (state.insertionTarget) {
    const resolved = resolveInsertionTarget(state.insertionTarget);
    if (resolved) {
      const can = canInsertRootsM(resolved.parentNode, cloned);
      if (can.ok) point = resolved;
      else { showInsertionPrompt(can.reason || "Cannot insert pattern here."); return; }
    }
  }
  if (!point) point=insertionPointForPattern(cloned);
  if(!point){
    showInsertionPrompt("Cannot insert pattern here — container does not allow this content.");
    return;
  }
  // Validate again atomic
  const parentForPoint = (() => {
    // find parentNode from point.siblings
    if (point.siblings === state.document.nodes) return null;
    // search for parent that owns siblings
    function findParent(nodes, targetSibs) {
      for (const n of nodes) {
        if (n.children === targetSibs) return n;
        const deeper = findParent(n.children||[], targetSibs);
        if (deeper) return deeper;
      }
      return null;
    }
    return findParent(state.document.nodes, point.siblings);
  })();
  const can = canInsertRootsM(parentForPoint, cloned);
  if (!can.ok) { showInsertionPrompt(can.reason); return; }
  let firstId = cloned[0].id;
  const ok = commitMutation(()=>{
    for(let i=0;i<cloned.length;i++) point.siblings.splice(point.index+i,0,cloned[i]);
  });
  if (!ok) return;
  clearInsertionTarget();
  state.selectedNodeId=firstId;
  state.__pendingScrollToId = firstId;
  changed();
  if (canvasController) setTimeout(()=>canvasController.scrollToNode(firstId, "smooth"), 60);
}

// Changed
function changed(options={}){
  documentInput.value=JSON.stringify(state.document);
  updateDirty();
  if(options.tree!==false) renderTree();
  if(options.inspector!==false) renderInspector();
  if (options.preview!==false) schedulePreview();
}
function renderAll(options={}){
  documentInput.value=JSON.stringify(state.document);
  renderTree();
  renderInspector();
  if(options.preview) schedulePreview();
}
function markSaved(){ syncBaseline(); if(dirtyElement){dirtyElement.textContent="Saved"; dirtyElement.className="editor-status is-saved";} }

// Preview handled via preview.js schedulePreview/updatePreview

// Inspector already imported

// Setup
function setupMetadataListeners(){
  const metadataFields = ["entry-title","entry-slug","entry-excerpt","entry-seo-title","entry-seo-description","entry-canonical-url","entry-seo-robots-index","entry-seo-robots-follow","entry-schema-mode","template-name","site-part-name"];
  metadataFields.forEach(id=>{
    const el=document.getElementById(id);
    if(!el) return;
    const evt=el.tagName==="SELECT"?"change":"input";
    el.addEventListener(evt, ()=>{
      updateDirty();
      clearTimeout(metadataTimer);
      metadataTimer=setTimeout(()=>schedulePreview(),600);
    });
  });
  const layoutEl=document.getElementById("entry-layout-template");
  if(layoutEl) layoutEl.addEventListener("change", ()=>{ updateDirty(); schedulePreview(); });
  form.addEventListener("input", ()=>updateDirty());
  form.addEventListener("change", ()=>updateDirty());
}
function setupSlug(){
  const title=document.getElementById("entry-title");
  const slug=document.getElementById("entry-slug");
  if(!title||!slug) return;
  const normalizeSlug=v=> v.toLowerCase().replace(/ą/g,"a").replace(/ć/g,"c").replace(/ę/g,"e").replace(/ł/g,"l").replace(/ń/g,"n").replace(/ó/g,"o").replace(/ś/g,"s").replace(/ź/g,"z").replace(/ż/g,"z").replace(/æ/g,"ae").replace(/ø/g,"o").replace(/ß/g,"ss").normalize("NFD").replace(/[\u0300-\u036f]/g,"").replace(/[^a-z0-9]+/g,"-").replace(/^-+|-+$/g,"").slice(0,100).replace(/-+$/g,"");
  let lastAuto=normalizeSlug(title.value);
  let manualEdit=slug.value!=="" && slug.value!==lastAuto;
  if(slug.value.trim()==="" && lastAuto!==""){ slug.value=lastAuto; manualEdit=false; }
  const ensureSlug=()=>{
    if(slug.value.trim()==="" && title.value.trim()!==""){
      slug.value=normalizeSlug(title.value)||"item";
      lastAuto=slug.value; manualEdit=false;
    }
  };
  title.addEventListener("input", ()=>{
    if(!manualEdit||slug.value===lastAuto){ slug.value=normalizeSlug(title.value); lastAuto=slug.value; manualEdit=false; }
  });
  slug.addEventListener("input", ()=>{ manualEdit=slug.value!==normalizeSlug(title.value); });
  form.addEventListener("submit", ()=>ensureSlug());
  document.querySelectorAll('button[data-on*="@post"]').forEach(btn=>{ btn.addEventListener("click", ()=>ensureSlug(), true); });
}

// Keyboard + beforeunload + observers same as old
document.addEventListener("keydown", (e)=>{
  const mod=e.metaKey||e.ctrlKey;
  if(mod && e.key==="s"){ e.preventDefault(); documentInput.value=JSON.stringify(state.document); const saveBtn=form.querySelector('button[name="save"]')||form.querySelector('button[data-on*="@post"]'); if(saveBtn) saveBtn.click(); else form.requestSubmit(); }
  if(mod && e.key==="z" && !e.shiftKey){ e.preventDefault(); if(undo()){ renderTree(); renderInspector(); updateBreadcrumbs(); updateDirty(); schedulePreview(); } }
  if(mod && (e.key==="Z" || (e.key==="z" && e.shiftKey) || e.key==="y")){ e.preventDefault(); if(redo()){ renderTree(); renderInspector(); updateBreadcrumbs(); updateDirty(); schedulePreview(); } }
});
window.addEventListener("beforeunload", (e)=>{
  if (isDirtyNow()) {
    e.preventDefault();
    e.returnValue = "";
  }
});
(function observeSave(){
  const statusRegion=document.getElementById("editor-status-region");
  if(!statusRegion) return;
  const observer=new MutationObserver(()=>{
    const dirty=document.getElementById("editor-dirty");
    if(!dirty) return;
    const txt=dirty.textContent.trim();
    if(txt==="Saved"||txt==="Published"){
      // sync baseline after successful save
      setTimeout(()=>syncBaseline(),0);
    }
  });
  observer.observe(statusRegion,{childList:true, subtree:true, characterData:true});
})();
(function setupSubmitGuards(){
  const saveBtn=form.querySelector('button[name="save"]');
  const publishBtn=form.querySelector('button[name="publish"]');
  const buttons=[saveBtn,publishBtn].filter(Boolean);
  form.querySelectorAll('button[type="submit"]').forEach(b=>{if(!buttons.includes(b)) buttons.push(b);});
  buttons.forEach(btn=>{
    btn.addEventListener("click", ()=>{
      const orig=btn.textContent;
      if(btn.dataset.busy==="1") return;
      btn.dataset.busy="1"; btn.dataset.origText=orig;
      if(btn.name==="publish"||btn.textContent.toLowerCase().includes("publish")) btn.textContent="Publishing…";
      else btn.textContent="Saving…";
      btn.disabled=true;
      const statusRegion=document.getElementById("editor-status-region");
      const errorEl=document.getElementById("editor-error");
      let timeout=setTimeout(()=>{btn.disabled=false; btn.textContent=orig; btn.dataset.busy="";},8000);
      const obs=new MutationObserver(()=>{
        const dirty=document.getElementById("editor-dirty");
        const errVisible=errorEl && !errorEl.hidden && errorEl.textContent.trim()!=="";
        const isSaved=dirty && (dirty.textContent.trim()==="Saved"||dirty.textContent.trim()==="Published");
        if(errVisible||isSaved){ clearTimeout(timeout); btn.disabled=false; btn.textContent=orig; btn.dataset.busy=""; obs.disconnect(); }
      });
      if(statusRegion) obs.observe(statusRegion,{childList:true, subtree:true, characterData:true});
      if(errorEl) obs.observe(errorEl,{attributes:true, childList:true, characterData:true, subtree:true});
      const onDatastar=()=>{
        clearTimeout(timeout);
        setTimeout(()=>{ if(btn.dataset.busy==="1"){btn.disabled=false; btn.textContent=orig; btn.dataset.busy="";} obs.disconnect(); document.removeEventListener('datastar-patch', onDatastar); },400);
      };
      document.addEventListener('datastar-patch', onDatastar,{once:true});
      setTimeout(()=>document.removeEventListener('datastar-patch', onDatastar),8000);
    });
  });
})();

const blockSearchEl=document.getElementById("block-search");
if(blockSearchEl) blockSearchEl.addEventListener("input", (e)=>{
  const v=e.target.value;
  if (state.libraryTab === "navigator") {
    const q = v.trim().toLowerCase();
    const container = document.getElementById("navigator-content");
    if (!container) return;
    if (!q) {
      // clear filter: restore collapsed state and re-render
      container.querySelectorAll(".navigator-node").forEach(n=>{ n.style.display=""; n.style.outline=""; });
      renderNavigator();
      return;
    }
    // Collect matched ids via document traversal (covers collapsed nodes)
    const matchedIds = new Set();
    const ancestors = new Map(); // nodeId -> parentId
    function walk(nodes, parentId) {
      for (const n of nodes) {
        if (parentId) ancestors.set(n.id, parentId);
        // check summary/type
        const def = definitionFor(n);
        const txt = `${def ? def.displayName : n.block} ${(() => {
          // replicate nodeSummary quickly
          const p = n.props || {};
          const s = n.settings || {};
          if (n.block==="core/heading"||n.block==="core/entry-title") return p.text||"";
          if (n.block==="core/text") return (p.text||"").slice(0,70);
          if (n.block==="core/button") return p.label||"";
          return "";
        })()} ${n.block}`.toLowerCase();
        if (txt.includes(q)) {
          matchedIds.add(n.id);
          // add ancestors
          let cur = n.id;
          while (ancestors.has(cur)) {
            const par = ancestors.get(cur);
            matchedIds.add(par);
            // expand
            if (state.collapsed.has(par)) state.collapsed.delete(par);
            cur = par;
          }
        }
        if (n.children) walk(n.children, n.id);
      }
    }
    walk(state.document.nodes, null);
    // Re-render to expand ancestors
    renderNavigator();
    // Highlight matches and hide non-matched
    const allNodes = container.querySelectorAll(".navigator-node");
    allNodes.forEach(n=>{
      const id = n.dataset.nodeId;
      if (matchedIds.has(id)) {
        n.style.outline = "1px solid #93c5fd";
        n.style.display = "";
      } else {
        // hide if not ancestor of match and not matched
        n.style.display = "none";
        n.style.outline = "";
      }
    });
    // Ensure ancestors of matches are visible even if they were hidden
    matchedIds.forEach(id=>{
      const el = container.querySelector(`[data-node-id="${CSS.escape ? CSS.escape(id) : id}"]`);
      if (el) el.style.display = "";
    });
  } else {
    renderCatalog(v); renderPatternCatalog(v);
  }
});
form.addEventListener("submit", ()=>{
  documentInput.value=JSON.stringify(state.document);
  syncBaseline();
});
document.addEventListener("click", (e)=>{
  if(e.target.matches(".editor-mode-btn")){
    const mode=e.target.dataset.mode;
    state.mode=mode;
    const workspace=document.querySelector(".editor-workspace");
    const previewPanel=document.querySelector(".editor-preview-panel");
    if(mode==="preview"){ if(workspace) workspace.style.display="none"; if(previewPanel) previewPanel.style.display="block"; schedulePreview(); }
    else { if(workspace) workspace.style.display=""; if(previewPanel) previewPanel.style.display=""; }
    document.querySelectorAll(".editor-mode-btn").forEach(btn=> btn.classList.toggle("is-active", btn.dataset.mode===mode));
  }
  if(e.target.matches(".editor-device-btn")){
    const w=e.target.dataset.width;
    state.previewWidth=w;
    document.querySelectorAll(".editor-device-btn").forEach(btn=> btn.classList.toggle("is-active", btn.dataset.width===w));
    const frame=previewElement.querySelector("iframe");
    if(frame) frame.style.width=w;
    if(canvasEl) {
      const wrap=document.getElementById("editor-canvas-wrap");
      const stage=document.getElementById("editor-canvas-stage");
      const target = stage || wrap;
      if(w==="100%"){ if(target) target.style.maxWidth="100%"; if(stage) stage.style.maxWidth="100%"; canvasEl.style.width="100%"; }
      else { if(target) target.style.maxWidth=w; if(stage) stage.style.maxWidth=w; canvasEl.style.width="100%"; }
      if(canvasController) setTimeout(()=>canvasController.updateOverlayPositions(), 100);
    }
  }
  if(e.target.matches("[data-inspector-tab]") || e.target.matches(".inspector-tab")){
    const tab=e.target.getAttribute('data-inspector-tab') || e.target.dataset.tab;
    const container = document.querySelector('[data-inspector-tabs]');
    const scope = container || document;
    scope.querySelectorAll("[data-inspector-tab], .inspector-tab").forEach(b=>{ b.classList.remove("is-active"); b.setAttribute("aria-selected","false"); });
    e.target.classList.add("is-active"); e.target.setAttribute("aria-selected","true");
    const blockPanel=document.getElementById("inspector-block-panel");
    const docPanel=document.getElementById("inspector-document-panel");
    if(tab==="document"){ if(blockPanel) blockPanel.hidden=true; if(docPanel) docPanel.hidden=false; renderDocumentInspector(); }
    else { if(blockPanel) blockPanel.hidden=false; if(docPanel) docPanel.hidden=true; }
  }
  if(e.target.id==="canvas-undo"){ if(undo()){ renderTree(); renderInspector(); updateBreadcrumbs(); updateDirty(); schedulePreview(); if(canvasController) canvasController.refresh(); } }
  if(e.target.id==="canvas-redo"){ if(redo()){ renderTree(); renderInspector(); updateBreadcrumbs(); updateDirty(); schedulePreview(); if(canvasController) canvasController.refresh(); } }
});

function renderDocumentInspector(){
  const slot=document.getElementById("document-inspector-slot");
  if(!slot) return;
  slot.replaceChildren();
  const info=document.createElement("div");
  info.className="document-inspector";
  const res=bootstrap.resource || {};
  const h3 = document.createElement("h3");
  h3.style.margin = "0 0 8px";
  h3.textContent = "About";
  const typeLabel = res.contentTypeId ? res.contentTypeId.charAt(0).toUpperCase()+res.contentTypeId.slice(1) : (res.type==="entry"? "Page":"Document");
  const statusEl = document.getElementById("editor-publish-state");
  const statusText = statusEl ? statusEl.textContent.trim() : "";
  const pubPath = document.getElementById("entry-slug") ? ("/"+(document.getElementById("entry-slug").value||"")) : "";
  const meta = document.createElement("div");
  meta.style.display = "grid";
  meta.style.gap = "4px";
  meta.style.fontSize = "13px";
  meta.style.marginTop = "6px";
  const row1 = document.createElement("div");
  const strong = document.createElement("strong");
  strong.style.fontWeight = "600";
  strong.textContent = typeLabel;
  const span = document.createElement("span");
  span.style.color = "#64748b";
  span.textContent = ` · ${statusText||"Draft"}`;
  row1.append(strong, span);
  const row2 = document.createElement("div");
  row2.style.fontSize = "12px";
  row2.style.color = "#64748b";
  row2.style.wordBreak = "break-all";
  row2.textContent = pubPath || (res.label || "");
  meta.append(row1, row2);
  info.append(h3, meta);
  if (res.label) {
    const labelRow = document.createElement("div");
    labelRow.style.fontSize = "12px";
    labelRow.style.marginTop = "6px";
    labelRow.style.color = "#334155";
    labelRow.textContent = res.label;
    info.append(labelRow);
  }
  const p2 = document.createElement("p");
  p2.className = "muted";
  p2.style.marginTop = "12px";
  p2.style.fontSize = "11px";
  p2.textContent = "Inspector Document shows publishing, URL and theme settings. Visual canvas is primary; Inspector Block tab edits selected block.";
  info.append(p2);
  slot.append(info);
  // Also try to move legacy Settings tab content if present (for entry) — grouped per AB spec
  const settingsTab=document.getElementById("tab-settings");
  if(settingsTab){
    // Avoid empty Classification — hide if no taxonomy panels
    const hasTaxonomy = settingsTab.querySelectorAll('[name^="taxonomy_"]').length > 0 || settingsTab.querySelector('.taxonomy-hierarchy');
    if(!hasTaxonomy){
      const classSection = settingsTab.querySelector('section h3');
      // Find Classification section and hide if empty
      settingsTab.querySelectorAll('section').forEach(sec=>{
        const h=sec.querySelector('h3');
        if(h && h.textContent.trim()==='Classification'){
          // Check if inner has panels
          if(sec.querySelectorAll('.taxonomy-hierarchy, [name^="taxonomy_"]').length===0){
            // If original empty state not rendered, leave hidden via slot clone logic
          }
        }
      });
    }
    const clone=settingsTab.cloneNode(true);
    clone.hidden=false;
    clone.id="doc-settings-clone";
    clone.style.display="grid";
    clone.style.gap="16px";
    // Remove duplicate ids and disable inputs to avoid duplicate submit
    clone.querySelectorAll("[id]").forEach(el=> el.id = el.id + "-clone");
    clone.querySelectorAll("[name]").forEach(el=> { el.disabled = true; el.setAttribute("data-clone-disabled","1"); });
    // Replace submit buttons with links to Settings tab
    clone.querySelectorAll('button[type="submit"], button[data-on*="@post"]').forEach(btn=>{
      const link = document.createElement("a");
      link.href = "#settings";
      link.className = btn.className || "button button-small";
      link.textContent = btn.textContent.trim() || "Edit in Settings";
      link.style.display = "inline-flex";
      link.style.alignItems = "center";
      link.addEventListener("click", (e)=>{ e.preventDefault(); document.getElementById("tab-btn-settings")?.click(); });
      btn.replaceWith(link);
    });
    slot.append(document.createElement("hr"));
    const settingsHeader=document.createElement("h3");
    settingsHeader.textContent="Publishing & Settings";
    settingsHeader.style.margin="12px 0 0";
    slot.append(settingsHeader);
    const settingsHint=document.createElement("p");
    settingsHint.className="muted";
    settingsHint.style.fontSize="11px";
    settingsHint.textContent="Read-only summary — use Settings tab to edit.";
    slot.append(settingsHint);
    slot.append(clone);
  }
  // SEO panel — compact redesign, clone into drawer as well (deduped)
  const seoTab=document.getElementById("tab-seo");
  if(seoTab){
    const seoClone=seoTab.cloneNode(true);
    seoClone.hidden=false;
    seoClone.id="doc-seo-clone";
    seoClone.style.display="grid";
    seoClone.style.gap="12px";
    seoClone.querySelectorAll("[id]").forEach(el=> el.id = el.id + "-clone");
    seoClone.querySelectorAll("[name]").forEach(el=> { el.disabled = true; });
    seoClone.querySelectorAll('button[type="submit"], button[data-on*="@post"]').forEach(btn=>{
      const link = document.createElement("a");
      link.href = "#seo";
      link.className = btn.className || "button button-small";
      link.textContent = btn.textContent.trim() || "Edit in SEO";
      link.addEventListener("click", (e)=>{ e.preventDefault(); document.getElementById("tab-btn-seo")?.click(); });
      btn.replaceWith(link);
    });
    slot.append(document.createElement("hr"));
    const seoHeader=document.createElement("h3");
    seoHeader.textContent="SEO";
    seoHeader.style.margin="12px 0 0";
    slot.append(seoHeader);
    const seoHint=document.createElement("p");
    seoHint.className="muted";
    seoHint.style.fontSize="11px";
    seoHint.textContent="Read-only — edit in SEO tab.";
    slot.append(seoHint);
    slot.append(seoClone);
  }
  // Sharing — move Share draft preview into Document → Sharing (AA)
  const shareSection=document.getElementById("share-preview-section");
  const shareDataEl=document.getElementById("share-preview-data");
  let shareClone=null;
  if(shareSection){
    shareClone=shareSection.cloneNode(true);
    shareClone.hidden=false;
    shareClone.id="doc-share-clone";
    shareClone.style.display="grid";
    shareClone.style.gap="10px";
    shareClone.style.marginTop="12px";
    shareClone.style.borderTop="1px solid var(--ui-border)";
    shareClone.style.paddingTop="12px";
    shareClone.querySelectorAll("[id]").forEach(el=> el.id = el.id + "-clone");
    shareClone.querySelectorAll("[name]").forEach(el=> { el.disabled=true; });
    // Replace forms inside clone with disabled representation to avoid nested form submit
    shareClone.querySelectorAll("form").forEach(f=>{
      f.querySelectorAll("[name]").forEach(el=> el.disabled=true);
      f.querySelectorAll('button').forEach(btn=> btn.disabled=true);
    });
    const entryId = document.getElementById('entry-id')?.value || bootstrap.resource?.id || "";
    if(!entryId){
      const msg=document.createElement("p");
      msg.className="muted";
      msg.style.fontSize="12px";
      msg.textContent="Save this draft before creating a share link.";
      shareClone.replaceChildren(msg);
      const h=document.createElement("h4");
      h.textContent="Sharing";
      shareClone.prepend(h);
    } else {
      const h=shareClone.querySelector('h3');
      if(!h){
        const nh=document.createElement("h4");
        nh.textContent="Sharing";
        shareClone.prepend(nh);
      } else {
        h.textContent="Sharing";
      }
      const hint=document.createElement("p");
      hint.className="muted";
      hint.style.fontSize="11px";
      hint.textContent="Use Sharing section above (Settings tab) to manage links.";
      shareClone.append(hint);
    }
    slot.append(document.createElement("hr"));
    slot.append(shareClone);
  } else if(shareDataEl){
    try{
      const data=JSON.parse(shareDataEl.textContent||"{}");
      const entryId=data.entryId||document.getElementById('entry-id')?.value||bootstrap.resource?.id||"";
      const csrf=data.csrfToken||document.querySelector('input[name="csrf_token"]')?.value||"";
      const links=data.previewLinks||[];
      const container=document.createElement("div");
      container.id="doc-share-clone";
      container.style.display="grid";
      container.style.gap="10px";
      container.style.marginTop="12px";
      container.style.borderTop="1px solid var(--ui-border)";
      container.style.paddingTop="12px";
      const h4=document.createElement("h4");
      h4.textContent="Sharing";
      container.append(h4);
      if(!entryId){
        const p=document.createElement("p");
        p.className="muted";
        p.style.fontSize="12px";
        p.textContent="Save this draft before creating a share link.";
        container.append(p);
      } else {
        const p=document.createElement("p");
        p.className="form-help";
        p.textContent="Create a temporary public link to share this draft without login.";
        container.append(p);
        // Use div + button (not nested <form> inside outer editor form) — avoid invalid HTML nesting
        const controls=document.createElement("div");
        controls.style.display="flex";
        controls.style.gap="8px";
        controls.style.alignItems="flex-end";
        controls.style.flexWrap="wrap";
        const label=document.createElement("label");
        label.textContent="Expires ";
        const sel=document.createElement("select");
        sel.className="form-control"; sel.style.width="auto";
        sel.id="preview-link-expires-clone";
        [["1h","1 hour"],["24h","24 hours"],["7d","7 days"],["30d","30 days"]].forEach(([v,lbl])=>{
          const opt=document.createElement("option");
          opt.value=v; opt.textContent=lbl; if(v==="24h") opt.selected=true;
          sel.append(opt);
        });
        label.append(sel);
        const btn=document.createElement("button");
        btn.type="button"; btn.className="button"; btn.textContent="Create link";
        btn.addEventListener("click", async ()=>{
          btn.disabled=true; btn.textContent="Creating…";
          try {
            const params=new URLSearchParams({ csrf_token: csrf, entry_id: entryId, expires: sel.value });
            const res=await fetch("/admin/preview-links", { method:"POST", headers:{"Content-Type":"application/x-www-form-urlencoded"}, body: params.toString() });
            if(!res.ok) throw new Error(await res.text());
            if(window.stratumToast) window.stratumToast("success","Preview link created");
            // trigger datastar or reload preview links section
            setTimeout(()=> location.reload(), 500);
          } catch(e){
            if(window.stratumToast) window.stratumToast("error", String(e.message||e));
            btn.disabled=false; btn.textContent="Create link";
          }
        });
        controls.append(label, btn);
        container.append(controls);
        if(links.length){
          const h5=document.createElement("h4");
          h5.textContent="Active links";
          container.append(h5);
          const ul=document.createElement("ul");
          ul.style.listStyle="none";
          ul.style.padding="0";
          ul.style.display="grid";
          ul.style.gap="8px";
          links.forEach(l=>{
            const li=document.createElement("li");
            li.style.display="flex";
            li.style.gap="8px";
            li.style.alignItems="center";
            li.style.justifyContent="space-between";
            li.style.padding="8px";
            li.style.border="1px solid var(--ui-border)";
            const span=document.createElement("span");
            span.style.fontSize="13px";
            span.textContent=`Created ${l.createdAt} · Expires ${l.expiresAt}`;
            const revokeBtn=document.createElement("button");
            revokeBtn.className="button button-small button-danger";
            revokeBtn.type="button";
            revokeBtn.textContent="Revoke";
            revokeBtn.addEventListener("click", async ()=>{
              revokeBtn.disabled=true;
              try {
                const params=new URLSearchParams({ csrf_token: csrf });
                const res=await fetch(`/admin/preview-links/${l.id}/revoke`, { method:"POST", headers:{"Content-Type":"application/x-www-form-urlencoded"}, body: params.toString() });
                if(!res.ok) throw new Error(await res.text());
                if(window.stratumToast) window.stratumToast("success","Link revoked");
                li.remove();
              } catch(e){
                if(window.stratumToast) window.stratumToast("error", String(e.message||e));
                revokeBtn.disabled=false;
              }
            });
            li.append(span, revokeBtn);
            ul.append(li);
          });
          container.append(ul);
        } else {
          const p2=document.createElement("p");
          p2.className="muted";
          p2.style.fontSize="13px";
          p2.textContent="No active preview links.";
          container.append(p2);
        }
      }
      slot.append(document.createElement("hr"));
      slot.append(container);
    }catch(e){}
  }
}

// Init
function setupPanelCollapsibles(){
  const library = document.querySelector(".block-library");
  const inspector = document.querySelector(".block-inspector");
  const workspace = document.querySelector(".editor-workspace");
  // restore from localStorage
  try {
    if (localStorage.getItem("stratum:panel-library-collapsed")==="1" && library) library.classList.add("is-collapsed");
    if (localStorage.getItem("stratum:panel-inspector-collapsed")==="1" && inspector) inspector.classList.add("is-collapsed");
  } catch(_) {}
  function toggle(panel, key){
    if (!panel) return;
    const collapsed = panel.classList.toggle("is-collapsed");
    try { localStorage.setItem(key, collapsed?"1":"0"); } catch(_) {}
    // trigger overlay recalc
    if (canvasController) setTimeout(()=>canvasController.updateOverlayPositions(), 50);
  }
  document.querySelectorAll('[data-collapse="library"], [data-panel-toggle="library"]').forEach(btn=>{
    btn.addEventListener("click", ()=> toggle(library, "stratum:panel-library-collapsed"));
  });
  document.querySelectorAll('[data-collapse="inspector"], [data-panel-toggle="inspector"]').forEach(btn=>{
    btn.addEventListener("click", ()=> toggle(inspector, "stratum:panel-inspector-collapsed"));
  });
  // drag autoscroll for canvas and navigator
  const scroller = document.getElementById("editor-canvas-wrap");
  const navTree = document.getElementById("navigator-tree");
  function setupAutoScroll(el){
    if (!el) return;
    let raf = null;
    let dir = 0;
    function loop(){
      if (dir !== 0) {
        el.scrollBy(0, dir*8);
        if (canvasController) canvasController.updateOverlayPositions();
      }
      raf = requestAnimationFrame(loop);
    }
    el.addEventListener("dragover", (e)=>{
      const rect = el.getBoundingClientRect();
      const threshold = 40;
      const y = e.clientY;
      if (y < rect.top + threshold) dir = -1;
      else if (y > rect.bottom - threshold) dir = 1;
      else dir = 0;
      if (dir !== 0 && !raf) loop();
      if (dir === 0 && raf) { cancelAnimationFrame(raf); raf=null; }
    });
    el.addEventListener("dragleave", ()=>{
      dir = 0;
      if (raf) { cancelAnimationFrame(raf); raf=null; }
    });
    el.addEventListener("drop", ()=>{
      dir = 0;
      if (raf) { cancelAnimationFrame(raf); raf=null; }
    });
    el.addEventListener("dragend", ()=>{
      dir = 0;
      if (raf) { cancelAnimationFrame(raf); raf=null; }
    });
  }
  setupAutoScroll(scroller);
  setupAutoScroll(navTree);
  setupAutoScroll(document.getElementById("navigator-content"));
}
if(document.readyState==="loading") document.addEventListener("DOMContentLoaded", init);
else init();
function init(){
  initLibraryTabs();
  renderCatalog("");
  renderTree();
  renderInspector();
  updateBreadcrumbs();
  initArchivePreviewContext();
  setupPanelCollapsibles();
  documentInput.value=JSON.stringify(state.document);
  initHistory();
  schedulePreview();
  setupMetadataListeners();
  setupSlug();
  // Ensure legacy fallback hidden when canvas present
  const fallback=document.getElementById("editor-legacy-fallback");
  const legacyPanel=document.getElementById("legacy-preview-panel");
  if(canvasEl && fallback) fallback.hidden=true;
  if(legacyPanel) legacyPanel.hidden=true;
  // Expose
  window.stratumEditor={ undo:()=>{ if(undo()){ renderTree(); renderInspector(); updateBreadcrumbs(); }}, redo:()=>{ if(redo()){ renderTree(); renderInspector(); updateBreadcrumbs(); }}, setMode:()=>{}, setPreviewWidth:()=>{} };
  // Drag canvas drop for library
  if (canvasEl && canvasController) {
    const wrap=document.getElementById("editor-canvas-wrap");
    if (wrap){
      wrap.addEventListener("dragover", (e)=>{
        const drag=window.__stratum_currentDrag;
        if(!drag) return;
        e.preventDefault();
        e.stopPropagation();
        const target=resolveCanvasDrop(e, drag);
        e.dataTransfer.dropEffect = target.ok ? (drag.type==="library"?"copy":"move") : "none";
        wrap.classList.add("canvas--dragover");
        canvasController.showDropIndicator(target.rect || null, target.position || "inside", target.ok);
      }, true);
      wrap.addEventListener("dragleave", (e)=>{
        if (e.relatedTarget && wrap.contains(e.relatedTarget)) return;
        wrap.classList.remove("canvas--dragover");
        canvasController.clearDropIndicator();
      }, true);
      wrap.addEventListener("drop", (e)=>{
        e.preventDefault();
        e.stopPropagation();
        wrap.classList.remove("canvas--dragover");
        const drag=window.__stratum_currentDrag;
        const target=drag ? resolveCanvasDrop(e, drag) : { ok:false, reason:"missing" };
        canvasController.clearDropIndicator();
        if(!drag) return;
        if(!target.ok){ showCanvasDropError(target.reason); return; }
        performDrop(drag, target.containerNode, target.index);
        window.__stratum_currentDrag=null;
      }, true);
    }
  }
}
