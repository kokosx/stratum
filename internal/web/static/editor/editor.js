// Stratum Visual Editor — unified SDT editor (modular)
import { state, bootstrap, definitions, definitionFor, pushHistory, maybePushHistory, undo, redo, updateDirty, syncBaseline, initHistory, hydrateNode, isDirtyNow } from "./state.js";
import { findNode, isContainer, createNode, duplicateSubtree, assignNewIDs, clonePatternNodes, insertionPoint, insertionPointForPattern, canInsertRoots, containerAccepts, isWithin, collectNodeIds } from "./tree.js";
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
window.__stratum_onSelectionChange = (nodeId, instanceKey, externalInfo) => {
  state.selectedNodeId = nodeId;
  state.selectedInstanceKey = instanceKey || null;
  renderNavigator();
  renderInspector(externalInfo || null);
  updateBreadcrumbs();
  // sync legacy tree selection
  renderTree();
};
window.__stratum_renderTree = renderTree;

// Canvas controller
let canvasController = null;
if (canvasEl) {
  canvasController = new CanvasController(canvasEl, canvasOverlay);
  window.__stratum_canvasController = canvasController;
}

// Legacy tree rendering (fallback) — kept for progressive enhancement and hidden legacy fallback
function renderTree() {
  if (!treeElement) return;
  treeElement.replaceChildren();
  treeElement.append(renderDropZone(state.document.nodes, null));
  if (emptyElement) emptyElement.hidden = state.document.nodes.length > 0;
  // also update navigator
  renderNavigator();
  updateBreadcrumbs();
}

function renderDropZone(siblings, containerNode) {
  const zone = document.createElement("div");
  zone.className = containerNode ? "node__children" : "tree__root";
  if (siblings.length === 0) {
    const slot = document.createElement("div");
    slot.className = "drop-slot drop-slot--empty";
    const inner = document.createElement("div");
    inner.className = "node__empty";
    inner.textContent = containerNode ? "Drop blocks here" : "Drag blocks here to begin";
    slot.append(inner);
    attachSlot(slot, containerNode, 0);
    zone.append(slot);
    return zone;
  }
  siblings.forEach((child, index) => {
    const slot = document.createElement("div");
    slot.className = "drop-slot";
    attachSlot(slot, containerNode, index);
    zone.append(slot);
    zone.append(renderNode(child));
  });
  const tail = document.createElement("div");
  tail.className = "drop-slot";
  attachSlot(tail, containerNode, siblings.length);
  zone.append(tail);
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
  actions.append(actionButton("⧉","Duplicate", ()=>duplicateNode(node.id)));
  if (canIndent(node.id)) actions.append(actionButton("↳","Indent", ()=>indentNode(node.id)));
  if (canOutdent(node.id)) actions.append(actionButton("↰","Outdent", ()=>outdentNode(node.id)));
  actions.append(actionButton("↑","Move up", ()=>moveNode(node.id,-1)));
  actions.append(actionButton("↓","Move down", ()=>moveNode(node.id,1)));
  actions.append(actionButton("✕","Remove", ()=>removeNode(node.id), true));
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
  // Use shared dragdrop validation (also checks external)
  const editable = true; // root is always editable, external check handled in canvas wrapper
  const v = validateDropTarget(containerNode, block, drag, editable);
  if (!v.ok) return;
  if (!containerAccepts(containerNode, block, drag)) return;
  const target = containerNode? containerNode.children : state.document.nodes;
  let node;
  if (drag.type==="library"){
    pushHistory();
    node = createNode(drag.definition);
    target.splice(index,0,node);
  } else {
    const found = findNode(drag.nodeId);
    if (!found) return;
    if (containerNode && isWithin(drag.nodeId, containerNode)) return;
    if (found.parent===containerNode && found.index===index) return;
    pushHistory();
    found.siblings.splice(found.index,1);
    let ti=index;
    if (found.parent===containerNode && found.index < index) ti-=1;
    target.splice(ti,0,found.node);
    node=found.node;
  }
  state.selectedNodeId=node.id;
  state.selectedInstanceKey=null;
  if (canvasController) canvasController.selectNode(node.id, null);
  changed();
}
window.__stratum_performNavigatorDrop = (drag, targetNode)=>{
  // drop dragged node onto targetNode (as child if container, else sibling)
  if (!drag) return;
  const block = drag.type==="library"? drag.definition.block : findNode(drag.nodeId)?.node.block;
  // Try to insert as child of target if container accepts, else after target
  const targetInfo = findNode(targetNode.id);
  if (!targetInfo) return;
  const def = definitionFor(targetNode);
  if (def && isContainer(targetNode) && containerAccepts(targetNode, block, drag)) {
    performDrop(drag, targetNode, targetNode.children.length);
  } else {
    // insert after target in its siblings
    performDrop(drag, targetInfo.parent, targetInfo.index+1);
  }
};

// Node operations
function addBlock(definition){
  pushHistory();
  const point = insertionPoint(definition.block);
  const node = createNode(definition);
  point.siblings.splice(point.index,0,node);
  state.selectedNodeId=node.id;
  if (canvasController) canvasController.selectNode(node.id, null);
  changed();
}
function moveNode(id, offset){
  const found=findNode(id); if(!found) return;
  const next=found.index+offset; if(next<0||next>=found.siblings.length) return;
  pushHistory();
  [found.siblings[found.index], found.siblings[next]]=[found.siblings[next], found.siblings[found.index]];
  state.selectedNodeId=id;
  state.selectedInstanceKey=null;
  changed();
}
function canIndent(id){
  const found=findNode(id); if(!found||found.index<1) return false;
  const prev=found.siblings[found.index-1];
  const prevDef=definitionFor(prev);
  if(!prevDef||prevDef.schema.children.mode==="none") return false;
  const childrenAllow = (def, block, count)=>{
    if(!def) return false; const r=def.schema.children;
    if(r.mode==="none") return false;
    if(r.max!==undefined&&r.max!==null&&count>=r.max) return false;
    return r.mode==="any"||(r.mode==="allowed"&&r.blocks.includes(block));
  };
  return childrenAllow(prevDef, found.node.block, prev.children.length);
}
function indentNode(id){
  const found=findNode(id); if(!found||found.index<1) return;
  const prev=found.siblings[found.index-1];
  const prevDef=definitionFor(prev);
  if(!prevDef||prevDef.schema.children.mode==="none") return;
  const childrenAllow = (def, block, count)=>{
    if(!def) return false; const r=def.schema.children;
    if(r.mode==="none") return false;
    if(r.max!==undefined&&r.max!==null&&count>=r.max) return false;
    return r.mode==="any"||(r.mode==="allowed"&&r.blocks.includes(block));
  };
  if(!childrenAllow(prevDef, found.node.block, prev.children.length)) return;
  pushHistory();
  const [moved]=found.siblings.splice(found.index,1);
  prev.children.push(moved);
  changed();
}
function canOutdent(id){
  const found=findNode(id); if(!found||!found.parent) return false;
  const parentFound=findNode(found.parent.id); if(!parentFound) return false;
  const newContainer=parentFound.parent;
  if(!newContainer) return true;
  const childrenAllow = (def, block, count)=>{
    if(!def) return false; const r=def.schema.children;
    if(r.mode==="none") return false;
    if(r.max!==undefined&&r.max!==null&&count>=r.max) return false;
    return r.mode==="any"||(r.mode==="allowed"&&r.blocks.includes(block));
  };
  return childrenAllow(definitionFor(newContainer), found.node.block, newContainer.children.length);
}
function outdentNode(id){
  const found=findNode(id); if(!found||!found.parent) return;
  const parentFound=findNode(found.parent.id); if(!parentFound) return;
  const newContainer=parentFound.parent;
  const childrenAllow = (def, block, count)=>{
    if(!def) return false; const r=def.schema.children;
    if(r.mode==="none") return false;
    if(r.max!==undefined&&r.max!==null&&count>=r.max) return false;
    return r.mode==="any"||(r.mode==="allowed"&&r.blocks.includes(block));
  };
  if(newContainer && !childrenAllow(definitionFor(newContainer), found.node.block, newContainer.children.length)) return;
  pushHistory();
  const [moved]=found.siblings.splice(found.index,1);
  const target=newContainer? newContainer.children : state.document.nodes;
  target.splice(parentFound.index+1,0,moved);
  changed();
}
function removeNode(id){
  const found=findNode(id); if(!found) return;
  const hasChildren = (found.node.children||[]).length>0 || (found.node.children||[]).some(n=> (n.children||[]).length>0);
  if (hasChildren){
    const def=definitionFor(found.node);
    const label=def?.displayName||found.node.block;
    if(!confirm(`Remove "${label}" and all its children?`)) return;
  }
  pushHistory();
  found.siblings.splice(found.index,1);
  state.collapsed.delete(id);
  if(state.selectedNodeId===id) state.selectedNodeId=found.parent?.id||null;
  changed();
}
function duplicateNode(id){
  const found=findNode(id); if(!found) return;
  pushHistory();
  const dupe=duplicateSubtree(found.node);
  found.siblings.splice(found.index+1,0,dupe);
  state.selectedNodeId=dupe.id;
  changed();
}
function insertPattern(patternId){
  const pattern=state.patterns.find(p=>p.id===patternId);
  if(!pattern) return;
  const cloned=clonePatternNodes(pattern);
  if(!cloned.length) return;
  const point=insertionPointForPattern(cloned);
  if(!point){
    if(errorElement){ errorElement.textContent="Cannot insert pattern here — container does not allow this content."; errorElement.hidden=false; setTimeout(()=>errorElement.hidden=true,3000);}
    else alert("Cannot insert pattern here");
    return;
  }
  pushHistory();
  for(let i=0;i<cloned.length;i++) point.siblings.splice(point.index+i,0,cloned[i]);
  state.selectedNodeId=cloned[0].id;
  changed();
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
if(blockSearchEl) blockSearchEl.addEventListener("input", (e)=>{ const v=e.target.value; renderCatalog(v); renderPatternCatalog(v); });
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
      if(w==="100%"){ if(wrap) wrap.style.maxWidth="100%"; canvasEl.style.width="100%"; }
      else { if(wrap) wrap.style.maxWidth=w; canvasEl.style.width=w; }
      if(canvasController) setTimeout(()=>canvasController.updateOverlayPositions(), 100);
    }
  }
  if(e.target.matches(".inspector-tab")){
    const tab=e.target.dataset.tab;
    document.querySelectorAll(".inspector-tab").forEach(b=>{ b.classList.remove("is-active"); b.setAttribute("aria-selected","false"); });
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
  // Move existing Settings tab content into inspector document panel for unified view
  // For now, show capabilities-based info
  const info=document.createElement("div");
  info.className="document-inspector";
  const res=bootstrap.resource || {};
  const caps=bootstrap.capabilities || {};
  info.innerHTML=`<h3 style="margin:0 0 8px">${res.label||res.id||"Document"}</h3>
    <p class="muted" style="font-size:12px">${res.type||""} ${res.kind? "· "+res.kind:""} ${res.contentTypeId? "· "+res.contentTypeId:""}</p>
    <div style="display:grid; gap:6px; margin-top:12px; font-size:12px">
      ${Object.entries(caps).map(([k,v])=> `<div><strong>${k}:</strong> ${v?"yes":"no"}</div>`).join("")}
    </div>
    <p class="muted" style="margin-top:12px;font-size:11px">Switch to Settings tab for full metadata (title, slug, SEO, fields, taxonomies). Visual canvas is primary; Inspector Block tab edits selected block.</p>`;
  slot.append(info);
  // Also try to move legacy Settings tab content if present (for entry)
  const settingsTab=document.getElementById("tab-settings");
  if(settingsTab){
    const clone=settingsTab.cloneNode(true);
    clone.hidden=false;
    clone.id="doc-settings-clone";
    clone.style.display="grid";
    clone.style.gap="16px";
    // Remove duplicate ids
    clone.querySelectorAll("[id]").forEach(el=> el.id = el.id + "-clone");
    slot.append(document.createElement("hr"));
    slot.append(clone);
  }
}

// Init
if(document.readyState==="loading") document.addEventListener("DOMContentLoaded", init);
else init();
function init(){
  initLibraryTabs();
  renderCatalog("");
  renderTree();
  renderInspector();
  updateBreadcrumbs();
  initArchivePreviewContext();
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
