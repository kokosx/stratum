// insertion.js — insertion target resolution + schema legality (generic, no block hardcode)
import { state, blockCatalog, definitionForBlock, displayNameForBlock, findDocumentNode, findDocumentParent, isContainerNode } from "./state.js";

let target = null; // {parentId: string|null, index:number, contextInstanceKey?: string}
const targetListeners = new Set();

function notifyTarget(next) {
  for (const fn of targetListeners) {
    try { fn(next); } catch (_) {}
  }
}

export function getInsertionTarget() {
  return target ? { ...target } : null;
}

export function setInsertionTarget(next) {
  if (!next || typeof next !== "object") return;
  const normalized = { parentId: next.parentId ?? null, index: Number(next.index) || 0 };
  if (next.contextInstanceKey) normalized.contextInstanceKey = String(next.contextInstanceKey);
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

export function canInsert(parentNode, definition, index) {
  // parent null => root: allowed if block is available in current editor context (catalog)
  if (!parentNode) {
    if (!definition || !definition.block) return { ok: false, reason: "Unknown block." };
    const catalog = blockCatalog();
    const entry = catalog.find((item) => item.block === definition.block);
    if (!entry || entry.hidden) return { ok: false, reason: `"${definition.displayName || definition.block}" is not available in this context.` };
    if (index != null) {
      const count = state.document?.nodes ? state.document.nodes.length : 0;
      if (index < 0 || index > count) return { ok: false, reason: "Invalid insertion index." };
    }
    return { ok: true, reason: "" };
  }
  const def = definitionForBlock(parentNode.block, parentNode.version);
  if (!def) return { ok: false, reason: "Unknown container." };
  const rule = ruleForDef(def);
  if (rule.mode === "none") return { ok: false, reason: `${displayNameForBlock(parentNode.block)} does not allow child blocks.` };
  if (rule.mode === "allowed" && !rule.blocks.includes(definition.block)) {
    const label = definition.displayName || displayNameForBlock(definition.block);
    return { ok: false, reason: `"${label}" is not allowed inside ${displayNameForBlock(parentNode.block)}.` };
  }
  // enforce catalog + hidden for nested insertion as well (§5, §36 legal-only UI is not enough if API allows illegal)
  const catalogEntry = blockCatalog().find((item) => item.block === definition.block);
  if (!catalogEntry || catalogEntry.hidden) return { ok: false, reason: `"${definition.displayName || definition.block}" is not available in this context.` };
  if (rule.max != null) {
    const count = parentNode.children ? parentNode.children.length : 0;
    if (count >= rule.max) {
      const label = displayNameForBlock(parentNode.block);
      const unit = rule.max === 1 ? "child block" : "child blocks";
      return { ok: false, reason: `${label} allows at most ${rule.max} ${unit}.` };
    }
  }
  if (index != null) {
    const count = parentNode.children ? parentNode.children.length : 0;
    if (index < 0 || index > count) return { ok: false, reason: "Invalid insertion index." };
  }
  return { ok: true, reason: "" };
}

// Whether parent could accept at least one legal insertion at given index
export function hasLegalInsertion(parentNode, index) {
  if (!parentNode) {
    // root: true if any non-hidden catalog entry
    return blockCatalog().some((c) => !c.hidden);
  }
  const def = definitionForBlock(parentNode.block, parentNode.version);
  if (!def) return false;
  const rule = ruleForDef(def);
  if (rule.mode === "none") return false;
  if (rule.max != null && parentNode.children && parentNode.children.length >= rule.max) return false;
  if (rule.mode === "any") return true;
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
  const catalog = blockCatalog();
  if (!catalog.length) return [];
  if (!parentNode) {
    // root: all catalog blocks are legal (context already filtered server-side)
    return catalog.slice();
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
  return blockCatalog().length > 0;
}
