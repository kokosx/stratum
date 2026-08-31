// context-menu.js — small shared context menu for Navigator + Canvas
import { state, definitions, definitionFor } from "./state.js";
import { findNode, isContainer } from "./tree.js";
import { canInsert, canRemove, canDuplicate, canMove, hasLegalInsertion } from "./mutations.js";

let currentMenu = null;
let onCloseCleanup = null;

function closeMenu() {
  if (currentMenu) {
    currentMenu.remove();
    currentMenu = null;
  }
  if (onCloseCleanup) {
    const fn = onCloseCleanup;
    onCloseCleanup = null;
    fn();
  }
  document.removeEventListener("mousedown", onDocDown);
  document.removeEventListener("keydown", onKeyDown);
  if (state._closeMenu) delete state._closeMenu;
}

function onDocDown(e) {
  if (currentMenu && !currentMenu.contains(e.target)) {
    closeMenu();
  }
}

function onKeyDown(e) {
  if (e.key === "Escape") {
    closeMenu();
  }
}

function buildActionsForNode(nodeId) {
  const found = findNode(nodeId);
  if (!found) return [];
  const node = found.node;
  const parent = found.parent;
  const def = definitionFor(node);
  const isContainerNode = isContainer(node);
  // Check if external (not editable) — caller should filter, but here handle
  // Detect external via window exposure? Actually external nodes have editable false but we still have nodeId. For external, we shouldn't show menu here.
  // Determine editability via index lookup
  let editable = true;
  try {
    const ctrl = window.__stratum_canvasController;
    if (ctrl && ctrl.nodeToKeys) {
      const keys = ctrl.nodeToKeys.get(nodeId) || [];
      if (keys.length) {
        const info = ctrl.index.get(keys[0]);
        if (info) editable = info.editable;
      }
    }
  } catch (_) {}
  if (!editable) {
    // External: only Edit owner
    const actions = [];
    try {
      const infoKeys = window.__stratum_canvasController ? window.__stratum_canvasController.nodeToKeys.get(nodeId) : null;
      let info = null;
      if (infoKeys && infoKeys.length) info = window.__stratum_canvasController.index.get(infoKeys[0]);
      if (info && info.ownerId) {
        const label = info.ownerLabel || info.ownerId.slice(0,8);
        const typeLabel = info.ownerType === "site-part" ? "Site Part" : info.ownerType === "layout-template" ? "Template" : info.ownerType || "External";
        actions.push({ label: `Edit ${label || typeLabel}`, action: () => {
          let url = "";
          if (info.ownerType === "site-part") url = `/admin/appearance/site-parts/${info.ownerId}/edit`;
          else if (info.ownerType === "layout-template") url = `/admin/appearance/templates/${info.ownerId}/edit`;
          if (url) window.location.href = url;
        }});
      }
    } catch(_) {}
    if (!actions.length) actions.push({ label: "External content", disabled: true, reason: "Read-only" });
    return actions;
  }

  const actions = [];
  // Add inside (only for containers) — probe hasLegalInsertion so allowed list is respected
  if (isContainerNode) {
    let disabled = false;
    let reason = "";
    if (!hasLegalInsertion(node, node.children.length)) {
      disabled = true;
      const rule = def?.schema?.children;
      if (rule) {
        if (rule.mode === "none") reason = `${def.displayName} does not allow child blocks.`;
        else if (rule.max != null && node.children.length >= rule.max) reason = `${def.displayName} allows at most ${rule.max} child blocks.`;
        else if (rule.mode === "allowed") reason = `${def.displayName} allows only ${rule.blocks ? rule.blocks.length : 0} block type(s).`;
        else reason = "Cannot add here.";
      }
    }
    actions.push({ label: "Add inside", action: () => {
      if (window.__stratum_setInsertionTarget) window.__stratum_setInsertionTarget(nodeId, node.children.length);
      document.querySelectorAll("[data-library-tab]").forEach(t=>{ if((t.getAttribute('data-library-tab')||t.dataset.tab)==="blocks") t.click(); });
      if (window.__stratum_renderCatalog) window.__stratum_renderCatalog();
    }, disabled, reason });
  }
  // Add before — check legal
  {
    let disabled = false; let reason = "";
    if (parent) {
      if (!hasLegalInsertion(parent, found.index)) {
        disabled = true;
        const pdef = definitionFor(parent);
        const rule = pdef?.schema?.children;
        if (rule && rule.mode==="none") reason = `${pdef.displayName} does not allow child blocks.`;
        else if (rule && rule.max!=null && parent.children.length >= rule.max) reason = `${pdef.displayName} allows at most ${rule.max} child blocks.`;
        else reason = "Cannot add here.";
      }
    }
    actions.push({ label: "Add before", action: () => {
      const pId = parent ? parent.id : null;
      if (window.__stratum_setInsertionTarget) window.__stratum_setInsertionTarget(pId, found.index);
      document.querySelectorAll("[data-library-tab]").forEach(t=>{ if((t.getAttribute('data-library-tab')||t.dataset.tab)==="blocks") t.click(); });
      if (window.__stratum_renderCatalog) window.__stratum_renderCatalog();
    }, disabled, reason });
  }
  {
    let disabled = false; let reason = "";
    if (parent) {
      if (!hasLegalInsertion(parent, found.index+1)) {
        disabled = true;
        const pdef = definitionFor(parent);
        const rule = pdef?.schema?.children;
        if (rule && rule.mode==="none") reason = `${pdef.displayName} does not allow child blocks.`;
        else if (rule && rule.max!=null && parent.children.length >= rule.max) reason = `${pdef.displayName} allows at most ${rule.max} child blocks.`;
        else reason = "Cannot add here.";
      }
    }
    actions.push({ label: "Add after", action: () => {
      const pId = parent ? parent.id : null;
      if (window.__stratum_setInsertionTarget) window.__stratum_setInsertionTarget(pId, found.index+1);
      document.querySelectorAll("[data-library-tab]").forEach(t=>{ if((t.getAttribute('data-library-tab')||t.dataset.tab)==="blocks") t.click(); });
      if (window.__stratum_renderCatalog) window.__stratum_renderCatalog();
    }, disabled, reason });
  }
  actions.push("sep");
  // Duplicate
  {
    const res = canDuplicate(nodeId);
    actions.push({ label: "Duplicate", action: () => { if (window.__stratum_duplicateNode) window.__stratum_duplicateNode(nodeId); }, disabled: !res.ok, reason: res.reason });
  }
  // Move up
  {
    let disabled = found.index === 0;
    let reason = "";
    if (!disabled) {
      const res = canMove(nodeId, parent, found.index-1);
      if (!res.ok) { disabled = true; reason = res.reason; }
    } else { reason = "Already at the top."; }
    actions.push({ label: "Move up", action: () => { if (window.__stratum_moveNode) window.__stratum_moveNode(nodeId, -1); }, disabled, reason });
  }
  {
    let disabled = found.index === found.siblings.length-1;
    let reason = "";
    if (!disabled) {
      const res = canMove(nodeId, parent, found.index+2);
      if (!res.ok) { disabled = true; reason = res.reason; }
    } else { reason = "Already at the bottom."; }
    actions.push({ label: "Move down", action: () => { if (window.__stratum_moveNode) window.__stratum_moveNode(nodeId, 1); }, disabled, reason });
  }
  actions.push("sep");
  {
    const res = canRemove(nodeId);
    actions.push({ label: "Delete", danger: true, action: () => { if (window.__stratum_removeNode) window.__stratum_removeNode(nodeId); }, disabled: !res.ok, reason: res.reason });
  }
  return actions;
}

export function openContextMenu({ anchor, nodeId, actions: customActions, onClose }) {
  closeMenu();
  const rect = anchor.getBoundingClientRect();
  const menu = document.createElement("div");
  menu.className = "context-menu";
  menu.setAttribute("role", "menu");

  const actions = customActions || buildActionsForNode(nodeId);

  actions.forEach(item => {
    if (item === "sep") {
      const sep = document.createElement("div");
      sep.className = "context-menu__separator";
      sep.setAttribute("role", "separator");
      menu.append(sep);
      return;
    }
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "context-menu__item" + (item.danger ? " context-menu__item--danger" : "");
    btn.setAttribute("role", "menuitem");
    if (item.disabled) {
      btn.disabled = true;
      btn.setAttribute("aria-disabled", "true");
      if (item.reason) btn.title = item.reason;
    } else if (item.reason) {
      btn.title = item.reason;
    }
    const labelSpan = document.createElement("span");
    labelSpan.textContent = item.label;
    btn.append(labelSpan);
    if (item.disabled && item.reason) {
      const hint = document.createElement("span");
      hint.className = "context-menu__hint";
      hint.title = item.reason;
      hint.textContent = "ⓘ";
      btn.append(hint);
    }
    btn.addEventListener("click", (e) => {
      e.preventDefault(); e.stopPropagation();
      if (btn.disabled) {
        if (item.reason && window.stratumToast) window.stratumToast("error", item.reason);
        return;
      }
      closeMenu();
      if (item.action) item.action();
    });
    menu.append(btn);
  });

  document.body.append(menu);
  // Positioning: try below anchor, flip if overflow
  const menuRect = menu.getBoundingClientRect();
  let top = rect.bottom + 6;
  let left = rect.left;
  // Clamp horizontal
  if (left + menuRect.width > window.innerWidth - 8) {
    left = window.innerWidth - menuRect.width - 8;
  }
  if (left < 8) left = 8;
  if (top + menuRect.height > window.innerHeight - 8) {
    // flip above
    top = rect.top - menuRect.height - 6;
    if (top < 8) top = 8;
  }
  menu.style.top = top + "px";
  menu.style.left = left + "px";

  currentMenu = menu;
  onCloseCleanup = onClose || null;
  // Store close handler for selection change
  state._closeMenu = closeMenu;
  // Delay doc listener to avoid immediate close from anchor click
  setTimeout(() => {
    document.addEventListener("mousedown", onDocDown);
    document.addEventListener("keydown", onKeyDown);
  }, 0);

  // Close on selection change
  const origOnSel = window.__stratum_onSelectionChange;
  // Not overriding, just ensure close on next selection via state._closeMenu called elsewhere

  return menu;
}

export function closeContextMenu() { closeMenu(); }

export function buildActionsForNodeExport(nodeId) { return buildActionsForNode(nodeId); }

// Expose for canvas toolbar
window.__stratum_openContextMenu = openContextMenu;
window.__stratum_closeContextMenu = closeMenu;
window.__stratum_buildActions = buildActionsForNode;
