// insertion.js — insertion target resolution + schema legality (generic, no block hardcode)
import { state, blockCatalog, definitionForBlock, displayNameForBlock, findDocumentNode, findDocumentParent, isContainerNode } from "./state.js";

let target = null; // {parentId: string|null, index:number, source:"selection"|"contextual", contextInstanceKey?: string}
const targetListeners = new Set();

function notifyTarget(next) {
  for (const fn of targetListeners) {
    try { fn(next); } catch (_) {}
  }
}

export function getInsertionTarget() {
  return target ? { ...target } : null;
}

export function getInsertionSource() {
  return target ? target.source : null;
}

export function setInsertionTarget(next, opts) {
  if (!next || typeof next !== "object") return;
  let source = null;
  if (opts && (opts.source === "contextual" || opts.source === "selection")) source = opts.source;
  else if (next.source === "contextual" || next.source === "selection") source = next.source;
  else source = "selection";
  const normalized = { parentId: next.parentId ?? null, index: Number(next.index) || 0, source };
  if (next.contextInstanceKey) normalized.contextInstanceKey = String(next.contextInstanceKey);
  // validate parent existence; dangling parent treated as invalid, not root teleport
  if (normalized.parentId != null && !findDocumentNode(normalized.parentId)) {
    // stale target after undo/race — clear instead of teleporting to root
    clearInsertionTarget();
    return;
  }
  // clamp index defensively (but validation will also clamp)
  if (normalized.parentId === null) {
    const len = state.document?.nodes?.length || 0;
    normalized.index = Math.max(0, Math.min(normalized.index, len));
  } else {
    const parent = findDocumentNode(normalized.parentId);
    if (parent) {
      const len = (parent.children || []).length;
      normalized.index = Math.max(0, Math.min(normalized.index, len));
    }
  }
  target = normalized;
  notifyTarget({ ...target });
}

export function clearInsertionTarget() {
  if (target === null) return;
  target = null;
  notifyTarget(null);
}

export function subscribeInsertionTarget(listener) {
  if (typeof listener !== "function") return () => {};
  targetListeners.add(listener);
  return () => targetListeners.delete(listener);
}

// --- Schema legality (single generic validator) ---

function ruleForDef(def) {
  return def?.schema?.children || { mode: "none" };
}

function canChildAcceptParent(childBlock, parentNode) {
  const childDef = definitionForBlock(childBlock);
  if (!childDef) return { ok: true, reason: "" };
  const parents = childDef.schema?.placement?.parents;
  if (!parents || parents.length === 0) return { ok: true, reason: "" };
  if (!parentNode) {
    return { ok: false, reason: `"${displayNameForBlock(childBlock)}" cannot be placed at the top level.` };
  }
  if (!parents.includes(parentNode.block)) {
    return { ok: false, reason: `"${displayNameForBlock(childBlock)}" cannot be placed inside "${displayNameForBlock(parentNode.block)}".` };
  }
  return { ok: true, reason: "" };
}

// Shared structural placement core. Operation distinguishes creation vs relocation.
// creation ("insert") enforces catalog/hidden; relocation ("move") does not block
// an already-existing valid node solely because catalog marks it hidden (§14).
function coreCanPlace({ parentNode, block, operation, isSameParent, index }) {
  const op = operation === "move" ? "move" : "insert";
  // root: context-sensitive
  if (!parentNode) {
    if (op === "insert") {
      if (!block) return { ok: false, reason: "Unknown block." };
      const catalog = blockCatalog();
      const entry = catalog.find((item) => item.block === block);
      if (!entry || entry.hidden) return { ok: false, reason: `"${block}" is not available in this context.` };
      const placement = canChildAcceptParent(block, null);
      if (!placement.ok) return placement;
      if (index != null) {
        const count = state.document?.nodes ? state.document.nodes.length : 0;
        if (index < 0 || index > count) return { ok: false, reason: "Invalid insertion index." };
      }
      return { ok: true, reason: "" };
    }
    // move to root: enforce placement.parents then index bounds (root has no children mode)
    const placement = canChildAcceptParent(block, null);
    if (!placement.ok) return placement;
    if (index != null) {
      const count = state.document?.nodes ? state.document.nodes.length : 0;
      // for same-parent root move, count is root length including moving node; raw index valid 0..count
      const maxIdx = count;
      if (index < 0 || index > maxIdx) return { ok: false, reason: "Invalid insertion index." };
    }
    return { ok: true, reason: "" };
  }
  const def = definitionForBlock(parentNode.block, parentNode.version);
  if (!def) return { ok: false, reason: "Unknown container." };
  const rule = ruleForDef(def);
  const placementEarly = canChildAcceptParent(block, parentNode);
  if (!placementEarly.ok) return placementEarly;
  if (rule.mode === "none") return { ok: false, reason: `${displayNameForBlock(parentNode.block)} does not allow child blocks.` };
  if (rule.mode === "allowed" && !rule.blocks.includes(block)) {
    const label = displayNameForBlock(block);
    return { ok: false, reason: `"${label}" is not allowed inside ${displayNameForBlock(parentNode.block)}.` };
  }
  if (op === "insert") {
    const catalogEntry = blockCatalog().find((item) => item.block === block);
    if (!catalogEntry || catalogEntry.hidden) return { ok: false, reason: `"${block}" is not available in this context.` };
    if (rule.max != null) {
      const count = parentNode.children ? parentNode.children.length : 0;
      if (count >= rule.max) {
        const label = displayNameForBlock(parentNode.block);
        const unit = rule.max === 1 ? "child block" : "child blocks";
        return { ok: false, reason: `${label} allows at most ${rule.max} ${unit}.` };
      }
    }
  } else {
    // move: skip catalog/hidden, enforce max on POST-MOVE structure
    if (rule.max != null) {
      const count = parentNode.children ? parentNode.children.length : 0;
      // same-parent reorder does not increase count
      if (!isSameParent && count >= rule.max) {
        const label = displayNameForBlock(parentNode.block);
        const unit = rule.max === 1 ? "child block" : "child blocks";
        return { ok: false, reason: `${label} allows at most ${rule.max} ${unit}.` };
      }
    }
  }
  if (index != null) {
    const count = parentNode.children ? parentNode.children.length : 0;
    if (index < 0 || index > count) {
      // Special: for move cross-parent, count is dest count pre-move, so same bound
      // For same-parent, count is source==dest length, still 0..count valid
      return { ok: false, reason: "Invalid insertion index." };
    }
  }
  return { ok: true, reason: "" };
}

export function canInsert(parentNode, definition, index) {
  if (!definition || !definition.block) return { ok: false, reason: "Unknown block." };
  const block = definition.block;
  return coreCanPlace({ parentNode, block, operation: "insert", isSameParent: false, index });
}

function containsDescendant(ancestorNode, targetId) {
  if (!ancestorNode || !targetId) return false;
  if (ancestorNode.id === targetId) return true;
  for (const ch of ancestorNode.children || []) {
    if (containsDescendant(ch, targetId)) return true;
  }
  return false;
}

export function canRemove(nodeId) {
  const found = findDocumentParent(nodeId);
  if (!found) return { ok: false, reason: "Block not found." };
  const parent = found.parent;
  if (!parent) return { ok: true, reason: "" };
  const parentDef = definitionForBlock(parent.block, parent.version);
  if (!parentDef) return { ok: true, reason: "" };
  const rule = ruleForDef(parentDef);
  if (rule.min != null) {
    const after = (parent.children || []).length - 1;
    if (after < rule.min) {
      const label = displayNameForBlock(parent.block);
      const unit = rule.min === 1 ? "child block" : "child blocks";
      return { ok: false, reason: `${label} requires at least ${rule.min} ${unit}.` };
    }
  }
  return { ok: true, reason: "" };
}

export function canMove(nodeId, parentId, index) {
  if (!nodeId || typeof nodeId !== "string") return { ok: false, reason: "Block not found." };
  const src = findDocumentParent(nodeId);
  if (!src) return { ok: false, reason: "Block not found." };
  const movingNode = src.node;
  const block = movingNode.block;
  // block definition must exist (§15)
  const movingDef = definitionForBlock(movingNode.block, movingNode.version);
  if (!movingDef) return { ok: false, reason: "Unknown block." };
  const targetParentId = parentId ?? null;
  let targetParentNode = null;
  if (targetParentId != null) {
    targetParentNode = findDocumentNode(targetParentId);
    if (!targetParentNode) return { ok: false, reason: "Parent not found." };
  }
  // cycle protection §8
  if (targetParentNode) {
    if (containsDescendant(movingNode, targetParentNode.id)) {
      return { ok: false, reason: "Cannot move a block inside itself." };
    }
  }
  const sameParent = (src.parent ? src.parent.id : null) === targetParentId;
  // index bounds pre-removal — strict finite check (NaN/undefined must reject, not coerce to 0)
  const rawIndex = Number(index);
  if (!Number.isFinite(rawIndex)) return { ok: false, reason: "Invalid insertion index." };
  // destination length pre-move
  const destCount = targetParentNode ? (targetParentNode.children || []).length : (state.document?.nodes?.length || 0);
  if (rawIndex < 0 || rawIndex > destCount) return { ok: false, reason: "Invalid insertion index." };
  // source min when moving out
  if (!sameParent) {
    const rem = canRemove(nodeId);
    if (!rem.ok) return rem;
  }
  // effective index after removal for no-op detection (§5) and final placement
  let effective = rawIndex;
  if (sameParent && src.index < rawIndex) effective = rawIndex - 1;
  if (sameParent && src.index === effective) {
    // No-op boundaries before/after self
    return { ok: false, reason: "Already at that position." };
  }
  const canPlace = coreCanPlace({ parentNode: targetParentNode, block, operation: "move", isSameParent: sameParent, index: effective });
  if (!canPlace.ok) return canPlace;
  return { ok: true, reason: "" };
}

export function findSource(nodeId) {
  const info = findDocumentParent(nodeId);
  if (!info) return null;
  return {
    node: info.node,
    parent: info.parent,
    parentId: info.parent ? info.parent.id : null,
    siblings: info.siblings,
    index: info.index,
  };
}

// Whether parent could accept at least one legal insertion at given index
export function hasLegalInsertion(parentNode, index) {
  if (!parentNode) {
    // root: true if any non-hidden catalog entry can be legally inserted (respects placement.parents)
    for (const item of blockCatalog()) {
      if (item.hidden) continue;
      const res = canInsert(null, item, index != null ? index : (state.document?.nodes ? state.document.nodes.length : 0));
      if (res.ok) return true;
    }
    return false;
  }
  const def = definitionForBlock(parentNode.block, parentNode.version);
  if (!def) return false;
  const rule = ruleForDef(def);
  if (rule.mode === "none") return false;
  if (rule.max != null && parentNode.children && parentNode.children.length >= rule.max) return false;
  if (rule.mode === "any") {
    for (const item of blockCatalog()) {
      if (item.hidden) continue;
      const ch = canInsert(parentNode, item, index);
      if (ch.ok) return true;
    }
    return false;
  }
  if (rule.mode === "allowed") {
    if (!Array.isArray(rule.blocks) || rule.blocks.length === 0) return false;
    // need at least one allowed block that is actually in catalog and not hidden, and canInsert would succeed
    const catalogBlocks = new Set(blockCatalog().map((c) => c.block));
    for (const blk of rule.blocks) {
      if (!catalogBlocks.has(blk)) continue;
      // find definition for block
      let defForBlk = definitionForBlock(blk) || blockCatalog().find((c) => c.block === blk);
      if (!defForBlk) continue;
      if (defForBlk.hidden) continue;
      const probe = { block: blk, displayName: defForBlk.displayName || displayNameForBlock(blk) };
      const ch = canInsert(parentNode, probe, index);
      if (ch.ok) return true;
    }
    return false;
  }
  return false;
}

export function legalBlocksFor(parentNode, index) {
  const catalog = blockCatalog().filter((c) => !c.hidden);
  if (!catalog.length) return [];
  if (!parentNode) {
    const outRoot = [];
    for (const item of catalog) {
      const res = canInsert(null, item, index != null ? index : (state.document?.nodes ? state.document.nodes.length : 0));
      if (res.ok) outRoot.push(item);
    }
    return outRoot;
  }
  const out = [];
  for (const item of catalog) {
    const res = canInsert(parentNode, item, index != null ? index : (parentNode.children ? parentNode.children.length : 0));
    if (res.ok) out.push(item);
  }
  return out;
}

// Resolve target to nodes
export function resolveInsertionTarget(t) {
  if (!t) return null;
  if (t.parentId == null) {
    const siblings = state.document?.nodes || [];
    const idx = Math.max(0, Math.min(Number(t.index) || 0, siblings.length));
    return { parentNode: null, siblings, index: idx };
  }
  const found = findDocumentParent(t.parentId);
  if (!found) return null;
  const parentNode = found.node;
  const siblings = parentNode.children || [];
  const idx = Math.max(0, Math.min(Number(t.index) || 0, siblings.length));
  return { parentNode, siblings, index: idx };
}

// Global fallback for Blocks panel without explicit target (§38)
export function resolveGlobalInsertion(definition) {
  if (!definition) return null;
  // 1. selected editable container accepts block → append inside
  const selNodeId = state.selection?.nodeId || null;
  if (selNodeId) {
    const selNode = findDocumentNode(selNodeId);
    if (selNode && state.selection.editable !== false) {
      if (isContainerNode(selNode)) {
        const c = canInsert(selNode, definition, selNode.children ? selNode.children.length : 0);
        if (c.ok) return { parentId: selNode.id, index: (selNode.children || []).length };
      }
      // 2. otherwise insert after selected editable node in its SDT parent/root if legal
      const parentInfo = findDocumentParent(selNode.id);
      if (parentInfo) {
        const pNode = parentInfo.parent;
        const idx = parentInfo.index + 1;
        const c2 = canInsert(pNode, definition, idx);
        if (c2.ok) return { parentId: pNode ? pNode.id : null, index: idx };
      }
    }
  }
  // 3. empty document → root index 0
  if ((state.document?.nodes || []).length === 0) {
    const c = canInsert(null, definition, 0);
    if (c.ok) return { parentId: null, index: 0 };
  }
  // 4. otherwise no safe location
  return null;
}

export function hasAnyLegalInsertion() {
  return hasLegalInsertion(null, 0);
}

// --- Blocks panel explicit target (§19-20) ---

export function targetForBlocksFromSelection() {
  const sel = state.selection;
  if (!sel || !sel.nodeId || sel.editable === false) return null;
  const selNode = findDocumentNode(sel.nodeId);
  if (!selNode) return null;
  // 1. inside selected container if it has at least one legal child
  if (isContainerNode(selNode)) {
    const insideIdx = (selNode.children || []).length;
    if (hasLegalInsertion(selNode, insideIdx)) {
      return { parentId: selNode.id, index: insideIdx };
    }
  }
  // 2. after selected node in its parent/root
  const parentInfo = findDocumentParent(selNode.id);
  if (!parentInfo) return null;
  const parentNode = parentInfo.parent;
  const afterIdx = parentInfo.index + 1;
  if (hasLegalInsertion(parentNode, afterIdx)) {
    return { parentId: parentNode ? parentNode.id : null, index: afterIdx };
  }
  return null;
}

export function describeBlocksTarget(target, selection) {
  if (!target) return { main: "Add block", sub: "Choose a position on the canvas" };
  const sel = selection || state.selection;
  // Check if target corresponds to "after selected" (selected cannot contain or inside not legal)
  if (sel && sel.nodeId) {
    const selNode = findDocumentNode(sel.nodeId);
    if (selNode) {
      const parentInfo = findDocumentParent(selNode.id);
      if (parentInfo) {
        const afterParentId = parentInfo.parent ? parentInfo.parent.id : null;
        const afterIdx = parentInfo.index + 1;
        if (target.parentId === afterParentId && target.index === afterIdx) {
          // after selected (selected is leaf or full)
          const display = displayNameForBlock(selNode.block);
          return { main: "Add block", sub: `After ${display}` };
        }
      }
    }
  }
  // otherwise inside container
  if (target.parentId != null) {
    const parentNode = findDocumentNode(target.parentId);
    if (parentNode) {
      const display = displayNameForBlock(parentNode.block);
      return { main: "Add block", sub: `Inside ${display}` };
    }
  }
  // root between blocks
  const nodes = state.document?.nodes || [];
  if (target.index === 0 && nodes.length === 0) return { main: "Add block", sub: "Page content" };
  if (target.index === 0) return { main: "Add block", sub: "Before first block" };
  if (target.index === nodes.length) return { main: "Add block", sub: "After last block" };
  return { main: "Add block", sub: "Between blocks" };
}
